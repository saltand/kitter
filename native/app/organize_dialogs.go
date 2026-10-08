// organize_dialogs.go renders the tag/group modals from
// organize_flows.rs: tags manager, tag assignment, groups manager,
// delete-group confirmation, and move-to-group.
package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/saltand/kitter/native/core/tags"
)

// tagsDialog is the tag manager modal.
func (a *App) tagsDialog(c *ui.Context) {
	f := &a.TagsFlow
	if !f.Open {
		return
	}
	p := a.Palette()
	scope := f.Scope
	title := a.T("技能标签", "Skill tags")
	if scope == TagScopeProjects {
		title = a.T("项目标签", "Project tags")
	}
	state := a.tagsFor(scope)

	ui.Modal(c, &f.Open, func() {
		ui.Column(c).Width(400).Background(p.Elevated).Radius(12).Clip().Children(func() {
			// Header.
			ui.Row(c).Padding(14, 16).AlignItems(ui.Center).Children(func() {
				ui.Text(c, title).FontSize(16).Bold().Grow(1)
				if ui.Icon(c, iconSVG("x.svg")).Size(30, 30).TextColor(p.Text).
					Background(p.Raised).Radius(8).Cursor(ui.CursorPointer).
					Key("close-tags-modal").Clicked() {
					f.Open = false
				}
			})
			ui.Box(c).Height(1).Background(p.Border)
			ui.Scroll(c).Grow(1).MinHeight(0).MaxHeight(560).Padding(10, 16, 18, 16).Children(func() {
				// Delete confirmation panel.
				if f.DeletePending != nil {
					id := *f.DeletePending
					var name string
					if tag := state.GetTag(id); tag != nil {
						name = tag.Name
					}
					ui.Row(c).Padding(10, 12).Radius(8).Background(p.DangerSoft).
						AlignItems(ui.Center).Gap(10).Children(func() {
						ui.Text(c, fmt.Sprintf("%s #%s?", a.T("删除标签", "Delete tag"), name)).
							FontSize(13).Grow(1)
						if ui.Text(c, a.T("取消", "Cancel")).FontSize(12).Padding(4, 10).Radius(6).
							Cursor(ui.CursorPointer).Key("tag-delete-cancel").Clicked() {
							f.DeletePending = nil
						}
						if ui.Text(c, a.T("删除", "Delete")).FontSize(12).Padding(4, 10).Radius(6).
							Background(p.Danger).TextColor(ui.RGB(0xFF, 0xFF, 0xFF)).Cursor(ui.CursorPointer).
							Key("tag-delete-confirm").Clicked() {
							a.deleteTag(id)
						}
					})
				}

				// Rows: roots + children (one level).
				for _, root := range state.Roots() {
					a.tagRow(c, scope, state, root, false)
					for _, child := range state.Children(root.ID) {
						a.tagRow(c, scope, state, child, true)
					}
				}

				// New tag row.
				if f.Edit == nil {
					if ui.Text(c, a.T("＋ 新建标签", "+ New tag")).FontSize(13).TextColor(p.Muted).
						Padding(6, 8).Radius(6).Cursor(ui.CursorPointer).
						Key("tag-create-root").Clicked() {
						a.startTagEdit(TagEdit{Kind: TagEditCreateRoot})
					}
				} else {
					a.tagEditRow(c)
				}
				if f.Error != "" {
					ui.Text(c, f.Error).FontSize(12).TextColor(p.Danger).Margin(8, 0, 0, 0).Wrap()
				}
			})
		})
	})
}

