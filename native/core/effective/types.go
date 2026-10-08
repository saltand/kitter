// types.go ports the shared value types from src/effective_skills.rs:
// AgentKind, SkillVisibility, SkillScope, SkillSource, EffectiveSkill,
// AgentContextEstimate, group_effective_skills, SkillRoot,
// SkillMetadata, NameCollision and DiscoveryContext.
package effective

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
)

// AgentKind is effective_skills::AgentKind.
type AgentKind int

const (
	AgentCodex AgentKind = iota
	AgentClaudeCode
	AgentCursor
	AgentOpenCode
	AgentCopilot
	AgentAntigravity
	AgentAmp
	AgentDroid
	AgentPi
	AgentGrok
	AgentOpenClaw
	AgentHermes
)

// ID is AgentKind::id.
func (k AgentKind) ID() string {
	switch k {
	case AgentCodex:
		return "codex"
	case AgentClaudeCode:
		return "claude-code"
	case AgentCursor:
		return "cursor"
	case AgentOpenCode:
		return "opencode"
	case AgentCopilot:
		return "copilot"
	case AgentAntigravity:
		return "antigravity"
	case AgentAmp:
		return "amp"
	case AgentDroid:
		return "droid"
	case AgentPi:
		return "pi"
	case AgentGrok:
		return "grok"
	case AgentOpenClaw:
		return "openclaw"
	case AgentHermes:
		return "hermes"
	}
	return ""
}

// MarshalJSON serializes AgentKind with serde's rename_all=snake_case
// ("claude_code", not the CLI's claude-code ID).
func (k AgentKind) MarshalJSON() ([]byte, error) {
	return json.Marshal(k.snakeName())
}

func (k AgentKind) snakeName() string {
	switch k {
	case AgentCodex:
		return "codex"
	case AgentClaudeCode:
		return "claude_code"
	case AgentCursor:
		return "cursor"
	case AgentOpenCode:
		return "open_code"
	case AgentCopilot:
		return "copilot"
	case AgentAntigravity:
		return "antigravity"
	case AgentAmp:
		return "amp"
	case AgentDroid:
		return "droid"
	case AgentPi:
		return "pi"
	case AgentGrok:
		return "grok"
	case AgentOpenClaw:
		return "open_claw"
	case AgentHermes:
		return "hermes"
	}
	return ""
}

// MarshalJSON serializes SkillScope with serde's rename_all=snake_case.
func (s SkillScope) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.snakeName())
}

func (s SkillScope) snakeName() string {
	switch s {
	case ScopeLocal:
		return "local"
	case ScopeRepository:
		return "repository"
	case ScopeUser:
		return "user"
	case ScopeSystem:
		return "system"
	}
	return ""
}

// MarshalJSON serializes SkillSource with serde's internally tagged
// shape: {"type":"filesystem"} | {"type":"builtin"} |
// {"type":"plugin","id":...,"display_name":...}.
func (s SkillSource) MarshalJSON() ([]byte, error) {
	switch s.Kind {
	case SourceBuiltin:
		return json.Marshal(map[string]string{"type": "builtin"})
	case SourcePlugin:
		return json.Marshal(struct {
			Type        string `json:"type"`
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		}{Type: "plugin", ID: s.ID, DisplayName: s.DisplayName})
	default:
		return json.Marshal(map[string]string{"type": "filesystem"})
	}
}

// MarshalJSON emits the serde field shape of EffectiveSkill: Option
// fields become null when absent.
func (s EffectiveSkill) MarshalJSON() ([]byte, error) {
	type skillJSON struct {
		ID          string          `json:"id"`
		Name        string          `json:"name"`
		Description string          `json:"description"`
		WhenToUse   *string         `json:"when_to_use"`
		Path        string          `json:"path"`
		RootPath    *string         `json:"root_path"`
		PromptPath  *string         `json:"prompt_path"`
		Scope       SkillScope      `json:"scope"`
		Visibility  SkillVisibility `json:"visibility"`
		Source      SkillSource     `json:"source"`
	}
	var whenToUse, rootPath, promptPath *string
	if s.HasWhenToUse {
		whenToUse = &s.WhenToUse
	}
	if s.RootPath != "" {
		rootPath = &s.RootPath
	}
	if s.PromptPath != "" {
		promptPath = &s.PromptPath
	}
	return json.Marshal(skillJSON{
		ID: s.ID, Name: s.Name, Description: s.Description,
		WhenToUse: whenToUse, Path: s.Path, RootPath: rootPath,
		PromptPath: promptPath, Scope: s.Scope,
		Visibility: s.Visibility, Source: s.Source,
	})
}

