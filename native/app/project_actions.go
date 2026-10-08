// project_actions.go ports src/ui/project_actions.rs and the
// snapshot/estimate scheduling from mod.rs: browse_project, remove,
// project_snapshot/context_estimate_snapshot + their generation
// guards, refresh_context_estimate.
package app

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/saltand/kitter/native/core/effective"
)

// estimateProjectFn is the estimate_project seam (tests inject fakes).
var estimateProjectFn = effective.EstimateProject

// browseProject is browse_project: directory picker, remember_project,
// select it (or, while the install modal is open, switch its target).
func (a *App) browseProject() {
	a.spawn(func() {
		paths, err := PickDirectory(a.T("选择", "Choose"))
		if err != nil || len(paths) == 0 {
			return
		}
		path := paths[0]
		a.Apply(func() {
			a.Projects.OpenProject = path
			if !a.InstallFlow.Modal {
				a.Projects.GlobalProjectView = false
				a.Projects.SelectedProjectAgent = nil
				a.Projects.ProjectAgentsExpanded = false
			}
			a.Library.Config.RememberProject(path)
			_ = a.Library.Save()
			if a.InstallFlow.Modal {
				a.InstallFlow.Global = homeDir() == path
			}
		})
	})
}

// selectProject opens a project in the detail pane.
func (a *App) selectProject(path string) {
	a.Projects.OpenProject = path
	a.Projects.GlobalProjectView = false
	a.Projects.SelectedProjectAgent = nil
	a.Projects.ProjectAgentsExpanded = false
}

// selectGlobalProject selects the Global view.
func (a *App) selectGlobalProject() {
	a.Projects.GlobalProjectView = true
	a.Projects.SelectedProjectAgent = nil
	a.Projects.ProjectAgentsExpanded = false
}

// removeProject is the context-menu remove action.
func (a *App) removeProject(c *ui.Context, path string) {
	a.Library.Config.RemoveProject(path)
	if a.Projects.OpenProject == path {
		a.Projects.OpenProject = ""
		a.Projects.GlobalProjectView = true
	}
	_ = a.Library.Save()
}

// contextEstimateSnapshot is context_estimate_snapshot.
func (a *App) contextEstimateSnapshot(path string) []effective.AgentContextEstimate {
	cached, ok := a.contextEstimates[path]
	if ok && time.Since(cached.scannedAt) < contextEstimateCacheTTL {
		return cached.estimates
	}
	gen := a.projectsGen
	if !a.contextTasks[path] {
		a.contextTasks[path] = true
		p := path
		a.spawn(func() {
			estimates := estimateProjectFn(p)
			a.Apply(func() {
				if a.projectsGen != gen || !a.contextTasks[p] {
					return
				}
				delete(a.contextTasks, p)
				a.contextEstimates[p] = contextEstimateCache{scannedAt: time.Now(), estimates: estimates}
			})
		})
	}
	if ok {
		return cached.estimates
	}
	return nil
}

// refreshContextEstimate is refresh_context_estimate.
func (a *App) refreshContextEstimate(path string) {
	a.projectsGen++
	a.contextTasks = map[string]bool{}
	delete(a.contextEstimates, path)
}

// clearContextEstimates drops every cached estimate (install flows).
func (a *App) clearContextEstimates() {
	a.contextEstimates = map[string]contextEstimateCache{}
}

// refreshProjectAndEstimate invalidates one project's snapshot +
// estimate (used after install/uninstall/removal).
func (a *App) refreshProjectAndEstimate(path string) {
	a.projectsGen++
	delete(a.projectSnapshots, path)
	a.projectTasks = map[string]bool{}
	a.contextTasks = map[string]bool{}
	a.contextEstimates = map[string]contextEstimateCache{}
}
