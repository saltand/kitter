// selection_actions.go ports ui/selection_actions.rs: the selection
// gestures plus the actions they drive (detail selection, manual-only
// toggle, delete flow).
package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/saltand/kitter/native/core/model"
	"github.com/saltand/kitter/native/core/project"
)

// SelectedSkill is KitterApp::selected_skill.
func (a *App) SelectedSkill() *model.SkillSummary {
	primary, ok := a.Skills.Selection.Primary()
	if !ok {
		return nil
	}
	for i := range a.Skills.Items {
		if SkillStorageName(&a.Skills.Items[i]) == primary {
			return &a.Skills.Items[i]
		}
	}
	return nil
}

// selectedSkillKeys is KitterApp::selected_skill_keys.
func (a *App) selectedSkillKeys() []string {
	return a.Skills.Selection.SelectedIn(a.skillOrder())
}

// selectedLibraryTargets is KitterApp::selected_library_targets.
func (a *App) selectedLibraryTargets() []model.SkillSummary {
	selected := map[string]bool{}
	for _, key := range a.selectedSkillKeys() {
		selected[key] = true
	}
	var out []model.SkillSummary
	for _, skill := range a.Skills.Items {
		if selected[SkillStorageName(&skill)] {
			out = append(out, skill)
		}
	}
	return out
}

// setDetailSelection is KitterApp::set_detail_selection.
func (a *App) setDetailSelection(storageName string, has bool) {
	a.Skills.Selection.SetPrimary(storageName, has)
	a.Skills.Tab = DetailInstalls
	a.Skills.SelectedFile = "SKILL.md"
	a.Skills.CollapsedContentDirs = map[string]bool{}
	a.Skills.contentSnapshot = false
	a.Skills.contentGen++
}

// finishSkillSelectionMode is KitterApp::finish_skill_selection_mode.
func (a *App) finishSkillSelectionMode() {
	primary, has := a.Skills.Selection.Finish(a.skillOrder())
	a.setDetailSelection(primary, has)
}

// clearSkillSelection is KitterApp::clear_skill_selection.
func (a *App) clearSkillSelection() {
	a.Skills.Selection.Clear()
	a.setDetailSelection("", false)
}

// selectAllVisibleSkills is KitterApp::select_all_visible_skills.
func (a *App) selectAllVisibleSkills(visible []string) {
	primary, has := a.Skills.Selection.SelectAll(visible)
	a.setDetailSelection(primary, has)
}

// toggleSkillSelection is KitterApp::toggle_skill_selection.
func (a *App) toggleSkillSelection(storageName string) {
	primary, has := a.Skills.Selection.Toggle(storageName, a.skillOrder())
	a.setDetailSelection(primary, has)
}

// selectSkillFromClick is KitterApp::select_skill_from_click.
func (a *App) selectSkillFromClick(storageName string, mods ui.Modifiers, visibleOrder []string) {
	var primary string
	var has bool
	switch {
	case mods&ui.Shift != 0:
		primary, has = a.Skills.Selection.SelectRange(storageName, visibleOrder)
	case mods&ui.Cmd != 0:
		primary, has = a.Skills.Selection.Toggle(storageName, visibleOrder)
	default:
		primary, has = a.Skills.Selection.SelectOne(storageName)
	}
	a.setDetailSelection(primary, has)
}

// prepareSkillContextSelection is KitterApp::prepare_skill_context_selection.
func (a *App) prepareSkillContextSelection(storageName string) {
	primary, has := a.Skills.Selection.SelectForContext(storageName)
	a.setDetailSelection(primary, has)
}

// setSkillKitterManual is KitterApp::set_skill_kitter_manual.
func (a *App) setSkillKitterManual(c *ui.Context, storageName string, enabled bool) {
	err := a.Library.SetKitterManualByStorage(storageName, enabled)
	if err == nil {
		a.ReloadSkills()
		if enabled {
			a.showNotice(c, a.T("已设为仅手动触发", "Set as manual-only"))
		} else {
			a.showNotice(c, a.T("已还原自动触发", "Restored automatic invocation"))
		}
		return
	}
	a.showNotice(c, a.ErrorMessage(err))
}

