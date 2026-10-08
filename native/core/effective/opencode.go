// opencode.go ports the OpenCode adapter: config roots from
// opencode.json/jsonc (JSONC parsing, skills array or skills.paths),
// permission rules (wildcard matching), the customize-opencode builtin,
// v2 path-derived ids and autoinvoke visibility.
package effective

import (
	"path/filepath"
	"strings"
)

// openCodePolicy is OpenCodePolicy.
type openCodePolicy struct{ policyBase }

func newOpenCodePolicy() *openCodePolicy {
	return &openCodePolicy{policyBase{agent: AgentOpenCode}}
}

func (p *openCodePolicy) Roots(ctx *DiscoveryContext) []*SkillRoot {
	configHome := opencodeConfigHome(ctx)
	var roots []*SkillRoot
	var ancestorDirs []string
	if !isGlobalContext(ctx) {
		ancestorDirs = cwdToBoundary(ctx.Cwd, ctx.RepositoryRoot)
	}
	disableExternal := envTruthy("OPENCODE_DISABLE_EXTERNAL_SKILLS")
	disableClaude := envTruthy("OPENCODE_DISABLE_CLAUDE_CODE") ||
		envTruthy("OPENCODE_DISABLE_CLAUDE_CODE_SKILLS")
	if !disableExternal {
		if !disableClaude {
			roots = append(roots, NewSkillRoot(filepath.Join(ctx.Home, ".claude/skills"), ScopeUser).WithRootMarkdown())
			for i := len(ancestorDirs) - 1; i >= 0; i-- {
				roots = append(roots, NewSkillRoot(filepath.Join(ancestorDirs[i], ".claude/skills"), ScopeRepository).WithRootMarkdown())
			}
		}
		roots = append(roots, NewSkillRoot(filepath.Join(ctx.Home, ".agents/skills"), ScopeUser).WithRootMarkdown())
		for i := len(ancestorDirs) - 1; i >= 0; i-- {
			roots = append(roots, NewSkillRoot(filepath.Join(ancestorDirs[i], ".agents/skills"), ScopeRepository).WithRootMarkdown())
		}
	}
	for _, relative := range []string{"skills", "skill"} {
		roots = append(roots, NewSkillRoot(filepath.Join(configHome, relative), ScopeUser).WithRootMarkdown())
	}
	for i := len(ancestorDirs) - 1; i >= 0; i-- {
		for _, relative := range []string{"skills", "skill"} {
			roots = append(roots, NewSkillRoot(filepath.Join(ancestorDirs[i], ".opencode", relative), ScopeRepository).WithRootMarkdown())
		}
	}
	if custom, ok := envOr("OPENCODE_CONFIG_DIR"); ok {
		for _, relative := range []string{"skill", "skills"} {
			roots = append(roots, NewSkillRoot(filepath.Join(custom, relative), ScopeUser).WithRootMarkdown())
		}
	}
	if isDarwin {
		for _, relative := range []string{"skill", "skills"} {
			roots = append(roots, NewSkillRoot(filepath.Join("/Library/Application Support/opencode", relative), ScopeSystem).WithRootMarkdown())
		}
	}
	for _, sp := range opencodeConfig(ctx).skillPaths {
		roots = append(roots, NewSkillRoot(sp.path, sp.scope).WithRootMarkdown())
	}
	return roots
}

func (p *openCodePolicy) Builtins(ctx *DiscoveryContext) []EffectiveSkill {
	if opencodeSkillPermission(ctx, "customize-opencode") == "deny" {
		return nil
	}
	return []EffectiveSkill{{
		ID:          "customize-opencode",
		Name:        "customize-opencode",
		Description: "Use ONLY when the user is editing or creating opencode's own configuration: opencode.json, opencode.jsonc, files under .opencode/, or files under ~/.config/opencode/. Also use when creating or fixing opencode agents, subagents, skills, plugins, MCP servers, or permission rules. Do not use for the user's own application code, or for any project that is not configuring opencode itself.",
		Path:        "<built-in>",
		Scope:       ScopeSystem,
		Visibility:  VisibilityAutomatic,
		Source:      SkillSource{Kind: SourceBuiltin},
	}}
}

func (p *openCodePolicy) IsEnabled(_ string, _ *SkillMetadata, ctx *DiscoveryContext) bool {
	return opencodeSkillPermission(ctx, "*") != "deny"
}

func (p *openCodePolicy) IsEnabledForRoot(_ *SkillRoot, skillDir string, m *SkillMetadata, ctx *DiscoveryContext) bool {
	return p.IsEnabled(skillDir, m, ctx)
}

