// effective_port_test.go ports the #[test] functions from
// src/effective_skills.rs that exercise the policy framework.
package effective

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saltand/kitter/native/core/agents"
)

func ctx(cwd, home, repo string) *DiscoveryContext {
	return &DiscoveryContext{Cwd: cwd, Home: home, RepositoryRoot: repo}
}

func skillNames(e AgentContextEstimate) map[string]bool {
	names := map[string]bool{}
	for _, s := range e.Skills {
		names[s.Name] = true
	}
	return names
}

func hasSkill(e AgentContextEstimate, name string) bool { return skillNames(e)[name] }
func hasSkillP(e *AgentContextEstimate, name string) bool { return e != nil && skillNames(*e)[name] }

// manual_skills_are_discovered_but_not_counted
func TestManualSkillsAreDiscoveredButNotCounted(t *testing.T) {
	temp := t.TempDir()
	project := filepath.Join(temp, "repo", "subdir")
	if err := os.MkdirAll(filepath.Join(project, ".agents/skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(temp, "repo/.git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, filepath.Join(project, ".agents/skills"), "automatic", "")
	writeSkill(t, filepath.Join(project, ".agents/skills"), "manual",
		"disable-model-invocation: true\n")
	c := ctx(project, filepath.Join(temp, "home"), filepath.Join(temp, "repo"))
	estimate := estimateWithPolicy(newPiAdapter(), c)
	if estimate.DiscoveredCount != 2 {
		t.Fatalf("discovered %d", estimate.DiscoveredCount)
	}
	if estimate.ModelVisibleCount != 1 {
		t.Fatalf("visible %d", estimate.ModelVisibleCount)
	}
	if estimate.ManualOnlyCount != 1 {
		t.Fatalf("manual %d", estimate.ManualOnlyCount)
	}
}

// invocation_extensions_are_only_applied_by_providers_that_support_them
func TestInvocationExtensionsOnlyAppliedBySupportingProviders(t *testing.T) {
	temp := t.TempDir()
	skill := writeSkill(t, temp, "manual",
		"disable-model-invocation: true\nmetadata:\n  opencode:\n    autoinvoke: false\n")
	metadata := readMetadataWithProfile(filepath.Join(skill, "SKILL.md"), ProfileStrictFrontmatter)
	if metadata == nil {
		t.Fatal("metadata")
	}
	c := ctx(temp, filepath.Join(temp, "home"), "")
	manual := []agentSkillPolicy{
		newClaudeCodeAdapter(), newCursorPolicy(), newOpenCodePolicy(),
		newCopilotPolicy(), newDroidPolicy(), newPiAdapter(),
		newGrokAdapter(), newOpenClawPolicy(),
	}
	for _, policy := range manual {
		if got := policy.Visibility(skill, metadata, c); got != VisibilityManualOnly {
			t.Fatalf("%s should honor manual invocation metadata, got %v", policy.Agent().Label(), got)
		}
	}
	auto := []agentSkillPolicy{newAntigravityPolicy(), newAmpPolicy(), newHermesPolicy()}
	for _, policy := range auto {
		if got := policy.Visibility(skill, metadata, c); got != VisibilityAutomatic {
			t.Fatalf("%s must not honor unsupported invocation metadata, got %v", policy.Agent().Label(), got)
		}
	}
}

// codex_token_estimate_is_not_capped_at_two_thousand
func TestCodexTokenEstimateNotCappedAtTwoThousand(t *testing.T) {
	rendered := strings.Repeat("x", 8001)
	if got := approxTokenCount(rendered); got != 2001 {
		t.Fatalf("got %d", got)
	}
}

// pi_uses_nearest_skill_when_names_collide
func TestPiUsesNearestSkillWhenNamesCollide(t *testing.T) {
	temp := t.TempDir()
	repo := filepath.Join(temp, "repo")
	cwd := filepath.Join(repo, "nested")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, filepath.Join(cwd, ".agents/skills"), "same", "")
	writeSkill(t, filepath.Join(repo, ".agents/skills"), "same", "")
	c := ctx(cwd, filepath.Join(temp, "home"), repo)
	estimate := estimateWithPolicy(newPiAdapter(), c)
	if estimate.DiscoveredCount != 1 {
		t.Fatalf("discovered %d", estimate.DiscoveredCount)
	}
	if !strings.HasPrefix(estimate.Skills[0].Path, cwd) {
		t.Fatalf("wrong skill: %s", estimate.Skills[0].Path)
	}
}

