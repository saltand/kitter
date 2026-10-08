// pi.go ports the Pi adapter: trust.json gating, .pi/settings.json
// skill paths, npm package roots from package.json pi.skills, the
// PiIgnored scan profile and the XML catalog render.
package effective

import (
	"os"
	"path/filepath"
	"strings"
)

// piAdapter is PiAdapter.
type piAdapter struct{ policyBase }

func newPiAdapter() *piAdapter {
	return &piAdapter{policyBase{agent: AgentPi}}
}

func (a *piAdapter) Roots(ctx *DiscoveryContext) []*SkillRoot {
	projectTrusted := !isGlobalContext(ctx) && piProjectIsTrusted(ctx)
	projectConfigured, userConfigured := piConfiguredSkillRoots(ctx, projectTrusted)
	var roots []*SkillRoot
	roots = append(roots, projectConfigured...)
	if projectTrusted {
		roots = append(roots, NewSkillRoot(filepath.Join(ctx.Cwd, ".pi/skills"), ScopeLocal).WithRootMarkdown())
		var dirs []string
		if ctx.RepositoryRoot != "" {
			dirs = cwdToBoundary(ctx.Cwd, ctx.RepositoryRoot)
		} else {
			dirs = cwdToBoundary(ctx.Cwd, "")
		}
		userAgents := canonicalize(filepath.Join(ctx.Home, ".agents/skills"))
		for _, dir := range dirs {
			path := filepath.Join(dir, ".agents/skills")
			if canonicalize(path) == userAgents {
				continue
			}
			roots = append(roots, NewSkillRoot(path, ScopeRepository))
		}
	}
	roots = append(roots, userConfigured...)
	roots = append(roots, NewSkillRoot(filepath.Join(piAgentDir(ctx), "skills"), ScopeUser).WithRootMarkdown())
	roots = append(roots, NewSkillRoot(filepath.Join(ctx.Home, ".agents/skills"), ScopeUser))
	if projectTrusted {
		roots = append(roots, piPackageSkillRoots(
			filepath.Join(ctx.Cwd, ".pi/settings.json"),
			filepath.Join(ctx.Cwd, ".pi/npm/node_modules"),
			ScopeRepository)...)
	}
	roots = append(roots, piPackageSkillRoots(
		filepath.Join(piAgentDir(ctx), "settings.json"),
		filepath.Join(piAgentDir(ctx), "npm/node_modules"),
		ScopeUser)...)
	return roots
}

func (a *piAdapter) IsEnabled(_ string, metadata *SkillMetadata, _ *DiscoveryContext) bool {
	return metadata.HasExplicitDescription && strings.TrimSpace(metadata.Description) != ""
}

func (a *piAdapter) IsEnabledForRoot(_ *SkillRoot, skillDir string, m *SkillMetadata, ctx *DiscoveryContext) bool {
	return a.IsEnabled(skillDir, m, ctx)
}
func (a *piAdapter) IsEnabledForEntry(root *SkillRoot, skillDir, _ string, m *SkillMetadata, ctx *DiscoveryContext) bool {
	return a.IsEnabledForRoot(root, skillDir, m, ctx)
}
func (a *piAdapter) EffectiveNameForRoot(_ *SkillRoot, skillDir string, m *SkillMetadata) string {
	return m.Name
}
func (a *piAdapter) EffectiveIDForRoot(_ *SkillRoot, skillDir, _ string, m *SkillMetadata) string {
	return m.Name
}
func (a *piAdapter) PromptPathForEntry(_ *SkillRoot, _, skillFile string, _ *SkillMetadata, _ *DiscoveryContext) string {
	return skillFile
}
func (a *piAdapter) NameCollision() NameCollision { return CollisionFirstWins }
func (a *piAdapter) Visibility(_ string, metadata *SkillMetadata, _ *DiscoveryContext) SkillVisibility {
	if frontmatterVisibility(metadata) == VisibilityManualOnly {
		return VisibilityManualOnly
	}
	return VisibilityAutomatic
}
func (a *piAdapter) VisibilityForRoot(_ *SkillRoot, skillDir string, m *SkillMetadata, ctx *DiscoveryContext) SkillVisibility {
	return a.Visibility(skillDir, m, ctx)
}

func (a *piAdapter) RenderVisibleMetadata(skills []EffectiveSkill) string {
	if len(skills) == 0 {
		return ""
	}
	lines := []string{
		"",
		"",
		"The following skills provide specialized instructions for specific tasks.",
		"Use the read tool to load a skill's file when the task matches its description.",
		"When a skill file references a relative path, resolve it against the skill directory (parent of SKILL.md / dirname of the path) and use that absolute path in tool commands.",
		"",
		"<available_skills>",
	}
	for _, skill := range skills {
		lines = append(lines, "  <skill>")
		lines = append(lines, "    <name>"+xmlEscape(skill.Name)+"</name>")
		lines = append(lines, "    <description>"+xmlEscape(skill.Description)+"</description>")
		lines = append(lines, "    <location>"+xmlEscape(skill.PromptPath)+"</location>")
		lines = append(lines, "  </skill>")
	}
	lines = append(lines, "</available_skills>")
	return strings.Join(lines, "\n")
}

