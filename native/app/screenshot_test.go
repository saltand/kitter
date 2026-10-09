// screenshot_test.go renders the main pages and dialogs to PNGs for
// visual review. It is skipped unless KITTER_SHOTS names an output
// directory; text needs Pango and fonts, see scripts/ui-shots.sh.
package app

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/saltand/kitter/native/core/model"
)

func TestScreenshots(t *testing.T) {
	out := os.Getenv("KITTER_SHOTS")
	if out == "" {
		t.Skip("set KITTER_SHOTS to an output directory")
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, dark := range []bool{false, true} {
		suffix := "light"
		if dark {
			suffix = "dark"
		}
		t.Run(suffix, func(t *testing.T) { renderScenes(t, out, suffix, dark) })
	}
}

func renderScenes(t *testing.T, out, suffix string, dark bool) {
	// Global installs land under HOME; keep them in the sandbox.
	t.Setenv("HOME", t.TempDir())
	app := newTestApp(t)
	app.SetDark(dark)
	tt := ui.NewTester(app.View, 1200, 720)
	tt.SetDark(dark)
	shot := func(name string) {
		t.Helper()
		// Async loads queue applies that the next frame shows and that
		// may queue more; settle twice.
		for i := 0; i < 2; i++ {
			waitIdle(t, app)
			tt.Frame()
		}
		f, err := os.Create(filepath.Join(out, name+"-"+suffix+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}

	// Library: a few skills, one group, one tag, one with installs.
	demo := importSkill(t, app, "frontend-design", "Create distinctive, production-grade frontend interfaces with high design quality.")
	importSkill(t, app, "pdf", "Extract text and tables from PDF files, fill forms, merge documents.")
	importSkill(t, app, "webapp-testing", "Toolkit for testing local web applications with Playwright.")
	addSkillFile(t, app, "frontend-design", "references/guide.md", "# Guide\n\nUse bold typography.\n")
	if group, err := app.Library.CreateGroup("Writing"); err == nil {
		pdf := findSkill(t, app, "pdf")
		app.GroupsFlow.MoveSkills = []string{SkillStorageName(pdf)}
		app.moveSelectedSkillToGroup(&group.ID)
	}
	if tagID, err := app.SkillTags.Add("web", nil); err == nil {
		app.TagsFlow.AssignmentKeys = []string{SkillStorageName(&demo)}
		app.TagsFlow.Scope = TagScopeSkills
		app.toggleAssignTag(tagID)
	}

	proj := filepath.Join(t.TempDir(), "my-project")
	fixtureSkill(t, filepath.Join(proj, ".claude", "skills", "claude-only"), "claude-only", "A project skill for Claude Code.")
	fixtureSkill(t, filepath.Join(proj, ".agents", "skills", "shared-skill"), "shared-skill", "A skill shared by agents.")

	storage := SkillStorageName(findSkill(t, app, "frontend-design"))
	app.Skills.Selection.Replace([]string{storage})
	app.InstallFlow.Global = true
	app.InstallFlow.SelectedTargets = map[model.InstallTarget]bool{model.TargetClaudeCode: true, model.TargetCodex: true}
	app.installSelected(nil)
	app.Projects.OpenProject = proj
	app.InstallFlow.Global = false
	app.installSelected(nil)
	app.InstallFlow.Modal = false
	app.ReloadSkills()

	app.Page = PageSkills
	app.Skills.Selection.Replace([]string{storage})
	app.Skills.Tab = DetailInstalls
	shot("01-skills-installs")

	if err := tt.Click(app.T("内容", "Content")); err == nil {
		waitIdle(t, app)
		tt.Frame()
		if tt.Click("guide.md") != nil {
			tt.Click("references/guide.md")
		}
	}
	shot("02-skills-content")
	app.Skills.Tab = DetailInstalls

	app.Skills.Selection.Replace([]string{storage, SkillStorageName(findSkill(t, app, "webapp-testing"))})
	shot("03-skills-multi")
	app.Skills.Selection.Replace([]string{storage})

	app.openInstallDialog()
	shot("04-install-dialog")
	app.InstallFlow.Modal = false

	app.openAddModal(AddLocal)
	shot("05-add-dialog")
	app.closeAddModal()

	app.openTagDialog(TagScopeSkills)
	shot("06-tags-dialog")
	app.TagsFlow.Open = false

	app.openGroupDialog()
	shot("07-groups-dialog")
	app.GroupsFlow.Open = false

	app.Page = PageProjects
	app.Projects.OpenProject = ""
	shot("08-projects-empty")
	app.selectProject(proj)
	shot("09-projects-skills")

	app.Page = PageSettings
	shot("10-settings")
}
