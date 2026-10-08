// add_flow.go ports the AddSkill flow state and actions from
// src/ui/state.rs (AddFlowState), add_actions.rs (scanning, importing,
// adopting, cancelling) and mod.rs (AddKind/AddTask). Scanning and
// adopting run in goroutines and deliver through App.Apply, guarded by
// a generation counter; cancellation uses context.Context.
package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/saltand/kitter/native/core/adoption"
	"github.com/saltand/kitter/native/core/library"
	"github.com/saltand/kitter/native/core/source"
)

// AddKind is ui::AddKind.
type AddKind int

const (
	AddLocal AddKind = iota
	AddNpx
	AddClaude
	AddExisting
)

// AddTask is ui::AddTask.
type AddTask int

const (
	AddScanning AddTask = iota
	AddImporting
)

// AddFlowState is AddFlowState in state.rs.
type AddFlowState struct {
	Open           bool
	Kind           AddKind
	Task           *AddTask
	Scan           *source.SkillScan
	AdoptionScan   *adoption.AdoptionScan
	Selected       map[string]bool
	Error          string
	GroupEnabled   bool
	GroupName      string
	PrimaryInput   string
	AdoptionRoot   string
	AdoptionCancel context.CancelFunc
	scanGen        uint64
	adoptionGen    uint64
	adoptionRows     []adoptionRow
	adoptionExpanded map[string]bool
	adoptionList   ui.ListState
}

// PickDirectory is the directory-chooser seam: mygo.Dialog.Open in the
// app, a stub in tests. It returns chosen paths (nil when cancelled).
var PickDirectory = func(title string) ([]string, error) {
	return mygo.Dialog.Open(mygo.OpenDialogOptions{
		Title:     title,
		Directory: true,
	})
}

// busy reports whether an add-flow task is running (is_scanning ||
// is_importing in Rust).
func (a *App) addBusy() bool { return a.AddFlow.Task != nil }

// openAddModal opens the dialog (set_add_kind defaulting to Local from
// the Add button).
func (a *App) openAddModal(kind AddKind) {
	a.AddFlow = AddFlowState{
		Open:         true,
		Kind:         kind,
		Selected:     map[string]bool{},
		GroupEnabled: true,
		adoptionList: ui.ListState{},
	}
}

// closeAddModal is close_dialog for the add flow: it cancels any running
// scan/adopt so a later apply() becomes a no-op.
func (a *App) closeAddModal() {
	a.AddFlow.scanGen++
	a.AddFlow.adoptionGen++
	if a.AddFlow.AdoptionCancel != nil {
		a.AddFlow.AdoptionCancel()
		a.AddFlow.AdoptionCancel = nil
	}
	a.AddFlow = AddFlowState{}
}

// setAddKind switches source kind, resetting per-kind inputs.
func (a *App) setAddKind(kind AddKind) {
	a.AddFlow.Kind = kind
	a.AddFlow.Scan = nil
	a.AddFlow.AdoptionScan = nil
	a.AddFlow.Selected = map[string]bool{}
	a.AddFlow.Error = ""
	a.AddFlow.PrimaryInput = ""
	a.AddFlow.adoptionRows = nil
}

// scanLocalFolder is scan_local_folder.
func (a *App) scanLocalFolder(path string) {
	if a.addBusy() {
		return
	}
	task := AddScanning
	a.AddFlow.Task = &task
	a.AddFlow.Scan = nil
	a.AddFlow.Selected = map[string]bool{}
	a.AddFlow.Error = ""
	a.AddFlow.GroupEnabled = true
	a.AddFlow.scanGen++
	gen := a.AddFlow.scanGen
	go func() {
		scan, err := source.ScanLocal(path)
		a.Apply(func() {
			if a.AddFlow.scanGen != gen {
				return
			}
			a.AddFlow.Task = nil
			if err != nil {
				a.AddFlow.Error = a.ErrorMessage(err)
				return
			}
			a.AddFlow.GroupName = scan.DefaultGroupName()
			a.AddFlow.Scan = scan
		})
	}()
}