// MarshalJSON emits the serde field order of AgentContextEstimate.
func (e AgentContextEstimate) MarshalJSON() ([]byte, error) {
	type estimateJSON struct {
		Agent             AgentKind        `json:"agent"`
		DiscoveredCount   int              `json:"discovered_count"`
		ModelVisibleCount int              `json:"model_visible_count"`
		ManualOnlyCount   int              `json:"manual_only_count"`
		NameOnlyCount     int              `json:"name_only_count"`
		ConditionalCount  int              `json:"conditional_count"`
		EstimatedTokens   int              `json:"estimated_tokens"`
		Skills            []EffectiveSkill `json:"skills"`
	}
	skills := e.Skills
	if skills == nil {
		skills = []EffectiveSkill{}
	}
	return json.Marshal(estimateJSON{
		Agent: e.Agent, DiscoveredCount: e.DiscoveredCount,
		ModelVisibleCount: e.ModelVisibleCount,
		ManualOnlyCount:   e.ManualOnlyCount,
		NameOnlyCount:     e.NameOnlyCount,
		ConditionalCount:  e.ConditionalCount,
		EstimatedTokens:   e.EstimatedTokens, Skills: skills,
	})
}

// Label is AgentKind::label.
func (k AgentKind) Label() string {
	switch k {
	case AgentCodex:
		return "Codex"
	case AgentClaudeCode:
		return "Claude Code"
	case AgentCursor:
		return "Cursor"
	case AgentOpenCode:
		return "OpenCode"
	case AgentCopilot:
		return "GitHub Copilot"
	case AgentAntigravity:
		return "Antigravity"
	case AgentAmp:
		return "Amp"
	case AgentDroid:
		return "Droid"
	case AgentPi:
		return "Pi"
	case AgentGrok:
		return "Grok"
	case AgentOpenClaw:
		return "OpenClaw"
	case AgentHermes:
		return "Hermes"
	}
	return ""
}

// IsGlobalOnly is AgentKind::is_global_only.
func (k AgentKind) IsGlobalOnly() bool {
	return k == AgentOpenClaw || k == AgentHermes
}

// SkillScope is effective_skills::SkillScope.
type SkillScope int

const (
	ScopeLocal SkillScope = iota
	ScopeRepository
	ScopeUser
	ScopeSystem
)

// SkillSourceKind mirrors the SkillSource tag.
type SkillSourceKind int

const (
	SourceFilesystem SkillSourceKind = iota
	SourceBuiltin
	SourcePlugin
)

// SkillSource is effective_skills::SkillSource.
type SkillSource struct {
	Kind        SkillSourceKind
	ID          string // Plugin
	DisplayName string // Plugin
}

// IsPlugin is SkillSource::is_plugin.
func (s SkillSource) IsPlugin() bool { return s.Kind == SourcePlugin }

// PluginID is SkillSource::plugin_id.
func (s SkillSource) PluginID() (string, bool) {
	if s.Kind != SourcePlugin {
		return "", false
	}
	return s.ID, true
}

// PluginDisplayName is SkillSource::plugin_display_name.
func (s SkillSource) PluginDisplayName() (string, bool) {
	if s.Kind != SourcePlugin {
		return "", false
	}
	return s.DisplayName, true
}

// EffectiveSkill is effective_skills::EffectiveSkill.
type EffectiveSkill struct {
	ID           string
	Name         string
	Description  string
	WhenToUse    string // "" == None
	HasWhenToUse bool
	Path         string
	RootPath     string // "" == None
	PromptPath   string // "" == None
	Scope        SkillScope
	Visibility   SkillVisibility
	Source       SkillSource
}

// IsPlugin is EffectiveSkill::is_plugin.
func (s *EffectiveSkill) IsPlugin() bool { return s.Source.IsPlugin() }

// AgentContextEstimate is effective_skills::AgentContextEstimate.
type AgentContextEstimate struct {
	Agent             AgentKind
	DiscoveredCount   int
	ModelVisibleCount int
	ManualOnlyCount   int
	NameOnlyCount     int
	ConditionalCount  int
	EstimatedTokens   int
	Skills            []EffectiveSkill
}

// PluginSkills is AgentContextEstimate::plugin_skills.
func (e *AgentContextEstimate) PluginSkills() []*EffectiveSkill {
	var out []*EffectiveSkill
	for i := range e.Skills {
		if e.Skills[i].IsPlugin() {
			out = append(out, &e.Skills[i])
		}
	}
	return out
}

// EffectiveSkillGroup is group_effective_skills' element.
type EffectiveSkillGroup struct {
	Entries []GroupEntry
}

// GroupEntry is one (agent, skill) pair in a group.
type GroupEntry struct {
	Agent AgentKind
	Skill *EffectiveSkill
}

// Name is EffectiveSkillGroup::name.
func (g *EffectiveSkillGroup) Name() string { return g.Entries[0].Skill.Name }

