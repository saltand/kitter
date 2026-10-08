// Package effective holds the parts of src/effective_skills.rs that the
// M1 core needs: manual-skill detection from SKILL.md frontmatter and the
// Codex agents/openai.yaml policy file. Full per-agent discovery arrives
// with M4.
package effective

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SkillVisibility mirrors SkillVisibility (serde snake_case).
type SkillVisibility string

const (
	VisibilityAutomatic   SkillVisibility = "automatic"
	VisibilityNameOnly    SkillVisibility = "name_only"
	VisibilityManualOnly  SkillVisibility = "manual_only"
	VisibilityConditional SkillVisibility = "conditional"
)

// IsManualSkill is is_manual_skill.
func IsManualSkill(skillDir string) bool {
	content, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		return false
	}
	return ParseVisibility(string(content)) == VisibilityManualOnly || codexManualOnly(skillDir)
}

// HasDisableModelInvocation is has_disable_model_invocation.
func HasDisableModelInvocation(skillDir string) bool {
	content, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		return false
	}
	return ContentDisablesModelInvocation(string(content))
}

// ContentDisablesModelInvocation is content_disables_model_invocation.
func ContentDisablesModelInvocation(content string) bool {
	return ParseVisibility(content) == VisibilityManualOnly
}

// ParseVisibility is parse_visibility: manual-only wins over conditional.
func ParseVisibility(content string) SkillVisibility {
	frontmatter, ok := YAMLFrontmatter(content)
	if !ok {
		return VisibilityAutomatic
	}
	var parsed map[string]yaml.Node
	if err := yaml.Unmarshal([]byte(frontmatter), &parsed); err != nil {
		return VisibilityAutomatic
	}
	if node := nodeAt(parsed, "disable-model-invocation"); yamlBool(node) {
		return VisibilityManualOnly
	}
	if yamlPathsConfigured(nodeAt(parsed, "paths")) {
		return VisibilityConditional
	}
	return VisibilityAutomatic
}

func nodeAt(m map[string]yaml.Node, key string) *yaml.Node {
	if n, ok := m[key]; ok {
		return &n
	}
	return nil
}

func yamlBool(n *yaml.Node) bool {
	if n == nil {
		return false
	}
	var b bool
	if err := n.Decode(&b); err != nil {
		return false
	}
	return b
}

// YAMLFrontmatter is yaml_frontmatter: the text between the opening `---`
// line and the next `---` line (or a trailing one at EOF).
func YAMLFrontmatter(content string) (string, bool) {
	var rest string
	if strings.HasPrefix(content, "---\r\n") {
		rest = content[5:]
	} else if strings.HasPrefix(content, "---\n") {
		rest = content[4:]
	} else {
		return "", false
	}
	type candidate struct{ at int }
	var ends []candidate
	if i := strings.Index(rest, "\n---\n"); i >= 0 {
		ends = append(ends, candidate{i})
	}
	if i := strings.Index(rest, "\r\n---\r\n"); i >= 0 {
		ends = append(ends, candidate{i})
	}
	if strings.HasSuffix(rest, "\n---") {
		ends = append(ends, candidate{len(rest) - 4})
	}
	if len(ends) == 0 {
		return "", false
	}
	end := ends[0].at
	for _, e := range ends[1:] {
		if e.at < end {
			end = e.at
		}
	}
	return rest[:end], true
}

// YAMLPathsConfigured is yaml_paths_configured.
func yamlPathsConfigured(value *yaml.Node) bool {
	if value == nil {
		return false
	}
	switch value.Kind {
	case yaml.ScalarNode:
		if value.Tag == "!!null" {
			return false
		}
		return strings.TrimSpace(value.Value) != ""
	case yaml.SequenceNode:
		return len(value.Content) > 0
	default:
		return true
	}
}

func codexManualOnly(skillDir string) bool {
	content, err := os.ReadFile(filepath.Join(skillDir, "agents", "openai.yaml"))
	if err != nil {
		return false
	}
	var data struct {
		Policy *struct {
			AllowImplicitInvocation *bool `yaml:"allow_implicit_invocation"`
		} `yaml:"policy"`
	}
	if err := yaml.Unmarshal(content, &data); err != nil {
		return false
	}
	if data.Policy == nil || data.Policy.AllowImplicitInvocation == nil {
		return false
	}
	return !*data.Policy.AllowImplicitInvocation
}

// approxTokenCount is approx_token_count: ceil(len/4), the
// provider-independent estimate used when an exact tokenizer is not
// available.
func approxTokenCount(rendered string) int {
	return (len(rendered) + 3) / 4
}

// jsonUnmarshal is encoding/json.Unmarshal, wrapped to keep call sites
// one line.
func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
