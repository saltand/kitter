// organize_flows_test.go covers the M5 organize dialogs: tag CRUD,
// tag assignment + filter, group CRUD + move, and language switch.
package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/saltand/kitter/native/core/config"
	"github.com/saltand/kitter/native/core/tags"
)

// TestTagCreateRenameDelete covers the manager's create root, rename,
// and delete paths via the real actions.
func TestTagCreateRenameDelete(t *testing.T) {
	app := newTestApp(t)
	app.openTagDialog(TagScopeSkills)

	// Create.
	app.startTagEdit(TagEdit{Kind: TagEditCreateRoot})
	app.TagsFlow.NameInput = "work"
	app.commitTagEdit()
	if app.TagsFlow.Edit != nil {
		t.Fatalf("edit not committed: %s", app.TagsFlow.Error)
	}
	var work tags.TagID
	for _, tag := range app.SkillTags.TagsList() {
		if tag.Name == "work" {
			work = tag.ID
		}
	}
	if work == 0 {
		t.Fatal("tag 'work' not created")
	}

	// Rename.
	app.startTagEdit(TagEdit{Kind: TagEditRename, ID: work})
	if app.TagsFlow.NameInput != "work" {
		t.Fatalf("rename input %q", app.TagsFlow.NameInput)
	}
	app.TagsFlow.NameInput = "office"
	app.commitTagEdit()
	if got := app.SkillTags.GetTag(work).Name; got != "office" {
		t.Fatalf("renamed to %q", got)
	}

	// Delete.
	app.deleteTag(work)
	if app.SkillTags.GetTag(work) != nil {
		t.Fatal("tag not deleted")
	}
}

// TestTagChildLimit verifies the one-level nesting rule.
func TestTagChildLimit(t *testing.T) {
	app := newTestApp(t)
	root, err := app.SkillTags.Add("parent", nil)
	if err != nil {
		t.Fatal(err)
	}
	child, err := app.SkillTags.Add("child", &root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.SkillTags.Add("grandchild", &child); err == nil {
		t.Fatal("grandchild must be rejected")
	}
}

// TestAssignTagAndFilter assigns a tag to a skill then filters the list.
func TestAssignTagAndFilter(t *testing.T) {
	app := newTestApp(t)
	skill := findSkill(t, app, "kitter")
	storage := SkillStorageName(skill)

	tagID, err := app.SkillTags.Add("mine", nil)
	if err != nil {
		t.Fatal(err)
	}
	app.TagsFlow.AssignmentKeys = []string{storage}
	app.TagsFlow.Scope = TagScopeSkills
	app.toggleAssignTag(tagID)

	if !app.SkillTags.IsAssigned(storage, tagID) {
		t.Fatal("tag not assigned")
	}

	// Filter the list by the tag — the skill still matches.
	app.HasTagFilter = true
	app.SelectedTagFilter = tagID
	visible := 0
	for i := range app.Skills.Items {
		if app.SkillTags.MatchesFilter(SkillStorageName(&app.Skills.Items[i]), tagID) {
			visible++
		}
	}
	if visible != 1 {
		t.Fatalf("filtered %d skills, want 1", visible)
	}
}

// TestGroupCreateMoveDelete creates a group, moves the skill in, and
// deletes it both ways.
func TestGroupCreateMoveDelete(t *testing.T) {
	app := newTestApp(t)
	skill := importSkill(t, app, "grp-skill", "demo")
	storage := SkillStorageName(&skill)

	group, err := app.Library.CreateGroup("alpha")
	if err != nil {
		t.Fatal(err)
	}
	app.GroupsFlow.MoveSkills = []string{storage}
	app.moveSelectedSkillToGroup(&group.ID)
	if got := findSkill(t, app, "grp-skill").Record.GroupID; got == nil || *got != group.ID {
		t.Fatalf("skill group %v", got)
	}

	// Delete without skills: the skill falls back to ungrouped.
	app.deleteSkillGroup(group.ID, false)
	if got := findSkill(t, app, "grp-skill").Record.GroupID; got != nil {
		t.Fatalf("group %v should be ungrouped", got)
	}
}

// TestGroupDeleteWithSkills covers delete_skills=true.
func TestGroupDeleteWithSkills(t *testing.T) {
	app := newTestApp(t)
	importSkill(t, app, "del-skill", "demo")
	group, err := app.Library.CreateGroup("beta")
	if err != nil {
		t.Fatal(err)
	}
	storage := SkillStorageName(findSkill(t, app, "del-skill"))
	app.GroupsFlow.MoveSkills = []string{storage}
	app.moveSelectedSkillToGroup(&group.ID)

	app.deleteSkillGroup(group.ID, true)
	if skillPresent(app, "del-skill") {
		t.Fatal("skill should be deleted with the group")
	}
}

// TestLanguageSwitchPersists verifies set_language writes config.json
// and the UI text follows.
func TestLanguageSwitchPersists(t *testing.T) {
	app := newTestApp(t)
	app.setLanguage(config.LanguageEn)
	tt := ui.NewTester(app.View, 1200, 720)
	if !tt.HasText("Settings") {
		t.Fatalf("english sidebar missing: %q", tt.Texts())
	}
	raw, err := os.ReadFile(filepath.Join(app.Library.DataDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !contains(raw, []byte(`"language":"en"`)) && !contains(raw, []byte(`"language": "en"`)) {
		t.Fatalf("config.json missing language=en: %s", raw)
	}
	app.setLanguage(config.LanguageZhCn)
	tt.Frame()
	if !tt.HasText("设置") {
		t.Fatalf("chinese sidebar missing: %q", tt.Texts())
	}
}

// TestCheckUpdatesShowsBadge renders the settings update row while
// the count flag is set — the row turns into an "Updates (N)" badge.
func TestCheckUpdatesShowsBadge(t *testing.T) {
	app := newTestApp(t)
	app.UpdateCount = 2
	app.Page = PageSettings
	tt := ui.NewTester(app.View, 1200, 720)
	if !tt.HasText("Updates (2)") && !tt.HasText("可更新 (2)") {
		t.Fatalf("update badge missing: %q", tt.Texts())
	}
}

// contains is a small []byte.Contains wrapper.
func contains(haystack, needle []byte) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if string(haystack[i:i+len(needle)]) == string(needle) {
			return true
		}
	}
	return false
}
