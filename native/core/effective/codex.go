// codex.go ports the Codex adapter: recursive scan, strict frontmatter,
// config.toml plugin/disabled-skill parsing, codex plugin roots, and
// the codex catalog render.
package effective

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// codexAdapter is CodexAdapter.
type codexAdapter struct{ policyBase }

func newCodexAdapter() *codexAdapter {
	return &codexAdapter{policyBase{agent: AgentCodex}}
}

func (a *codexAdapter) Roots(ctx *DiscoveryContext) []*SkillRoot {
	var roots []*SkillRoot
	if !isGlobalContext(ctx) {
		projectConfigRoot := ctx.Cwd
		if ctx.RepositoryRoot != "" {
			projectConfigRoot = ctx.RepositoryRoot
		}
		roots = append(roots, NewSkillRoot(filepath.Join(projectConfigRoot, ".codex/skills"), ScopeRepository))
		if projectConfigRoot != ctx.Cwd {
			roots = append(roots, NewSkillRoot(filepath.Join(ctx.Cwd, ".codex/skills"), ScopeLocal))
		}
		for _, dir := range cwdToBoundary(ctx.Cwd, ctx.RepositoryRoot) {
			roots = append(roots, NewSkillRoot(filepath.Join(dir, ".agents/skills"), ScopeRepository))
		}
	}
	codexHome := codexHome(ctx)
	roots = append(roots, NewSkillRoot(filepath.Join(codexHome, "skills"), ScopeUser))
	roots = append(roots, NewSkillRoot(filepath.Join(ctx.Home, ".agents/skills"), ScopeUser))
	roots = append(roots, NewSkillRoot(filepath.Join(codexHome, "skills/.system"), ScopeSystem).WithoutDirectorySymlinks())
	if isUnix {
		roots = append(roots, NewSkillRoot("/etc/codex/skills", ScopeSystem))
	}
	roots = append(roots, codexPluginRoots(codexHome)...)
	return roots
}

func (a *codexAdapter) IsEnabled(_ string, metadata *SkillMetadata, ctx *DiscoveryContext) bool {
	if !metadata.HasExplicitDescription {
		return false
	}
	disabled := codexDisabledSkills(codexHome(ctx), ctx.Home)
	sourceFile := canonicalize(metadata.SourcePath)
	sourceDir := canonicalize(filepath.Dir(metadata.SourcePath))
	return !disabled[sourceFile] && !disabled[sourceDir]
}

func (a *codexAdapter) IsEnabledForRoot(_ *SkillRoot, skillDir string, m *SkillMetadata, ctx *DiscoveryContext) bool {
	return a.IsEnabled(skillDir, m, ctx)
}
func (a *codexAdapter) IsEnabledForEntry(root *SkillRoot, skillDir, _ string, m *SkillMetadata, ctx *DiscoveryContext) bool {
	return a.IsEnabledForRoot(root, skillDir, m, ctx)
}
func (a *codexAdapter) EffectiveNameForRoot(_ *SkillRoot, skillDir string, m *SkillMetadata) string {
	return a.EffectiveName(skillDir, m)
}
func (a *codexAdapter) EffectiveIDForRoot(_ *SkillRoot, skillDir, _ string, m *SkillMetadata) string {
	return a.EffectiveName(skillDir, m)
}
func (a *codexAdapter) PromptPathForEntry(_ *SkillRoot, _, skillFile string, _ *SkillMetadata, _ *DiscoveryContext) string {
	return skillFile
}
func (a *codexAdapter) Visibility(skillDir string, _ *SkillMetadata, _ *DiscoveryContext) SkillVisibility {
	if codexManualOnly(skillDir) {
		return VisibilityManualOnly
	}
	return VisibilityAutomatic
}
func (a *codexAdapter) VisibilityForRoot(_ *SkillRoot, skillDir string, m *SkillMetadata, ctx *DiscoveryContext) SkillVisibility {
	return a.Visibility(skillDir, m, ctx)
}
func (a *codexAdapter) RenderVisibleMetadata(skills []EffectiveSkill) string {
	return renderCodexListing(skills, 8000).Text
}
func (a *codexAdapter) RenderInitialCatalog(skills []EffectiveSkill) CatalogRender {
	return renderCodexListing(skills, 8000)
}

// codexHome is codex_home.
func codexHome(ctx *DiscoveryContext) string {
	if v, ok := envOr("CODEX_HOME"); ok {
		return v
	}
	return filepath.Join(ctx.Home, ".codex")
}

// enabledPluginIDs is enabled_plugin_ids (line parser for
// [plugins."id"] … enabled = true).
func enabledPluginIDs(config string) []string {
	var current string
	hasCurrent := false
	var enabled []string
	for _, raw := range strings.Split(config, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "[plugins.\"") && strings.HasSuffix(line, "\"]") {
			current = line[len("[plugins.\"") : len(line)-2]
			hasCurrent = true
		} else if strings.HasPrefix(line, "[") {
			hasCurrent = false
		} else if line == "enabled = true" && hasCurrent {
			enabled = append(enabled, current)
			hasCurrent = false
		}
	}
	return enabled
}

