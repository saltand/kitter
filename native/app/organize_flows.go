// organize_flows.go ports src/ui/organize_flows.rs +
// tags_groups_actions.rs: the Tags dialog (create/rename/delete with
// one-level nesting), AssignTags, MoveGroup, DeleteGroup, plus the
// group drag/drop primitives.
package app

import (
	"context"
	"fmt"

	"github.com/saltand/kitter/native/core/library"
	"github.com/saltand/kitter/native/core/source"
	"github.com/saltand/kitter/native/core/tags"
)

// TagScope is ui::TagScope.
type TagScope int

const (
	TagScopeSkills TagScope = iota
	TagScopeProjects
)

// TagEdit is ui::TagEdit.
type TagEdit struct {
	Kind   TagEditKind
	ID     tags.TagID // Rename target, or parent for CreateChild
}

// TagEditKind classifies TagEdit.
type TagEditKind int

const (
	TagEditCreateRoot TagEditKind = iota
	TagEditCreateChild
	TagEditRename
)

// GroupEdit is ui::GroupEdit.
type GroupEdit struct {
	Kind GroupEditKind
	ID   string // Rename target
}

// GroupEditKind classifies GroupEdit.
type GroupEditKind int

const (
	GroupEditCreate GroupEditKind = iota
	GroupEditRename
)

// TagAssignmentTarget is ui::TagAssignmentTarget.
type TagAssignmentTarget struct {
	Scope TagScope
	Keys  []string
	Label string
}

// TagsFlow is ui::TagsFlowState minus the InputState entity (Go uses
// plain strings).
type TagsFlow struct {
	Scope              TagScope
	Open               bool
	AssignOpen         bool
	NameInput          string
	Edit               *TagEdit
	DeletePending      *tags.TagID
	Error              string
	AssignmentKeys     []string
	AssignmentLabel    string
	ReturnToAssignment *TagAssignmentTarget
}

// GroupsFlow is ui::GroupsFlowState.
type GroupsFlow struct {
	Open          bool
	DeleteOpen    bool
	MoveOpen      bool
	NameInput     string
	Edit          *GroupEdit
	DeletePending string
	DeleteSkills  bool
	MoveSkills    []string
	Error         string
}

// tagsFor returns the TagState for the dialog's scope.
func (a *App) tagsFor(scope TagScope) *tags.TagState {
	if scope == TagScopeProjects {
		return a.projectTagStates
	}
	return a.SkillTags
}

// openTagDialog is open_tag_dialog.
func (a *App) openTagDialog(scope TagScope) {
	f := &a.TagsFlow
	f.Scope = scope
	f.AssignmentKeys = nil
	f.AssignmentLabel = ""
	f.ReturnToAssignment = nil
	f.Edit = nil
	f.DeletePending = nil
	f.Error = ""
	f.Open = true
}

// openTagCreationFromAssignment is open_tag_creation_from_assignment.
func (a *App) openTagCreationFromAssignment() {
	f := &a.TagsFlow
	if len(f.AssignmentKeys) == 0 {
		return
	}
	target := &TagAssignmentTarget{
		Scope: f.Scope,
		Keys:  append([]string(nil), f.AssignmentKeys...),
		Label: f.AssignmentLabel,
	}
	if target.Label == "" {
		target.Label = joinComma(target.Keys)
	}
	scope := target.Scope
	a.openTagDialog(scope)
	a.TagsFlow.ReturnToAssignment = target
}

// openTagAssignmentTarget is open_tag_assignment_target.
func (a *App) openTagAssignmentTarget(target TagAssignmentTarget) {
	if target.Scope == TagScopeSkills {
		a.Skills.Selection.Replace(target.Keys)
	}
	f := &a.TagsFlow
	f.Scope = target.Scope
	f.AssignmentKeys = target.Keys
	f.AssignmentLabel = target.Label
	f.AssignOpen = true
}

// openTagAssignmentDialogForSelection is
// open_tag_assignment_dialog_for_selection.
func (a *App) openTagAssignmentDialogForSelection(skills []string) {
	if len(skills) == 0 {
		return
	}
	f := &a.TagsFlow
	f.ReturnToAssignment = nil
	a.Skills.Selection.Replace(skills)
	f.Scope = TagScopeSkills
	f.AssignmentKeys = skills
	if len(skills) == 1 {
		if skill := a.SelectedSkill(); skill != nil {
			f.AssignmentLabel = skill.Record.Name
		} else {
			f.AssignmentLabel = skills[0]
		}
	} else if a.UsesEnglish() {
		f.AssignmentLabel = Counted(len(skills), "skill", "skills")
	} else {
		f.AssignmentLabel = fmt.Sprintf("%d 个技能", len(skills))
	}
	f.AssignOpen = true
}