// GroupEffectiveSkills is group_effective_skills: merges entries that
// share a name or a canonical filesystem path.
func GroupEffectiveSkills(entries []GroupEntry) []*EffectiveSkillGroup {
	type candidate struct {
		entry          GroupEntry
		filesystemPath string // canonicalized; "" when not filesystem
	}
	var groups [][]candidate
	for _, entry := range entries {
		c := candidate{entry: entry}
		if entry.Skill.Source.Kind == SourceFilesystem {
			c.filesystemPath = canonicalize(entry.Skill.Path)
		}
		var matching []int
		for index, group := range groups {
			for _, other := range group {
				if other.entry.Skill.Name == c.entry.Skill.Name {
					matching = append(matching, index)
					break
				}
				if c.filesystemPath != "" && other.filesystemPath == c.filesystemPath {
					matching = append(matching, index)
					break
				}
			}
		}
		if len(matching) == 0 {
			groups = append(groups, []candidate{c})
			continue
		}
		var merged []candidate
		for i := len(matching) - 1; i >= 0; i-- {
			idx := matching[i]
			merged = append(groups[idx], merged...)
			groups = append(groups[:idx], groups[idx+1:]...)
		}
		merged = append(merged, c)
		groups = append(groups, merged)
	}
	result := make([]*EffectiveSkillGroup, 0, len(groups))
	for _, group := range groups {
		g := &EffectiveSkillGroup{}
		for _, c := range group {
			g.Entries = append(g.Entries, c.entry)
		}
		result = append(result, g)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name() < result[j].Name() })
	return result
}

// DiscoveryContext is effective_skills::DiscoveryContext.
type DiscoveryContext struct {
	Cwd            string
	Home           string
	RepositoryRoot string // "" == None
}

// SkillRoot is effective_skills::SkillRoot.
type SkillRoot struct {
	Path                    string
	Scope                   SkillScope
	IncludeRootMarkdown     bool
	FlatMarkdownOnly        bool
	DirectChildrenOnly      bool
	FollowDirectorySymlinks bool
	ExactSkillFile          string // "" == None
	Source                  SkillSource
}

// NewSkillRoot is SkillRoot::new.
func NewSkillRoot(path string, scope SkillScope) *SkillRoot {
	return &SkillRoot{
		Path:                    path,
		Scope:                   scope,
		FollowDirectorySymlinks: true,
		Source:                  SkillSource{Kind: SourceFilesystem},
	}
}

// PluginSkillRoot is SkillRoot::plugin.
func PluginSkillRoot(path string, scope SkillScope, id, displayName string) *SkillRoot {
	root := NewSkillRoot(path, scope)
	root.Source = SkillSource{Kind: SourcePlugin, ID: id, DisplayName: displayName}
	return root
}

// WithSource is SkillRoot::with_source.
func (r *SkillRoot) WithSource(source SkillSource) *SkillRoot {
	r.Source = source
	return r
}

// WithRootMarkdown is SkillRoot::with_root_markdown.
func (r *SkillRoot) WithRootMarkdown() *SkillRoot {
	r.IncludeRootMarkdown = true
	return r
}

// FlatMarkdown is SkillRoot::flat_markdown.
func (r *SkillRoot) FlatMarkdown() *SkillRoot {
	r.IncludeRootMarkdown = true
	r.FlatMarkdownOnly = true
	return r
}

// DirectChildren is SkillRoot::direct_children.
func (r *SkillRoot) DirectChildren() *SkillRoot {
	r.DirectChildrenOnly = true
	return r
}

// WithoutDirectorySymlinks is SkillRoot::without_directory_symlinks.
func (r *SkillRoot) WithoutDirectorySymlinks() *SkillRoot {
	r.FollowDirectorySymlinks = false
	return r
}

// ExactSkillRoot is SkillRoot::exact.
func ExactSkillRoot(skillFile string, scope SkillScope) *SkillRoot {
	return &SkillRoot{
		Path:                    filepath.Dir(skillFile),
		Scope:                   scope,
		IncludeRootMarkdown:     true,
		FollowDirectorySymlinks: true,
		ExactSkillFile:          skillFile,
		Source:                  SkillSource{Kind: SourceFilesystem},
	}
}

// SkillMetadata is effective_skills::SkillMetadata.
type SkillMetadata struct {
	Name                   string
	Description            string
	HasExplicitDescription bool
	WhenToUse              string
	HasWhenToUse           bool
	SourcePath             string
}

// NameCollision is effective_skills::NameCollision.
type NameCollision int

const (
	CollisionKeepAll NameCollision = iota
	CollisionFirstWins
	CollisionLastWins
)

// Name is AgentKind-free helper: strip @marketplace suffix from a plugin
// id for display (plugin_display_fallback).
func pluginDisplayFallback(pluginID string) string {
	if i := strings.Index(pluginID, "@"); i >= 0 {
		return pluginID[:i]
	}
	return pluginID
}