// pi_only_uses_cwd_pi_directory_and_accepts_root_markdown
func TestPiOnlyUsesCwdPiDirectoryAndAcceptsRootMarkdown(t *testing.T) {
	temp := t.TempDir()
	repo := filepath.Join(temp, "repo")
	cwd := filepath.Join(repo, "nested")
	writeFile(t, filepath.Join(cwd, ".pi/skills/direct.md"),
		"---\nname: direct\ndescription: Direct skill\n---\n")
	writeFile(t, filepath.Join(cwd, ".pi/skills/second.md"),
		"---\nname: second\ndescription: Second skill\n---\n")
	writeFile(t, filepath.Join(repo, ".git/HEAD"), "")
	writeSkill(t, filepath.Join(repo, ".pi/skills"), "parent-pi", "")
	root := NewSkillRoot(filepath.Join(cwd, ".pi/skills"), ScopeLocal).WithRootMarkdown()
	if got := len(scan(root, PiIgnoredProfile())); got != 2 {
		t.Fatalf("scan %d", got)
	}
	if readMetadataWithProfile(filepath.Join(cwd, ".pi/skills/direct.md"), ProfileStrictFrontmatter) == nil {
		t.Fatal("metadata")
	}
	c := ctx(cwd, filepath.Join(temp, "home"), repo)
	estimate := estimateWithPolicy(newPiAdapter(), c)
	if estimate.DiscoveredCount != 2 {
		t.Fatalf("discovered %d: %v", estimate.DiscoveredCount, skillNames(estimate))
	}
	if !hasSkill(estimate, "direct") || !hasSkill(estimate, "second") {
		t.Fatalf("missing: %v", skillNames(estimate))
	}
}

// pi_loads_settings_paths_for_other_agent_skill_roots
func TestPiLoadsSettingsPathsForOtherAgentSkillRoots(t *testing.T) {
	temp := t.TempDir()
	home := filepath.Join(temp, "home")
	repo := filepath.Join(temp, "repo")
	cwd := filepath.Join(repo, "src")
	writeFile(t, filepath.Join(repo, ".git/HEAD"), "")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, filepath.Join(home, ".claude/skills"), "user-claude", "")
	writeSkill(t, filepath.Join(home, ".codex/skills"), "user-codex", "")
	writeSkill(t, filepath.Join(repo, ".claude/skills"), "project-claude", "")
	writeSkill(t, filepath.Join(repo, ".agents/skills"), "shared", "")
	writeFile(t, filepath.Join(home, ".pi/agent/settings.json"),
		`{"skills":["~/.claude/skills","~/.codex/skills"]}`)
	writeFile(t, filepath.Join(cwd, ".pi/settings.json"),
		`{"skills":["../../.claude/skills"]}`)
	c := ctx(cwd, home, repo)
	estimate := estimateWithPolicy(newPiAdapter(), c)
	for _, name := range []string{"user-claude", "user-codex", "project-claude", "shared"} {
		if !hasSkill(estimate, name) {
			t.Fatalf("Pi did not discover configured skill %s (have %v)", name, skillNames(estimate))
		}
	}
}

// pi_loads_static_skills_from_configured_packages
func TestPiLoadsStaticSkillsFromConfiguredPackages(t *testing.T) {
	temp := t.TempDir()
	home := filepath.Join(temp, "home")
	project := filepath.Join(temp, "project")
	pkg := filepath.Join(project, ".pi/npm/node_modules/example-pi-package")
	writeFile(t, filepath.Join(project, ".pi/settings.json"),
		`{"packages":["npm:example-pi-package@1.0.0"]}`)
	writeFile(t, filepath.Join(pkg, "package.json"),
		`{"pi":{"skills":["resources/release"]}}`)
	writeFile(t, filepath.Join(pkg, "resources/release/SKILL.md"),
		"---\nname: package-release\ndescription: Release from package\n---\n")
	estimates := EstimateProjectWithHome(project, home)
	var pi *AgentContextEstimate
	for i := range estimates {
		if estimates[i].Agent == AgentPi {
			pi = &estimates[i]
		}
	}
	if pi == nil || !hasSkillP(pi, "package-release") {
		t.Fatalf("package-release not discovered: %v", pi)
	}
}

// pi_requires_a_valid_description_and_uses_the_real_prompt_wrapper
func TestPiRequiresValidDescriptionAndRealPromptWrapper(t *testing.T) {
	temp := t.TempDir()
	project := filepath.Join(temp, "project")
	writeFile(t, filepath.Join(project, ".pi/skills/no-description/SKILL.md"),
		"---\nname: no-description\n---\nBody")
	writeSkill(t, filepath.Join(project, ".pi/skills"), "valid", "")
	c := ctx(project, filepath.Join(temp, "home"), "")
	estimate := estimateWithPolicy(newPiAdapter(), c)
	if hasSkill(estimate, "no-description") {
		t.Fatal("no-description should be filtered")
	}
	adapter := newPiAdapter()
	rendered := adapter.RenderVisibleMetadata(estimate.Skills)
	if !strings.HasPrefix(rendered, "\n\nThe following skills provide specialized instructions") {
		t.Fatalf("render prefix: %q", rendered[:60])
	}
	if !strings.Contains(rendered, "<location>") {
		t.Fatal("missing <location>")
	}
	if estimate.EstimatedTokens != approxTokenCount(rendered) {
		t.Fatalf("tokens %d != %d", estimate.EstimatedTokens, approxTokenCount(rendered))
	}
}