// tagRow is one row in the tag manager (name + child indicator + edit
// affordances).
func (a *App) tagRow(c *ui.Context, scope TagScope, state *tags.TagState, tag *tags.Tag, child bool) {
	p := a.Palette()
	f := &a.TagsFlow
	editing := f.Edit != nil && f.Edit.Kind == TagEditRename && f.Edit.ID == tag.ID
	row := ui.Row(c).Height(34).AlignItems(ui.Center).Gap(6).
		Radius(6).Cursor(ui.CursorPointer).
		Key(fmt.Sprintf("tag-row-%d", tag.ID))
	if child {
		row.Margin(0, 0, 0, 26)
	}
	// Tag drag: reorder within the same parent level.
	row.Drag(tagDrag{Scope: scope, Parent: tag.Parent, ID: tag.ID, Name: tag.Name})
	if over, ok := ui.DragOver[tagDrag](row); ok {
		bounds := row.Bounds()
		_, py, _ := row.PointerPosition()
		f.DropTarget = tagDropPoint(&over, scope, tag.Parent, tag.ID,
			0, bounds.H, py)
	}
	if dropped, ok := ui.Drop[tagDrag](row); ok {
		if f.DropTarget != nil && f.DropTarget.TagID == tag.ID {
			a.applyTagDrop(&dropped, f.DropTarget)
		}
		f.DropTarget = nil
	}
	// Highlight the drop line (accent bar above/below the row).
	if f.DropTarget != nil && f.DropTarget.TagID == tag.ID {
		if f.DropTarget.Position == tagDropBefore {
			row.Border(1, p.Accent).BorderWidth(1, 0, 0, 0)
		} else {
			row.Border(1, p.Accent).BorderWidth(0, 0, 1, 0)
		}
	}
	row.Children(func() {
		name := "#" + tag.Name
		if child {
			name = "#" + tag.Name // nested under parent; display the leaf
		}
		ui.Text(c, name).FontSize(13).Font(FontMono).Grow(1).SingleLine()
		if !child && state.Children(tag.ID) != nil && len(state.Children(tag.ID)) == 0 {
			// Root with no children can gain one.
		}
		// Add-child button (roots only, Rust allows one level).
		if !child {
			if ui.Icon(c, iconSVG("plus.svg")).Size(22, 22).TextColor(p.Muted).
				Radius(5).Cursor(ui.CursorPointer).
				Tooltip(a.T("新建子标签", "New child tag")).Label(a.T("新建子标签", "New child tag")).
				Key(fmt.Sprintf("tag-add-child-%d", tag.ID)).Clicked() {
				a.startTagEdit(TagEdit{Kind: TagEditCreateChild, ID: tag.ID})
			}
		}
		if ui.Icon(c, iconSVG("pencil.svg")).Size(22, 22).TextColor(p.Muted).
			Radius(5).Cursor(ui.CursorPointer).
			Tooltip(a.T("重命名", "Rename")).Label(a.T("重命名", "Rename")).
			Key(fmt.Sprintf("tag-rename-%d", tag.ID)).Clicked() {
			a.startTagEdit(TagEdit{Kind: TagEditRename, ID: tag.ID})
		}
		if ui.Icon(c, iconSVG("trash.svg")).Size(22, 22).TextColor(p.Danger).
			Radius(5).Cursor(ui.CursorPointer).
			Tooltip(a.T("删除", "Delete")).Label(a.T("删除", "Delete")).
			Key(fmt.Sprintf("tag-delete-%d", tag.ID)).Clicked() {
			id := tag.ID
			f.DeletePending = &id
			f.Edit = nil
			f.Error = ""
		}
	})
	if editing {
		row.Background(p.Selected)
	}
}

// tagEditRow is the inline name input + confirm/cancel for create or
// rename.
func (a *App) tagEditRow(c *ui.Context) {
	f := &a.TagsFlow
	p := a.Palette()
	ui.Row(c).Height(36).Padding(0, 8).Radius(6).AlignItems(ui.Center).Gap(6).
		Border(1, p.Accent).Margin(4, 0, 4, 0).Children(func() {
		ui.TextInput(c, &f.NameInput).Grow(1).FontSize(13).
			Label(a.T("标签名", "Tag name"))
		if ui.Icon(c, iconSVG("check.svg")).Size(22, 22).TextColor(p.Accent).
			Cursor(ui.CursorPointer).Key("tag-edit-confirm").Clicked() {
			a.commitTagEdit()
		}
		if ui.Icon(c, iconSVG("x.svg")).Size(22, 22).TextColor(p.Muted).
			Cursor(ui.CursorPointer).Key("tag-edit-cancel").Clicked() {
			f.Edit = nil
			f.Error = ""
		}
	})
}

