// Package agents mirrors src/agents.rs: install target metadata, project
// and user-level directory layout, and the Agent badge order.
package agents

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/saltand/kitter/native/core/model"
)

// GlobalTargetRoot mirrors global_target_root: user-level roots must not
// be derived from project-relative directories.
func GlobalTargetRoot(home string, target model.InstallTarget) string {
	switch target {
	case model.TargetCodex:
		base := os.Getenv("CODEX_HOME")
		if base == "" {
			base = filepath.Join(home, ".codex")
		}
		return filepath.Join(base, "skills")
	case model.TargetPi:
		if value := os.Getenv("PI_CODING_AGENT_DIR"); value != "" {
			if rel, ok := strings.CutPrefix(value, "~/"); ok {
				return filepath.Join(home, rel, "skills")
			}
			return filepath.Join(home, value, "skills")
		}
		return filepath.Join(home, ".pi/agent", "skills")
	case model.TargetOpenCode:
		base := os.Getenv("XDG_CONFIG_HOME")
		if base == "" {
			base = filepath.Join(home, ".config")
		}
		return filepath.Join(base, "opencode", "skills")
	case model.TargetAntigravity:
		return filepath.Join(home, ".gemini/config/skills")
	case model.TargetCopilot:
		return filepath.Join(home, ".copilot/skills")
	default:
		return filepath.Join(home, TargetDirectory(target))
	}
}

// InstallationRoot mirrors installation_root: a project equal to the home
// directory means a user-level (global) install.
func InstallationRoot(project string, target model.InstallTarget) string {
	if home, err := os.UserHomeDir(); err == nil && home == project {
		return GlobalTargetRoot(project, target)
	}
	return filepath.Join(project, TargetDirectory(target))
}

// InstallTargetInfo mirrors InstallTargetInfo.
type InstallTargetInfo struct {
	Target   model.InstallTarget
	Name     string
	IconPath string
}

// ProjectInstallTargets is PROJECT_INSTALL_TARGETS, in Rust order.
var ProjectInstallTargets = []model.InstallTarget{
	model.TargetUniversal,
	model.TargetCodex,
	model.TargetClaudeCode,
	model.TargetCursor,
	model.TargetOpenCode,
	model.TargetPi,
	model.TargetGrok,
	model.TargetAntigravity,
	model.TargetDroid,
	model.TargetCopilot,
}

// IndependentInstallTargets is INDEPENDENT_INSTALL_TARGETS.
var IndependentInstallTargets = []InstallTargetInfo{
	{model.TargetCodex, "Codex", "icons/provider-codex.svg"},
	{model.TargetClaudeCode, "Claude Code", "icons/provider-claude.svg"},
	{model.TargetCursor, "Cursor", "icons/provider-cursor.svg"},
	{model.TargetOpenCode, "OpenCode", "icons/provider-opencode.svg"},
	{model.TargetPi, "Pi", "icons/provider-pi.svg"},
	{model.TargetGrok, "Grok", "icons/provider-grok.svg"},
	{model.TargetAntigravity, "Antigravity", "icons/provider-antigravity.svg"},
	{model.TargetDroid, "Droid", "icons/provider-droid.svg"},
	{model.TargetCopilot, "GitHub Copilot", "icons/provider-copilot.svg"},
}

// AgentIconInfo mirrors AgentIconInfo: presentation metadata for badges.
type AgentIconInfo struct {
	ID             string
	Name           string
	IconPath       string
	InstallTargets []model.InstallTarget
	SharedAgents   bool
	GlobalOnly     bool
}

// SupportsTarget mirrors AgentIconInfo::supports_target.
func (a AgentIconInfo) SupportsTarget(target model.InstallTarget) bool {
	for _, t := range a.InstallTargets {
		if t == target {
			return true
		}
	}
	return target == model.TargetUniversal && a.SharedAgents
}

// AgentIconOrder is AGENT_ICON_ORDER, the single source of truth for badge
// order.
var AgentIconOrder = []AgentIconInfo{
	{ID: "codex", Name: "Codex", IconPath: "icons/provider-codex.svg",
		InstallTargets: []model.InstallTarget{model.TargetCodex}, SharedAgents: true},
	{ID: "claude-code", Name: "Claude Code", IconPath: "icons/provider-claude.svg",
		InstallTargets: []model.InstallTarget{model.TargetClaudeCode}},
	{ID: "cursor", Name: "Cursor", IconPath: "icons/provider-cursor.svg",
		InstallTargets: []model.InstallTarget{model.TargetCursor, model.TargetClaudeCode, model.TargetCodex},
		SharedAgents:   true},
	{ID: "opencode", Name: "OpenCode", IconPath: "icons/provider-opencode.svg",
		InstallTargets: []model.InstallTarget{model.TargetOpenCode, model.TargetClaudeCode},
		SharedAgents:   true},
	{ID: "pi", Name: "Pi", IconPath: "icons/provider-pi.svg",
		InstallTargets: []model.InstallTarget{model.TargetPi}, SharedAgents: true},
	{ID: "grok", Name: "Grok", IconPath: "icons/provider-grok.svg",
		InstallTargets: []model.InstallTarget{model.TargetGrok, model.TargetClaudeCode, model.TargetCursor},
		SharedAgents:   true},
	{ID: "openclaw", Name: "OpenClaw", IconPath: "icons/provider-openclaw.svg",
		InstallTargets: []model.InstallTarget{}, GlobalOnly: true},
	{ID: "hermes", Name: "Hermes", IconPath: "icons/provider-hermes.svg",
		InstallTargets: []model.InstallTarget{}, GlobalOnly: true},
	{ID: "droid", Name: "Droid", IconPath: "icons/provider-droid.svg",
		InstallTargets: []model.InstallTarget{model.TargetDroid, model.TargetAntigravity},
		SharedAgents:   true},
	{ID: "amp", Name: "Amp", IconPath: "icons/provider-amp.svg",
		InstallTargets: []model.InstallTarget{model.TargetClaudeCode}, SharedAgents: true},
	{ID: "antigravity", Name: "Antigravity", IconPath: "icons/provider-antigravity.svg",
		InstallTargets: []model.InstallTarget{model.TargetAntigravity}, SharedAgents: true},
	{ID: "copilot", Name: "GitHub Copilot", IconPath: "icons/provider-copilot.svg",
		InstallTargets: []model.InstallTarget{model.TargetCopilot, model.TargetClaudeCode},
		SharedAgents:   true},
	{ID: "trae", Name: "Trae", IconPath: "icons/provider-trae.svg",
		InstallTargets: []model.InstallTarget{}, SharedAgents: true},
}

// TargetDirectory is target_directory: the project-relative skills dir.
func TargetDirectory(target model.InstallTarget) string {
	switch target {
	case model.TargetUniversal:
		return ".agents/skills"
	case model.TargetCodex:
		return ".codex/skills"
	case model.TargetClaudeCode:
		return ".claude/skills"
	case model.TargetCursor:
		return ".cursor/skills"
	case model.TargetOpenCode:
		return ".opencode/skills"
	case model.TargetPi:
		return ".pi/skills"
	case model.TargetGrok:
		return ".grok/skills"
	case model.TargetAntigravity:
		return ".agent/skills"
	case model.TargetDroid:
		return ".factory/skills"
	case model.TargetCopilot:
		return ".github/skills"
	}
	return ""
}