// codex_config_parsers_only_keep_enabled_plugins_and_disabled_skills
func TestCodexConfigParsersOnlyKeepEnabledPluginsAndDisabledSkills(t *testing.T) {
	temp := t.TempDir()
	skill := filepath.Join(temp, "demo/SKILL.md")
	writeFile(t, skill, "---\nname: demo\ndescription: Test\n---\n")
	config := fmt.Sprintf("[plugins.\"on@market\"]\nenabled = true\n[plugins.\"off@market\"]\nenabled = false\n[[skills.config]]\npath = \"%s\"\nenabled = false\n", skill)
	writeFile(t, filepath.Join(temp, "config.toml"), config)
	ids := enabledPluginIDs(config)
	if len(ids) != 1 || ids[0] != "on@market" {
		t.Fatalf("ids %v", ids)
	}
	disabled := codexDisabledSkills(temp, temp)
	if !disabled[canonicalize(skill)] {
		t.Fatalf("disabled %v", disabled)
	}
	entries := skillConfigEntries(config)
	if len(entries) != 1 || entries[0][0] != skill || entries[0][1] != false {
		t.Fatalf("entries %v", entries)
	}
}

// codex_enabled_skill_config_selects_but_does_not_add_a_root
func TestCodexEnabledSkillConfigSelectsButDoesNotAddRoot(t *testing.T) {
	temp := t.TempDir()
	home := filepath.Join(temp, "home")
	project := filepath.Join(temp, "project")
	external := filepath.Join(temp, "external")
	writeFile(t, filepath.Join(home, ".codex/config.toml"),
		fmt.Sprintf("[[skills.config]]\npath = \"%s\"\nenabled = true\n", external))
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, external, "configured-only", "")
	estimates := EstimateProjectWithHome(project, home)
	var codex *AgentContextEstimate
	for i := range estimates {
		if estimates[i].Agent == AgentCodex {
			codex = &estimates[i]
		}
	}
	if codex == nil || hasSkillP(codex, "configured-only") {
		t.Fatal("enabled config skill should not add a root")
	}
}

// claude_uses_direct_children_body_fallback_and_keeps_same_names
func TestClaudeUsesDirectChildrenBodyFallbackAndKeepsSameNames(t *testing.T) {
	temp := t.TempDir()
	home := filepath.Join(temp, "home")
	project := filepath.Join(temp, "project")
	writeFile(t, filepath.Join(project, ".claude/skills/same/SKILL.md"), "Project fallback description")
	writeFile(t, filepath.Join(home, ".claude/skills/same/SKILL.md"), "User fallback description")
	writeFile(t, filepath.Join(project, ".claude/skills/group/nested/SKILL.md"), "Nested description")
	estimates := EstimateProjectWithHome(project, home)
	var claude *AgentContextEstimate
	for i := range estimates {
		if estimates[i].Agent == AgentClaudeCode {
			claude = &estimates[i]
		}
	}
	same := 0
	for _, s := range claude.Skills {
		if s.Name == "same" {
			same++
			if s.Description == "Project fallback description" {
				// found
			}
		}
	}
	if same != 2 {
		t.Fatalf("same count %d", same)
	}
	foundProject := false
	for _, s := range claude.Skills {
		if s.Name == "same" && s.Description == "Project fallback description" {
			foundProject = true
		}
	}
	if !foundProject {
		t.Fatal("project fallback missing")
	}
	for _, s := range claude.Skills {
		if s.Path == filepath.Join(project, ".claude/skills/group/nested/SKILL.md") {
			t.Fatal("nested should not be discovered (direct children only)")
		}
	}
}

