// openclaw.go ports the OpenClaw and Hermes adapters: openclaw.json
// config (extraDirs, disabled entries, plugin allow/deny, extensions
// dir) and hermes config.yaml external_dirs.
package effective

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// openClawPolicy is OpenClawPolicy.
type openClawPolicy struct{ policyBase }

func newOpenClawPolicy() *openClawPolicy {
	return &openClawPolicy{policyBase{agent: AgentOpenClaw}}
}

func (p *openClawPolicy) Roots(ctx *DiscoveryContext) []*SkillRoot {
	stateDir := openclawStateDir(ctx)
	config := openclawConfig(ctx)
	var roots []*SkillRoot
	if !isGlobalContext(ctx) {
		roots = append(roots, NewSkillRoot(filepath.Join(ctx.Cwd, "skills"), ScopeLocal))
		roots = append(roots, NewSkillRoot(filepath.Join(ctx.Cwd, ".agents/skills"), ScopeLocal))
	}
	roots = append(roots, NewSkillRoot(filepath.Join(ctx.Home, ".agents/skills"), ScopeUser))
	roots = append(roots, NewSkillRoot(filepath.Join(stateDir, "skills"), ScopeUser))
	for _, path := range openclawExtraSkillDirs(config, ctx) {
		roots = append(roots, NewSkillRoot(path, ScopeUser))
	}
	roots = append(roots, openclawPluginRoots(config, stateDir, ctx)...)
	return roots
}

func (p *openClawPolicy) IsEnabled(_ string, metadata *SkillMetadata, ctx *DiscoveryContext) bool {
	return !openclawDisabledSkills(openclawConfig(ctx))[metadata.Name]
}
func (p *openClawPolicy) IsEnabledForRoot(_ *SkillRoot, skillDir string, m *SkillMetadata, ctx *DiscoveryContext) bool {
	return p.IsEnabled(skillDir, m, ctx)
}
func (p *openClawPolicy) IsEnabledForEntry(root *SkillRoot, skillDir, _ string, m *SkillMetadata, ctx *DiscoveryContext) bool {
	return p.IsEnabledForRoot(root, skillDir, m, ctx)
}
func (p *openClawPolicy) EffectiveNameForRoot(_ *SkillRoot, skillDir string, m *SkillMetadata) string {
	return m.Name
}
func (p *openClawPolicy) EffectiveIDForRoot(_ *SkillRoot, skillDir, _ string, m *SkillMetadata) string {
	return m.Name
}
func (p *openClawPolicy) NameCollision() NameCollision { return CollisionFirstWins }
func (p *openClawPolicy) Visibility(_ string, m *SkillMetadata, _ *DiscoveryContext) SkillVisibility {
	return frontmatterVisibility(m)
}
func (p *openClawPolicy) VisibilityForRoot(_ *SkillRoot, skillDir string, m *SkillMetadata, ctx *DiscoveryContext) SkillVisibility {
	return p.Visibility(skillDir, m, ctx)
}
func (p *openClawPolicy) RenderVisibleMetadata(skills []EffectiveSkill) string {
	if len(skills) == 0 {
		return ""
	}
	var body []string
	for _, s := range skills {
		body = append(body, "  <skill>\n    <name>"+xmlEscape(s.Name)+"</name>\n    <description>"+xmlEscape(s.Description)+"</description>\n  </skill>")
	}
	return truncateChars("<available_skills>\n"+strings.Join(body, "\n")+"\n</available_skills>", 30000)
}
func (p *openClawPolicy) RenderInitialCatalog(skills []EffectiveSkill) CatalogRender {
	return CatalogRender{Text: p.RenderVisibleMetadata(skills), IncludedCount: len(skills)}
}

// openclawStateDir is openclaw_state_dir.
func openclawStateDir(ctx *DiscoveryContext) string {
	if v, ok := envOr("OPENCLAW_STATE_DIR"); ok {
		return v
	}
	if v, ok := envOr("CLAWDBOT_STATE_DIR"); ok {
		return v
	}
	return filepath.Join(ctx.Home, ".openclaw")
}

// openclawConfig is openclaw_config.
func openclawConfig(ctx *DiscoveryContext) map[string]any {
	stateDir := openclawStateDir(ctx)
	path := filepath.Join(stateDir, "openclaw.json")
	if v, ok := envOr("OPENCLAW_CONFIG_PATH"); ok {
		path = v
	}
	value := readJSONCFile(path)
	if value == nil {
		return map[string]any{}
	}
	return value
}

// openclawExtraSkillDirs is openclaw_extra_skill_dirs.
func openclawExtraSkillDirs(config map[string]any, ctx *DiscoveryContext) []string {
	var out []string
	for _, item := range jsonArrayAt(config, "skills", "load", "extraDirs") {
		if s, ok := item.(string); ok {
			out = append(out, resolveAgentPath(s, ctx.Home, ctx.Cwd))
		}
	}
	return out
}

// openclawDisabledSkills is openclaw_disabled_skills.
func openclawDisabledSkills(config map[string]any) map[string]bool {
	out := map[string]bool{}
	entries, _ := jsonGet(config, "skills", "entries").(map[string]any)
	for name, entry := range entries {
		if obj, ok := entry.(map[string]any); ok {
			if enabled, ok := obj["enabled"].(bool); ok && !enabled {
				out[name] = true
			}
		}
	}
	return out
}