// scanAddSource is scan_add_source (Npx/Claude/Existing).
func (a *App) scanAddSource() {
	if a.AddFlow.Kind == AddExisting {
		a.scanExistingSkills()
		return
	}
	input := strings.TrimSpace(a.AddFlow.PrimaryInput)
	if a.addBusy() || input == "" {
		return
	}
	kind := a.AddFlow.Kind
	task := AddScanning
	a.AddFlow.Task = &task
	a.AddFlow.Scan = nil
	a.AddFlow.Selected = map[string]bool{}
	a.AddFlow.Error = ""
	a.AddFlow.GroupEnabled = true
	a.AddFlow.scanGen++
	gen := a.AddFlow.scanGen
	go func() {
		var scan *source.SkillScan
		var err error
		switch kind {
		case AddNpx:
			scan, err = source.ScanNpx(context.Background(), input)
		case AddClaude:
			scan, err = source.ScanClaude(context.Background(), input)
		default:
			err = errors.New("请选择技能文件夹")
		}
		a.Apply(func() {
			if a.AddFlow.scanGen != gen {
				return
			}
			a.AddFlow.Task = nil
			if err != nil {
				a.AddFlow.Error = a.ErrorMessage(err)
				return
			}
			a.AddFlow.GroupName = scan.DefaultGroupName()
			a.AddFlow.Scan = scan
		})
	}()
}

// scanExistingSkills is scan_existing_skills.
func (a *App) scanExistingSkills() {
	if a.addBusy() {
		return
	}
	home := homeDir()
	if home == "" {
		return
	}
	var roots []string
	if a.AddFlow.AdoptionRoot != "" {
		roots = []string{a.AddFlow.AdoptionRoot}
	} else {
		roots = append([]string{home}, a.Library.Config.RecentProjects...)
	}
	libraryDir := a.Library.Config.LibraryDir
	managed := a.Skills.Items
	ctx, cancel := context.WithCancel(context.Background())
	a.AddFlow.AdoptionCancel = cancel
	a.AddFlow.AdoptionScan = nil
	a.AddFlow.Selected = map[string]bool{}
	a.AddFlow.adoptionRows = nil
	a.AddFlow.Error = ""
	task := AddScanning
	a.AddFlow.Task = &task
	a.AddFlow.adoptionGen++
	gen := a.AddFlow.adoptionGen
	go func() {
		scan, err := adoption.ScanRoots(ctx, home, roots, libraryDir, managed)
		a.Apply(func() {
			if a.AddFlow.adoptionGen != gen {
				cancel()
				return
			}
			a.AddFlow.AdoptionCancel = nil
			a.AddFlow.Task = nil
			if err != nil {
				a.AddFlow.Error = a.ErrorMessage(err)
				return
			}
			a.AddFlow.Selected = scan.DefaultSelection()
			if len(scan.Candidates) == 0 {
				a.AddFlow.Error = a.T("没有发现可托管的技能", "No skills available to adopt")
			}
			a.AddFlow.AdoptionScan = scan
			a.buildAdoptionRows()
		})
	}()
}

// browseAddLocal is browse_add_local.
func (a *App) browseAddLocal() {
	if a.addBusy() {
		return
	}
	go func() {
		paths, err := PickDirectory(a.T("选择", "Choose"))
		if err != nil || len(paths) == 0 {
			return
		}
		path := paths[0]
		a.Apply(func() {
			a.scanLocalFolder(path)
		})
	}()
}

// browseAdoptionRoot is browse_adoption_root.
func (a *App) browseAdoptionRoot() {
	if a.addBusy() {
		return
	}
	go func() {
		paths, err := PickDirectory(a.T("选择", "Choose"))
		if err != nil || len(paths) == 0 {
			return
		}
		path := paths[0]
		a.Apply(func() {
			a.AddFlow.AdoptionRoot = path
			a.AddFlow.AdoptionScan = nil
			a.AddFlow.Selected = map[string]bool{}
			a.AddFlow.adoptionRows = nil
		})
	}()
}

