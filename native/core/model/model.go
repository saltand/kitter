// Package model holds the shared data types of the Kitter core, mirroring
// src/model.rs of the Rust implementation. JSON encoding matches serde
// exactly so both versions can share one data directory.
package model

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// SkillOrigin records where a Skill in the library came from.
//
// serde: #[serde(tag = "type", rename_all = "snake_case")] — an internally
// tagged enum, e.g. {"type":"npx","repository":…,"skill":…}.
type SkillOrigin struct {
	Type         string `json:"type"`                  // builtin|npx|claude_marketplace|git|local|unknown
	Repository   string `json:"repository,omitempty"`  // npx, git
	Skill        string `json:"skill,omitempty"`       // npx, claude_marketplace (as plugin)
	Plugin       string `json:"plugin,omitempty"`      // claude_marketplace
	Subdir       string `json:"subdir,omitempty"`      // git (Option<String>, no skip_serializing_if → always present? see below)
	Path         string `json:"path,omitempty"`        // local
	SourceRoot   string `json:"source_root,omitempty"` // local, skip_serializing_if none
	SourceHash   string `json:"source_hash,omitempty"` // npx, skip_serializing_if none
	HasSubdir    bool   `json:"-"`
	HasSourceRt  bool   `json:"-"`
	HasSourceHsh bool   `json:"-"`
}

// The struct above cannot model the exact serde shape on its own
// (git.subdir serializes as null, not omitted), so custom marshaling below
// produces byte-compatible output.

type originJSON struct {
	Type       string  `json:"type"`
	Repository *string `json:"repository,omitempty"`
	Skill      *string `json:"skill,omitempty"`
	SourceHash *string `json:"source_hash,omitempty"`
	Plugin     *string `json:"plugin,omitempty"`
	Subdir     *string `json:"subdir"`
	Path       *string `json:"path,omitempty"`
	SourceRoot *string `json:"source_root,omitempty"`
}

// StrPtr returns *s; used by constructors and tests for serde Option<String>.
func StrPtr(s string) *string { return &s }

func strptr(s string) *string { return &s }

// MarshalJSON implements the serde internal-tag shape for SkillOrigin.
func (o SkillOrigin) MarshalJSON() ([]byte, error) {
	j := originJSON{Type: o.Type}
	switch o.Type {
	case "npx":
		j.Repository = strptr(o.Repository)
		j.Skill = strptr(o.Skill)
		if o.HasSourceHsh {
			j.SourceHash = strptr(o.SourceHash)
		}
	case "claude_marketplace":
		j.Plugin = strptr(o.Plugin)
		j.Skill = strptr(o.Skill)
	case "git":
		j.Repository = strptr(o.Repository)
		if o.HasSubdir {
			j.Subdir = strptr(o.Subdir)
		} else {
			j.Subdir = nil // serialized as null: subdir has no skip_serializing_if
		}
	case "local":
		j.Path = strptr(o.Path)
		if o.HasSourceRt {
			j.SourceRoot = strptr(o.SourceRoot)
		}
	}
	return marshalOrdered(j)
}

// marshalOrdered emits the JSON object with serde's field order.
func marshalOrdered(j originJSON) ([]byte, error) {
	var b strings.Builder
	b.WriteString(`{"type":`)
	t, _ := json.Marshal(j.Type)
	b.Write(t)
	write := func(name string, v *string, force bool) {
		if v != nil {
			b.WriteString(`,"` + name + `":`)
			s, _ := json.Marshal(*v)
			b.Write(s)
		} else if force {
			b.WriteString(`,"` + name + `":null`)
		}
	}
	// serde preserves declaration order of the variant fields.
	switch j.Type {
	case "npx":
		write("repository", j.Repository, false)
		write("skill", j.Skill, false)
		write("source_hash", j.SourceHash, false)
	case "claude_marketplace":
		write("plugin", j.Plugin, false)
		write("skill", j.Skill, false)
	case "git":
		write("repository", j.Repository, false)
		write("subdir", j.Subdir, true)
	case "local":
		write("path", j.Path, false)
		write("source_root", j.SourceRoot, false)
	}
	b.WriteByte('}')
	return []byte(b.String()), nil
}

// UnmarshalJSON reads the serde internal-tag shape.
func (o *SkillOrigin) UnmarshalJSON(data []byte) error {
	var j originJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return err
	}
	o.Type = j.Type
	if o.Type == "" {
		o.Type = "unknown" // serde #[default] on the enum
	}
	o.Repository = deref(j.Repository)
	o.Skill = deref(j.Skill)
	o.Plugin = deref(j.Plugin)
	o.Path = deref(j.Path)
	o.Subdir = deref(j.Subdir)
	o.SourceRoot = deref(j.SourceRoot)
	o.SourceHash = deref(j.SourceHash)
	o.HasSubdir = j.Subdir != nil
	o.HasSourceRt = j.SourceRoot != nil
	o.HasSourceHsh = j.SourceHash != nil
	return nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Constructors keep the tagged-enum semantics readable.

