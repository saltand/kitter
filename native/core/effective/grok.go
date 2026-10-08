// grok.go ports the Grok adapter: config.toml [skills] key parsing,
// name normalization/collision rekeying, ignore/disabled lists, and the
// startup-listing render.
package effective

import (
	"path/filepath"
	"strings"
)

// grokAdapter is GrokAdapter.
type grokAdapter struct{ policyBase }

func newGrokAdapter() *grokAdapter {
	return &grokAdapter{policyBase{agent: AgentGrok}}
}

func (a *grokAdapter) Roots(ctx *DiscoveryContext) []*SkillRoot {
	var roots []*SkillRoot
	if !isGlobalContext(ctx) {
		for _, dir := range cwdToBoundary(ctx.Cwd, ctx.RepositoryRoot) {
			scope := ScopeRepository
			if dir == ctx.Cwd {
				scope = ScopeLocal
			}
			for _, config := range []string{".grok", ".agents", ".claude", ".cursor"} {
				roots = append(roots, NewSkillRoot(filepath.Join(dir, config, "skills"), scope))
				roots = append(roots, NewSkillRoot(filepath.Join(dir, config, "commands"), scope).FlatMarkdown())
			}
		}
	}
	for _, configRoot := range []string{
		grokHome(ctx),
		filepath.Join(ctx.Home, ".agents"),
		filepath.Join(ctx.Home, ".claude"),
		filepath.Join(ctx.Home, ".cursor"),
	} {
		roots = append(roots, NewSkillRoot(filepath.Join(configRoot, "skills"), ScopeUser))
		roots = append(roots, NewSkillRoot(filepath.Join(configRoot, "commands"), ScopeUser).FlatMarkdown())
	}
	roots = append(roots, grokConfiguredSkillRoots(ctx)...)
	return roots
}

func (a *grokAdapter) EffectiveName(_ string, metadata *SkillMetadata) string {
	return normalizeGrokName(metadata.Name)
}
func (a *grokAdapter) EffectiveNameForRoot(_ *SkillRoot, skillDir string, m *SkillMetadata) string {
	return a.EffectiveName(skillDir, m)
}
func (a *grokAdapter) EffectiveIDForRoot(_ *SkillRoot, skillDir, _ string, m *SkillMetadata) string {
	return a.EffectiveName(skillDir, m)
}
func (a *grokAdapter) IsEnabled(_ string, metadata *SkillMetadata, ctx *DiscoveryContext) bool {
	canonical := canonicalize(metadata.SourcePath)
	for _, value := range grokConfigValues(ctx, "ignore") {
		path := resolveAgentPath(value, ctx.Home, grokHome(ctx))
		if strings.HasPrefix(canonical, canonicalize(path)) {
			return false
		}
	}
	return true
}
func (a *grokAdapter) IsEnabledForRoot(_ *SkillRoot, skillDir string, m *SkillMetadata, ctx *DiscoveryContext) bool {
	return a.IsEnabled(skillDir, m, ctx)
}
func (a *grokAdapter) IsEnabledForEntry(root *SkillRoot, skillDir, _ string, m *SkillMetadata, ctx *DiscoveryContext) bool {
	return a.IsEnabledForRoot(root, skillDir, m, ctx)
}
func (a *grokAdapter) PromptPathForEntry(_ *SkillRoot, _, skillFile string, _ *SkillMetadata, _ *DiscoveryContext) string {
	return skillFile
}
func (a *grokAdapter) Visibility(_ string, metadata *SkillMetadata, ctx *DiscoveryContext) SkillVisibility {
	for _, name := range grokConfigValues(ctx, "disabled") {
		if normalizeGrokName(name) == normalizeGrokName(metadata.Name) {
			return VisibilityManualOnly
		}
	}
	return frontmatterVisibility(metadata)
}
func (a *grokAdapter) VisibilityForRoot(_ *SkillRoot, skillDir string, m *SkillMetadata, ctx *DiscoveryContext) SkillVisibility {
	return a.Visibility(skillDir, m, ctx)
}
func (a *grokAdapter) RenderVisibleMetadata(skills []EffectiveSkill) string {
	if len(skills) == 0 {
		return ""
	}
	return renderGrokListing(skills, 400000).Text
}
func (a *grokAdapter) RenderInitialCatalog(skills []EffectiveSkill) CatalogRender {
	return renderGrokListing(skills, 400000)
}