func (a *piAdapter) RenderInitialCatalog(skills []EffectiveSkill) CatalogRender {
	return CatalogRender{Text: a.RenderVisibleMetadata(skills), IncludedCount: len(skills)}
}

// piAgentDir is pi_agent_dir.
func piAgentDir(ctx *DiscoveryContext) string {
	if v, ok := envOr("PI_CODING_AGENT_DIR"); ok {
		return resolveAgentPath(v, ctx.Home, ctx.Home)
	}
	return filepath.Join(ctx.Home, ".pi/agent")
}

// piProjectIsTrusted is pi_project_is_trusted: trust.json maps path →
// bool; the nearest ancestor entry wins; missing/invalid → trusted.
func piProjectIsTrusted(ctx *DiscoveryContext) bool {
	content, err := readFileString(filepath.Join(piAgentDir(ctx), "trust.json"))
	if err != nil {
		return true
	}
	var entries map[string]any
	if err := jsonUnmarshal([]byte(content), &entries); err != nil {
		return true
	}
	current := canonicalize(ctx.Cwd)
	for {
		if decision, ok := entries[current]; ok {
			if b, ok := decision.(bool); ok {
				return b
			}
			return true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return true
		}
		current = parent
	}
}

// piConfiguredSkillRoots is pi_configured_skill_roots.
func piConfiguredSkillRoots(ctx *DiscoveryContext, projectTrusted bool) (projectRoots, userRoots []*SkillRoot) {
	if projectTrusted {
		projectRoots = piSettingsSkillRoots(
			filepath.Join(ctx.Cwd, ".pi/settings.json"),
			filepath.Join(ctx.Cwd, ".pi"),
			ctx, ScopeRepository)
	}
	agentDir := piAgentDir(ctx)
	userRoots = piSettingsSkillRoots(
		filepath.Join(agentDir, "settings.json"),
		agentDir, ctx, ScopeUser)
	return
}

// piPackageSkillRoots is pi_package_skill_roots.
func piPackageSkillRoots(settingsPath, installRoot string, scope SkillScope) []*SkillRoot {
	settings := readJSONCFile(settingsPath)
	if settings == nil {
		return nil
	}
	packages, ok := settings["packages"].([]any)
	if !ok {
		return nil
	}
	var roots []*SkillRoot
	for _, pkg := range packages {
		var source string
		if s, ok := pkg.(string); ok {
			source = s
		} else if obj, ok := pkg.(map[string]any); ok {
			source, _ = obj["source"].(string)
		}
		packageName := piNPMPackageName(source)
		if packageName == "" {
			continue
		}
		packageRoot := filepath.Join(installRoot, packageName)
		manifest := readJSONFile(filepath.Join(packageRoot, "package.json"))
		var entries []any
		if manifest != nil {
			if pi, ok := manifest["pi"].(map[string]any); ok {
				entries, _ = pi["skills"].([]any)
			}
		}
		if entries != nil {
			for _, entry := range entries {
				s, ok := entry.(string)
				if !ok || strings.HasPrefix(s, "!") || strings.Contains(s, "*") {
					continue
				}
				path := filepath.Join(packageRoot, s)
				if isFile(path) {
					roots = append(roots, ExactSkillRoot(path, scope))
				} else {
					roots = append(roots, NewSkillRoot(path, scope).WithRootMarkdown())
				}
			}
		} else {
			roots = append(roots, NewSkillRoot(filepath.Join(packageRoot, "skills"), scope).WithRootMarkdown())
		}
	}
	return roots
}

// piNPMPackageName is pi_npm_package_name.
func piNPMPackageName(source string) string {
	source = strings.TrimPrefix(source, "npm:")
	if strings.HasPrefix(source, "git:") || strings.Contains(source, "://") || strings.HasPrefix(source, "github:") {
		return ""
	}
	if strings.HasPrefix(source, "@") {
		slash := strings.Index(source, "/")
		if slash < 0 {
			return ""
		}
		version := len(source)
		if idx := strings.Index(source[slash+1:], "@"); idx >= 0 {
			version = slash + 1 + idx
		}
		return source[:version]
	}
	if i := strings.Index(source, "@"); i >= 0 {
		return source[:i]
	}
	return source
}

// piSettingsSkillRoots is pi_settings_skill_roots.
func piSettingsSkillRoots(settingsPath, relativeBase string, ctx *DiscoveryContext, scope SkillScope) []*SkillRoot {
	value := readJSONCFile(settingsPath)
	if value == nil {
		return nil
	}
	entries, ok := value["skills"]
	if !ok {
		return nil
	}
	var list []any
	if arr, ok := entries.([]any); ok {
		list = arr
	} else if obj, ok := entries.(map[string]any); ok {
		list, _ = obj["customDirectories"].([]any)
	}
	var roots []*SkillRoot
	for _, item := range list {
		entry, ok := item.(string)
		if !ok || strings.HasPrefix(entry, "!") {
			continue
		}
		path := resolveAgentPath(entry, ctx.Home, relativeBase)
		if filepath.Ext(path) == ".md" {
			roots = append(roots, ExactSkillRoot(path, scope))
		} else {
			roots = append(roots, NewSkillRoot(path, scope).WithRootMarkdown())
		}
	}
	return roots
}

var _ = os.PathSeparator