// importScannedSkills is import_scanned_skills (source scans). In
// Rust the import runs synchronously on the main thread while the
// modal blocks; in Go it must still run on the UI thread (inside
// Apply) because SkillLibrary is not internally synchronized — the
// skills page reads it every frame.
func (a *App) importScannedSkills() {
	if a.AddFlow.Kind == AddExisting {
		a.adoptSelectedSkills()
		return
	}
	if a.addBusy() || a.AddFlow.Scan == nil || len(a.AddFlow.Selected) == 0 {
		return
	}
	scan := a.AddFlow.Scan
	selected := a.AddFlow.Selected
	groupName := ""
	if a.AddFlow.GroupEnabled {
		groupName = a.AddFlow.GroupName
	}
	task := AddImporting
	a.AddFlow.Task = &task
	a.AddFlow.Error = ""
	go func() {
		// The scan snapshot is immutable; queue the library mutation for
		// the UI thread.
		a.Apply(func() {
			summary, err := scan.ImportSelected(a.Library, selected, groupName)
			a.AddFlow.Task = nil
			if err != nil {
				a.AddFlow.Error = a.ErrorMessage(err)
				return
			}
			a.ReloadSkills()
			a.closeAddModal()
			if a.UsesEnglish() {
				if summary.Skipped == 0 {
					a.notice(fmt.Sprintf("Added %d %s", summary.Added, plural(summary.Added)))
				} else {
					a.notice(fmt.Sprintf("Added %d %s; %d skipped", summary.Added, plural(summary.Added), summary.Skipped))
				}
			} else if summary.Skipped == 0 {
				a.notice(fmt.Sprintf("已添加 %d 个技能", summary.Added))
			} else {
				a.notice(fmt.Sprintf("已添加 %d 个，跳过 %d 个已存在", summary.Added, summary.Skipped))
			}
		})
	}()
}

// adoptSelectedSkills is adopt_selected_skills. lib is a fresh
// SkillLibrary (Rust's SkillLibrary::open_in(data_dir())), so the
// goroutine mutates nothing shared; only the post-adopt apply touches
// app state.
func (a *App) adoptSelectedSkills() {
	if a.addBusy() || len(a.AddFlow.Selected) == 0 || a.AddFlow.AdoptionScan == nil {
		return
	}
	scan := a.AddFlow.AdoptionScan
	selected := a.AddFlow.Selected
	dataDir := a.Library.DataDir
	task := AddImporting
	a.AddFlow.Task = &task
	a.AddFlow.Error = ""
	english := a.UsesEnglish()
	go func() {
		lib, err := library.OpenIn(dataDir)
		if err != nil {
			a.Apply(func() {
				a.AddFlow.Task = nil
				a.AddFlow.Error = a.ErrorMessage(err)
			})
			return
		}
		succeeded := map[string]bool{}
		adoptedStorages := map[string]string{} // identity -> storage name
		var failures []string
		for _, candidate := range scan.Candidates {
			if !selected[candidate.ID] {
				continue
			}
			err := func() error {
				for _, variant := range scan.Variants(candidate) {
					if err := variant.Verify(); err != nil {
						return err
					}
				}
				adopted, err := lib.Adopt(candidate, scan.ReferencesFor(candidate))
				if err == nil {
					adoptedStorages[candidate.Identity()] = adopted
				}
				return err
			}()
			if err == nil {
				succeeded[candidate.Identity()] = true
				continue
			}
			failures = append(failures, fmt.Sprintf("%s: %s", candidate.Name, ErrorMessageText(err.Error(), english)))
		}
		a.Apply(func() {
			a.AddFlow.Task = nil
			a.Library = lib
			a.ReloadSkills()
			if a.AddFlow.AdoptionScan != nil {
				a.AddFlow.AdoptionScan.Retain(func(c *adoption.AdoptionCandidate) bool {
					return !succeeded[c.Identity()]
				})
				for id := range a.AddFlow.Selected {
					if !a.AddFlow.AdoptionScan.ContainsID(id) {
						delete(a.AddFlow.Selected, id)
					}
				}
				a.buildAdoptionRows()
			}
			if len(failures) == 0 {
				a.closeAddModal()
				if len(succeeded) == 1 {
					for _, storage := range adoptedStorages {
						a.Skills.Selection.SelectOne(storage)
					}
				}
				if a.UsesEnglish() {
					a.notice(fmt.Sprintf("Adopted %d %s", len(succeeded), plural(len(succeeded))))
				} else {
					a.notice(fmt.Sprintf("已托管 %d 个技能", len(succeeded)))
				}
			} else {
				a.AddFlow.Error = strings.Join(failures, "\n")
			}
		})
	}()
}

func plural(n int) string {
	if n == 1 {
		return "skill"
	}
	return "skills"
}