// grokHome is grok_home.
func grokHome(ctx *DiscoveryContext) string {
	if v, ok := envOr("GROK_HOME"); ok {
		return v
	}
	return filepath.Join(ctx.Home, ".grok")
}

// grokConfiguredSkillRoots is grok_configured_skill_roots.
func grokConfiguredSkillRoots(ctx *DiscoveryContext) []*SkillRoot {
	home := grokHome(ctx)
	var roots []*SkillRoot
	for _, value := range grokConfigValues(ctx, "paths") {
		path := resolveAgentPath(value, ctx.Home, home)
		if isFile(path) {
			roots = append(roots, ExactSkillRoot(path, ScopeUser))
		} else {
			roots = append(roots, NewSkillRoot(path, ScopeUser))
		}
	}
	return roots
}

// grokConfigValues is grok_config_values: line parser for the [skills]
// table in config.toml, key = [ "a", "b" ].
func grokConfigValues(ctx *DiscoveryContext, key string) []string {
	content, err := readFileString(filepath.Join(grokHome(ctx), "config.toml"))
	if err != nil {
		return nil
	}
	inSkills := false
	var values []string
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "[skills]" {
			inSkills = true
			continue
		}
		if strings.HasPrefix(line, "[") {
			inSkills = false
			continue
		}
		if !inSkills {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(k) != key {
			continue
		}
		v = strings.TrimSpace(v)
		v = strings.TrimPrefix(v, "[")
		v = strings.TrimSuffix(v, "]")
		for _, item := range strings.Split(v, ",") {
			item = strings.TrimSpace(item)
			if len(item) >= 2 && strings.HasPrefix(item, "\"") && strings.HasSuffix(item, "\"") {
				values = append(values, item[1:len(item)-1])
			}
		}
	}
	return values
}

// normalizeGrokName is normalize_grok_name.
func normalizeGrokName(name string) string {
	var normalized strings.Builder
	previousDash := false
	for _, ch := range strings.ToLower(strings.TrimSpace(name)) {
		if ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' {
			normalized.WriteRune(ch)
			previousDash = false
		} else if !previousDash && normalized.Len() > 0 {
			normalized.WriteByte('-')
			previousDash = true
		}
	}
	out := strings.TrimRight(normalized.String(), "-")
	runes := []rune(out)
	if len(runes) > 64 {
		out = string(runes[:64])
	}
	return out
}

// resolveGrokCollisions is adapters.rs::resolve_grok_collisions.
func resolveGrokCollisions(skills []EffectiveSkill) []EffectiveSkill {
	resolved := make([]EffectiveSkill, 0, len(skills))
	type winner struct {
		scope SkillScope
		index int
	}
	names := map[string]winner{}
	for _, skill := range skills {
		w, ok := names[skill.ID]
		if !ok {
			names[skill.ID] = winner{skill.Scope, len(resolved)}
			resolved = append(resolved, skill)
			continue
		}
		if w.scope != skill.Scope {
			continue
		}
		directoryName := grokPathIdentity(&skill)
		if directoryName != skill.ID {
			if _, taken := names[directoryName]; !taken {
				skill.ID = directoryName
				skill.Name = directoryName
				names[directoryName] = winner{skill.Scope, len(resolved)}
				resolved = append(resolved, skill)
				continue
			}
		} else {
			winnerDir := grokPathIdentity(&resolved[w.index])
			if winnerDir != skill.ID {
				if _, taken := names[winnerDir]; !taken {
					contested := skill.ID
					resolved[w.index].ID = winnerDir
					resolved[w.index].Name = winnerDir
					delete(names, contested)
					names[winnerDir] = w
					names[contested] = winner{skill.Scope, len(resolved)}
					resolved = append(resolved, skill)
				}
			}
		}
	}
	return resolved
}

// grokPathIdentity is grok_path_identity.
func grokPathIdentity(skill *EffectiveSkill) string {
	var raw string
	if filepath.Base(skill.Path) == "SKILL.md" {
		raw = filepath.Base(filepath.Dir(skill.Path))
	} else {
		base := filepath.Base(skill.Path)
		raw = strings.TrimSuffix(base, filepath.Ext(base))
	}
	if normalized := normalizeGrokName(raw); normalized != "" {
		return normalized
	}
	return skill.ID
}