// grok_loads_flat_commands_and_renders_startup_listing_fields
func TestGrokLoadsFlatCommandsAndRendersStartupListingFields(t *testing.T) {
	temp := t.TempDir()
	home := filepath.Join(temp, "home")
	project := filepath.Join(temp, "project")
	commands := filepath.Join(project, ".grok/commands")
	writeFile(t, filepath.Join(commands, "Release Notes.md"), "Prepare release notes")
	writeFile(t, filepath.Join(commands, "nested/SKILL.md"),
		"---\nname: nested-command\ndescription: Must stay hidden\n---\n")
	estimates := EstimateProjectWithHome(project, home)
	var grok *AgentContextEstimate
	for i := range estimates {
		if estimates[i].Agent == AgentGrok {
			grok = &estimates[i]
		}
	}
	var command *EffectiveSkill
	for i := range grok.Skills {
		if grok.Skills[i].Name == "release-notes" {
			command = &grok.Skills[i]
		}
	}
	if command == nil {
		t.Fatalf("release-notes missing: %v", skillNames(*grok))
	}
	rendered := newGrokAdapter().RenderVisibleMetadata([]EffectiveSkill{*command})
	if !strings.HasPrefix(rendered, "<system-reminder>") {
		t.Fatal("prefix")
	}
	for _, want := range []string{"The following skills are available for use:", "Absolute path:", command.Path} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing %q", want)
		}
	}
	if hasSkillP(grok, "nested-command") {
		t.Fatal("nested-command should stay hidden")
	}
}

// groups_agent_specific_names_for_the_same_filesystem_skill
func TestGroupsAgentSpecificNamesForSameFilesystemSkill(t *testing.T) {
	temp := t.TempDir()
	home := filepath.Join(temp, "home")
	project := filepath.Join(temp, "project")
	command := filepath.Join(home, ".claude/commands/build_from_zero.md")
	writeFile(t, command, "Build from zero")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	estimates := EstimateProjectWithHome(project, home)
	var entries []GroupEntry
	for i := range estimates {
		for j := range estimates[i].Skills {
			entries = append(entries, GroupEntry{estimates[i].Agent, &estimates[i].Skills[j]})
		}
	}
	groups := GroupEffectiveSkills(entries)
	var group *EffectiveSkillGroup
	for _, g := range groups {
		for _, e := range g.Entries {
			if e.Skill.Path == command {
				group = g
			}
		}
	}
	if group == nil {
		t.Fatal("group missing")
	}
	if group.Name() != "build_from_zero" {
		t.Fatalf("name %q", group.Name())
	}
	names := map[string]bool{}
	agents := map[AgentKind]bool{}
	for _, e := range group.Entries {
		names[e.Skill.Name] = true
		agents[e.Agent] = true
	}
	if !names["build_from_zero"] || !names["build-from-zero"] {
		t.Fatalf("names %v", names)
	}
	if !agents[AgentClaudeCode] || !agents[AgentGrok] {
		t.Fatalf("agents %v", agents)
	}
}

// grok_rekeys_same_scope_frontmatter_collisions_by_directory
func TestGrokRekeysSameScopeFrontmatterCollisionsByDirectory(t *testing.T) {
	temp := t.TempDir()
	home := filepath.Join(temp, "home")
	project := filepath.Join(temp, "project")
	skills := filepath.Join(project, ".grok/skills")
	for _, dir := range []string{"original", "copied"} {
		writeFile(t, filepath.Join(skills, dir, "SKILL.md"),
			"---\nname: shared-name\ndescription: Shared\n---\n")
	}
	estimates := EstimateProjectWithHome(project, home)
	var grok *AgentContextEstimate
	for i := range estimates {
		if estimates[i].Agent == AgentGrok {
			grok = &estimates[i]
		}
	}
	names := skillNames(*grok)
	if !names["shared-name"] {
		t.Fatalf("names %v", names)
	}
	if !names["copied"] && !names["original"] {
		t.Fatalf("expected directory rekey: %v", names)
	}
}

// codex_plugin_manifest_paths_and_source_are_preserved
func TestCodexPluginManifestPathsAndSourceArePreserved(t *testing.T) {
	temp := t.TempDir()
	home := filepath.Join(temp, "home")
	codexHome := filepath.Join(home, ".codex")
	pluginRoot := filepath.Join(codexHome, "plugins/cache/market/deployer/1.0.0")
	writeFile(t, filepath.Join(codexHome, "config.toml"),
		"[plugins.\"deployer@market\"]\nenabled = true\n")
	writeFile(t, filepath.Join(pluginRoot, ".codex-plugin/plugin.json"),
		`{"name":"deployer","interface":{"displayName":"Deployer"},"paths":{"skills":["contributions/release"]}}`)
	writeSkill(t, filepath.Join(pluginRoot, "contributions"), "release", "")
	writeSkill(t, filepath.Join(pluginRoot, "skills"), "decoy", "")
	repo := filepath.Join(temp, "repo")
	writeFile(t, filepath.Join(repo, ".git/HEAD"), "")
	c := ctx(repo, home, repo)
	estimate := estimateWithPolicy(newCodexAdapter(), c)
	plugins := estimate.PluginSkills()
	if len(plugins) != 1 {
		t.Fatalf("plugins %v", plugins)
	}
	if plugins[0].Name != "release" {
		t.Fatalf("name %q", plugins[0].Name)
	}
	if plugins[0].Path != filepath.Join(pluginRoot, "contributions/release/SKILL.md") {
		t.Fatalf("path %q", plugins[0].Path)
	}
	if plugins[0].Source.Kind != SourcePlugin || plugins[0].Source.ID != "deployer@market" ||
		plugins[0].Source.DisplayName != "Deployer" {
		t.Fatalf("source %+v", plugins[0].Source)
	}
}

