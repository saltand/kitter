// claude.go ports the ClaudeCode adapter: direct-children scan with
// body-fallback metadata, settings.json skillOverrides/enabledPlugins,
// plugin roots from installed_plugins.json, and the
// system-reminder listing render.
package effective

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// claudeCodeAdapter is ClaudeCodeAdapter.
type claudeCodeAdapter struct{ policyBase }

func newClaudeCodeAdapter() *claudeCodeAdapter {
	return &claudeCodeAdapter{policyBase{agent: AgentClaudeCode}}
}

func (a *claudeCodeAdapter) Roots(ctx *DiscoveryContext) []*SkillRoot {
	var roots []*SkillRoot
	// macOS managed roots (cfg target_os = "macos").
	if isDarwin {
		managed := "/Library/Application Support/ClaudeCode/.claude"
		roots = append(roots, NewSkillRoot(filepath.Join(managed, "skills"), ScopeSystem))
		roots = append(roots, NewSkillRoot(filepath.Join(managed, "commands"), ScopeSystem).FlatMarkdown())
	} else if isUnixNotDarwin {
		roots = append(roots, NewSkillRoot("/etc/claude-code/.claude/skills", ScopeSystem))
	}
	userConfig := claudeConfigDir(ctx)
	roots = append(roots, NewSkillRoot(filepath.Join(userConfig, "skills"), ScopeUser))
	roots = append(roots, NewSkillRoot(filepath.Join(userConfig, "commands"), ScopeUser).FlatMarkdown())
	if !isGlobalContext(ctx) {
		for _, dir := range cwdToBoundary(ctx.Cwd, ctx.RepositoryRoot) {
			roots = append(roots, NewSkillRoot(filepath.Join(dir, ".claude/skills"), ScopeRepository))
			roots = append(roots, NewSkillRoot(filepath.Join(dir, ".claude/commands"), ScopeRepository).FlatMarkdown())
		}
	}
	roots = append(roots, claudePluginRoots(ctx)...)
	return roots
}

func (a *claudeCodeAdapter) EffectiveName(skillDir string, metadata *SkillMetadata) string {
	localName := claudeLocalName(skillDir, metadata)
	if plugin := claudePluginName(skillDir); plugin != "" {
		return plugin + ":" + localName
	}
	return localName
}

func (a *claudeCodeAdapter) EffectiveNameForRoot(root *SkillRoot, skillDir string, metadata *SkillMetadata) string {
	localName := claudeLocalName(skillDir, metadata)
	if plugin, ok := root.Source.PluginDisplayName(); ok {
		return plugin + ":" + localName
	}
	return a.EffectiveName(skillDir, metadata)
}

func (a *claudeCodeAdapter) EffectiveIDForRoot(root *SkillRoot, skillDir, _ string, m *SkillMetadata) string {
	return a.EffectiveNameForRoot(root, skillDir, m)
}

func (a *claudeCodeAdapter) IsEnabled(skillDir string, metadata *SkillMetadata, ctx *DiscoveryContext) bool {
	if claudePluginName(skillDir) != "" {
		return metadata.HasExplicitDescription || metadata.HasWhenToUse
	}
	return claudeSkillOverride(ctx, a.EffectiveName(skillDir, metadata)) != "off"
}

func (a *claudeCodeAdapter) IsEnabledForRoot(root *SkillRoot, skillDir string, m *SkillMetadata, ctx *DiscoveryContext) bool {
	if root.Source.IsPlugin() {
		return m.HasExplicitDescription || m.HasWhenToUse
	}
	return a.IsEnabled(skillDir, m, ctx)
}

func (a *claudeCodeAdapter) IsEnabledForEntry(root *SkillRoot, skillDir, _ string, m *SkillMetadata, ctx *DiscoveryContext) bool {
	return a.IsEnabledForRoot(root, skillDir, m, ctx)
}

func (a *claudeCodeAdapter) Visibility(skillDir string, metadata *SkillMetadata, ctx *DiscoveryContext) SkillVisibility {
	if claudePluginName(skillDir) != "" {
		return frontmatterVisibility(metadata)
	}
	switch claudeSkillOverride(ctx, a.EffectiveName(skillDir, metadata)) {
	case "name-only":
		return VisibilityNameOnly
	case "user-invocable-only":
		return VisibilityManualOnly
	case "on":
		return VisibilityAutomatic
	default:
		return frontmatterVisibility(metadata)
	}
}

func (a *claudeCodeAdapter) VisibilityForRoot(root *SkillRoot, skillDir string, m *SkillMetadata, ctx *DiscoveryContext) SkillVisibility {
	if root.Source.IsPlugin() {
		return frontmatterVisibility(m)
	}
	return a.Visibility(skillDir, m, ctx)
}

func (a *claudeCodeAdapter) RenderVisibleMetadata(skills []EffectiveSkill) string {
	if len(skills) == 0 {
		return ""
	}
	var entries [][2]string
	for _, skill := range skills {
		if skill.Visibility == VisibilityNameOnly {
			entries = append(entries, [2]string{skill.Name, ""})
			continue
		}
		description := skill.Description
		if skill.HasWhenToUse && skill.WhenToUse != "" {
			description = description + " - " + skill.WhenToUse
		}
		entries = append(entries, [2]string{skill.Name, truncateChars(description, 250)})
	}
	return "<system-reminder>\n" + renderClaudeListing(entries, 8000) + "\n</system-reminder>"
}