// assignTagsDialog is tag_assignment_modal.
func (a *App) assignTagsDialog(c *ui.Context) {
	f := &a.TagsFlow
	if !f.AssignOpen {
		return
	}
	p := a.Palette()
	scope := f.Scope
	label := f.AssignmentLabel
	if label == "" {
		label = joinComma(f.AssignmentKeys)
	}
	state := a.tagsFor(scope)

	ui.Modal(c, &f.AssignOpen, func() {
		ui.Column(c).Width(360).Background(p.Elevated).Radius(12).Clip().Children(func() {
			ui.Row(c).Padding(14, 16).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					ui.Text(c, a.T("设置标签", "Set tags")).FontSize(16).Bold()
					ui.Text(c, label).FontSize(12).Font(FontMono).TextColor(p.Muted).
						SingleLine().Margin(3, 0, 0, 0)
				})
				if ui.Icon(c, iconSVG("x.svg")).Size(30, 30).TextColor(p.Text).
					Background(p.Raised).Radius(8).Cursor(ui.CursorPointer).
					Key("close-tag-assignment").Clicked() {
					f.AssignOpen = false
				}
			})
			ui.Box(c).Height(1).Background(p.Border)
			ui.Scroll(c).Grow(1).MinHeight(0).MaxHeight(480).Padding(8, 16, 16, 16).Children(func() {
				if len(state.TagsList()) == 0 {
					ui.Text(c, a.T("还没有标签", "No tags yet")).FontSize(13).TextColor(p.Muted).
						Padding(10, 0, 10, 0)
				}
				for _, tag := range state.TagsList() {
					tag := tag
					a.assignTagRow(c, scope, tag.ID, tag.Name, tag.Parent != nil)
				}
				if ui.Text(c, a.T("＋ 新建标签", "+ New tag")).FontSize(13).TextColor(p.Muted).
					Padding(6, 8).Radius(6).Cursor(ui.CursorPointer).
					Key("new-tag-from-assignment").Clicked() {
					f.AssignOpen = false
					a.openTagCreationFromAssignment()
				}
			})
		})
	})
}

// assignTagRow is tag_assignment_row.
func (a *App) assignTagRow(c *ui.Context, scope TagScope, tagID tags.TagID, name string, isChild bool) {
	p := a.Palette()
	f := &a.TagsFlow
	keys := f.AssignmentKeys
	state := a.tagsFor(scope)
	all := len(keys) > 0
	any := false
	for _, key := range keys {
		if state.IsAssigned(key, tagID) {
			any = true
		} else {
			all = false
		}
	}
	mixed := !all && any
	row := ui.Row(c).Height(34).Padding(0, 10).AlignItems(ui.Center).Radius(6).
		Cursor(ui.CursorPointer).Key(fmt.Sprintf("dialog-assign-tag-%d", tagID))
	if isChild {
		row.Margin(0, 0, 0, 24)
	}
	row.Children(func() {
		textColor := p.Text
		if isChild {
			textColor = p.Secondary
		}
		ui.Text(c, "#"+name).FontSize(13).Font(FontMono).TextColor(textColor).Grow(1).SingleLine()
		if all {
			ui.Icon(c, iconSVG("check.svg")).TextColor(p.Text)
		} else if mixed {
			ui.Text(c, "—").FontSize(12).TextColor(p.Muted)
		}
	})
	row.OnClick(func() {
		a.toggleAssignTag(tagID)
	})
}

// groupsDialog is the group manager modal.
func (a *App) groupsDialog(c *ui.Context) {
	f := &a.GroupsFlow
	if !f.Open {
		return
	}
	p := a.Palette()
	ui.Modal(c, &f.Open, func() {
		ui.Column(c).Width(360).Background(p.Elevated).Radius(12).Clip().Children(func() {
			ui.Row(c).Padding(14, 16).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.T("分组", "Groups")).FontSize(16).Bold().Grow(1)
				if ui.Icon(c, iconSVG("x.svg")).Size(30, 30).TextColor(p.Text).
					Background(p.Raised).Radius(8).Cursor(ui.CursorPointer).
					Key("close-groups-modal").Clicked() {
					f.Open = false
				}
			})
			ui.Box(c).Height(1).Background(p.Border)
			ui.Scroll(c).Grow(1).MinHeight(0).MaxHeight(480).Padding(10, 16, 18, 16).Children(func() {
				groups := a.Library.Groups()
				if len(groups) == 0 {
					ui.Text(c, a.T("还没有分组", "No groups yet")).FontSize(13).TextColor(p.Muted).
						Padding(10, 0)
				}
				for _, group := range groups {
					group := group
					a.groupRow(c, group.ID, group.Name)
				}
				if f.Edit == nil {
					if ui.Text(c, a.T("＋ 新建分组", "+ New group")).FontSize(13).TextColor(p.Muted).
						Padding(6, 8).Radius(6).Cursor(ui.CursorPointer).
						Key("group-create").Clicked() {
						a.startGroupEdit(GroupEdit{Kind: GroupEditCreate})
					}
				} else {
					a.groupEditRow(c)
				}
				if f.Error != "" {
					ui.Text(c, f.Error).FontSize(12).TextColor(p.Danger).Margin(8, 0, 0, 0).Wrap()
				}
			})
		})
	})
}