func (p *openCodePolicy) IsEnabledForEntry(root *SkillRoot, skillDir, skillFile string, _ *SkillMetadata, ctx *DiscoveryContext) bool {
	return opencodeSkillPermission(ctx, pathDerivedSkillID(root, skillDir, skillFile)) != "deny"
}

func (p *openCodePolicy) EffectiveNameForRoot(_ *SkillRoot, skillDir string, m *SkillMetadata) string {
	return m.Name
}

func (p *openCodePolicy) EffectiveIDForRoot(root *SkillRoot, skillDir, skillFile string, _ *SkillMetadata) string {
	return pathDerivedSkillID(root, skillDir, skillFile)
}

func (p *openCodePolicy) NameCollision() NameCollision { return CollisionLastWins }

func (p *openCodePolicy) Visibility(_ string, metadata *SkillMetadata, _ *DiscoveryContext) SkillVisibility {
	return opencodeVisibility(metadata)
}

func (p *openCodePolicy) VisibilityForRoot(_ *SkillRoot, skillDir string, m *SkillMetadata, ctx *DiscoveryContext) SkillVisibility {
	return p.Visibility(skillDir, m, ctx)
}

func (p *openCodePolicy) RenderVisibleMetadata(skills []EffectiveSkill) string {
	if len(skills) == 0 {
		return ""
	}
	var body []string
	for _, s := range skills {
		body = append(body, "  <skill>\n    <id>"+xmlEscape(s.ID)+"</id>\n    <name>"+xmlEscape(s.Name)+"</name>\n    <description>"+xmlEscape(s.Description)+"</description>\n  </skill>")
	}
	return "Skills provide specialized instructions and workflows for specific tasks.\nUse the skill tool to load a skill when a task matches its description.\n<available_skills>\n" + strings.Join(body, "\n") + "\n</available_skills>"
}

func (p *openCodePolicy) RenderInitialCatalog(skills []EffectiveSkill) CatalogRender {
	return CatalogRender{Text: p.RenderVisibleMetadata(skills), IncludedCount: len(skills)}
}

// opencodeVisibility is opencode_visibility.
func opencodeVisibility(metadata *SkillMetadata) SkillVisibility {
	if !metadata.HasExplicitDescription || strings.TrimSpace(metadata.Description) == "" {
		return VisibilityManualOnly
	}
	content, err := readFileString(metadata.SourcePath)
	if err != nil {
		return VisibilityAutomatic
	}
	frontmatter, ok := YAMLFrontmatter(content)
	if !ok {
		return VisibilityAutomatic
	}
	var parsed skillFrontmatter
	if yamlUnmarshalErr([]byte(frontmatter), &parsed) != nil {
		return VisibilityAutomatic
	}
	if yamlOpencodeAutoinvokeDisabled(&parsed.Metadata) {
		return VisibilityManualOnly
	}
	return VisibilityAutomatic
}

// yamlOpencodeAutoinvokeDisabled is yaml_opencode_autoinvoke_disabled.
func yamlOpencodeAutoinvokeDisabled(metadata *yamlNode) bool {
	if metadata == nil || metadata.Kind == 0 {
		return false
	}
	if v := yamlNodeGet(metadata, "opencode/autoinvoke"); v != nil && yamlValueFalse(v) {
		return true
	}
	opencode := yamlNodeGet(metadata, "opencode")
	if opencode == nil {
		return false
	}
	autoinvoke := yamlNodeGet(opencode, "autoinvoke")
	return autoinvoke != nil && yamlValueFalse(autoinvoke)
}

// openCodeConfigSnapshot is OpenCodeConfigSnapshot.
type openCodeConfigSnapshot struct {
	skillPaths      []skillPath
	permissionRules [][2]string
}

type skillPath struct {
	path  string
	scope SkillScope
}

// opencodeConfigHome is opencode_config_home.
func opencodeConfigHome(ctx *DiscoveryContext) string {
	base := filepath.Join(ctx.Home, ".config")
	if v, ok := envOr("XDG_CONFIG_HOME"); ok {
		base = v
	}
	return filepath.Join(base, "opencode")
}