// openProjectTagAssignmentDialog is open_project_tag_assignment_dialog.
func (a *App) openProjectTagAssignmentDialog(path string) {
	f := &a.TagsFlow
	f.ReturnToAssignment = nil
	f.Scope = TagScopeProjects
	f.AssignmentKeys = []string{projectTagKey(path)}
	f.AssignmentLabel = baseName(path)
	f.AssignOpen = true
}

// startTagEdit is start_tag_edit.
func (a *App) startTagEdit(edit TagEdit) {
	f := &a.TagsFlow
	scope := f.Scope
	switch edit.Kind {
	case TagEditRename:
		if tag := a.tagsFor(scope).GetTag(edit.ID); tag != nil {
			f.NameInput = tag.Name
		} else {
			f.NameInput = ""
		}
	default:
		f.NameInput = ""
	}
	f.Edit = &edit
	f.DeletePending = nil
	f.Error = ""
}

// commitTagEdit is commit_tag_edit.
func (a *App) commitTagEdit() {
	f := &a.TagsFlow
	if f.Edit == nil {
		return
	}
	name := f.NameInput
	scope := f.Scope
	var err error
	switch f.Edit.Kind {
	case TagEditCreateRoot:
		_, err = a.tagsFor(scope).Add(name, nil)
	case TagEditCreateChild:
		parent := f.Edit.ID
		_, err = a.tagsFor(scope).Add(name, &parent)
	case TagEditRename:
		err = a.tagsFor(scope).Rename(f.Edit.ID, name)
	}
	if err != nil {
		f.Error = a.ErrorMessage(err)
		return
	}
	f.Edit = nil
	f.Error = ""
	a.persistTags()
	if target := f.ReturnToAssignment; target != nil {
		f.ReturnToAssignment = nil
		f.Open = false
		a.openTagAssignmentTarget(*target)
	}
}

// deleteTag confirms and deletes a tag (with children).
func (a *App) deleteTag(id tags.TagID) {
	f := &a.TagsFlow
	scope := f.Scope
	a.tagsFor(scope).Delete(id)
	a.persistTags()
	// Clear the filter when the selected tag was deleted.
	if scope == TagScopeSkills && a.SelectedTagFilter == id {
		a.HasTagFilter = false
	}
	if scope == TagScopeProjects && a.Projects.SelectedProjectTagFilter == id {
		a.Projects.SelectedProjectTagFilter = 0
	}
	f.DeletePending = nil
}

// toggleAssignTag is the assignment-row click.
func (a *App) toggleAssignTag(tagID tags.TagID) {
	f := &a.TagsFlow
	scope := f.Scope
	keys := f.AssignmentKeys
	all := len(keys) > 0
	for _, key := range keys {
		if !a.tagsFor(scope).IsAssigned(key, tagID) {
			all = false
		}
	}
	add := !all
	for _, key := range keys {
		if a.tagsFor(scope).IsAssigned(key, tagID) != add {
			a.tagsFor(scope).ToggleAssignment(key, tagID)
		}
	}
	a.persistTags()
}

// openGroupDialog is open_group_dialog.
func (a *App) openGroupDialog() {
	f := &a.GroupsFlow
	f.Edit = nil
	f.DeletePending = ""
	f.Error = ""
	f.Open = true
}

// openGroupDeleteDialog is open_group_delete_dialog.
func (a *App) openGroupDeleteDialog(id string) {
	f := &a.GroupsFlow
	f.Edit = nil
	f.DeletePending = id
	f.DeleteSkills = false
	f.Error = ""
	f.DeleteOpen = true
}

// openMoveGroupDialogForSelection is
// open_move_group_dialog_for_selection.
func (a *App) openMoveGroupDialogForSelection(skills []string) {
	a.GroupsFlow.MoveSkills = skills
	a.GroupsFlow.MoveOpen = true
}

// startGroupEdit is start_group_edit.
func (a *App) startGroupEdit(edit GroupEdit) {
	f := &a.GroupsFlow
	switch edit.Kind {
	case GroupEditRename:
		for _, group := range a.Library.Groups() {
			if group.ID == edit.ID {
				f.NameInput = group.Name
			}
		}
	default:
		f.NameInput = ""
	}
	f.Edit = &edit
	f.DeletePending = ""
	f.Error = ""
}

