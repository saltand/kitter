// Package app is the root of the MyGo native UI, mirroring src/ui.
//
// Concurrency: all state lives on the main thread — the view builds there
// and event handlers run there. Work spawned in goroutines applies its
// results through App.apply, which funnels them into win.Update (main
// thread) in the real app, so no mutex guards view state.
package app

import (
	"os"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/saltand/kitter/native/core/config"
	"github.com/saltand/kitter/native/core/library"
	"github.com/saltand/kitter/native/core/model"
	"github.com/saltand/kitter/native/core/tags"
)

// Page is the top-level navigation page (ui::Page in Rust).
type Page string

const (
	PageSkills   Page = "skills"
	PageProjects Page = "projects"
	PageSettings Page = "settings"
)

// App is KitterApp: all UI state lives here.
type App struct {
	Library *library.SkillLibrary

	// SkillTags mirrors tags_flow.skills (resident, loaded at startup).
	SkillTags *tags.TagState
	// selectedSkillFilter mirrors tags_flow.selected_skill_filter.
	SelectedTagFilter tags.TagID
	HasTagFilter      bool

	// Language resolution: config.language == system follows the OS.
	languageOverride config.Language

	// dark mirrors mygo.Theme.IsDark() (or tt.SetDark in tests).
	dark bool

	Page     Page
	ShellWin *mygo.Window

	// Skills page state (ui::state::SkillsState).
	Skills SkillsState

	sidebarSelected string
	skillList       ui.ListState
	skillsSelected  int
}

// SkillsState is SkillsState in ui/state.rs plus the model list.
type SkillsState struct {
	Items   []model.SkillSummary
	Search  string
	Err     error
	Loading bool

	// Collapsed groups, persisted to config (collapsed_skill_groups).
	CollapsedGroups map[string]bool
}

// NewApp opens the library under dir and returns the app model.
func NewApp(dir string) (*App, error) {
	lib, err := library.OpenIn(dir)
	if err != nil {
		return nil, err
	}
	skillTags, _ := tags.LoadTagStatesFrom(lib.DataDir)
	a := &App{
		Library:         lib,
		SkillTags:       skillTags,
		Page:            PageSkills,
		sidebarSelected: string(PageSkills),
	}
	a.skillsSelected = -1
	a.skillList.Selected = &a.skillsSelected
	a.Skills.CollapsedGroups = map[string]bool{}
	for id := range lib.Config.CollapsedSkillGroups {
		a.Skills.CollapsedGroups[id] = true
	}
	a.ReloadSkills()
	return a, nil
}

// UsesEnglish is KitterApp::uses_english.
func (a *App) UsesEnglish() bool {
	language := a.Library.Config.Language
	if a.languageOverride != "" {
		language = a.languageOverride
	}
	switch language {
	case config.LanguageEn:
		return true
	case config.LanguageZhCn:
		return false
	default:
		return config.SystemLanguage() == config.LanguageEn
	}
}

// Dark reports the current appearance. The main window sets it from
// mygo.Theme.IsDark(); tests may set it directly.
func (a *App) Dark() bool { return a.dark }

// SetDark updates the appearance flag (called from Theme.OnUpdated and
// once at startup).
func (a *App) SetDark(dark bool) { a.dark = dark }

// ReloadSkills refreshes the skills list from the library.
func (a *App) ReloadSkills() {
	a.Skills.Items, a.Skills.Err = a.Library.List()
	a.Skills.Loading = false
}

// apply runs fn on the UI thread: through the window's Update in the real
// app, inline in tests where ShellWin is nil.
func (a *App) apply(fn func()) {
	if a.ShellWin != nil {
		a.ShellWin.Update(fn)
	} else {
		fn()
	}
}

// homeDir mirrors dirs::home_dir ("" when unknown).
func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

// persistCollapsedGroups is KitterApp::persist_collapsed_groups
// (feat/persist-collapsed): copy the set into the config and save it.
func (a *App) persistCollapsedGroups() {
	set := map[string]bool{}
	for id := range a.Skills.CollapsedGroups {
		set[id] = true
	}
	a.Library.Config.CollapsedSkillGroups = set
	_ = a.Library.Config.SaveTo(a.Library.DataDir)
}

// persistTags persists the skills tag state (persist_tags).
func (a *App) persistTags() {
	_, projects := tags.LoadTagStatesFrom(a.Library.DataDir)
	_ = tags.SaveTagStatesTo(a.Library.DataDir, a.SkillTags, projects)
}