func (a *claudeCodeAdapter) RenderInitialCatalog(skills []EffectiveSkill) CatalogRender {
	return CatalogRender{Text: a.RenderVisibleMetadata(skills), IncludedCount: len(skills)}
}

// claudeConfigDir is claude_config_dir.
func claudeConfigDir(ctx *DiscoveryContext) string {
	if v, ok := envOr("CLAUDE_CONFIG_DIR"); ok {
		return v
	}
	return filepath.Join(ctx.Home, ".claude")
}

// claudeSettingsFiles is claude_settings_files.
func claudeSettingsFiles(ctx *DiscoveryContext) []string {
	projectRoot := ctx.Cwd
	if ctx.RepositoryRoot != "" {
		projectRoot = ctx.RepositoryRoot
	}
	files := []string{
		filepath.Join(ctx.Home, ".claude/settings.json"),
		filepath.Join(projectRoot, ".claude/settings.json"),
		filepath.Join(projectRoot, ".claude/settings.local.json"),
	}
	if isDarwin {
		files = append(files, "/Library/Application Support/ClaudeCode/managed-settings.json")
	} else if isUnixNotDarwin {
		files = append(files, "/etc/claude-code/managed-settings.json")
	}
	return files
}

// claudeSkillOverrides is claude_skill_overrides: later files override.
func claudeSkillOverrides(ctx *DiscoveryContext) map[string]string {
	overrides := map[string]string{}
	for _, file := range claudeSettingsFiles(ctx) {
		value := readJSONFile(file)
		if value == nil {
			continue
		}
		entries, ok := value["skillOverrides"].(map[string]any)
		if !ok {
			continue
		}
		for name, state := range entries {
			if s, ok := state.(string); ok {
				overrides[name] = s
			}
		}
	}
	return overrides
}

// claudeSkillOverride is claude_skill_override.
func claudeSkillOverride(ctx *DiscoveryContext, name string) string {
	return claudeSkillOverrides(ctx)[name]
}

// claudePluginRoots is claude_plugin_roots.
func claudePluginRoots(ctx *DiscoveryContext) []*SkillRoot {
	enabled := map[string]bool{}
	for _, file := range claudeSettingsFiles(ctx) {
		value := readJSONFile(file)
		if value == nil {
			continue
		}
		plugins, ok := value["enabledPlugins"].(map[string]any)
		if !ok {
			continue
		}
		for id, state := range plugins {
			if b, ok := state.(bool); ok {
				if b {
					enabled[id] = true
				} else {
					delete(enabled, id)
				}
			}
		}
	}
	value := readJSONFile(filepath.Join(ctx.Home, ".claude/plugins/installed_plugins.json"))
	if value == nil {
		return nil
	}
	plugins, ok := value["plugins"].(map[string]any)
	if !ok {
		return nil
	}
	projectRoot := ctx.Cwd
	if ctx.RepositoryRoot != "" {
		projectRoot = ctx.RepositoryRoot
	}
	globalOnly := isGlobalContext(ctx)
	var roots []*SkillRoot
	// Rust iterates a serde_json Map (sorted keys).
	for _, id := range sortedKeys(plugins) {
		if !enabled[id] {
			continue
		}
		installations, ok := plugins[id].([]any)
		if !ok {
			continue
		}
		for _, installation := range installations {
			inst, ok := installation.(map[string]any)
			if !ok {
				continue
			}
			scopeStr, _ := inst["scope"].(string)
			applies := scopeStr == "user" ||
				(!globalOnly && scopeStr == "project" &&
					inst["projectPath"] == projectRoot)
			if !applies {
				continue
			}
			path, ok := inst["installPath"].(string)
			if !ok {
				continue
			}
			scope := ScopeSystem
			switch scopeStr {
			case "project":
				scope = ScopeRepository
			case "user":
				scope = ScopeUser
			}
			roots = append(roots, pluginSkillRoots(
				path, ".claude-plugin/plugin.json", id, "skills", scope,
				pluginDisplayFallback(id))...)
		}
	}
	return roots
}

// claudePluginName is claude_plugin_name: the segment after
// .claude/plugins/cache/<marketplace>/ in a 5-component window.
func claudePluginName(skillDir string) string {
	parts := strings.Split(filepath.ToSlash(skillDir), "/")
	for i := 0; i+4 < len(parts); i++ {
		if parts[i] == ".claude" && parts[i+1] == "plugins" && parts[i+2] == "cache" {
			return parts[i+4]
		}
	}
	return ""
}

// claudeLocalName is claude_local_name.
func claudeLocalName(skillDir string, metadata *SkillMetadata) string {
	if filepath.Base(metadata.SourcePath) != "SKILL.md" {
		return metadata.Name
	}
	if base := filepath.Base(skillDir); base != "" && base != "/" && base != "." {
		return base
	}
	return metadata.Name
}

// readJSONFile reads strict JSON (no JSONC normalization — matches
// Rust serde_json::from_str on the raw content). Parse failures → nil.
func readJSONFile(path string) map[string]any {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var value map[string]any
	if err := jsonUnmarshal(data, &value); err != nil {
		return nil
	}
	return value
}

// sortedKeys returns map keys sorted (serde_json Map iteration order).
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// cfg gates mirror Rust's #[cfg(...)] on the discovery roots. The Go
// build is cross-compiled, so these are runtime checks.
var isDarwin = runtime.GOOS == "darwin"
var isUnixNotDarwin = runtime.GOOS != "darwin" && runtime.GOOS != "windows"