// groupRow is one row in the group manager.
func (a *App) groupRow(c *ui.Context, id, name string) {
	p := a.Palette()
	f := &a.GroupsFlow
	editing := f.Edit != nil && f.Edit.Kind == GroupEditRename && f.Edit.ID == id
	row := ui.Row(c).Height(34).Padding(0, 8).AlignItems(ui.Center).Gap(6).
		Radius(6).Key("group-row-" + id)
	// Management-scope drag: reorder in the groups dialog.
	row.Drag(groupDrag{Scope: GroupDragManagement, ID: id, Name: name})
	if over, ok := ui.DragOver[groupDrag](row); ok {
		bounds := row.Bounds()
		_, py, _ := row.PointerPosition()
		f.DropTarget = groupDropPoint(&over, GroupDragManagement, id,
			0, bounds.H, py)
	}
	if dropped, ok := ui.Drop[groupDrag](row); ok {
		if f.DropTarget != nil && f.DropTarget.ID == id {
			a.applyGroupDrop(&dropped, f.DropTarget)
		}
		f.DropTarget = nil
	}
	if f.DropTarget != nil && f.DropTarget.ID == id && f.DropTarget.Scope == GroupDragManagement {
		if f.DropTarget.Position == tagDropBefore {
			row.Border(1, p.Accent).BorderWidth(1, 0, 0, 0)
		} else {
			row.Border(1, p.Accent).BorderWidth(0, 0, 1, 0)
		}
	}
	row.Children(func() {
		ui.Text(c, name).FontSize(13).Grow(1).SingleLine()
		if ui.Icon(c, iconSVG("pencil.svg")).Size(22, 22).TextColor(p.Muted).
			Radius(5).Cursor(ui.CursorPointer).
			Tooltip(a.T("重命名", "Rename")).Label(a.T("重命名", "Rename")).
			Key("group-rename-" + id).Clicked() {
			a.startGroupEdit(GroupEdit{Kind: GroupEditRename, ID: id})
		}
		if ui.Icon(c, iconSVG("trash.svg")).Size(22, 22).TextColor(p.Danger).
			Radius(5).Cursor(ui.CursorPointer).
			Tooltip(a.T("删除分组", "Delete group")).Label(a.T("删除分组", "Delete group")).
			Key("group-delete-" + id).Clicked() {
			a.openGroupDeleteDialog(id)
		}
	})
	if editing {
		row.Background(p.Selected)
	}
}

// groupEditRow is the inline group name input.
func (a *App) groupEditRow(c *ui.Context) {
	f := &a.GroupsFlow
	p := a.Palette()
	ui.Row(c).Height(36).Padding(0, 8).Radius(6).AlignItems(ui.Center).Gap(6).
		Border(1, p.Accent).Margin(4, 0, 4, 0).Children(func() {
		ui.TextInput(c, &f.NameInput).Grow(1).FontSize(13).
			Label(a.T("分组名", "Group name"))
		if ui.Icon(c, iconSVG("check.svg")).Size(22, 22).TextColor(p.Accent).
			Cursor(ui.CursorPointer).Key("group-edit-confirm").Clicked() {
			a.commitGroupEdit()
		}
		if ui.Icon(c, iconSVG("x.svg")).Size(22, 22).TextColor(p.Muted).
			Cursor(ui.CursorPointer).Key("group-edit-cancel").Clicked() {
			f.Edit = nil
			f.Error = ""
		}
	})
}

