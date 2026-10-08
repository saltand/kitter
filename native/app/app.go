// Package app is the root of the MyGo native UI, mirroring src/ui.
//
// Concurrency: all state lives on the main thread — the view builds there
// and event handlers run there. Work spawned in goroutines applies its
// results through App.apply, which funnels them into win.Update (main
// thread) in the real app, so no mutex guards view-read state.
package app

import (
	"os"
	"sync"
	"time"

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

// DetailTab is ui::DetailTab.
type DetailTab int

const (
	DetailInstalls DetailTab = iota
	DetailContent
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
	splitSize       float32
	skillList       ui.ListState

	// Projects-state slice (ui::ProjectsState): per-root skill snapshots.
	// snapMu guards only the fields written by background goroutines
	// through apply(); nothing else locks. In the real app apply already
	// serializes onto the main thread, so the mutex is a no-op there.
	snapMu           sync.Mutex
	projectsGen      uint64
	projectSnapshots map[string][]model.ProjectSkill
	projectTasks     map[string]bool
}

// SkillsState is SkillsState in ui/state.rs plus the model list.
type SkillsState struct {
	Items   []model.SkillSummary
	Search  string
	Err     error
	Loading bool

	// Selection (ui/skill_selection.rs).
	Selection SkillSelection

	// Detail pane.
	Tab           DetailTab
	SelectedFile  string
	ExpandedSkill string // expanded_description: record name when expanded
	ContentScroll ui.ScrollState

	// Collapsed sets.
	CollapsedGroups      map[string]bool // group ids, persisted to config
	CollapsedContentDirs map[string]bool // relative dir paths

	// Content tab snapshot (ContentSnapshot in mod.rs). Loaded async.
	ContentFiles    []string
	ContentText     string
	ContentSkill    string // storage name the snapshot is for
	ContentFile     string // file the snapshot is for
	contentGen      uint64 // generation counter dropping stale loads
	contentSnapshot bool   // whether the snapshot is populated

	// Delete confirmation (DeleteConfirmation::LibrarySkills subset).
	DeleteSkills []model.SkillSummary
	DeleteOpen   bool
	DeleteBusy   bool

	// DeleteConfirmation::ProjectSkill.
	DeleteProject      string
	DeleteProjectSkill *model.ProjectSkill
	DeleteSelected     map[string]bool
}

// NewApp opens the library under dir and returns the app model.
func NewApp(dir string) (*App, error) {
	lib, err := library.OpenIn(dir)
	if err != nil {
		return nil, err
	}
	skillTags, _ := tags.LoadTagStatesFrom(lib.DataDir)
	a := &App{
		Library:          lib,
		SkillTags:        skillTags,
		Page:             PageSkills,
		sidebarSelected:  string(PageSkills),
		splitSize:        320,
		projectSnapshots: map[string][]model.ProjectSkill{},
		projectTasks:     map[string]bool{},
	}
	a.Skills.SelectedFile = "SKILL.md"
	a.Skills.CollapsedGroups = map[string]bool{}
	for id := range lib.Config.CollapsedSkillGroups {
		a.Skills.CollapsedGroups[id] = true
	}
	a.Skills.CollapsedContentDirs = map[string]bool{}
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

// ReloadSkills refreshes the skills list from the library (refresh()).
func (a *App) ReloadSkills() {
	a.Skills.Items, a.Skills.Err = a.Library.List()
	a.Skills.Loading = false
	a.Skills.ContentSkill = ""
	a.Skills.contentSnapshot = false
	a.projectsGen++
	a.projectSnapshots = map[string][]model.ProjectSkill{}
	a.projectTasks = map[string]bool{}
	a.Skills.Selection.Reconcile(a.skillOrder())
}

// skillOrder is the storage-name order selection semantics use.
func (a *App) skillOrder() []string {
	order := make([]string, 0, len(a.Skills.Items))
	for _, skill := range a.Skills.Items {
		order = append(order, SkillStorageName(&skill))
	}
	return order
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

// showNotice is KitterApp::show_notice (toast; mygo auto-dismisses).
func (a *App) showNotice(c *ui.Context, message string) {
	c.Toast(message)
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

// tick is a clock for staleness comparisons where Rust uses Instant.
func tick() uint64 { return uint64(time.Now().UnixNano()) }

var _ = tick