// codex_reads_project_codex_skills_as_well_as_ancestor_agents_skills
func TestCodexReadsProjectCodexSkillsAsWellAsAncestorAgentsSkills(t *testing.T) {
	temp := t.TempDir()
	repo := filepath.Join(temp, "repo")
	cwd := filepath.Join(repo, "nested")
	writeFile(t, filepath.Join(repo, ".git/HEAD"), "")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, filepath.Join(repo, ".codex/skills"), "codex-local", "")
	writeSkill(t, filepath.Join(repo, ".agents/skills"), "shared", "")
	c := ctx(cwd, filepath.Join(temp, "home"), repo)
	estimate := estimateWithPolicy(newCodexAdapter(), c)
	if !hasSkill(estimate, "codex-local") || !hasSkill(estimate, "shared") {
		t.Fatalf("skills %v", skillNames(estimate))
	}
}

// project_estimates_are_derived_from_the_selected_path
func TestProjectEstimatesAreDerivedFromSelectedPath(t *testing.T) {
	temp := t.TempDir()
	home := filepath.Join(temp, "home")
	emptyProject := filepath.Join(temp, "empty-project")
	projectWithSkill := filepath.Join(temp, "project-with-skill")
	if err := os.MkdirAll(emptyProject, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(projectWithSkill, 0o755); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, filepath.Join(projectWithSkill, ".agents/skills"), "selected-project-only", "")
	empty := EstimateProjectWithHome(emptyProject, home)
	selected := EstimateProjectWithHome(projectWithSkill, home)
	for _, agent := range []AgentKind{AgentCodex, AgentOpenCode, AgentPi, AgentGrok} {
		var emptyCount, selectedCount int
		var selectedEstimate *AgentContextEstimate
		for i := range empty {
			if empty[i].Agent == agent {
				emptyCount = empty[i].DiscoveredCount
			}
		}
		for i := range selected {
			if selected[i].Agent == agent {
				selectedEstimate = &selected[i]
				selectedCount = selected[i].DiscoveredCount
			}
		}
		if selectedEstimate == nil || selectedCount != emptyCount+1 {
			t.Fatalf("%s: selected %d empty %d", agent.ID(), selectedCount, emptyCount)
		}
		if !hasSkillP(selectedEstimate, "selected-project-only") {
			t.Fatalf("%s missing", agent.ID())
		}
	}
	for _, e := range empty {
		if e.Agent.IsGlobalOnly() {
			t.Fatal("global-only agent in project estimates")
		}
	}
	for _, e := range selected {
		if e.Agent.IsGlobalOnly() {
			t.Fatal("global-only agent in project estimates")
		}
	}
}

// global_estimates_do_not_include_parent_project_roots
func TestGlobalEstimatesDoNotIncludeParentProjectRoots(t *testing.T) {
	temp := t.TempDir()
	home := filepath.Join(temp, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, filepath.Join(temp, ".agents/skills"), "parent-project", "")
	writeSkill(t, filepath.Join(home, ".agents/skills"), "user-skill", "")
	estimates := EstimateProjectWithHome(home, home)
	for _, agent := range []AgentKind{AgentCodex, AgentOpenCode, AgentPi} {
		var estimate *AgentContextEstimate
		for i := range estimates {
			if estimates[i].Agent == agent {
				estimate = &estimates[i]
			}
		}
		if !hasSkillP(estimate, "user-skill") {
			t.Fatalf("%s missing user-skill", agent.ID())
		}
		if hasSkillP(estimate, "parent-project") {
			t.Fatalf("%s includes parent-project", agent.ID())
		}
	}
	global := 0
	for _, e := range estimates {
		if e.Agent.IsGlobalOnly() {
			global++
		}
	}
	if global != 2 {
		t.Fatalf("global-only count %d", global)
	}
}