func OriginBuiltin() SkillOrigin { return SkillOrigin{Type: "builtin"} }
func OriginUnknown() SkillOrigin { return SkillOrigin{Type: "unknown"} }
func OriginNpx(repository, skill string, sourceHash *string) SkillOrigin {
	o := SkillOrigin{Type: "npx", Repository: repository, Skill: skill}
	if sourceHash != nil {
		o.SourceHash = *sourceHash
		o.HasSourceHsh = true
	}
	return o
}
func OriginClaudeMarketplace(plugin, skill string) SkillOrigin {
	return SkillOrigin{Type: "claude_marketplace", Plugin: plugin, Skill: skill}
}
func OriginGit(repository string, subdir *string) SkillOrigin {
	o := SkillOrigin{Type: "git", Repository: repository}
	if subdir != nil {
		o.Subdir = *subdir
		o.HasSubdir = true
	}
	return o
}
func OriginLocal(path string, sourceRoot *string) SkillOrigin {
	o := SkillOrigin{Type: "local", Path: path}
	if sourceRoot != nil {
		o.SourceRoot = *sourceRoot
		o.HasSourceRt = true
	}
	return o
}

// IsBuiltin reports whether this is the built-in Kitter skill.
func (o SkillOrigin) IsBuiltin() bool { return o.Type == "builtin" }

// Source returns the coarse SkillSource of this origin (source() in Rust).
func (o SkillOrigin) Source() SkillSource {
	switch o.Type {
	case "builtin":
		return SkillSource{Type: "builtin"}
	case "npx":
		return SkillSource{Type: "npx", Repository: o.Repository}
	case "claude_marketplace":
		return SkillSource{Type: "claude_marketplace", Plugin: o.Plugin}
	case "git":
		s := SkillSource{Type: "git", Repository: o.Repository}
		if o.HasSubdir {
			s.Subdir = o.Subdir
			s.HasSubdir = true
		}
		return s
	case "local":
		path := o.Path
		if o.HasSourceRt && o.SourceRoot != "" {
			path = o.SourceRoot
		}
		return SkillSource{Type: "local", Path: path}
	default:
		return SkillSource{Type: "unknown"}
	}
}

// Label is origin.source().label() in Rust.
func (o SkillOrigin) Label() string { return o.Source().Label() }

// IdentityKey is source.key() + "::" + name in Rust.
func (o SkillOrigin) IdentityKey(name string) string {
	return o.Source().Key() + "::" + name
}

// SkillSource is the coarse provenance of a Skill.
//
// serde: #[serde(tag = "type", rename_all = "snake_case")].
type SkillSource struct {
	Type       string `json:"-"`
	Repository string `json:"-"`
	Plugin     string `json:"-"`
	Subdir     string `json:"-"`
	Path       string `json:"-"`
	HasSubdir  bool   `json:"-"`
}

type sourceJSON struct {
	Type       string  `json:"type"`
	Repository *string `json:"repository,omitempty"`
	Plugin     *string `json:"plugin,omitempty"`
	Subdir     *string `json:"subdir"`
	Path       *string `json:"path,omitempty"`
}

func (s SkillSource) MarshalJSON() ([]byte, error) {
	var b strings.Builder
	b.WriteString(`{"type":`)
	t, _ := json.Marshal(s.Type)
	b.Write(t)
	write := func(name string, v *string, force bool) {
		if v != nil {
			b.WriteString(`,"` + name + `":`)
			sv, _ := json.Marshal(*v)
			b.Write(sv)
		} else if force {
			b.WriteString(`,"` + name + `":null`)
		}
	}
	switch s.Type {
	case "npx":
		write("repository", strptr(s.Repository), false)
	case "claude_marketplace":
		write("plugin", strptr(s.Plugin), false)
	case "git":
		write("repository", strptr(s.Repository), false)
		if s.HasSubdir {
			write("subdir", strptr(s.Subdir), true)
		} else {
			write("subdir", nil, true)
		}
	case "local":
		write("path", strptr(s.Path), false)
	}
	b.WriteByte('}')
	return []byte(b.String()), nil
}

func (s *SkillSource) UnmarshalJSON(data []byte) error {
	var j sourceJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return err
	}
	s.Type = j.Type
	if s.Type == "" {
		s.Type = "unknown"
	}
	s.Repository = deref(j.Repository)
	s.Plugin = deref(j.Plugin)
	s.Subdir = deref(j.Subdir)
	s.Path = deref(j.Path)
	s.HasSubdir = j.Subdir != nil
	return nil
}