// commitGroupEdit is commit_group_edit.
func (a *App) commitGroupEdit() {
	f := &a.GroupsFlow
	if f.Edit == nil {
		return
	}
	name := f.NameInput
	var err error
	switch f.Edit.Kind {
	case GroupEditCreate:
		_, err = a.Library.CreateGroup(name)
	case GroupEditRename:
		err = a.Library.RenameGroup(f.Edit.ID, name)
	}
	if err != nil {
		f.Error = a.ErrorMessage(err)
		return
	}
	f.Edit = nil
	f.Error = ""
	a.ReloadSkills()
}

// moveSelectedSkillToGroup is move_selected_skill_to_group.
func (a *App) moveSelectedSkillToGroup(groupID *string) {
	f := &a.GroupsFlow
	if len(f.MoveSkills) == 0 {
		f.MoveOpen = false
		return
	}
	skills := append([]string(nil), f.MoveSkills...)
	var failure string
	for _, skill := range skills {
		if err := a.Library.AssignGroupByStorage(skill, groupID); err != nil {
			failure = a.ErrorMessage(err)
			break
		}
	}
	if failure == "" {
		f.MoveSkills = nil
		f.MoveOpen = false
		a.ReloadSkills()
		a.notice(a.T("已更新技能分组", "Skill group updated"))
	} else {
		a.notice(failure)
	}
}

// deleteSkillGroup is delete_skill_group.
func (a *App) deleteSkillGroup(id string, deleteSkills bool) {
	f := &a.GroupsFlow
	names, err := a.Library.DeleteGroup(id, deleteSkills)
	if err != nil {
		f.Error = a.ErrorMessage(err)
		return
	}
	f.DeleteOpen = false
	f.DeletePending = ""
	// Drop the deleted storages from the selection.
	order := a.skillOrder()
	a.Skills.Selection.Remove(names, order)
	a.ReloadSkills()
	if deleteSkills {
		a.notice(a.T("已删除分组及其中的技能", "Group and its skills deleted"))
	} else {
		a.notice(a.T("已删除分组，技能已移到未分组", "Group deleted; skills are now ungrouped"))
	}
}

// joinComma is ", ".join.
func joinComma(keys []string) string {
	out := ""
	for i, k := range keys {
		if i > 0 {
			out += ", "
		}
		out += k
	}
	return out
}


// checkAllUpdates is check_all_updates: run source.CheckUpdates in a
// background library instance; the count + errors come back through
// Apply.
func (a *App) checkAllUpdates() {
	if a.CheckingUpdates {
		return
	}
	a.CheckingUpdates = true
	dataDir := a.Library.DataDir
	a.spawn(func() {
		lib, err := library.OpenIn(dataDir)
		var count int
		var checkErr error
		if err == nil {
			count, checkErr = source.CheckUpdates(context.Background(), lib)
		} else {
			checkErr = err
		}
		a.Apply(func() {
			a.CheckingUpdates = false
			var message string
			switch {
			case checkErr != nil:
				message = a.ErrorMessage(checkErr)
			case count == 0:
				a.Library = lib
				message = a.T("所有技能都是最新版本", "All skills are up to date")
			default:
				a.Library = lib
				a.UpdateCount = count
				if a.UsesEnglish() {
					message = fmt.Sprintf("%s can be updated", Counted(count, "skill", "skills"))
				} else {
					message = fmt.Sprintf("发现 %d 个可更新的技能", count)
				}
			}
			a.notice(message)
			a.ReloadSkills()
		})
	})
}

// updateSkill is update_skill: refresh one skill's copy from its
// source.
func (a *App) updateSkill(storageName string) {
	if a.UpdatingSkill != "" {
		return
	}
	a.UpdatingSkill = storageName
	dataDir := a.Library.DataDir
	a.spawn(func() {
		lib, err := library.OpenIn(dataDir)
		var updateErr error
		if err == nil {
			updateErr = source.UpdateByStorage(context.Background(), lib, storageName)
		} else {
			updateErr = err
		}
		a.Apply(func() {
			a.UpdatingSkill = ""
			var message string
			if updateErr != nil {
				message = a.ErrorMessage(updateErr)
			} else {
				a.Library = lib
				message = a.T("技能已更新", "Skill updated")
			}
			a.notice(message)
			a.ReloadSkills()
		})
	})
}