// compatible_agents_scan_both_agents_and_claude_roots
func TestCompatibleAgentsScanBothAgentsAndClaudeRoots(t *testing.T) {
	temp := t.TempDir()
	repo := filepath.Join(temp, "repo")
	home := filepath.Join(temp, "home")
	writeFile(t, filepath.Join(repo, ".git/HEAD"), "")
	writeSkill(t, filepath.Join(repo, ".agents/skills"), "shared-agents", "")
	writeSkill(t, filepath.Join(repo, ".claude/skills"), "shared-claude", "")
	c := ctx(repo, home, repo)
	for _, policy := range []agentSkillPolicy{newCursorPolicy(), newCopilotPolicy(), newAmpPolicy()} {
		estimate := estimateWithPolicy(policy, c)
		if !hasSkill(estimate, "shared-agents") || !hasSkill(estimate, "shared-claude") {
			t.Fatalf("%s: %v", policy.Agent().ID(), skillNames(estimate))
		}
	}
}

// cursor_deduplicates_same_named_skills_across_compatible_roots
func TestCursorDeduplicatesSameNamedSkillsAcrossCompatibleRoots(t *testing.T) {
	temp := t.TempDir()
	repo := filepath.Join(temp, "repo")
	home := filepath.Join(temp, "home")
	writeFile(t, filepath.Join(repo, ".git/HEAD"), "")
	writeSkill(t, filepath.Join(repo, ".cursor/skills"), "same", "")
	writeSkill(t, filepath.Join(repo, ".agents/skills"), "same", "")
	writeSkill(t, filepath.Join(repo, ".claude/skills"), "same", "")
	c := ctx(repo, home, repo)
	estimate := estimateWithPolicy(newCursorPolicy(), c)
	if estimate.DiscoveredCount != 1 || estimate.ModelVisibleCount != 1 {
		t.Fatalf("counts %d %d", estimate.DiscoveredCount, estimate.ModelVisibleCount)
	}
	if !strings.HasPrefix(estimate.Skills[0].Path, filepath.Join(repo, ".cursor/skills")) {
		t.Fatalf("path %s", estimate.Skills[0].Path)
	}
}

// every_effective_detector_has_a_matching_agent_badge: every AgentKind
// produced by inspect has an icon in agents.AgentIconOrder.
func TestEveryEffectiveDetectorHasMatchingAgentBadge(t *testing.T) {
	temp := t.TempDir()
	home := filepath.Join(temp, "home")
	estimates := append(
		EstimateProjectWithHome(temp, home),
		EstimateProjectWithHome(home, home)...,
	)
	badgeIDs := map[string]bool{}
	for _, icon := range agents.AgentIconOrder {
		badgeIDs[icon.ID] = true
	}
	for _, estimate := range estimates {
		if !badgeIDs[estimate.Agent.ID()] {
			t.Fatalf("missing badge for %s", estimate.Agent.ID())
		}
	}
}

// disabled_codex_plugin_does_not_appear_even_when_cached
func TestDisabledCodexPluginDoesNotAppearEvenWhenCached(t *testing.T) {
	temp := t.TempDir()
	home := filepath.Join(temp, "home")
	codexHome := filepath.Join(home, ".codex")
	pluginRoot := filepath.Join(codexHome, "plugins/cache/market/off/1.0.0")
	writeFile(t, filepath.Join(codexHome, "config.toml"),
		"[plugins.\"off@market\"]\nenabled = false\n")
	writeFile(t, filepath.Join(pluginRoot, ".codex-plugin/plugin.json"),
		`{"name":"Off","skills":"skills"}`)
	writeSkill(t, filepath.Join(pluginRoot, "skills"), "hidden", "")
	repo := filepath.Join(temp, "repo")
	writeFile(t, filepath.Join(repo, ".git/HEAD"), "")
	c := ctx(repo, home, repo)
	estimate := estimateWithPolicy(newCodexAdapter(), c)
	for _, s := range estimate.PluginSkills() {
		if s.Name == "hidden" {
			t.Fatal("disabled plugin leaked")
		}
	}
}

// claude_plugin_manifest_string_path_and_source_are_preserved
func TestClaudePluginManifestStringPathAndSourceArePreserved(t *testing.T) {
	temp := t.TempDir()
	home := filepath.Join(temp, "home")
	pluginRoot := filepath.Join(home, "plugins/deployer")
	writeFile(t, filepath.Join(pluginRoot, ".claude-plugin/plugin.json"),
		`{"name":"Deployer UI","skills":"contributions/release"}`)
	writeFile(t, filepath.Join(home, ".claude/settings.json"),
		`{"enabledPlugins":{"deployer@market":true}}`)
	writeFile(t, filepath.Join(home, ".claude/plugins/installed_plugins.json"),
		fmt.Sprintf(`{"plugins":{"deployer@market":[{"scope":"user","installPath":%q}]}}`, pluginRoot))
	writeSkill(t, filepath.Join(pluginRoot, "contributions"), "release", "")
	writeSkill(t, filepath.Join(pluginRoot, "skills"), "decoy", "")
	repo := filepath.Join(temp, "repo")
	writeFile(t, filepath.Join(repo, ".git/HEAD"), "")
	c := ctx(repo, home, repo)
	estimate := estimateWithPolicy(newClaudeCodeAdapter(), c)
	plugins := estimate.PluginSkills()
	if len(plugins) != 1 {
		t.Fatalf("plugins %v", plugins)
	}
	if plugins[0].Name != "Deployer UI:release" {
		t.Fatalf("name %q", plugins[0].Name)
	}
	if id, ok := plugins[0].Source.PluginID(); !ok || id != "deployer@market" {
		t.Fatalf("id %q", id)
	}
	if name, ok := plugins[0].Source.PluginDisplayName(); !ok || name != "Deployer UI" {
		t.Fatalf("display %q", name)
	}
}

