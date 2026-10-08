// Package app is the root of the MyGo native UI, mirroring src/ui.
package app

import (
	"sync"

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

// App is AppModel: all UI state lives here (ui::KitterApp).
type App struct {
	mu sync.Mutex // guards fields written by background goroutines

	Library *library.SkillLibrary

	// Language resolution: config.language == system follows the OS.
	languageOverride config.Language

	// dark mirrors mygo.Theme.IsDark() (or tt.SetDark in tests).
	dark bool

	Page     Page
	ShellWin *mygo.Window

	// Skills page state (M1: minimal list).
	Skills          SkillsState
	skillList       ui.ListState
	skillsSelected  int
	sidebarSelected string
}

// SkillsState is the slice of ui::state.rs the skills page needs.
type SkillsState struct {
	Items   []model.SkillSummary
	Search  string
	Err     error
	Loading bool
}

// NewApp opens the library under dir and returns the app model.
func NewApp(dir string) (*App, error) {
	lib, err := library.OpenIn(dir)
	if err != nil {
		return nil, err
	}
	a := &App{
		Library:         lib,
		Page:            PageSkills,
		sidebarSelected: string(PageSkills),
	}
	a.skillsSelected = -1
	a.skillList.Selected = &a.skillsSelected
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
func (a *App) Dark() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.dark
}

// SetDark updates the appearance flag (called from Theme.OnUpdated and
// once at startup).
func (a *App) SetDark(dark bool) {
	a.mu.Lock()
	a.dark = dark
	a.mu.Unlock()
}

// ReloadSkills refreshes the skills list from the library.
func (a *App) ReloadSkills() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Skills.Items, a.Skills.Err = a.Library.List()
	a.Skills.Loading = false
}

// SkillTagState loads the skills tag state on demand (M5 will keep it
// resident like Rust's TagsFlow).
func (a *App) SkillTagState() *tags.TagState {
	skills, _ := tags.LoadTagStatesFrom(a.Library.DataDir)
	return skills
}
