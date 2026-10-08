// policy.go ports the AgentSkillPolicy trait and the shared
// discovery/estimation machinery from effective_skills.rs: discover,
// estimate_with_policy/profile, estimate_skills, the metadata reader
// profiles, and the small helpers every agent uses.
package effective

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// envOr is env::var_os — a seam so tests can supply values without
// mutating the process environment. Keys not present in the override
// map fall through to os.LookupEnv.
var envOverride map[string]string

func envOr(name string) (string, bool) {
	if envOverride != nil {
		if v, ok := envOverride[name]; ok {
			return v, true
		}
	}
	return os.LookupEnv(name)
}

// envSplitPaths is env::split_paths (':' separated on unix).
func envSplitPaths(value string) []string {
	var out []string
	for _, p := range strings.Split(value, string(os.PathListSeparator)) {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// envTruthy is env_truthy.
func envTruthy(name string) bool {
	v, ok := envOr(name)
	if !ok {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// agentSkillPolicy is the AgentSkillPolicy trait.
type agentSkillPolicy interface {
	Agent() AgentKind
	Roots(ctx *DiscoveryContext) []*SkillRoot
	Builtins(ctx *DiscoveryContext) []EffectiveSkill
	IsEnabled(skillDir string, metadata *SkillMetadata, ctx *DiscoveryContext) bool
	EffectiveName(skillDir string, metadata *SkillMetadata) string
	EffectiveNameForRoot(root *SkillRoot, skillDir string, metadata *SkillMetadata) string
	EffectiveIDForRoot(root *SkillRoot, skillDir, skillFile string, metadata *SkillMetadata) string
	PromptPathForEntry(root *SkillRoot, skillDir, skillFile string, metadata *SkillMetadata, ctx *DiscoveryContext) string
	IsEnabledForRoot(root *SkillRoot, skillDir string, metadata *SkillMetadata, ctx *DiscoveryContext) bool
	IsEnabledForEntry(root *SkillRoot, skillDir, skillFile string, metadata *SkillMetadata, ctx *DiscoveryContext) bool
	NameCollision() NameCollision
	Visibility(skillDir string, metadata *SkillMetadata, ctx *DiscoveryContext) SkillVisibility
	VisibilityForRoot(root *SkillRoot, skillDir string, metadata *SkillMetadata, ctx *DiscoveryContext) SkillVisibility
	RenderVisibleMetadata(skills []EffectiveSkill) string
	RenderInitialCatalog(skills []EffectiveSkill) CatalogRender
	EstimateTokens(rendered string) int
}

// policyBase supplies the trait's default methods; each concrete policy
// embeds it and overrides what it needs.
type policyBase struct{ agent AgentKind }

func (p policyBase) Agent() AgentKind { return p.agent }
func (p policyBase) Builtins(*DiscoveryContext) []EffectiveSkill {
	return nil
}
func (p policyBase) IsEnabled(string, *SkillMetadata, *DiscoveryContext) bool { return true }
func (p policyBase) EffectiveName(_ string, m *SkillMetadata) string {
	return m.Name
}
func (p policyBase) PromptPathForEntry(*SkillRoot, string, string, *SkillMetadata, *DiscoveryContext) string {
	return ""
}
func (p policyBase) NameCollision() NameCollision { return CollisionKeepAll }
func (p policyBase) EstimateTokens(rendered string) int { return approxTokenCount(rendered) }

// MetadataProfile is effective_skills::MetadataProfile.
type MetadataProfile int

const (
	ProfileStrictFrontmatter MetadataProfile = iota
	ProfileBodyFallback
)

// estimateWithPolicy is estimate_with_policy.
func estimateWithPolicy(policy agentSkillPolicy, ctx *DiscoveryContext) AgentContextEstimate {
	return estimateWithProfile(policy, ctx,
		RecursiveProfile(maxIntValue, maxIntValue, maxIntValue),
		ProfileStrictFrontmatter)
}

const maxIntValue = int(^uint(0) >> 1)

// estimateWithProfile is estimate_with_profile.
func estimateWithProfile(policy agentSkillPolicy, ctx *DiscoveryContext, scanProfile ScanProfile, metadataProfile MetadataProfile) AgentContextEstimate {
	return estimateSkills(policy, discover(policy, ctx, scanProfile, metadataProfile))
}

// estimateSkills is estimate_skills.
func estimateSkills(policy agentSkillPolicy, skills []EffectiveSkill) AgentContextEstimate {
	var visible []EffectiveSkill
	for _, s := range skills {
		if s.Visibility == VisibilityAutomatic || s.Visibility == VisibilityNameOnly {
			visible = append(visible, s)
		}
	}
	rendered := policy.RenderInitialCatalog(visible)
	estimate := AgentContextEstimate{
		Agent:           policy.Agent(),
		DiscoveredCount: len(skills),
		ModelVisibleCount: rendered.IncludedCount,
		EstimatedTokens: policy.EstimateTokens(rendered.Text),
		Skills:          skills,
	}
	for _, s := range skills {
		switch s.Visibility {
		case VisibilityManualOnly:
			estimate.ManualOnlyCount++
		case VisibilityNameOnly:
			estimate.NameOnlyCount++
		case VisibilityConditional:
			estimate.ConditionalCount++
		}
	}
	return estimate
}

// discover is effective_skills::discover.
func discover(policy agentSkillPolicy, ctx *DiscoveryContext, scanProfile ScanProfile, metadataProfile MetadataProfile) []EffectiveSkill {
	result := policy.Builtins(ctx)
	ids := map[string]bool{}
	for _, s := range result {
		ids[s.ID] = true
	}
	canonicalPaths := map[string]bool{}
	for _, root := range policy.Roots(ctx) {
		for _, skillFile := range scan(root, scanProfile) {
			skillDir := filepath.Dir(skillFile)
			canonicalTarget := skillDir
			if root.ExactSkillFile != "" || (root.IncludeRootMarkdown && skillDir == root.Path) {
				canonicalTarget = skillFile
			}
			canonical := canonicalize(canonicalTarget)
			if canonicalPaths[canonical] {
				continue
			}
			canonicalPaths[canonical] = true
			metadata := readMetadataWithProfile(skillFile, metadataProfile)
			if metadata == nil {
				continue
			}
			if !policy.IsEnabledForEntry(root, skillDir, skillFile, metadata, ctx) {
				continue
			}
			name := policy.EffectiveNameForRoot(root, skillDir, metadata)
			id := policy.EffectiveIDForRoot(root, skillDir, skillFile, metadata)
			switch policy.NameCollision() {
			case CollisionFirstWins:
				if ids[id] {
					continue
				}
				ids[id] = true
			case CollisionLastWins:
				if ids[id] {
					for i := range result {
						if result[i].ID == id {
							result = append(result[:i], result[i+1:]...)
							break
						}
					}
				} else {
					ids[id] = true
				}
			case CollisionKeepAll:
			}
			result = append(result, EffectiveSkill{
				ID:          id,
				Visibility:  policy.VisibilityForRoot(root, skillDir, metadata, ctx),
				Name:        name,
				Description: metadata.Description,
				WhenToUse:   metadata.WhenToUse,
				HasWhenToUse: metadata.HasWhenToUse,
				Path:        skillFile,
				RootPath:    root.Path,
				PromptPath:  policy.PromptPathForEntry(root, skillDir, skillFile, metadata, ctx),
				Scope:       root.Scope,
				Source:      root.Source,
			})
		}
	}
	return result
}

// skillFrontmatter is effective_skills::SkillFrontmatter.
type skillFrontmatter struct {
	Name              *string   `yaml:"name"`
	Description       *string   `yaml:"description"`
	WhenToUse         *string   `yaml:"when_to_use"`
	DisableInvocation bool      `yaml:"disable-model-invocation"`
	Paths             yaml.Node `yaml:"paths"`
	Metadata          yaml.Node `yaml:"metadata"`
}

// readMetadataWithProfile is read_metadata_with_profile.
func readMetadataWithProfile(path string, profile MetadataProfile) *SkillMetadata {
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	content := string(contentBytes)
	var fallback string
	if filepath.Base(path) == "SKILL.md" {
		fallback = filepath.Base(filepath.Dir(path))
	} else {
		fallback = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	var parsed *skillFrontmatter
	if frontmatter, ok := YAMLFrontmatter(content); ok {
		var fm skillFrontmatter
		if err := yaml.Unmarshal([]byte(frontmatter), &fm); err == nil {
			parsed = &fm
		}
	}
	if profile == ProfileStrictFrontmatter && parsed == nil {
		return nil
	}
	hasExplicitDescription := parsed != nil && parsed.Description != nil
	name := fallback
	description := ""
	var whenToUse string
	var hasWhenToUse bool
	if parsed != nil {
		if parsed.Name != nil && strings.TrimSpace(*parsed.Name) != "" {
			name = *parsed.Name
		}
		if parsed.Description != nil {
			description = *parsed.Description
		}
		if parsed.WhenToUse != nil {
			whenToUse = *parsed.WhenToUse
			hasWhenToUse = true
		}
	}
	if !hasExplicitDescription {
		description = firstBodyParagraph(content)
	}
	return &SkillMetadata{
		Name:                   name,
		Description:            description,
		HasExplicitDescription: hasExplicitDescription,
		WhenToUse:              whenToUse,
		HasWhenToUse:           hasWhenToUse,
		SourcePath:             path,
	}
}

// firstBodyParagraph is first_body_paragraph.
func firstBodyParagraph(content string) string {
	_, hasFrontmatter := YAMLFrontmatter(content)
	delimiters := 2
	if hasFrontmatter {
		delimiters = 0
	}
	var paragraph []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimRight(line, "\r")
		if hasFrontmatter && strings.TrimSpace(trimmed) == "---" && delimiters < 2 {
			delimiters++
			continue
		}
		if delimiters < 2 {
			continue
		}
		if strings.TrimSpace(trimmed) == "" {
			if len(paragraph) > 0 {
				break
			}
			continue
		}
		paragraph = append(paragraph, strings.TrimSpace(trimmed))
	}
	return strings.Join(paragraph, " ")
}

// frontmatterVisibility is frontmatter_visibility.
func frontmatterVisibility(metadata *SkillMetadata) SkillVisibility {
	content, err := os.ReadFile(metadata.SourcePath)
	if err != nil {
		return VisibilityAutomatic
	}
	return ParseVisibility(string(content))
}

// validAgentSkillName is valid_agent_skill_name.
func validAgentSkillName(name string) bool {
	if name == "" || len(name) > 64 || strings.HasPrefix(name, "-") ||
		strings.HasSuffix(name, "-") || strings.Contains(name, "--") {
		return false
	}
	for i := 0; i < len(name); i++ {
		b := name[i]
		if !(b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '-') {
			return false
		}
	}
	return true
}

// compatibleSkillIsValid is compatible_skill_is_valid.
func compatibleSkillIsValid(metadata *SkillMetadata) bool {
	return metadata.HasExplicitDescription &&
		strings.TrimSpace(metadata.Description) != "" &&
		utf16Len(metadata.Description) <= 1024 &&
		validAgentSkillName(metadata.Name)
}

// utf16Len is encode_utf16().count().
func utf16Len(s string) int {
	count := 0
	for _, r := range s {
		if r > 0xFFFF {
			count += 2
		} else {
			count++
		}
	}
	return count
}

// renderNameDescriptionCatalog is render_name_description_catalog.
func renderNameDescriptionCatalog(skills []EffectiveSkill) string {
	if len(skills) == 0 {
		return ""
	}
	var body []string
	for _, s := range skills {
		body = append(body, "  <skill>\n    <name>"+xmlEscape(s.Name)+"</name>\n    <description>"+xmlEscape(s.Description)+"</description>\n  </skill>")
	}
	return "<available_skills>\n" + strings.Join(body, "\n") + "\n</available_skills>"
}

// pathDerivedSkillID is path_derived_skill_id.
func pathDerivedSkillID(root *SkillRoot, skillDir, skillFile string) string {
	if rel, err := filepath.Rel(root.Path, skillDir); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return filepath.Base(rel)
	}
	if rel, err := filepath.Rel(root.Path, skillDir); err == nil && rel == "." {
		// skill_dir == root.path: relative is empty in Rust (strip_prefix
		// yields ""), file_name() of "" is None → fall through to stem.
	}
	base := filepath.Base(skillFile)
	if stem := strings.TrimSuffix(base, filepath.Ext(base)); stem != "" {
		return stem
	}
	return "SKILL"
}

// xmlEscape is xml_escape.
func xmlEscape(value string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	return r.Replace(value)
}

// cwdToBoundary is cwd_to_boundary.
func cwdToBoundary(cwd, boundary string) []string {
	var result []string
	for dir := filepath.Clean(cwd); ; {
		result = append(result, dir)
		if boundary != "" && dir == filepath.Clean(boundary) {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return result
}

// isGlobalContext is is_global_context.
func isGlobalContext(ctx *DiscoveryContext) bool {
	return ctx.Cwd == ctx.Home
}

// findRepositoryRoot is find_repository_root.
func findRepositoryRoot(path string) string {
	for dir := filepath.Clean(path); ; {
		git := filepath.Join(dir, ".git")
		if _, err := os.Lstat(git); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// resolveAgentPath is resolve_agent_path.
func resolveAgentPath(value, home, relativeBase string) string {
	if value == "~" {
		return home
	}
	if strings.HasPrefix(value, "~/") {
		return filepath.Join(home, value[2:])
	}
	if filepath.IsAbs(value) {
		return value
	}
	return filepath.Join(relativeBase, value)
}

// expandUserPath is expand_user_path.
func expandUserPath(value string, ctx *DiscoveryContext) string {
	if strings.HasPrefix(value, "~/") {
		return filepath.Join(ctx.Home, value[2:])
	}
	if filepath.IsAbs(value) {
		return value
	}
	return filepath.Join(ctx.Cwd, value)
}

// parseJSONC parses JSON/JSONC via normalizeJSONC; parse failures
// return nil, matching Rust's `.ok()` on serde_json::from_str.
func parseJSONC(content string) map[string]any {
	var value map[string]any
	if err := jsonUnmarshal([]byte(normalizeJSONC(content)), &value); err != nil {
		return nil
	}
	return value
}

// readFileString is fs::read_to_string → Result.
func readFileString(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// yamlUnmarshalErr is serde_yaml::from_str → Result.
func yamlUnmarshalErr(data []byte, v any) error {
	return yaml.Unmarshal(data, v)
}

// yamlNode aliases yaml.v3 Node for the helpers below.
type yamlNode = yaml.Node

// yamlNodeGet is serde_yaml::Value::get for mapping nodes; it
// transparently unwraps a yaml.v3 document node.
func yamlNodeGet(node *yamlNode, key string) *yamlNode {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		return yamlNodeGet(node.Content[0], key)
	}
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

// yamlValueFalse reports serde's `as_bool() == Some(false) ||
// as_str() == Some("false")`.
func yamlValueFalse(node *yamlNode) bool {
	if node == nil || node.Kind != yaml.ScalarNode {
		return false
	}
	switch node.Tag {
	case "!!bool":
		var b bool
		if err := node.Decode(&b); err == nil {
			return !b
		}
		return false
	case "!!str":
		return node.Value == "false"
	}
	return false
}