// claude_keeps_same_named_paths_and_honors_visibility_overrides
func TestClaudeKeepsSameNamedPathsAndHonorsVisibilityOverrides(t *testing.T) {
	temp := t.TempDir()
	home := filepath.Join(temp, "home")
	repo := filepath.Join(temp, "repo")
	writeFile(t, filepath.Join(repo, ".git/HEAD"), "")
	writeSkill(t, filepath.Join(home, ".claude/skills"), "same", "")
	writeSkill(t, filepath.Join(repo, ".claude/skills"), "same", "")
	writeSkill(t, filepath.Join(repo, ".claude/skills"), "manual-claude",
		"disable-model-invocation: true\n")
	writeSkill(t, filepath.Join(repo, ".claude/skills"), "compact", "")
	writeFile(t, filepath.Join(repo, ".claude/settings.local.json"),
		`{"skillOverrides":{"compact":"name-only"}}`)
	c := ctx(repo, home, repo)
	estimate := estimateWithPolicy(newClaudeCodeAdapter(), c)
	same := 0
	var homeSame, repoSame bool
	for _, s := range estimate.Skills {
		if s.Name == "same" {
			same++
			if strings.HasPrefix(s.Path, home) {
				homeSame = true
			}
			if strings.HasPrefix(s.Path, repo) {
				repoSame = true
			}
		}
	}
	if same != 2 || !homeSame || !repoSame {
		t.Fatalf("same %d home %v repo %v", same, homeSame, repoSame)
	}
	if estimate.ManualOnlyCount < 1 {
		t.Fatal("manual")
	}
	if estimate.NameOnlyCount < 1 {
		t.Fatal("name-only")
	}
	for _, s := range estimate.Skills {
		if s.Name == "manual-claude" && s.Visibility != VisibilityManualOnly {
			t.Fatal("manual-claude visibility")
		}
	}
}

// opencode_ignores_invocation_extension_but_honors_deny_permission
func TestOpencodeIgnoresInvocationExtensionButHonorsDenyPermission(t *testing.T) {
	temp := t.TempDir()
	home := filepath.Join(temp, "home")
	repo := filepath.Join(temp, "repo")
	writeFile(t, filepath.Join(repo, ".git/HEAD"), "")
	writeSkill(t, filepath.Join(repo, ".opencode/skills"), "still-visible",
		"disable-model-invocation: true\n")
	writeSkill(t, filepath.Join(repo, ".opencode/skills"), "hidden", "")
	writeFile(t, filepath.Join(repo, "opencode.jsonc"),
		"{ // comment\n \"permission\": { \"skill\": { \"hidden\": \"deny\", }, },\n}")
	c := ctx(repo, home, repo)
	estimate := estimateWithPolicy(newOpenCodePolicy(), c)
	var visible *EffectiveSkill
	for i := range estimate.Skills {
		if estimate.Skills[i].Name == "still-visible" {
			visible = &estimate.Skills[i]
		}
	}
	if visible == nil || visible.Visibility != VisibilityAutomatic {
		t.Fatalf("still-visible %v", visible)
	}
	if hasSkill(estimate, "hidden") {
		t.Fatal("hidden should be denied")
	}
}

