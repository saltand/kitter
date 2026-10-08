package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/saltand/kitter/native/core/config"
	"github.com/saltand/kitter/native/core/model"
)

// The environment has no system fonts, so tests only assert texts and
// state, never pixels or sizes (docs §8).

func newTestApp(t *testing.T) *App {
	t.Helper()
	// Without the embedded JetBrains Mono the text system can't lay out
	// mono labels, which then have no box for the tester to click.
	if err := RegisterFonts(); err != nil {
		t.Logf("register fonts: %v", err)
	}
	dir := t.TempDir()
	t.Setenv("KITTER_HOME", dir)
	app, err := NewApp(dir)
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func fixtureSkill(t *testing.T, dir, name, description string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n"
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644)
}

func TestSidebarNavigationSwitchesPages(t *testing.T) {
	app := newTestApp(t)
	app.SetDark(false)
	tt := ui.NewTester(app.View, 1200, 720)

	// The default page is skills.
	if app.Page != PageSkills {
		t.Fatalf("page %s", app.Page)
	}
	if err := tt.Click(app.T("项目", "Projects")); err != nil {
		t.Fatal(err)
	}
	if app.Page != PageProjects {
		t.Fatalf("page %s", app.Page)
	}
	if !tt.HasText(app.T("还没有项目", "No projects yet")) {
		t.Fatalf("projects page not rendered: %q", tt.Texts())
	}
	if err := tt.Click(app.T("设置", "Settings")); err != nil {
		t.Fatal(err)
	}
	if app.Page != PageSettings {
		t.Fatalf("page %s", app.Page)
	}
	if !tt.HasText(app.T("技能库", "Skill library")) {
		t.Fatalf("settings page not rendered: %q", tt.Texts())
	}
}

func TestSkillsPageListsLibrary(t *testing.T) {
	app := newTestApp(t)
	// Import one extra skill next to the builtin.
	src := filepath.Join(app.Library.Config.LibraryDir, "..", "imported")
	fixtureSkill(t, src, "demo-skill", "a demo")
	if err := app.Library.Import(src, model.SkillRecord{
		Name:   "demo-skill",
		Origin: model.OriginLocal(src, nil),
	}); err != nil {
		t.Fatal(err)
	}
	app.ReloadSkills()
	tt := ui.NewTester(app.View, 1200, 720)
	if !tt.HasText("demo-skill") {
		t.Fatalf("skill not listed: %q", tt.Texts())
	}
	if !tt.HasText("kitter") {
		t.Fatalf("builtin skill not listed: %q", tt.Texts())
	}
}

func TestErrorMessageTableMatchesRust(t *testing.T) {
	// messages.rs::validation_and_source_errors_have_english_messages
	for _, pair := range messages {
		if got := ErrorMessageText(pair[0], true); got != pair[1] {
			t.Fatalf("%q → %q, want %q", pair[0], got, pair[1])
		}
		for _, r := range pair[1] {
			if r >= '一' && r <= '鿿' {
				t.Fatalf("english message has CJK: %q", pair[1])
			}
		}
	}
	if got := ErrorMessageText("标签名称不能包含 / ", true); got != "Tag names cannot contain /" {
		t.Fatal(got)
	}
}

func TestDiagnosticsDoNotLeakIntoUIErrors(t *testing.T) {
	if got := ErrorMessageText("Kitter 内置 Skill 不能删除", true); got != "This built-in skill cannot be deleted" {
		t.Fatal(got)
	}
	raw := "命令执行失败：internal command output /private/path"
	msg := ErrorMessageText(raw, true)
	if msg != "Could not fetch skills. Check the source and your connection, then try again" {
		t.Fatal(msg)
	}
	if got := ErrorMessageText("unknown backend failure", false); got != "操作未完成，请重试" {
		t.Fatal(got)
	}
}

func TestLanguageResolution(t *testing.T) {
	app := newTestApp(t)
	if app.UsesEnglish() != (config.SystemLanguage() == config.LanguageEn) {
		t.Fatal("system language not followed")
	}
	app.Library.Config.Language = config.LanguageEn
	if !app.UsesEnglish() {
		t.Fatal("english override")
	}
	app.Library.Config.Language = config.LanguageZhCn
	if app.UsesEnglish() {
		t.Fatal("chinese override")
	}
}