// opencodeConfig is opencode_config.
func opencodeConfig(ctx *DiscoveryContext) *openCodeConfigSnapshot {
	configHome := opencodeConfigHome(ctx)
	type fileScope struct {
		path  string
		scope SkillScope
	}
	files := []fileScope{
		{filepath.Join(configHome, "opencode.json"), ScopeUser},
		{filepath.Join(configHome, "opencode.jsonc"), ScopeUser},
	}
	if custom, ok := envOr("OPENCODE_CONFIG"); ok {
		files = append(files, fileScope{custom, ScopeUser})
	}
	if !isGlobalContext(ctx) {
		dirs := cwdToBoundary(ctx.Cwd, ctx.RepositoryRoot)
		for i := len(dirs) - 1; i >= 0; i-- {
			for _, name := range []string{"opencode.json", "opencode.jsonc"} {
				path := filepath.Join(dirs[i], name)
				if isFile(path) {
					files = append(files, fileScope{path, ScopeRepository})
				}
			}
		}
		if projectConfig, ok := envOr("OPENCODE_PROJECT_CONFIG"); ok {
			files = append(files, fileScope{projectConfig, ScopeRepository})
		}
	}
	if isDarwin {
		files = append(files,
			fileScope{"/Library/Application Support/opencode/opencode.json", ScopeSystem},
			fileScope{"/Library/Application Support/opencode/opencode.jsonc", ScopeSystem},
		)
	}
	snapshot := &openCodeConfigSnapshot{}
	defaultAgent := "build"
	var values []map[string]any
	var scopes []SkillScope
	for _, file := range files {
		value := readJSONCFile(file.path)
		if value == nil {
			continue
		}
		if agent, ok := value["default_agent"].(string); ok {
			defaultAgent = agent
		}
		values = append(values, value)
		scopes = append(scopes, file.scope)
	}
	for i, value := range values {
		var paths any
		if skills, ok := value["skills"]; ok {
			if arr, ok := skills.([]any); ok {
				paths = arr
			} else if obj, ok := skills.(map[string]any); ok {
				paths = obj["paths"]
			}
		}
		if arr, ok := paths.([]any); ok {
			for _, item := range arr {
				if s, ok := item.(string); ok && !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
					snapshot.skillPaths = append(snapshot.skillPaths, skillPath{expandUserPath(s, ctx), scopes[i]})
				}
			}
		}
		appendOpencodePermissionRules(opencodePermissionValue(value), &snapshot.permissionRules)
		agentValue := value["agents"]
		if agentValue == nil {
			agentValue = value["agent"]
		}
		if agentObj, ok := agentValue.(map[string]any); ok {
			if agentCfg, ok := agentObj[defaultAgent].(map[string]any); ok {
				appendOpencodePermissionRules(opencodePermissionValue(agentCfg), &snapshot.permissionRules)
			}
		}
	}
	return snapshot
}

// appendOpencodePermissionRules is append_opencode_permission_rules.
func appendOpencodePermissionRules(value any, rules *[][2]string) {
	if value == nil {
		return
	}
	switch v := value.(type) {
	case string:
		*rules = append(*rules, [2]string{"*", v})
	case []any:
		for _, entry := range v {
			obj, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			if action, ok := obj["action"].(string); ok && action != "skill" {
				continue
			}
			var resource, effect string
			if r, ok := obj["resource"].(string); ok {
				resource = r
			} else if r, ok := obj["pattern"].(string); ok {
				resource = r
			}
			if resource == "" {
				continue
			}
			if e, ok := obj["effect"].(string); ok {
				effect = e
			} else if e, ok := obj["action"].(string); ok {
				effect = e
			}
			if effect == "" {
				continue
			}
			*rules = append(*rules, [2]string{resource, effect})
		}
	case map[string]any:
		for _, pattern := range sortedKeys(v) {
			if action, ok := v[pattern].(string); ok {
				*rules = append(*rules, [2]string{pattern, action})
			}
		}
	}
}

// opencodeSkillPermission is opencode_skill_permission: last matching
// rule wins.
func opencodeSkillPermission(ctx *DiscoveryContext, name string) string {
	action := "allow"
	for _, rule := range opencodeConfig(ctx).permissionRules {
		if wildcardMatch(rule[0], name) {
			action = rule[1]
		}
	}
	return action
}

// wildcardMatch is wildcard_match.
func wildcardMatch(pattern, value string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == value
	}
	rest := value
	for i, part := range parts {
		if part == "" {
			continue
		}
		idx := strings.Index(rest, part)
		if idx < 0 {
			return false
		}
		if i == 0 && !strings.HasPrefix(pattern, "*") && idx != 0 {
			return false
		}
		rest = rest[idx+len(part):]
	}
	return strings.HasSuffix(pattern, "*") || rest == ""
}

// opencodePermissionValue is
// value.get("permissions").or_else(|| value.get("permission").and_then(|v| v.get("skill"))).
func opencodePermissionValue(value map[string]any) any {
	if v, ok := value["permissions"]; ok {
		return v
	}
	if perm, ok := value["permission"].(map[string]any); ok {
		return perm["skill"]
	}
	return nil
}

// jsonGet walks nested map keys.
func jsonGet(value map[string]any, keys ...string) any {
	var current any = value
	for _, key := range keys {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = obj[key]
	}
	return current
}
