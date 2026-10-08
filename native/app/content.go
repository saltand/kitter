// content.go ports KitterApp::content_snapshot plus the project
// snapshots the Installs tab reads, both fetched in goroutines and
// applied via App.apply (win.Update) with generation counters.
package app

import (
	"github.com/egoist/mygo/ui"
	"github.com/saltand/kitter/native/core/library"
	"github.com/saltand/kitter/native/core/model"
	"github.com/saltand/kitter/native/core/project"
)

// requestContentSnapshot is content_snapshot's lazy loader: spawn a file
// read off the UI thread; the generation counter drops stale results when
// the selection or file changed in the meantime.
func (a *App) requestContentSnapshot(skill *model.SkillSummary) {
	storageName := SkillStorageName(skill)
	file := a.Skills.SelectedFile
	if file == "" {
		file = "SKILL.md"
		a.Skills.SelectedFile = file
	}
	if a.Skills.contentSnapshot && a.Skills.ContentSkill == storageName && a.Skills.ContentFile == file {
		return // snapshot current
	}
	a.Skills.contentGen++
	gen := a.Skills.contentGen
	dataDir := a.Library.DataDir
	go func() {
		lib, err := library.OpenIn(dataDir)
		var files []string
		var content string
		if err == nil {
			files, _ = lib.FilesByStorage(storageName)
			content, err = lib.ReadFileByStorage(storageName, file)
		}
		a.apply(func() {
			if a.Skills.contentGen != gen {
				return // stale: selection moved on
			}
			a.Skills.contentSnapshot = true
			a.Skills.ContentSkill = storageName
			a.Skills.ContentFile = file
			a.Skills.ContentFiles = files
			if err != nil {
				a.Skills.ContentText = a.ErrorMessage(err)
			} else {
				a.Skills.ContentText = content
			}
		})
	}()
}

// projectSnapshot returns the cached snapshot for root, scheduling a
// fetch if missing (project_snapshot in mod.rs).
func (a *App) projectSnapshot(c *ui.Context, root string) []model.ProjectSkill {
	a.snapMu.Lock()
	defer a.snapMu.Unlock()
	if snap, ok := a.projectSnapshots[root]; ok {
		return snap
	}
	if a.projectTasks[root] {
		return nil
	}
	a.projectTasks[root] = true
	gen := a.projectsGen
	libraryDir := a.Library.Config.LibraryDir
	go func() {
		skills, _ := project.List(root, libraryDir)
		a.apply(func() {
			a.snapMu.Lock()
			defer a.snapMu.Unlock()
			delete(a.projectTasks, root)
			if gen != a.projectsGen {
				return
			}
			a.projectSnapshots[root] = skills
		})
	}()
	return nil
}

// requestProjectSnapshots is KitterApp::request_project_snapshots.
func (a *App) requestProjectSnapshots(c *ui.Context, roots []string) {
	for _, root := range roots {
		a.projectSnapshot(c, root)
	}
}

// skillInstallationsAt is KitterApp::skill_installations_at.
func (a *App) skillInstallationsAt(c *ui.Context, skill *model.SkillSummary, root string) []model.ProjectSkillInstallation {
	snapshot := a.projectSnapshot(c, root)
	var out []model.ProjectSkillInstallation
	for _, installed := range snapshot {
		if installed.Name != skill.Record.Name {
			continue
		}
		for _, inst := range installed.Installations {
			if sameFile(inst.Path, skill.Path) {
				out = append(out, inst)
			}
		}
	}
	return out
}

// skillInstalledAt is KitterApp::skill_installed_at.
func (a *App) skillInstalledAt(c *ui.Context, skill *model.SkillSummary, root string) bool {
	return len(a.skillInstallationsAt(c, skill, root)) > 0
}
