// policies.go ports the filesystem-compatible AgentSkillPolicy impls:
// Cursor, Copilot, Antigravity, Amp, Droid (all the legacy_adapter
// policies in adapters.rs), plus the shared compatible-filesystem
// helpers they use.
package effective

import (
	"os"
	"path/filepath"
	"strings"
)

// compatibleFilesystemRoots is compatible_filesystem_roots.
func compatibleFilesystemRoots(ctx *DiscoveryContext, projectRelative, userRelative []string, extraEnv string) []*SkillRoot {
	var roots []*SkillRoot
	if !isGlobalContext(ctx) {
		for _, dir := range cwdToBoundary(ctx.Cwd, ctx.RepositoryRoot) {
			for _, relative := range projectRelative {
				roots = append(roots, NewSkillRoot(filepath.Join(dir, relative), ScopeRepository))
			}
		}
	}
	for _, relative := range userRelative {
		roots = append(roots, NewSkillRoot(filepath.Join(ctx.Home, relative), ScopeUser))
	}
	if extraEnv != "" {
		if value, ok := envOr(extraEnv); ok {
			for _, path := range envSplitPaths(value) {
				if !filepath.IsAbs(path) {
					path = filepath.Join(ctx.Cwd, path)
				}
				roots = append(roots, NewSkillRoot(path, ScopeUser).WithRootMarkdown())
			}
		}
	}
	return roots
}

// compatiblePolicy is a mixin for Cursor/Copilot/Antigravity/Amp/Droid:
// FirstWins collision, frontmatter visibility (or forced Automatic),
// name+description catalog, approx tokens.
type compatiblePolicy struct {
	policyBase
	validity func(*SkillMetadata) bool
	forceAuto bool
}

func (p compatiblePolicy) IsEnabled(_ string, m *SkillMetadata, _ *DiscoveryContext) bool {
	if p.validity != nil {
		return p.validity(m)
	}
	return compatibleSkillIsValid(m)
}
func (p compatiblePolicy) EffectiveNameForRoot(_ *SkillRoot, skillDir string, m *SkillMetadata) string {
	return m.Name
}
func (p compatiblePolicy) EffectiveIDForRoot(_ *SkillRoot, skillDir, _ string, m *SkillMetadata) string {
	return m.Name
}
func (p compatiblePolicy) IsEnabledForRoot(_ *SkillRoot, skillDir string, m *SkillMetadata, ctx *DiscoveryContext) bool {
	return p.IsEnabled(skillDir, m, ctx)
}
func (p compatiblePolicy) IsEnabledForEntry(_ *SkillRoot, skillDir, _ string, m *SkillMetadata, ctx *DiscoveryContext) bool {
	return p.IsEnabledForRoot(nil, skillDir, m, ctx)
}
func (p compatiblePolicy) Visibility(_ string, m *SkillMetadata, _ *DiscoveryContext) SkillVisibility {
	if p.forceAuto {
		return VisibilityAutomatic
	}
	return frontmatterVisibility(m)
}
func (p compatiblePolicy) VisibilityForRoot(_ *SkillRoot, skillDir string, m *SkillMetadata, ctx *DiscoveryContext) SkillVisibility {
	return p.Visibility(skillDir, m, ctx)
}
func (p compatiblePolicy) RenderVisibleMetadata(skills []EffectiveSkill) string {
	return renderNameDescriptionCatalog(skills)
}
func (p compatiblePolicy) RenderInitialCatalog(skills []EffectiveSkill) CatalogRender {
	return CatalogRender{Text: p.RenderVisibleMetadata(skills), IncludedCount: len(skills)}
}
func (p compatiblePolicy) NameCollision() NameCollision { return CollisionFirstWins }

// cursorPolicy is CursorPolicy.
type cursorPolicy struct{ compatiblePolicy }

func newCursorPolicy() *cursorPolicy {
	p := &cursorPolicy{}
	p.agent = AgentCursor
	return p
}
func (p *cursorPolicy) Roots(ctx *DiscoveryContext) []*SkillRoot {
	return compatibleFilesystemRoots(ctx,
		[]string{".cursor/skills", ".agents/skills", ".claude/skills", ".codex/skills"},
		[]string{".cursor/skills", ".agents/skills", ".claude/skills", ".codex/skills"},
		"")
}

// copilotPolicy is CopilotPolicy.
type copilotPolicy struct{ compatiblePolicy }

func newCopilotPolicy() *copilotPolicy {
	p := &copilotPolicy{}
	p.agent = AgentCopilot
	return p
}
func (p *copilotPolicy) Roots(ctx *DiscoveryContext) []*SkillRoot {
	return compatibleFilesystemRoots(ctx,
		[]string{".github/skills", ".agents/skills", ".claude/skills"},
		[]string{".copilot/skills", ".agents/skills", ".claude/skills"},
		"COPILOT_SKILLS_DIRS")
}

// antigravityPolicy is AntigravityPolicy.
type antigravityPolicy struct{ compatiblePolicy }

func newAntigravityPolicy() *antigravityPolicy {
	p := &antigravityPolicy{}
	p.agent = AgentAntigravity
	p.forceAuto = true
	return p
}
func (p *antigravityPolicy) Roots(ctx *DiscoveryContext) []*SkillRoot {
	return compatibleFilesystemRoots(ctx,
		[]string{".agents/skills", ".agent/skills"},
		[]string{".gemini/config/skills", ".agents/skills", ".agent/skills"},
		"")
}

// ampPolicy is AmpPolicy.
type ampPolicy struct{ compatiblePolicy }