// opencode_uses_v2_ids_and_skills_array_sources
func TestOpencodeUsesV2IDsAndSkillsArraySources(t *testing.T) {
	temp := t.TempDir()
	home := filepath.Join(temp, "home")
	repo := filepath.Join(temp, "repo")
	writeFile(t, filepath.Join(repo, ".git/HEAD"), "")
	writeFile(t, filepath.Join(repo, "team-skills/release.md"),
		"---\nname: Release Guide\ndescription: Release workflow\nmetadata:\n  opencode:\n    autoinvoke: false\n---\nBody")
	writeFile(t, filepath.Join(repo, "opencode.jsonc"),
		`{"skills": ["./team-skills"]}`)
	c := ctx(repo, home, repo)
	estimate := estimateWithPolicy(newOpenCodePolicy(), c)
	var skill *EffectiveSkill
	for i := range estimate.Skills {
		if estimate.Skills[i].Name == "Release Guide" {
			skill = &estimate.Skills[i]
		}
	}
	if skill == nil {
		t.Fatalf("skills %v", skillNames(estimate))
	}
	if skill.ID != "release" {
		t.Fatalf("id %q", skill.ID)
	}
	if skill.Scope != ScopeRepository {
		t.Fatalf("scope %v", skill.Scope)
	}
	if skill.Visibility != VisibilityManualOnly {
		t.Fatalf("visibility %v", skill.Visibility)
	}
	if strings.Contains(newOpenCodePolicy().RenderVisibleMetadata(estimate.Skills), "<location>") {
		t.Fatal("should not render <location>")
	}
}

// opencode_v2_permissions_only_apply_skill_action_rules
func TestOpencodeV2PermissionsOnlyApplySkillActionRules(t *testing.T) {
	temp := t.TempDir()
	home := filepath.Join(temp, "home")
	repo := filepath.Join(temp, "repo")
	writeFile(t, filepath.Join(repo, ".git/HEAD"), "")
	writeSkill(t, filepath.Join(repo, ".opencode/skills"), "keep", "")
	writeFile(t, filepath.Join(repo, "opencode.json"),
		`{"permissions":[{"action":"edit","resource":"keep","effect":"deny"},{"action":"skill","resource":"keep","effect":"deny"}]}`)
	c := ctx(repo, home, repo)
	estimate := estimateWithPolicy(newOpenCodePolicy(), c)
	if hasSkill(estimate, "keep") {
		t.Fatal("keep should be denied")
	}
}

// openclaw_prefers_workspace_and_groups_enabled_plugin_skills
func TestOpenclawPrefersWorkspaceAndGroupsEnabledPluginSkills(t *testing.T) {
	temp := t.TempDir()
	home := filepath.Join(temp, "home")
	workspace := filepath.Join(temp, "workspace")
	state := filepath.Join(home, ".openclaw")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, filepath.Join(workspace, "skills"), "same", "")
	writeSkill(t, filepath.Join(home, ".agents/skills"), "same", "")
	writeFile(t, filepath.Join(state, "extensions/demo/openclaw.plugin.json"),
		`{"id":"demo","name":"Demo Plugin","skills":["skills"]}`)
	writeSkill(t, filepath.Join(state, "extensions/demo/skills"), "from-plugin", "")
	writeFile(t, filepath.Join(state, "openclaw.json"), `{"plugins":{"enabled":true}}`)
	c := ctx(workspace, home, "")
	estimate := estimateWithPolicy(newOpenClawPolicy(), c)
	same := 0
	var sameSkill *EffectiveSkill
	for i := range estimate.Skills {
		if estimate.Skills[i].Name == "same" {
			same++
			sameSkill = &estimate.Skills[i]
		}
	}
	if same != 1 || !strings.HasPrefix(sameSkill.Path, workspace) {
		t.Fatalf("same %d %v", same, sameSkill)
	}
	var plugin *EffectiveSkill
	for _, s := range estimate.PluginSkills() {
		if s.Name == "from-plugin" {
			plugin = s
		}
	}
	if plugin == nil {
		t.Fatal("from-plugin missing")
	}
	if id, ok := plugin.Source.PluginID(); !ok || id != "demo" {
		t.Fatalf("id %q", id)
	}
}

// hermes_uses_home_and_explicit_external_dirs_only
func TestHermesUsesHomeAndExplicitExternalDirsOnly(t *testing.T) {
	temp := t.TempDir()
	home := filepath.Join(temp, "home")
	hermes := filepath.Join(home, ".hermes")
	project := filepath.Join(temp, "project")
	writeSkill(t, filepath.Join(hermes, "skills"), "local", "")
	writeSkill(t, filepath.Join(hermes, "shared"), "external", "")
	writeSkill(t, filepath.Join(project, ".agents/skills"), "not-implicit", "")
	writeFile(t, filepath.Join(hermes, "config.yaml"),
		"skills:\n  external_dirs:\n    - shared\n")
	c := ctx(project, home, "")
	estimate := estimateWithPolicy(newHermesPolicy(), c)
	if !hasSkill(estimate, "local") || !hasSkill(estimate, "external") {
		t.Fatalf("skills %v", skillNames(estimate))
	}
	if hasSkill(estimate, "not-implicit") {
		t.Fatal("not-implicit should not appear")
	}
	if strings.Contains(newHermesPolicy().RenderVisibleMetadata(estimate.Skills), "SKILL.md") {
		t.Fatal("SKILL.md leaked into render")
	}
}
