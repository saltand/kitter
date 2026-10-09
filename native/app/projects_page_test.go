// projects_page_test.go covers the M4b projects page + install flow.
// UI tests assert only text/state (no pixels) since the test runner has
// no system fonts.
package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/saltand/kitter/native/core/effective"
	"github.com/saltand/kitter/native/core/model"
)

// TestAddProjectPersistsToConfig is browse_project → remember_project:
// picking a directory stores it in recent_projects and selects it.
func TestAddProjectPersistsToConfig(t *testing.T) {
	app := newTestApp(t)
	proj := t.TempDir()

	old := PickDirectory
	PickDirectory = func(string) ([]string, error) { return []string{proj}, nil }
	defer func() { PickDirectory = old }()

	app.browseProject()
	waitIdle(t, app)

	if app.Projects.OpenProject != proj {
		t.Fatalf("open_project %q, want %q", app.Projects.OpenProject, proj)
	}
	paths := app.Library.Config.ProjectPaths()
	if len(paths) == 0 || paths[0] != proj {
		t.Fatalf("project_paths %v", paths)
	}
	// config.json on disk has the project.
	raw, err := os.ReadFile(filepath.Join(app.Library.DataDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		RecentProjects []string `json:"recent_projects"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.RecentProjects) == 0 || doc.RecentProjects[0] != proj {
		t.Fatalf("recent_projects %v", doc.RecentProjects)
	}
}

// TestProjectViewEffectiveSkills is project_view's skills-tab content:
// a Claude Code + a Codex skill under .claude/skills + .agents/skills
// are discovered by the effective-skills scan and shown.
func TestProjectViewEffectiveSkills(t *testing.T) {
	app := newTestApp(t)
	proj := t.TempDir()
	// Project-local skills for claude-code + codex targets.
	fixtureSkill(t, filepath.Join(proj, ".claude", "skills", "claude-skill"), "claude-skill", "a claude skill")
	fixtureSkill(t, filepath.Join(proj, ".agents", "skills", "codex-skill"), "codex-skill", "a codex skill")

	app.Page = PageProjects
	app.selectProject(proj)
	tt := ui.NewTester(app.View, 1200, 720)
	waitIdle(t, app)
	// Flush a frame so the scanned rows render.
	tt.Frame()

	if !tt.HasText("claude-skill") {
		t.Fatalf("claude-skill missing: %q", tt.Texts())
	}
	if !tt.HasText("codex-skill") {
		t.Fatalf("codex-skill missing: %q", tt.Texts())
	}
	// Estimate panel shows agent cards.
	if !tt.HasText("Claude Code") {
		t.Fatalf("claude-code card missing: %q", tt.Texts())
	}
	if !tt.HasText("Codex") {
		t.Fatalf("codex card missing: %q", tt.Texts())
	}
	// Token estimate text appears on at least one card.
	found := false
	for _, text := range tt.Texts() {
		if strings.HasPrefix(text, "≈") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no ≈ token card: %q", tt.Texts())
	}
}

// TestInstallFlowCreatesLinks is install_selected: install a skill to
// two targets, the links exist in the project tree.
func TestInstallFlowCreatesLinks(t *testing.T) {
	app := newTestApp(t)
	proj := t.TempDir()

	// A library skill to install.
	src := filepath.Join(app.Library.DataDir, "..", "src")
	fixtureSkill(t, src, "demo-install", "demo")
	if err := app.Library.Import(src, model.SkillRecord{
		Name:   "demo-install",
		Origin: model.OriginLocal(src, nil),
	}); err != nil {
		t.Fatal(err)
	}
	app.ReloadSkills()
	skill := findSkill(t, app, "demo-install")

	app.Projects.OpenProject = proj
	app.InstallFlow.Modal = true
	app.InstallFlow.Global = false
	app.InstallFlow.SelectedTargets = map[model.InstallTarget]bool{
		model.TargetClaudeCode: true,
		model.TargetCodex:      true,
	}
	app.Skills.Selection.Replace([]string{SkillStorageName(skill)})

	app.installSelected(nil)

	claude := filepath.Join(proj, ".claude", "skills", "demo-install")
	codex := filepath.Join(proj, ".codex", "skills", "demo-install")
	for _, link := range []string{claude, codex} {
		if _, err := os.Lstat(link); err != nil {
			t.Fatalf("link %s missing: %v", link, err)
		}
	}
	if app.InstallFlow.Modal {
		t.Fatal("modal should close")
	}
}

// TestUninstallRemovesLinks: after install_selected, removing the
// project skill removes both links.
func TestUninstallRemovesLinks(t *testing.T) {
	app := newTestApp(t)
	proj := t.TempDir()
	src := filepath.Join(app.Library.DataDir, "..", "src2")
	fixtureSkill(t, src, "demo-uninstall", "demo")
	if err := app.Library.Import(src, model.SkillRecord{
		Name:   "demo-uninstall",
		Origin: model.OriginLocal(src, nil),
	}); err != nil {
		t.Fatal(err)
	}
	app.ReloadSkills()
	skill := findSkill(t, app, "demo-uninstall")

	app.Projects.OpenProject = proj
	app.InstallFlow.Modal = true
	app.InstallFlow.Global = false
	app.InstallFlow.SelectedTargets = map[model.InstallTarget]bool{
		model.TargetClaudeCode: true,
		model.TargetCodex:      true,
	}
	app.Skills.Selection.Replace([]string{SkillStorageName(skill)})
	app.installSelected(nil)

	claude := filepath.Join(proj, ".claude", "skills", "demo-uninstall")
	codex := filepath.Join(proj, ".codex", "skills", "demo-uninstall")

	// Delete every installation through the project-delete flow.
	app.projectSnapshot(nil, proj)
	waitIdle(t, app)
	installed := app.projectSnapshot(nil, proj)
	var skillRow model.ProjectSkill
	for _, s := range installed {
		if s.Name == "demo-uninstall" {
			skillRow = s
		}
	}
	if len(skillRow.Installations) == 0 {
		t.Fatal("no installations in snapshot")
	}
	app.openProjectDelete(proj, skillRow)
	app.deleteProjectSkill(nil)
	waitIdle(t, app)

	for _, link := range []string{claude, codex} {
		if _, err := os.Lstat(link); !os.IsNotExist(err) {
			t.Fatalf("link %s should be removed (err=%v)", link, err)
		}
	}
}

// TestStaleScanDoesNotClobberNewProject drives the real user path:
// open p1 (scan blocks), switch to p2 (scan completes first), then let
// p1's late result land. The detail pane must show p2's data, never
// p1's — the per-path snapshot cache keeps them separate.
func TestStaleScanDoesNotClobberNewProject(t *testing.T) {
	app := newTestApp(t)
	p1 := t.TempDir()
	p2 := t.TempDir()

	type gate struct{ started, release chan struct{} }
	gates := map[string]*gate{
		p1: {make(chan struct{}), make(chan struct{})},
		p2: {make(chan struct{}), make(chan struct{})},
	}
	old := projectListFn
	defer func() { projectListFn = old }()
	projectListFn = func(path, lib string) ([]model.ProjectSkill, error) {
		g := gates[path]
		if g == nil {
			return nil, nil
		}
		close(g.started)
		<-g.release
		return []model.ProjectSkill{{Name: "skill-" + filepath.Base(path)}}, nil
	}

	app.Page = PageProjects
	// The detail pane's skill names come from the effective-skill
	// estimate, not the snapshot — stub it empty so only sidebar counts
	// and the cache are observable.
	oldEst := estimateProjectFn
	defer func() { estimateProjectFn = oldEst }()
	estimateProjectFn = func(string) []effective.AgentContextEstimate { return nil }

	app.Library.Config.RememberProject(p1)
	app.Library.Config.RememberProject(p2)
	app.selectProject(p1)
	tt := ui.NewTester(app.View, 1200, 720)
	app.requestProjectSnapshots(nil, []string{p1})
	<-gates[p1].started // p1's scan is now blocked inside projectListFn

	// Switch to p2 through the real action; its scan must finish first.
	app.selectProject(p2)
	tt.Frame()
	<-gates[p2].started
	close(gates[p2].release)
	// p1 is still blocked; let it finish too so waitIdle can drain,
	// then render. p2's data was already committed first.
	close(gates[p1].release)
	waitIdle(t, app)
	tt.Frame()

	// p2's snapshot lives in its own cache slot; the open project sees
	// only that entry.
	if got := len(app.projectSnapshots[p2]); got != 1 {
		t.Fatalf("p2 snapshot %d, want 1", got)
	}
	if app.projectSnapshots[p2][0].Name != "skill-"+filepath.Base(p2) {
		t.Fatalf("p2 snapshot = %q", app.projectSnapshots[p2][0].Name)
	}
	// p1's late result still landed — but only under p1's key, so the
	// p2 detail never reads it.
	if len(app.projectSnapshots[p1]) == 0 {
		t.Fatal("p1 snapshot should still be cached under p1")
	}
	if app.projectSnapshots[p1][0].Name != "skill-"+filepath.Base(p1) {
		t.Fatalf("p1 snapshot = %q", app.projectSnapshots[p1][0].Name)
	}
	_ = tt
}

// TestStaleEstimateDoesNotClobberNewProject is the same ordering test
// for context_estimate_snapshot: p1's slow estimate must not render
// into p2's agent cards.
func TestStaleEstimateDoesNotClobberNewProject(t *testing.T) {
	app := newTestApp(t)
	p1 := t.TempDir()
	p2 := t.TempDir()

	type gate struct{ started, release chan struct{} }
	gates := map[string]*gate{
		p1: {make(chan struct{}), make(chan struct{})},
		p2: {make(chan struct{}), make(chan struct{})},
	}
	old := estimateProjectFn
	defer func() { estimateProjectFn = old }()
	estimateProjectFn = func(path string) []effective.AgentContextEstimate {
		g := gates[path]
		if g == nil {
			return nil
		}
		close(g.started)
		<-g.release
		return []effective.AgentContextEstimate{{
			Agent:             effective.AgentClaudeCode,
			EstimatedTokens:   42,
			ModelVisibleCount: 1,
			Skills: []effective.EffectiveSkill{{
				Name: "skill-" + filepath.Base(path),
			}},
		}}
	}

	app.Page = PageProjects
	app.selectProject(p1)
	tt := ui.NewTester(app.View, 1200, 720)
	// Trigger the estimate request for p1 (the detail pane calls it in
	// projectDetail).
	app.contextEstimateSnapshot(p1)
	<-gates[p1].started

	app.selectProject(p2)
	tt.Frame()
	<-gates[p2].started
	close(gates[p2].release)
	// Release p1's late estimate too, then drain and render — the
	// detail pane must only ever show p2's card data.
	close(gates[p1].release)
	waitIdle(t, app)
	tt.Frame()
	if !tt.HasText("skill-" + filepath.Base(p2)) {
		t.Fatalf("p2 estimate missing: %q", tt.Texts())
	}
	if tt.HasText("skill-" + filepath.Base(p1)) {
		t.Fatalf("stale p1 estimate rendered for p2: %q", tt.Texts())
	}
}
