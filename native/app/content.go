// content.go ports KitterApp::content_snapshot plus the project
// snapshots the Installs tab reads. Each spawns a goroutine doing only
// pure reads (file list/read, project scan) — the library is never
// reopened off the UI thread, so no registry writes or builtin sync
// race with the main SkillLibrary — then applies the result through
// App.apply (win.Update) guarded by a generation counter.
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
	// Resolve the directory on the UI thread; the goroutine does only
	// read-only filesystem work so it cannot race the registry.
	root, err := a.Library.SkillPathByStorage(storageName)
	a.spawn(func() {
		var files []string
		var content string
		if err == nil {
			files, _ = library.FilesInDir(root)
			content, err = library.ReadFileInDir(root, file)
		}
		a.Apply(func() {
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
	})
}

// projectListFn is project::list (test seam).
var projectListFn = project.List

// projectSnapshot returns the cached snapshot for root, scheduling a
// fetch if missing (project_snapshot in mod.rs).
func (a *App) projectSnapshot(c *ui.Context, root string) []model.ProjectSkill {
	if snap, ok := a.projectSnapshots[root]; ok {
		return snap
	}
	if a.projectTasks[root] {
		return nil
	}
	a.projectTasks[root] = true
	gen := a.projectsGen
	libraryDir := a.Library.Config.LibraryDir
	a.spawn(func() {
		skills, _ := projectListFn(root, libraryDir)
		a.Apply(func() {
			delete(a.projectTasks, root)
			if gen != a.projectsGen {
				return
			}
			a.projectSnapshots[root] = skills
		})
	})
	return nil
}

// requestProjectSnapshots is KitterApp::request_project_snapshots.
// Rust spawns one detached task per pending path, so a slow project
// never blocks another project's scan.
func (a *App) requestProjectSnapshots(c *ui.Context, roots []string) {
	gen := a.projectsGen
	for _, root := range roots {
		if _, ok := a.projectSnapshots[root]; ok {
			continue
		}
		if a.projectTasks[root] {
			continue
		}
		a.projectTasks[root] = true
		path := root
		libraryDir := a.Library.Config.LibraryDir
		a.spawn(func() {
			snapshot, _ := projectListFn(path, libraryDir)
			a.Apply(func() {
				if a.projectsGen != gen || !a.projectTasks[path] {
					return
				}
				delete(a.projectTasks, path)
				a.projectSnapshots[path] = snapshot
			})
		})
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