func SourceBuiltin() SkillSource { return SkillSource{Type: "builtin"} }
func SourceUnknown() SkillSource { return SkillSource{Type: "unknown"} }

// Key is SkillSource::key() in Rust.
func (s SkillSource) Key() string {
	switch s.Type {
	case "builtin":
		return "builtin"
	case "npx":
		return "npx:" + s.Repository
	case "claude_marketplace":
		return "claude:" + s.Plugin
	case "git":
		sub := ""
		if s.HasSubdir {
			sub = s.Subdir
		}
		return fmt.Sprintf("git:%s:%s", s.Repository, sub)
	case "local":
		return "local:" + s.Path
	default:
		return "unknown"
	}
}

// Label is SkillSource::label() in Rust.
func (s SkillSource) Label() string {
	switch s.Type {
	case "builtin":
		return "Kitter"
	case "npx":
		if strings.Contains(s.Repository, "vercel-labs/skills") {
			return "Vercel Skills"
		}
		return repositoryLabel(s.Repository)
	case "git":
		return repositoryLabel(s.Repository)
	case "claude_marketplace":
		return s.Plugin
	case "local":
		base := filepath.Base(s.Path)
		if base == "." || base == "/" || base == "" {
			return "本地导入"
		}
		return base
	default:
		return "本地技能"
	}
}

// repositoryLabel mirrors repository_label() in Rust.
func repositoryLabel(repository string) string {
	value := strings.TrimSuffix(repository, ".git")
	if v, ok := strings.CutPrefix(value, "git@github.com:"); ok {
		value = v
	}
	parts := strings.FieldsFunc(strings.TrimSuffix(value, "/"), func(r rune) bool { return r == '/' })
	var nonempty []string
	for _, p := range parts {
		if p != "" {
			nonempty = append(nonempty, p)
		}
	}
	if len(nonempty) >= 2 {
		return nonempty[len(nonempty)-2] + "/" + nonempty[len(nonempty)-1]
	}
	return value
}

// SkillSourceRecord tracks a source and the skills it provided.
type SkillSourceRecord struct {
	Source           SkillSource `json:"source"`
	DiscoveredSkills []string    `json:"discovered_skills"`
	AddedSkills      []string    `json:"added_skills"`
}

// SkillRecord is one library entry in registry.json.
type SkillRecord struct {
	Name            string      `json:"name"`
	StorageName     string      `json:"storage_name,omitempty"`
	Description     string      `json:"-"` // serde(skip): never persisted
	Origin          SkillOrigin `json:"origin"`
	UpdateAvailable bool        `json:"update_available"`
	GroupID         *string     `json:"group_id,omitempty"`
	LastOperatedAt  uint64      `json:"last_operated_at"`
	KitterManual    bool        `json:"kitter_manual"`
}

// IdentityKey is SkillRecord::identity_key() in Rust.
func (r SkillRecord) IdentityKey() string { return r.Origin.IdentityKey(r.Name) }

// SkillGroup is a user-defined group of library skills.
type SkillGroup struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt uint64 `json:"created_at"`
}

// SkillSummary is a SkillRecord joined with its on-disk directory.
type SkillSummary struct {
	Record            SkillRecord `json:"record"`
	Path              string      `json:"path"`
	InstalledProjects int         `json:"installed_projects"`
	ManualOnly        bool        `json:"manual_only"`
}

// InstallTarget is one agent directory kind.
//
// serde: #[serde(rename_all = "snake_case")] unit enum → a plain string.
type InstallTarget string

const (
	TargetUniversal   InstallTarget = "universal"
	TargetCodex       InstallTarget = "codex"
	TargetClaudeCode  InstallTarget = "claude_code"
	TargetCursor      InstallTarget = "cursor"
	TargetOpenCode    InstallTarget = "open_code"
	TargetPi          InstallTarget = "pi"
	TargetGrok        InstallTarget = "grok"
	TargetAntigravity InstallTarget = "antigravity"
	TargetDroid       InstallTarget = "droid"
	TargetCopilot     InstallTarget = "copilot"
)

// ProjectSkill groups one skill's installations across targets.
type ProjectSkill struct {
	Name          string                     `json:"name"`
	Installations []ProjectSkillInstallation `json:"installations"`
}

// ProjectSkillInstallation is one on-disk installation of a skill.
type ProjectSkillInstallation struct {
	Target  InstallTarget `json:"target"`
	Path    string        `json:"path"`
	Managed bool          `json:"managed"`
}
