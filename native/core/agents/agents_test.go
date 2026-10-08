package agents

import (
	"testing"

	"github.com/saltand/kitter/native/core/model"
)

// Mirrors agents.rs tests.

func TestAgentIconOrderMatchesUsagePriority(t *testing.T) {
	want := []string{
		"codex", "claude-code", "cursor", "opencode", "pi", "grok",
		"openclaw", "hermes", "droid", "amp", "antigravity", "copilot", "trae",
	}
	if len(AgentIconOrder) != len(want) {
		t.Fatalf("got %d agents, want %d", len(AgentIconOrder), len(want))
	}
	for i, agent := range AgentIconOrder {
		if agent.ID != want[i] {
			t.Fatalf("position %d: got %s want %s", i, agent.ID, want[i])
		}
	}
}

func TestOpenclawAndHermesAreGlobalOnly(t *testing.T) {
	var globalOnly []string
	for _, agent := range AgentIconOrder {
		if agent.GlobalOnly {
			globalOnly = append(globalOnly, agent.ID)
			if len(agent.InstallTargets) != 0 {
				t.Fatalf("%s is global-only but has install targets", agent.ID)
			}
		}
	}
	if len(globalOnly) != 2 || globalOnly[0] != "openclaw" || globalOnly[1] != "hermes" {
		t.Fatalf("got %v", globalOnly)
	}
}

func TestSharedAgentsAlsoHaveProviderSpecificTargets(t *testing.T) {
	for _, id := range []string{"codex", "cursor", "opencode", "pi", "grok", "droid", "antigravity", "copilot"} {
		var agent *AgentIconInfo
		for i := range AgentIconOrder {
			if AgentIconOrder[i].ID == id {
				agent = &AgentIconOrder[i]
			}
		}
		if agent == nil {
			t.Fatalf("%s missing", id)
		}
		if !agent.SharedAgents {
			t.Fatalf("%s should consume .agents/skills", id)
		}
		found := false
		for _, target := range agent.InstallTargets {
			if target != model.TargetUniversal {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s should have a provider-specific project target", id)
		}
	}
}

func TestCompatibilityBadgesCoverSharedProviderSpecificRoots(t *testing.T) {
	var cursor, openclaw *AgentIconInfo
	for i := range AgentIconOrder {
		switch AgentIconOrder[i].ID {
		case "cursor":
			cursor = &AgentIconOrder[i]
		case "openclaw":
			openclaw = &AgentIconOrder[i]
		}
	}
	for _, target := range []model.InstallTarget{
		model.TargetUniversal, model.TargetCursor, model.TargetClaudeCode, model.TargetCodex,
	} {
		if !cursor.SupportsTarget(target) {
			t.Fatalf("cursor should support %s", target)
		}
	}
	if openclaw.SupportsTarget(model.TargetUniversal) {
		t.Fatal("openclaw should not support universal")
	}
}

func TestGlobalTargetRootHonorsEnvOverrides(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", "")
	t.Setenv("PI_CODING_AGENT_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	if got := GlobalTargetRoot(home, model.TargetCodex); got != home+"/.codex/skills" {
		t.Fatal(got)
	}
	t.Setenv("CODEX_HOME", "/custom/codex")
	if got := GlobalTargetRoot(home, model.TargetCodex); got != "/custom/codex/skills" {
		t.Fatal(got)
	}
	t.Setenv("PI_CODING_AGENT_DIR", "~/pi-dir")
	if got := GlobalTargetRoot(home, model.TargetPi); got != home+"/pi-dir/skills" {
		t.Fatal(got)
	}
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	if got := GlobalTargetRoot(home, model.TargetOpenCode); got != "/xdg/opencode/skills" {
		t.Fatal(got)
	}
	if got := GlobalTargetRoot(home, model.TargetAntigravity); got != home+"/.gemini/config/skills" {
		t.Fatal(got)
	}
}