// codexDisabledSkills is codex_disabled_skills: [[skills.config]]
// entries with enabled = false, resolved then canonicalized.
func codexDisabledSkills(codexHome, home string) map[string]bool {
	data, err := os.ReadFile(filepath.Join(codexHome, "config.toml"))
	if err != nil {
		return nil
	}
	disabled := map[string]bool{}
	inSkill := false
	path := ""
	hasPath := false
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "[[skills.config]]" {
			inSkill = true
			hasPath = false
			path = ""
		} else if strings.HasPrefix(line, "[") {
			inSkill = false
			hasPath = false
		} else if inSkill {
			if strings.HasPrefix(line, "path = \"") && strings.HasSuffix(line, "\"") {
				path = line[len("path = \"") : len(line)-1]
				hasPath = true
			} else if line == "enabled = false" && hasPath {
				resolved := resolveAgentPath(path, home, codexHome)
				disabled[canonicalize(resolved)] = true
				hasPath = false
			}
		}
	}
	return disabled
}

// skillConfigEntries is the #[cfg(test)] skill_config_entries: ordered
// [[skills.config]] (path, enabled) pairs.
func skillConfigEntries(config string) [][2]any {
	var entries [][2]any
	inSkill := false
	path := ""
	hasPath := false
	enabled := true
	flush := func() {
		if hasPath {
			entries = append(entries, [2]any{path, enabled})
			hasPath = false
		}
	}
	for _, raw := range strings.Split(config, "\n") {
		line := strings.TrimSpace(raw)
		if line == "[[skills.config]]" {
			if inSkill {
				flush()
			}
			inSkill = true
			enabled = true
		} else if strings.HasPrefix(line, "[") {
			if inSkill {
				flush()
			}
			inSkill = false
		} else if inSkill {
			if strings.HasPrefix(line, "path = \"") && strings.HasSuffix(line, "\"") {
				path = line[len("path = \"") : len(line)-1]
				hasPath = true
			} else if line == "enabled = false" {
				enabled = false
			} else if line == "enabled = true" {
				enabled = true
			}
		}
	}
	if inSkill {
		flush()
	}
	return entries
}

// codexPluginRoots is codex_plugin_roots.
func codexPluginRoots(codexHome string) []*SkillRoot {
	data, err := os.ReadFile(filepath.Join(codexHome, "config.toml"))
	if err != nil {
		return nil
	}
	var roots []*SkillRoot
	for _, id := range enabledPluginIDs(string(data)) {
		parts := strings.Split(id, "@")
		if len(parts) < 2 {
			continue
		}
		plugin, marketplace := parts[0], parts[len(parts)-1]
		// rsplit_once('@') — plugin may itself contain '@'? Rust's
		// rsplit_once splits on the LAST '@'.
		if i := strings.LastIndex(id, "@"); i > 0 {
			plugin, marketplace = id[:i], id[i+1:]
		} else {
			continue
		}
		versions := filepath.Join(codexHome, "plugins/cache", marketplace, plugin)
		entries, err := os.ReadDir(versions)
		if err != nil {
			continue
		}
		var names []string
		for _, entry := range entries {
			if entry.IsDir() {
				names = append(names, entry.Name())
			}
		}
		if len(names) == 0 {
			continue
		}
		sort.Strings(names)
		pluginRoot := filepath.Join(versions, names[len(names)-1])
		for _, root := range pluginSkillRoots(pluginRoot, ".codex-plugin/plugin.json", id, "skills", ScopeSystem, pluginDisplayFallback(plugin)) {
			roots = append(roots, root.DirectChildren())
		}
	}
	return roots
}

// pluginSkillRoots is plugin_skill_roots.
func pluginSkillRoots(pluginRoot, manifestRelativePath, pluginID, fallbackSkillPath string, scope SkillScope, fallbackDisplayName string) []*SkillRoot {
	manifestPath := filepath.Join(pluginRoot, manifestRelativePath)
	displayName := fallbackDisplayName
	var skillPaths []string
	if isFile(manifestPath) {
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			return nil
		}
		var manifest map[string]any
		if err := jsonUnmarshal(data, &manifest); err != nil {
			return nil
		}
		name := ""
		if iface, ok := manifest["interface"].(map[string]any); ok {
			if v, ok := iface["displayName"].(string); ok {
				name = v
			}
		}
		if name == "" {
			if v, ok := manifest["displayName"].(string); ok {
				name = v
			}
		}
		if name == "" {
			if v, ok := manifest["display_name"].(string); ok {
				name = v
			}
		}
		if name == "" {
			if v, ok := manifest["name"].(string); ok {
				name = v
			}
		}
		if strings.TrimSpace(name) != "" {
			displayName = name
		}
		var skillsValue any
		if paths, ok := manifest["paths"].(map[string]any); ok {
			if v, has := paths["skills"]; has {
				skillsValue = v
			} else {
				skillsValue = manifest["skills"]
			}
		} else {
			skillsValue = manifest["skills"]
		}
		switch v := skillsValue.(type) {
		case nil:
			skillPaths = []string{fallbackSkillPath}
		case string:
			skillPaths = []string{v}
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok {
					skillPaths = append(skillPaths, s)
				}
			}
		default:
			// Explicit malformed contribution: do not fall back.
			skillPaths = nil
		}
	} else {
		skillPaths = []string{fallbackSkillPath}
	}
	source := SkillSource{Kind: SourcePlugin, ID: pluginID, DisplayName: displayName}
	var roots []*SkillRoot
	for _, relative := range skillPaths {
		if filepath.IsAbs(relative) {
			continue
		}
		path := filepath.Join(pluginRoot, relative)
		if isFile(path) {
			roots = append(roots, ExactSkillRoot(path, scope).WithSource(source))
		} else {
			roots = append(roots, NewSkillRoot(path, scope).WithSource(source))
		}
	}
	return roots
}

// isUnix is the #[cfg(unix)] system-root gate.
const isUnix = true