// openclawPluginRoots is openclaw_plugin_roots.
func openclawPluginRoots(config map[string]any, stateDir string, ctx *DiscoveryContext) []*SkillRoot {
	if enabled, ok := jsonGet(config, "plugins", "enabled").(bool); ok && !enabled {
		return nil
	}
	var allow map[string]bool
	if values := jsonArrayAt(config, "plugins", "allow"); values != nil {
		allow = map[string]bool{}
		for _, v := range values {
			if s, ok := v.(string); ok {
				allow[s] = true
			}
		}
	}
	deny := map[string]bool{}
	for _, v := range jsonArrayAt(config, "plugins", "deny") {
		if s, ok := v.(string); ok {
			deny[s] = true
		}
	}
	var candidates []string
	if entries, err := os.ReadDir(filepath.Join(stateDir, "extensions")); err == nil {
		for _, entry := range entries {
			candidates = append(candidates, filepath.Join(stateDir, "extensions", entry.Name()))
		}
	}
	for _, v := range jsonArrayAt(config, "plugins", "load", "paths") {
		if s, ok := v.(string); ok {
			candidates = append(candidates, resolveAgentPath(s, ctx.Home, stateDir))
		}
	}
	seen := map[string]bool{}
	var roots []*SkillRoot
	for _, candidate := range candidates {
		pluginRoot := candidate
		if isFile(candidate) {
			pluginRoot = filepath.Dir(candidate)
		}
		canonical := canonicalize(pluginRoot)
		if seen[canonical] {
			continue
		}
		seen[canonical] = true
		manifest := readJSONFile(filepath.Join(pluginRoot, "openclaw.plugin.json"))
		if manifest == nil {
			continue
		}
		id, ok := manifest["id"].(string)
		if !ok {
			continue
		}
		entryDisabled := false
		if plugins, ok := config["plugins"].(map[string]any); ok {
			if entries, ok := plugins["entries"].(map[string]any); ok {
				if entry, ok := entries[id].(map[string]any); ok {
					if e, ok := entry["enabled"].(bool); ok && !e {
						entryDisabled = true
					}
				}
			}
		}
		if deny[id] || (allow != nil && !allow[id]) || entryDisabled {
			continue
		}
		displayName := id
		if v, ok := manifest["name"].(string); ok {
			displayName = v
		}
		var skillPaths []string
		if arr, ok := manifest["skills"].([]any); ok {
			for _, v := range arr {
				if s, ok := v.(string); ok {
					skillPaths = append(skillPaths, s)
				}
			}
		}
		for _, relative := range skillPaths {
			if filepath.IsAbs(relative) || strings.Contains(filepath.ToSlash(relative), "..") {
				continue
			}
			roots = append(roots, PluginSkillRoot(
				filepath.Join(pluginRoot, relative), ScopeUser, id, displayName))
		}
	}
	return roots
}

// jsonArrayAt is config.pointer("/a/b/c") .as_array().
func jsonArrayAt(config map[string]any, keys ...string) []any {
	arr, _ := jsonGet(config, keys...).([]any)
	return arr
}

// hermesPolicy is HermesPolicy.
type hermesPolicy struct{ compatiblePolicy }

func newHermesPolicy() *hermesPolicy {
	p := &hermesPolicy{}
	p.agent = AgentHermes
	p.forceAuto = true
	p.validity = func(*SkillMetadata) bool { return true }
	return p
}

func (p *hermesPolicy) Roots(ctx *DiscoveryContext) []*SkillRoot {
	home := hermesHome(ctx)
	roots := []*SkillRoot{NewSkillRoot(filepath.Join(home, "skills"), ScopeUser)}
	for _, dir := range hermesExternalSkillDirs(ctx, home) {
		roots = append(roots, NewSkillRoot(dir, ScopeUser))
	}
	return roots
}

func (p *hermesPolicy) RenderVisibleMetadata(skills []EffectiveSkill) string {
	if len(skills) == 0 {
		return ""
	}
	var body []string
	for _, s := range skills {
		body = append(body, "- "+s.Name+": "+truncateChars(s.Description, 57))
	}
	return "<available_skills>\n" + strings.Join(body, "\n") + "\n</available_skills>"
}

// hermesHome is hermes_home.
func hermesHome(ctx *DiscoveryContext) string {
	if v, ok := envOr("HERMES_HOME"); ok {
		return v
	}
	return filepath.Join(ctx.Home, ".hermes")
}

// hermesExternalSkillDirs is hermes_external_skill_dirs.
func hermesExternalSkillDirs(ctx *DiscoveryContext, hermesHome string) []string {
	content, err := readFileString(filepath.Join(hermesHome, "config.yaml"))
	if err != nil {
		return nil
	}
	var config yamlNode
	if err := yamlUnmarshalErr([]byte(content), &config); err != nil {
		return nil
	}
	skills := yamlNodeGet(&config, "skills")
	if skills == nil {
		return nil
	}
	external := yamlNodeGet(skills, "external_dirs")
	if external == nil || external.Kind != yaml.SequenceNode {
		return nil
	}
	var out []string
	for _, item := range external.Content {
		if item.Kind == yaml.ScalarNode && item.Tag == "!!str" {
			out = append(out, resolveAgentPath(item.Value, ctx.Home, hermesHome))
		}
	}
	return out
}