func newAmpPolicy() *ampPolicy {
	p := &ampPolicy{}
	p.agent = AgentAmp
	p.forceAuto = true
	return p
}
func (p *ampPolicy) Roots(ctx *DiscoveryContext) []*SkillRoot {
	roots := []*SkillRoot{
		NewSkillRoot(filepath.Join(ctx.Home, ".config/agents/skills"), ScopeUser),
		NewSkillRoot(filepath.Join(ctx.Home, ".agents/skills"), ScopeUser),
		NewSkillRoot(filepath.Join(ctx.Home, ".config/amp/skills"), ScopeUser),
	}
	if !isGlobalContext(ctx) {
		for _, dir := range cwdToBoundary(ctx.Cwd, ctx.RepositoryRoot) {
			roots = append(roots,
				NewSkillRoot(filepath.Join(dir, ".agents/skills"), ScopeRepository),
				NewSkillRoot(filepath.Join(dir, ".claude/skills"), ScopeRepository),
			)
		}
	}
	roots = append(roots, NewSkillRoot(filepath.Join(ctx.Home, ".claude/skills"), ScopeUser))
	roots = append(roots, ampConfiguredSkillRoots(ctx)...)
	return roots
}

// ampConfiguredSkillRoots is amp_configured_skill_roots.
func ampConfiguredSkillRoots(ctx *DiscoveryContext) []*SkillRoot {
	var paths []string
	if value, ok := envOr("AMP_SKILL_PATHS"); ok {
		for _, path := range envSplitPaths(value) {
			if !filepath.IsAbs(path) {
				path = filepath.Join(ctx.Cwd, path)
			}
			paths = append(paths, path)
		}
	}
	configFiles := []string{
		filepath.Join(ctx.Home, ".config/amp/settings.json"),
		filepath.Join(ctx.Home, ".config/amp/config.json"),
		filepath.Join(ctx.Cwd, ".amp/settings.json"),
		filepath.Join(ctx.Cwd, ".amp/config.json"),
	}
	for _, file := range configFiles {
		if isGlobalContext(ctx) && strings.HasPrefix(file, filepath.Join(ctx.Cwd, ".amp")) {
			continue
		}
		value := readJSONCFile(file)
		if value == nil {
			continue
		}
		var skillValue any
		if skills, ok := value["skills"].(map[string]any); ok {
			if v, ok := skills["path"]; ok {
				skillValue = v
			} else if v, ok := skills["paths"]; ok {
				skillValue = v
			}
		}
		if skillValue == nil {
			skillValue = value["skillPath"]
		}
		if skillValue == nil {
			continue
		}
		switch v := skillValue.(type) {
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok {
					paths = append(paths, resolveAgentPath(s, ctx.Home, filepath.Dir(file)))
				}
			}
		case string:
			paths = append(paths, resolveAgentPath(v, ctx.Home, filepath.Dir(file)))
		}
	}
	var roots []*SkillRoot
	for _, path := range paths {
		roots = append(roots, NewSkillRoot(path, ScopeUser).WithRootMarkdown())
	}
	return roots
}

// droidPolicy is DroidPolicy.
type droidPolicy struct{ compatiblePolicy }

func newDroidPolicy() *droidPolicy {
	p := &droidPolicy{}
	p.agent = AgentDroid
	return p
}
func (p *droidPolicy) Roots(ctx *DiscoveryContext) []*SkillRoot {
	return compatibleFilesystemRoots(ctx,
		[]string{".factory/skills", ".agents/skills", ".agent/skills"},
		[]string{".factory/skills", ".agents/skills", ".agent/skills"},
		"")
}

// readJSONCFile reads a JSON/JSONC file through normalize_jsonc,
// tolerating comments and trailing commas. Parse failures return nil,
// matching Rust's `.ok()` on serde_json::from_str.
func readJSONCFile(path string) map[string]any {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return parseJSONC(string(content))
}

// normalizeJSONC is normalize_jsonc: strip // and /* */ comments
// (respecting string literals) then drop trailing commas before } or ].
func normalizeJSONC(content string) string {
	var out strings.Builder
	out.Grow(len(content))
	runes := []rune(content)
	i := 0
	inString, escaped := false, false
	for i < len(runes) {
		ch := runes[i]
		if inString {
			out.WriteRune(ch)
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
			i++
			continue
		}
		if ch == '"' {
			inString = true
			out.WriteRune(ch)
			i++
			continue
		}
		if ch == '/' && i+1 < len(runes) && runes[i+1] == '/' {
			for i < len(runes) && runes[i] != '\n' {
				i++
			}
			continue
		}
		if ch == '/' && i+1 < len(runes) && runes[i+1] == '*' {
			i += 2
			for i+1 < len(runes) && !(runes[i] == '*' && runes[i+1] == '/') {
				i++
			}
			i += 2
			continue
		}
		out.WriteRune(ch)
		i++
	}
	stripped := out.String()
	// Pass 2: drop commas whose next non-whitespace char is } or ].
	var normalized strings.Builder
	normalized.Grow(len(stripped))
	chars := []rune(stripped)
	i = 0
	inString, escaped = false, false
	for i < len(chars) {
		ch := chars[i]
		if inString {
			normalized.WriteRune(ch)
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
			i++
			continue
		}
		if ch == '"' {
			inString = true
			normalized.WriteRune(ch)
			i++
			continue
		}
		if ch == ',' {
			next := i + 1
			for next < len(chars) && (chars[next] == ' ' || chars[next] == '\t' || chars[next] == '\n' || chars[next] == '\r') {
				next++
			}
			if next < len(chars) && (chars[next] == '}' || chars[next] == ']') {
				i++
				continue
			}
		}
		normalized.WriteRune(ch)
		i++
	}
	return normalized.String()
}
