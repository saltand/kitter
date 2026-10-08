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

// TestStaleScanDoesNotClobberNewProject: a slow scan returning for a
// previous project is discarded by the generation guard after switching.
func TestStaleScanDoesNotClobberNewProject(t *testing.T) {
	app := newTestApp(t)
	p1 := t.TempDir()

	old := projectListFn
	defer func() { projectListFn = old }()
	release := make(chan string, 8)
	projectListFn = func(path, lib string) ([]model.ProjectSkill, error) {
		release <- path
		<-release // block until test lets each call finish
		return []model.ProjectSkill{{Name: "stale-" + filepath.Base(path)}}, nil
	}

	// Trigger a scan for p1 then immediately switch to p2 (bumping the
	// generation counter).
	app.requestProjectSnapshots(nil, []string{p1})
	started := <-release // the goroutine reached the scan for p1
	_ = started
	a := app
	a.projectsGen++ // invalidate as selectProject would
	// Now let the scan finish; the result must be discarded.
	release <- ""
	waitIdle(t, app)

	if _, ok := a.projectSnapshots[p1]; ok {
		t.Fatal("stale snapshot for p1 must be discarded")
	}
}