// deleteLibrarySkills is the DeleteConfirmation::LibrarySkills confirm
// branch of install_flow.rs::delete_confirmation_modal.
func (a *App) deleteLibrarySkills(c *ui.Context) {
	if a.Skills.DeleteBusy {
		return
	}
	a.Skills.DeleteBusy = true
	skills := a.Skills.DeleteSkills
	removed := 0
	var failures []string
	for _, skill := range skills {
		if err := a.Library.RemoveByStorage(SkillStorageName(&skill)); err != nil {
			failures = append(failures, err.Error())
		} else {
			removed++
		}
	}
	a.Skills.DeleteSkills = nil
	a.Skills.DeleteOpen = false
	a.Skills.DeleteBusy = false
	a.ReloadSkills()
	if len(failures) == 0 {
		a.showNotice(c, a.T("技能已删除", "Skills deleted"))
	} else if a.UsesEnglish() {
		a.showNotice(c, fmt.Sprintf("Deleted %d; %d failed", removed, len(failures)))
	} else {
		a.showNotice(c, fmt.Sprintf("已删除 %d 个，%d 个失败", removed, len(failures)))
	}
}

// openLibraryDelete opens the delete modal for library skills.
func (a *App) openLibraryDelete(skills []model.SkillSummary) {
	a.Skills.DeleteSkills = skills
	a.Skills.DeleteProject = ""
	a.Skills.DeleteProjectSkill = nil
	a.Skills.DeleteSelected = map[string]bool{}
	a.Skills.DeleteBusy = false
	a.Skills.DeleteOpen = true
}

// openProjectDelete opens the delete modal for a project installation
// (DeleteConfirmation::ProjectSkill).
func (a *App) openProjectDelete(projectPath string, skill model.ProjectSkill) {
	a.Skills.DeleteSkills = nil
	a.Skills.DeleteProject = projectPath
	a.Skills.DeleteProjectSkill = &skill
	a.Skills.DeleteSelected = map[string]bool{}
	for _, path := range uniqueInstallationPaths(skill.Installations) {
		a.Skills.DeleteSelected[path] = true
	}
	a.Skills.DeleteBusy = false
	a.Skills.DeleteOpen = true
}

// deleteProjectSkill is the DeleteConfirmation::ProjectSkill confirm
// branch of install_flow.rs::delete_confirmation_modal.
func (a *App) deleteProjectSkill(c *ui.Context) {
	if a.Skills.DeleteBusy || a.Skills.DeleteProjectSkill == nil {
		return
	}
	a.Skills.DeleteBusy = true
	skill := a.Skills.DeleteProjectSkill
	projectPath := a.Skills.DeleteProject
	var selected []*model.ProjectSkillInstallation
	for i := range skill.Installations {
		if a.Skills.DeleteSelected[skill.Installations[i].Path] {
			selected = append(selected, &skill.Installations[i])
		}
	}
	report := project.RemoveProjectSkills(selected)
	var message string
	globalScope := projectPath != "" && projectPath == homeDir()
	success := a.T("已从项目中移除", "Removed from project")
	if globalScope {
		success = a.T("已从用户级目录中移除", "Removed from user-level locations")
	}
	switch {
	case len(report.Failures) == 0:
		message = success
	case report.Removed > 0:
		if a.UsesEnglish() {
			message = fmt.Sprintf("Removed %s; %d could not be removed",
				Counted(report.Removed, "location", "locations"), len(report.Failures))
		} else {
			message = fmt.Sprintf("已移除 %d 个位置，另有 %d 个位置未能移除", report.Removed, len(report.Failures))
		}
	default:
		if a.UsesEnglish() {
			message = fmt.Sprintf("%s could not be removed. Check access and try again",
				Counted(len(report.Failures), "location", "locations"))
		} else {
			message = fmt.Sprintf("%d 个位置未能移除，请检查访问权限后重试", len(report.Failures))
		}
	}
	if report.Removed > 0 && !globalScope && projectPath != "" {
		a.Library.Config.TouchProject(projectPath)
		_ = a.Library.Save()
	}
	a.Skills.DeleteOpen = false
	a.Skills.DeleteProjectSkill = nil
	a.Skills.DeleteProject = ""
	a.Skills.DeleteSelected = map[string]bool{}
	a.Skills.DeleteBusy = false
	a.showNotice(c, message)
	a.ReloadSkills()
}