// deleteGroupDialog is the group delete confirmation with the optional
// "delete skills too" toggle.
func (a *App) deleteGroupDialog(c *ui.Context) {
	f := &a.GroupsFlow
	if !f.DeleteOpen {
		return
	}
	p := a.Palette()
	var name string
	for _, group := range a.Library.Groups() {
		if group.ID == f.DeletePending {
			name = group.Name
		}
	}
	ui.Modal(c, &f.DeleteOpen, func() {
		ui.Column(c).Width(360).Background(p.Elevated).Radius(12).Clip().Padding(16).Children(func() {
			ui.Text(c, fmt.Sprintf("%s %s?", a.T("删除分组", "Delete group"), name)).FontSize(15).Bold()
			ui.Text(c, a.T("同时删除其中的技能", "Also delete the skills inside")).
				FontSize(12).TextColor(p.Secondary).Margin(6, 0, 0, 0)
			ui.Row(c).Margin(10, 0, 0, 0).AlignItems(ui.Center).Gap(8).Children(func() {
				checked := f.DeleteSkills
				box := ui.Box(c).Size(18, 18).Radius(4).Border(1, p.Border).Cursor(ui.CursorPointer).
					Key("delete-group-toggle").Children(func() {
					if checked {
						ui.Icon(c, iconSVG("check.svg")).TextColor(p.Accent)
					}
				})
				box.OnClick(func() { f.DeleteSkills = !f.DeleteSkills })
				ui.Text(c, a.T("同时删除技能", "Delete skills too")).FontSize(13).
					Cursor(ui.CursorPointer).OnClick(func() { f.DeleteSkills = !f.DeleteSkills })
			})
			ui.Row(c).Margin(16, 0, 0, 0).Justify(ui.End).Gap(8).Children(func() {
				if ui.Text(c, a.T("取消", "Cancel")).FontSize(12).Padding(6, 14).Radius(8).
					Border(1, p.Border).Cursor(ui.CursorPointer).Key("delete-group-cancel").Clicked() {
					f.DeleteOpen = false
					f.DeletePending = ""
				}
				if ui.Text(c, a.T("删除", "Delete")).FontSize(12).Padding(6, 14).Radius(8).
					Background(p.Danger).TextColor(ui.RGB(0xFF, 0xFF, 0xFF)).Cursor(ui.CursorPointer).
					Key("delete-group-confirm").Clicked() {
					a.deleteSkillGroup(f.DeletePending, f.DeleteSkills)
				}
			})
			if f.Error != "" {
				ui.Text(c, f.Error).FontSize(12).TextColor(p.Danger).Margin(8, 0, 0, 0).Wrap()
			}
		})
	})
}

// moveGroupDialog is move_group_modal.
func (a *App) moveGroupDialog(c *ui.Context) {
	f := &a.GroupsFlow
	if !f.MoveOpen {
		return
	}
	p := a.Palette()
	ui.Modal(c, &f.MoveOpen, func() {
		ui.Column(c).Width(320).Background(p.Elevated).Radius(12).Clip().Padding(16).Children(func() {
			ui.Text(c, a.T("移动分组", "Move to group")).FontSize(15).Bold()
			var name string
			if len(f.MoveSkills) == 1 {
				if skill := a.SelectedSkill(); skill != nil {
					name = skill.Record.Name
				} else {
					name = f.MoveSkills[0]
				}
			} else if a.UsesEnglish() {
				name = Counted(len(f.MoveSkills), "skill", "skills")
			} else {
				name = fmt.Sprintf("%d 个技能", len(f.MoveSkills))
			}
			ui.Text(c, name).FontSize(12).Font(FontMono).TextColor(p.Muted).Margin(4, 0, 0, 0)

			// Rows.
			ui.Column(c).Margin(10, 0, 0, 0).Gap(2).Children(func() {
				none := ui.Row(c).Height(36).Padding(0, 10).AlignItems(ui.Center).Radius(5).
					Cursor(ui.CursorPointer).Key("move-group-none")
				none.Children(func() {
					ui.Text(c, a.T("不使用分组", "No group")).FontSize(13)
				})
				none.OnClick(func() { a.moveSelectedSkillToGroup(nil) })
				for _, group := range a.Library.Groups() {
					group := group
					row := ui.Row(c).Height(36).Padding(0, 10).AlignItems(ui.Center).Radius(5).
						Cursor(ui.CursorPointer).Key("move-group-" + group.ID)
					row.Children(func() {
						ui.Text(c, group.Name).FontSize(13)
					})
					gid := group.ID
					row.OnClick(func() { a.moveSelectedSkillToGroup(&gid) })
				}
			})
		})
	})
}
