// skills_page.go ports ui/skills_page.rs: the split-pane skills page —
// search, tag filter, pinned builtin, collapsible groups (persisted via
// config.collapsed_skill_groups, feat/persist-collapsed), SkillSelection
// multi-select, context menus, the Installs/Content detail tabs, and the
// delete confirmation modal.
package app

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/saltand/kitter/native/core/agents"
	"github.com/saltand/kitter/native/core/model"
	"github.com/saltand/kitter/native/core/project"
	"github.com/saltand/kitter/native/core/tags"
)

const (
	listWidthDefault = float32(280)
	descriptionMaxW  = float32(560)
)

// listEntry is one flattened row of the skills list: a group header
// (skill nil) or a skill row.
type listEntry struct {
	groupID string
	skill   *model.SkillSummary
	nested  bool
}

// installableSkillsByGroup is installable_skills_by_group in Rust.
func installableSkillsByGroup(skills []model.SkillSummary, groupIDs map[string]bool) map[string][]string {
	grouped := map[string][]string{}
	for i := range skills {
		skill := &skills[i]
		if skill.Record.GroupID != nil && groupIDs[*skill.Record.GroupID] {
			key := *skill.Record.GroupID
			grouped[key] = append(grouped[key], SkillStorageName(skill))
		}
	}
	return grouped
}

// skillsPage is KitterApp::skills_page.
func (a *App) skillsPage(c *ui.Context) {
	t := c.Theme()
	p := a.Palette()
	query := strings.ToLower(strings.TrimSpace(a.Skills.Search))

	var visible []*model.SkillSummary
	for i := range a.Skills.Items {
		skill := &a.Skills.Items[i]
		matchesQuery := query == "" ||
			strings.Contains(strings.ToLower(skill.Record.Name), query) ||
			strings.Contains(strings.ToLower(skill.Record.Description), query) ||
			strings.Contains(strings.ToLower(a.sourceLabel(&skill.Record.Origin)), query)
		matchesTag := !a.HasTagFilter ||
			a.SkillTags.MatchesFilter(SkillStorageName(skill), a.SelectedTagFilter)
		if matchesQuery && matchesTag {
			visible = append(visible, skill)
		}
	}
	configuredGroups := a.Library.Groups()
	groupIDs := map[string]bool{}
	for _, g := range configuredGroups {
		groupIDs[g.ID] = true
	}

	var pinned, ungrouped []*model.SkillSummary
	grouped := map[string][]*model.SkillSummary{}
	for _, skill := range visible {
		switch {
		case skill.Record.Origin.IsBuiltin():
			pinned = append(pinned, skill)
		case skill.Record.GroupID != nil && groupIDs[*skill.Record.GroupID]:
			key := *skill.Record.GroupID
			grouped[key] = append(grouped[key], skill)
		default:
			ungrouped = append(ungrouped, skill)
		}
	}
	byActivity := func(l, r *model.SkillSummary) bool {
		if l.Record.LastOperatedAt != r.Record.LastOperatedAt {
			return l.Record.LastOperatedAt > r.Record.LastOperatedAt
		}
		return l.Record.Name < r.Record.Name
	}
	for _, skills := range grouped {
		sort.SliceStable(skills, func(i, j int) bool { return byActivity(skills[i], skills[j]) })
	}
	sort.SliceStable(ungrouped, func(i, j int) bool { return byActivity(ungrouped[i], ungrouped[j]) })

	// Flatten into list rows: pinned skills, then per-group a header row
	// followed by its skills (unless collapsed), then ungrouped skills.
	// selectionOrder is the order shift-range selection follows.
	var rows []listEntry
	var selectionOrder []string
	push := func(skill *model.SkillSummary, nested bool) {
		rows = append(rows, listEntry{skill: skill, nested: nested})
		selectionOrder = append(selectionOrder, SkillStorageName(skill))
	}
	for _, skill := range pinned {
		push(skill, false)
	}
	for _, group := range configuredGroups {
		skills := grouped[group.ID]
		collapsed := query == "" && a.Skills.CollapsedGroups[group.ID]
		rows = append(rows, listEntry{groupID: group.ID})
		if collapsed {
			continue
		}
		for _, skill := range skills {
			push(skill, true)
		}
	}
	for _, skill := range ungrouped {
		push(skill, false)
	}

	ui.Split(c, &a.splitSize, func() {
		a.skillListPane(c, t, rows, configuredGroups, selectionOrder, len(visible))
	}, func() {
		a.skillDetail(c, t)
	}).Fill().MinWidth(0)
	_ = p
}

// skillListPane is the left pane of skills_page. Rows are the flattened
// list entries built by skillsPage.
func (a *App) skillListPane(c *ui.Context, t *ui.Theme, rows []listEntry, groups []model.SkillGroup, selectionOrder []string, visibleCount int) {
	p := a.Palette()
	query := strings.TrimSpace(a.Skills.Search)
	selCount := len(a.selectedSkillKeys())
	multiMode := a.Skills.Selection.IsMultiple()

	pane := ui.Column(c).Fill().MinWidth(0).Background(p.Surface)
	pane.Children(func() {
		// Panel header: title + count + add.
		ui.Row(c).Height(52).Shrink(0).Padding(0, 12).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Gap(7).AlignItems(ui.Center).Grow(1).MinWidth(0).Children(func() {
				ui.Text(c, a.T("技能", "Skills")).FontSize(14).Bold().SingleLine()
				ui.Text(c, fmt.Sprint(len(a.Skills.Items))).Font(FontMono).FontSize(12).TextColor(p.Muted)
			})
			if ui.Icon(c, iconSVG("plus.svg")).Size(28, 28).TextColor(p.Text).
				Tooltip(a.T("添加技能", "Add Skill")).
				Label(a.T("添加技能", "Add Skill")).
				Cursor(ui.CursorPointer).Clicked() {
				a.openAddModal(AddLocal)
			}
		})
		ui.Divider(c)

		ui.Box(c).Padding(8, 10).Children(func() {
			ui.SearchField(c, &a.Skills.Search).FillWidth().Label(a.T("搜索技能", "Search skills"))
		})

		// Filter line: "全部 N" or "#path  N" plus the tag-filter menu button.
		ui.Row(c).Padding(0, 16, 6, 16).AlignItems(ui.Center).Children(func() {
			label := fmt.Sprintf("%s  %d", a.T("全部", "All"), visibleCount)
			if a.HasTagFilter {
				if path, ok := a.SkillTags.Path(a.SelectedTagFilter); ok {
					label = fmt.Sprintf("#%s  %d", path, visibleCount)
				}
			}
			ui.Text(c, label).FontSize(12).TextColor(p.Muted).SingleLine()
			ui.Spacer(c).Grow(1)
			iconColor := p.Muted
			if a.HasTagFilter {
				iconColor = p.Accent
			}
			ui.Icon(c, iconSVG("hash.svg")).Size(28, 28).TextColor(iconColor).
				Label(a.T("筛选标签", "Filter by tag")).Tooltip(a.T("筛选标签", "Filter by tag")).
				Menu(func(m *ui.Menu) { a.tagFilterMenu(m) })
		})

		if multiMode {
			ui.Row(c).Padding(0, 16, 6, 16).AlignItems(ui.Center).Children(func() {
				var text string
				if a.UsesEnglish() {
					text = fmt.Sprintf("%s selected", Counted(selCount, "skill", "skills"))
				} else {
					text = fmt.Sprintf("已选 %d 个技能", selCount)
				}
				ui.Text(c, text).FontSize(12).TextColor(p.Secondary)
				ui.Spacer(c).Grow(1)
				if ui.Text(c, a.T("全选", "All")).FontSize(12).TextColor(p.Secondary).
					Cursor(ui.CursorPointer).Key("select-all-visible-skills").Clicked() {
					a.selectAllVisibleSkills(selectionOrder)
				}
				ui.Box(c).Width(6)
				if ui.Text(c, a.T("清除", "Clear")).FontSize(12).TextColor(p.Secondary).
					Cursor(ui.CursorPointer).Key("clear-skill-selection").Clicked() {
					a.clearSkillSelection()
				}
			})
		}

		if len(a.Skills.Items) > 0 && len(rows) == 0 && query != "" {
			ui.Text(c, a.T("没有匹配的技能", "No matching skills")).FontSize(12).
				TextColor(p.Muted).Padding(20, 12)
			return
		}
		a.skillList.Key = func(row int) any {
			if row < 0 || row >= len(rows) {
				return row
			}
			if rows[row].skill != nil {
				return SkillStorageName(rows[row].skill)
			}
			return "group:" + rows[row].groupID
		}
		a.skillList.Label = func(row int) string {
			if row < 0 || row >= len(rows) {
				return ""
			}
			if rows[row].skill != nil {
				return rows[row].skill.Record.Name
			}
			for _, g := range groups {
				if g.ID == rows[row].groupID {
					return g.Name
				}
			}
			return ""
		}
		a.skillList.Header = func(row int) bool {
			return row >= 0 && row < len(rows) && rows[row].skill == nil
		}
		ui.List(c, &a.skillList, len(rows), func(row int) {
			e := rows[row]
			if e.skill == nil {
				a.groupHeaderRow(c, groups, e.groupID, query == "" && a.Skills.CollapsedGroups[e.groupID])
				return
			}
			a.skillListRow(c, e.skill, e.nested, selectionOrder)
		}).Grow(1).MinHeight(0).Padding(0, 8, 8, 8)
	})
}

// groupHeaderRow is the group header row of skills_page.
func (a *App) groupHeaderRow(c *ui.Context, groups []model.SkillGroup, groupID string, collapsed bool) {
	p := a.Palette()
	var group model.SkillGroup
	for _, g := range groups {
		if g.ID == groupID {
			group = g
		}
	}
	count := 0
	for i := range a.Skills.Items {
		if g := a.Skills.Items[i].Record.GroupID; g != nil && *g == groupID {
			count++
		}
	}
	chevron := "chevron-down.svg"
	if collapsed {
		chevron = "chevron-right.svg"
	}
	// The outer wrapper owns group drops; the inner row owns skill
	// drops. mygo's per-element accepts holds a single type check, so
	// one element can not take both kinds — hitChain tries innermost
	// first, which gives each kind its own target (Rust's row took
	// both through separate on_drop handlers).
	var header ui.Element
	wrap := ui.Column(c).FillWidth()
	wrap.Children(func() {
		header = ui.Row(c).Height(36).Padding(0, 8).Margin(3, 0, 0, 0).Radius(7).Gap(6).
			AlignItems(ui.Center).TextColor(p.Secondary).Cursor(ui.CursorPointer).
			Key("skill-group-" + groupID).Label("skill-group-" + groupID).
			Drag(groupDrag{Scope: GroupDragList, ID: groupID, Name: group.Name}).
			ContextMenu(func(m *ui.Menu) {
				if m.Item(a.T("重命名", "Rename")).Chosen() {
					a.startGroupEdit(GroupEdit{Kind: GroupEditRename, ID: groupID})
					a.GroupsFlow.Open = true
				}
				if m.Item(a.T("删除分组", "Delete group")).Chosen() {
					a.openGroupDeleteDialog(groupID)
				}
			})
		header.Children(func() {
			ui.Icon(c, iconSVG(chevron)).TextColor(p.Muted)
			ui.Text(c, group.Name).Font(FontMono).FontSize(13).SingleLine().Grow(1).MinWidth(0)
			ui.Text(c, fmt.Sprint(count)).Font(FontMono).FontSize(12).TextColor(p.Muted)
		})
	})
	// Group drops register on the row's wrapper so the row's own
	// accepts slot stays free for skill drops (one element takes one
	// drag kind in mygo; hitChain walks ancestors until a kind takes).
	// Group drops on the wrapper: DragOver stores the live target, the
	// release frame's Drop consumes it (Rust's on_drag_move →
	// drop_target → on_drop).
	if over, ok := ui.DragOver[groupDrag](wrap); ok {
		bounds := wrap.Bounds()
		_, py, _ := wrap.PointerPosition()
		a.GroupsFlow.DropTarget = groupDropPoint(&over, GroupDragList, groupID,
			0, bounds.H, py)
	}
	if dropped, ok := ui.Drop[groupDrag](wrap); ok {
		if a.GroupsFlow.DropTarget != nil && a.GroupsFlow.DropTarget.ID == groupID {
			a.applyGroupDrop(&dropped, a.GroupsFlow.DropTarget)
		}
		a.GroupsFlow.DropTarget = nil
	}
	// Skill drops land on the inner row.
	if _, ok := ui.DragOver[skillDrag](header); ok {
		header.Background(p.Selected)
	}
	if dropped, ok := ui.Drop[skillDrag](header); ok {
		gid := groupID
		a.GroupsFlow.MoveSkills = []string{dropped.Name}
		a.moveSelectedSkillToGroup(&gid)
	}
	if header.Clicked() {
		if a.Skills.CollapsedGroups[groupID] {
			delete(a.Skills.CollapsedGroups, groupID)
		} else {
			a.Skills.CollapsedGroups[groupID] = true
		}
		a.persistCollapsedGroups()
	}
}

// skillListRow is KitterApp::skill_list_row.
func (a *App) skillListRow(c *ui.Context, skill *model.SkillSummary, nested bool, visibleOrder []string) {
	p := a.Palette()
	t := c.Theme()
	storageName := SkillStorageName(skill)
	builtIn := skill.Record.Origin.IsBuiltin()
	selCount := len(a.selectedSkillKeys())
	selected := a.Skills.Selection.Contains(storageName)
	multiSelection := selected && selCount > 1
	protected := builtIn || (multiSelection && a.selectionIncludesBuiltin())

	revealLabel := a.T("在访达中显示", "Show in Finder")
	if runtime.GOOS != "darwin" {
		revealLabel = a.T("在文件夹中显示", "Show in folder")
	}

	installLabel := a.T("安装技能", "Install Skill")
	setTagsLabel := a.T("设置标签", "Set tags")
	deleteLabel := a.T("删除技能", "Delete Skill")
	moveLabel := a.T("移动分组", " Move to group")
	if multiSelection {
		if a.UsesEnglish() {
			installLabel = fmt.Sprintf("Install %d skills", selCount)
			setTagsLabel = fmt.Sprintf("Set tags (%d)", selCount)
			deleteLabel = fmt.Sprintf("Delete %d skills", selCount)
			moveLabel = fmt.Sprintf("Move %d skills to group", selCount)
		} else {
			installLabel = fmt.Sprintf("安装 %d 个技能", selCount)
			setTagsLabel = fmt.Sprintf("设置标签（%d）", selCount)
			deleteLabel = fmt.Sprintf("删除 %d 个技能", selCount)
			moveLabel = fmt.Sprintf("移动 %d 个技能到分组", selCount)
		}
	}
	// kitter_manual_enabled: None | Some(false)=restore | Some(true)=set.
	var manualItem *struct {
		Label   string
		Enabled bool
	}
	if !builtIn {
		if skill.Record.KitterManual {
			manualItem = &struct {
				Label   string
				Enabled bool
			}{a.T("还原自动触发", "Restore automatic"), false}
		} else if !skill.ManualOnly {
			manualItem = &struct {
				Label   string
				Enabled bool
			}{a.T("设为仅手动", "Set as manual-only"), true}
		}
	}
	revealPath := skill.Path

	row := ui.Row(c).MinHeight(44).Padding(6, 8).Gap(8).
		AlignItems(ui.Center).Radius(8).Cursor(ui.CursorPointer).
		Key("skill-" + storageName)
	if !builtIn {
		row.Drag(skillDrag{Name: storageName})
	}
	if nested {
		row.Margin(2, 0, 2, 12)
	} else {
		row.Margin(2, 0)
	}
	if selected {
		row.Background(p.Selected)
	}
	row.ContextMenu(func(m *ui.Menu) {
		if m.Item(installLabel).Chosen() {
			a.openInstallDialog()
		}
		if !multiSelection {
			if m.Item(revealLabel).Chosen() {
				mygo.Shell.ShowItemInFolder(revealPath)
			}
			if manualItem != nil {
				item := manualItem
				if m.Item(item.Label).Chosen() {
					a.setSkillKitterManual(c, storageName, item.Enabled)
				}
			}
		}
		if m.Item(setTagsLabel).Chosen() {
			a.openTagAssignmentDialogForSelection(a.Skills.Selection.SelectedIn(a.skillOrder()))
		}
		if !protected {
			if m.Item(moveLabel).Chosen() {
				a.openMoveGroupDialogForSelection(a.Skills.Selection.SelectedIn(a.skillOrder()))
			}
			if m.Item(deleteLabel).Chosen() {
				targets := a.selectedLibraryTargets()
				if len(targets) == 0 {
					return
				}
				a.openLibraryDelete(targets)
			}
		}
	})
	row.Children(func() {
		if a.Skills.Selection.IsMultiple() {
			box := ui.Box(c).Size(18, 18).Shrink(0).Radius(5).
				Key("select-skill-" + storageName).Cursor(ui.CursorPointer)
			if selected {
				box.Background(p.Accent).Children(func() {
					ui.Icon(c, iconSVG("check.svg")).TextColor(p.OnAccent)
				})
			} else {
				box.Border(1, p.BorderStrong)
			}
			if box.Clicked() {
				a.toggleSkillSelection(storageName)
			}
		}
		iconName := "package.svg"
		if builtIn {
			iconName = "crown.svg"
		}
		ui.Icon(c, iconSVG(iconName)).TextColor(p.Secondary).Shrink(0)
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, skill.Record.Name).Font(FontMono).FontSize(13).SingleLine()
			ui.Text(c, a.skillInstallSummary(c, skill)).FontSize(12).TextColor(t.TextMuted).SingleLine()
		})
		a.skillManualBadge(c, skill)
	})
	if row.RightClicked() {
		a.prepareSkillContextSelection(storageName)
	}
	if row.Clicked() {
		a.selectSkillFromClick(storageName, c.Modifiers(), visibleOrder)
	}
}

// selectionIncludesBuiltin reports whether the multi-selection covers the
// protected builtin skill (protected_selection in skill_list_row).
func (a *App) selectionIncludesBuiltin() bool {
	for i := range a.Skills.Items {
		skill := &a.Skills.Items[i]
		if skill.Record.Origin.IsBuiltin() && a.Skills.Selection.Contains(SkillStorageName(skill)) {
			return true
		}
	}
	return false
}

// skillInstallSummary is the muted line under a skill's name.
func (a *App) skillInstallSummary(c *ui.Context, skill *model.SkillSummary) string {
	if home := homeDir(); home != "" && a.skillInstalledAt(c, skill, home) {
		if skill.InstalledProjects == 0 {
			return a.T("已全局安装", "Installed globally")
		}
		if a.UsesEnglish() {
			return fmt.Sprintf("Global · %s", Counted(skill.InstalledProjects, "project", "projects"))
		}
		return fmt.Sprintf("全局 · %d 个项目", skill.InstalledProjects)
	}
	if skill.InstalledProjects == 0 {
		return a.T("尚未安装", "Not installed")
	}
	if a.UsesEnglish() {
		return fmt.Sprintf("Installed in %s", Counted(skill.InstalledProjects, "project", "projects"))
	}
	return fmt.Sprintf("已安装到 %d 个项目", skill.InstalledProjects)
}

// skillManualBadge is KitterApp::skill_manual_badge.
func (a *App) skillManualBadge(c *ui.Context, skill *model.SkillSummary) {
	p := a.Palette()
	var label string
	var color ui.Color
	if skill.Record.KitterManual {
		label = a.T("已设为手动", "Set as manual")
		color = p.Accent
	} else if skill.ManualOnly {
		label = a.T("手动", "Manual")
		color = p.Muted
	} else {
		return
	}
	ui.Row(c).Height(20).Padding(0, 7).Radius(7.5).Background(p.Raised).Shrink(0).
		AlignItems(ui.Center).Gap(4).Children(func() {
		ui.Icon(c, iconSVG("hand.svg")).TextColor(color)
		ui.Text(c, label).FontSize(11).TextColor(color).SingleLine()
	})
}

// tagFilterMenu builds the tag filter menu (tag_filter_control).
func (a *App) tagFilterMenu(m *ui.Menu) {
	allLabel := a.T("全部技能", "All skills")
	if m.Item(allLabel).Checked(!a.HasTagFilter).Chosen() {
		a.HasTagFilter = false
	}
	for _, tag := range a.SkillTags.Roots() {
		a.tagFilterItem(m, tag, false)
		for _, child := range a.SkillTags.Children(tag.ID) {
			a.tagFilterItem(m, child, true)
		}
	}
}

func (a *App) tagFilterItem(m *ui.Menu, tag *tags.Tag, _ bool) {
	id := tag.ID
	label := fmt.Sprintf("#%s  %d", tag.Name, a.SkillTags.Count(tag.ID))
	if m.Item(label).Checked(a.HasTagFilter && a.SelectedTagFilter == id).Chosen() {
		a.HasTagFilter = true
		a.SelectedTagFilter = id
	}
}

// skillDetail is KitterApp::skill_detail.
func (a *App) skillDetail(c *ui.Context, t *ui.Theme) {
	p := a.Palette()
	if a.Skills.Selection.IsMultiple() && a.Skills.Selection.Len() > 1 {
		a.multiSkillDetail(c)
		return
	}
	skill := a.SelectedSkill()
	if skill == nil {
		ui.Column(c).Grow(1).Center().Children(func() {
			ui.Text(c, a.T("添加第一个技能开始使用 Kitter", "Add your first skill to get started")).
				TextColor(t.TextMuted)
		})
		return
	}
	storageName := SkillStorageName(skill)

	ui.Column(c).Grow(1).MinWidth(0).Fill().Children(func() {
		// Header: icon, name, origin, actions, description, tag labels.
		ui.Box(c).Padding(24, 24, 16, 24).Children(func() {
			ui.Row(c).AlignItems(ui.Start).Children(func() {
				ui.Box(c).Size(40, 40).Radius(8).Background(p.Raised).Shrink(0).Center().Children(func() {
					ui.Icon(c, iconSVG("package.svg")).TextColor(p.Secondary)
				})
				ui.Column(c).Grow(1).MinWidth(0).Margin(0, 0, 0, 14).Children(func() {
					ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
						ui.Text(c, skill.Record.Name).Font(FontMono).FontSize(18).Bold().Selectable()
						a.skillManualBadge(c, skill)
					})
					ui.Text(c, a.sourceLabel(&skill.Record.Origin)).FontSize(12).
						TextColor(p.Muted).Margin(4, 0, 0, 0).Selectable()
				})
				ui.Row(c).Gap(8).Children(func() {
					if skill.Record.UpdateAvailable {
						if ui.Icon(c, iconSVG("rotate-cw.svg")).Size(30, 30).TextColor(p.Secondary).
							Background(p.Raised).Radius(8).Cursor(ui.CursorPointer).
							Label(a.T("更新", "Update")).Tooltip(a.T("更新", "Update")).
							Key("update-skill").Clicked() {
							a.updateSkill(storageName)
						}
					}
					if ui.Icon(c, iconSVG("trash.svg")).Size(30, 30).TextColor(p.Danger).
						Background(p.DangerSoft).Radius(8).Cursor(ui.CursorPointer).
						Label(a.T("删除技能", "Delete Skill")).Tooltip(a.T("删除技能", "Delete Skill")).
						Key("delete-skill").Clicked() {
						a.openLibraryDelete([]model.SkillSummary{*skill})
					}
					if ui.Icon(c, iconSVG("download.svg")).Size(30, 30).TextColor(p.Secondary).
						Background(p.Raised).Radius(8).Cursor(ui.CursorPointer).
						Label(a.T("安装技能", "Install Skill")).Tooltip(a.T("安装技能", "Install Skill")).Clicked() {
						a.openInstallDialog()
					}
				})
			})
			description := skill.Record.Description
			if description == "" {
				description = a.T("这个技能暂时没有描述。", "This skill does not have a description yet.")
			}
			ui.Text(c, description).FontSize(14).TextColor(p.Secondary).
				Margin(16, 0, 0, 0).MaxWidth(descriptionMaxW).Selectable()
			a.skillTagLabels(c, storageName)
		})

		// Tabs: Installs / Content.
		ui.Row(c).Height(40).Padding(0, 24).Gap(2).AlignItems(ui.Center).
			Border(1, p.Border).Shrink(0).Children(func() {
			a.detailTab(c, a.T("安装情况", "Installs"), DetailInstalls)
			a.detailTab(c, a.T("内容", "Content"), DetailContent)
		})
		if a.Skills.Tab == DetailInstalls {
			a.installsTab(c, skill)
		} else {
			a.contentTab(c, skill)
		}
	})
}

// detailTab is KitterApp::tab_button.
func (a *App) detailTab(c *ui.Context, label string, tab DetailTab) {
	p := a.Palette()
	active := a.Skills.Tab == tab
	el := ui.Text(c, label).FontSize(13).Height(30).Padding(0, 8).Radius(8).
		Cursor(ui.CursorPointer).Key("detail-tab-" + label)
	if active {
		el.Background(p.Selected).TextColor(p.Text)
	} else {
		el.TextColor(p.Muted)
	}
	if el.Clicked() {
		a.Skills.Tab = tab
	}
}

// skillTagLabels renders the assigned #tag pills (skill_tag_controls).
func (a *App) skillTagLabels(c *ui.Context, storageName string) {
	p := a.Palette()
	assigned := a.SkillTags.AssignedTags(storageName)
	ui.Row(c).Margin(12, 0, 0, 0).Gap(8).Wrap().Children(func() {
		for _, tag := range assigned {
			path, ok := a.SkillTags.Path(tag.ID)
			if !ok {
				continue
			}
			id := tag.ID
			if ui.Text(c, "#"+path).Font(FontMono).FontSize(12).TextColor(p.Secondary).
				Padding(0, 2).Radius(5).Cursor(ui.CursorPointer).
				Key(fmt.Sprintf("skill-tag-%d", id)).Label("#" + path).Clicked() {
				a.HasTagFilter = true
				a.SelectedTagFilter = id
			}
		}
	})
}

// multiSkillDetail is KitterApp::multi_skill_detail.
func (a *App) multiSkillDetail(c *ui.Context) {
	p := a.Palette()
	var names []string
	for i := range a.Skills.Items {
		skill := &a.Skills.Items[i]
		if a.Skills.Selection.Contains(SkillStorageName(skill)) {
			names = append(names, skill.Record.Name)
		}
	}
	ui.Column(c).Grow(1).MinWidth(0).Padding(28, 24, 24, 24).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(40, 40).Radius(8).Background(p.Raised).Center().Children(func() {
				ui.Icon(c, iconSVG("package.svg")).TextColor(p.Secondary)
			})
			ui.Column(c).Grow(1).MinWidth(0).Margin(0, 0, 0, 14).Children(func() {
				var title string
				if a.UsesEnglish() {
					title = fmt.Sprintf("%s selected", Counted(len(names), "skill", "skills"))
				} else {
					title = fmt.Sprintf("已选 %d 个技能", len(names))
				}
				ui.Text(c, title).FontSize(18).Bold()
				ui.Text(c, a.T("批量操作将应用到全部选中项", "Batch actions apply to all selected skills")).
					FontSize(12).TextColor(p.Muted).Margin(4, 0, 0, 0)
			})
		})
		ui.Column(c).Margin(20, 0, 0, 0).Gap(4).Children(func() {
			preview := names
			if len(preview) > 6 {
				preview = preview[:6]
			}
			for _, name := range preview {
				ui.Row(c).Height(30).Padding(0, 10).Radius(8).Gap(8).AlignItems(ui.Center).
					Background(p.Raised).Children(func() {
					ui.Icon(c, iconSVG("package.svg")).TextColor(p.Secondary)
					ui.Text(c, name).Font(FontMono).FontSize(13).SingleLine()
				})
			}
			if len(names) > 6 {
				var more string
				if a.UsesEnglish() {
					more = fmt.Sprintf("and %d more", len(names)-6)
				} else {
					more = fmt.Sprintf("还有 %d 个", len(names)-6)
				}
				ui.Text(c, more).FontSize(12).TextColor(p.Muted).Margin(4, 0, 0, 0)
			}
		})
	})
}

// installsTab is KitterApp::installs_tab.
func (a *App) installsTab(c *ui.Context, skill *model.SkillSummary) {
	p := a.Palette()
	home := homeDir()
	var roots []string
	if home != "" {
		roots = append(roots, home)
	}
	for _, path := range a.Library.Config.ProjectPaths() {
		if path != home {
			roots = append(roots, path)
		}
	}
	a.requestProjectSnapshots(c, roots)

	var projects []string
	for _, root := range roots {
		if a.skillInstalledAt(c, skill, root) {
			projects = append(projects, root)
		}
	}
	ui.Scroll(c).Grow(1).MinHeight(0).Padding(24, 24).Children(func() {
		if len(projects) == 0 {
			ui.Column(c).Margin(34, 0, 0, 0).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconSVG("package.svg")).TextColor(p.Muted)
				ui.Text(c, a.T("尚未安装", "Not installed yet")).FontSize(12).
					TextColor(p.Muted).Margin(10, 0, 0, 0)
			})
			return
		}
		var summary string
		if a.UsesEnglish() {
			summary = fmt.Sprintf("Installed in %s", Counted(len(projects), "location", "locations"))
		} else {
			summary = fmt.Sprintf("已安装到 %d 个位置", len(projects))
		}
		ui.Text(c, summary).FontSize(12).TextColor(p.Muted).Margin(0, 0, 10, 0)
		for _, path := range projects {
			name := filepath.Base(path)
			if path == home {
				name = a.T("全局", "Global")
			}
			installations := a.skillInstallationsAt(c, skill, path)
			var targets []model.InstallTarget
			for _, inst := range installations {
				targets = append(targets, inst.Target)
			}
			projectPath := path
			skillCopy := model.ProjectSkill{Name: skill.Record.Name, Installations: installations}
			ui.Row(c).Height(64).Padding(0, 14).AlignItems(ui.Center).Gap(11).
				Border(1, p.Border).Children(func() {
				ui.Icon(c, iconSVG("folder.svg")).TextColor(p.Secondary).Shrink(0)
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					ui.Text(c, name).Font(FontMono).FontSize(13).Selectable().SingleLine()
					ui.Text(c, displayPath(projectPath)).Font(FontMono).FontSize(12).
						TextColor(p.Muted).Selectable().SingleLine()
				})
				a.agentBadges(c, targets, false)
				if ui.Icon(c, iconSVG("trash.svg")).Size(30, 30).TextColor(p.Danger).
					Background(p.DangerSoft).Radius(8).Margin(0, 0, 0, 10).Cursor(ui.CursorPointer).
					Label(a.T("移除", "Remove")).Tooltip(a.T("移除", "Remove")).
					Key("remove-install-" + projectPath).Clicked() {
					a.openProjectDelete(projectPath, skillCopy)
				}
			})
		}
	})
}

// contentTab is KitterApp::content_tab: a file tree plus the selectable
// file preview.
func (a *App) contentTab(c *ui.Context, skill *model.SkillSummary) {
	p := a.Palette()
	a.requestContentSnapshot(skill)
	if !a.Skills.contentSnapshot || a.Skills.ContentSkill != SkillStorageName(skill) {
		ui.Row(c).Grow(1).Center().Children(func() { ui.Spinner(c) })
		return
	}
	files := a.Skills.ContentFiles

	// Collect every parent directory of every file (content_tab).
	dirs := map[string]bool{}
	for _, file := range files {
		parent := parentPath(file)
		for parent != "" {
			dirs[parent] = true
			parent = parentPath(parent)
		}
	}
	type entry struct {
		path string
		dir  bool
	}
	var entries []entry
	for d := range dirs {
		entries = append(entries, entry{d, true})
	}
	for _, f := range files {
		entries = append(entries, entry{f, false})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		li := strings.ToLower(entries[i].path)
		lj := strings.ToLower(entries[j].path)
		if li != lj {
			return li < lj
		}
		return entries[i].dir && !entries[j].dir // dirs first on ties
	})

	ui.Row(c).Grow(1).MinHeight(0).MinWidth(0).Children(func() {
		tree := ui.Scroll(c).Width(220).Shrink(0).MinHeight(0).Padding(8, 0)
		tree.Background(p.Surface)
		tree.Children(func() {
			for _, e := range entries {
				path := e.path
				// Entries hidden by a collapsed ancestor are skipped.
				hidden := false
				start := parentPath(path)
				for dir := start; dir != ""; dir = parentPath(dir) {
					if a.Skills.CollapsedContentDirs[dir] {
						hidden = true
						break
					}
				}
				if hidden {
					continue
				}
				if e.dir {
					depth := pathDepth(path) - 1
					collapsed := a.Skills.CollapsedContentDirs[path]
					chevron := "chevron-down.svg"
					folder := "folder-open.svg"
					if collapsed {
						chevron = "chevron-right.svg"
						folder = "folder.svg"
					}
					dirRow := ui.Row(c).Height(36).Margin(0, 6).Padding(0, 9, 0, 3+float32(depth)*14).
						Radius(7).Gap(6).AlignItems(ui.Center).Cursor(ui.CursorPointer).
						Key("directory-" + path)
					dirRow.Children(func() {
						ui.Icon(c, iconSVG(chevron)).TextColor(p.Muted).Size(14, 14).Shrink(0)
						ui.Icon(c, iconSVG(folder)).TextColor(p.Secondary).Size(18, 18).Shrink(0)
						ui.Text(c, baseName(path)).Font(FontMono).FontSize(12).SingleLine().Grow(1).MinWidth(0)
					})
					if dirRow.Clicked() {
						if a.Skills.CollapsedContentDirs[path] {
							delete(a.Skills.CollapsedContentDirs, path)
						} else {
							a.Skills.CollapsedContentDirs[path] = true
						}
					}
					continue
				}
				depth := pathDepth(parentPath(path))
				selected := path == a.Skills.SelectedFile
				row := ui.Row(c).Height(30).Margin(0, 6).Padding(0, 9, 0, 3+float32(depth)*14).
					Radius(7).Gap(6).AlignItems(ui.Center).Cursor(ui.CursorPointer).
					Label("file-" + path)
				if selected {
					row.Background(p.Selected)
				}
				row.OnClick(func() {
					a.Skills.SelectedFile = path
					a.Skills.contentSnapshot = false
					a.Skills.contentGen++
					a.Skills.ContentScroll.X, a.Skills.ContentScroll.Y = 0, 0
				})
				row.Children(func() {
					ui.Box(c).Size(14, 14).Shrink(0)
					ui.Icon(c, iconSVG("file.svg")).TextColor(p.Muted).Size(18, 18).Shrink(0)
					ui.Text(c, baseName(path)).Font(FontMono).FontSize(12).SingleLine().Grow(1).MinWidth(0)
				})
			}
		})
		ui.Box(c).Width(1).Shrink(0).Background(p.Border)
		ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
			ui.Row(c).Height(40).Padding(0, 14).Shrink(0).AlignItems(ui.Center).
				Background(p.Surface).Border(1, p.Border).Children(func() {
				ui.Text(c, a.Skills.SelectedFile).Font(FontMono).FontSize(12).
					TextColor(p.Secondary).Selectable().SingleLine()
			})
			ui.Scroll(c).Grow(1).MinHeight(0).TrackScroll(&a.Skills.ContentScroll).Padding(22).Children(func() {
				ui.Text(c, a.Skills.ContentText).Font(FontMono).FontSize(13).
					TextColor(p.Secondary).Selectable().Wrap()
			})
		})
	})
}

// parentPath / pathDepth / baseName treat skill-relative paths as
// slash-separated, matching Rust's PathBuf handling of relative paths.
func parentPath(path string) string {
	idx := strings.LastIndex(path, "/")
	if idx <= 0 {
		return ""
	}
	return path[:idx]
}

func pathDepth(path string) int {
	if path == "" {
		return 0
	}
	return strings.Count(path, "/") + 1
}

func baseName(path string) string {
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[idx+1:]
	}
	return path
}

// sourceLabel is KitterApp::source_label.
func (a *App) sourceLabel(origin *model.SkillOrigin) string {
	source := origin.Source()
	switch {
	case source.Type == "unknown":
		return a.T("本地技能", "Local skill")
	case source.Type == "local" && baseName(source.Path) == source.Path:
		return a.T("本地导入", "Local import")
	default:
		return source.Label()
	}
}

// agentBadges is KitterApp::agent_badges + render_agent_badges: up to
// five overlapping provider icons, then a "+N" count.
func (a *App) agentBadges(c *ui.Context, targets []model.InstallTarget, includeGlobalOnly bool) {
	var visible []agents.AgentIconInfo
	for _, agent := range agents.AgentIconOrder {
		if agent.GlobalOnly {
			if includeGlobalOnly {
				visible = append(visible, agent)
			}
			continue
		}
		for _, target := range targets {
			if agent.SupportsTarget(target) {
				visible = append(visible, agent)
				break
			}
		}
	}
	if len(visible) == 0 {
		ui.Box(c).Width(0)
		return
	}
	shown := visible
	hidden := 0
	if len(shown) > 5 {
		hidden = len(shown) - 5
		shown = shown[:5]
	}
	p := a.Palette()
	ui.Row(c).Shrink(0).AlignItems(ui.Center).Children(func() {
		for i, agent := range shown {
			icon := ui.Icon(c, iconSVG(agent.IconPath)).
				Size(25, 25).TextColor(ui.RGB(0x33, 0x33, 0x33)).
				Tooltip(agent.Name).Label(agent.Name)
			if i > 0 {
				icon.Margin(0, 0, 0, -5)
			}
		}
		if hidden > 0 {
			ui.Text(c, fmt.Sprintf("+%d", hidden)).Font(FontMono).FontSize(9).
				TextColor(p.Muted).Margin(0, 0, 0, -5).
				Label(fmt.Sprintf("+%d agents", hidden))
		}
	})
}

// deleteModal renders the DeleteConfirmation modal for both library and
// project deletions.
func (a *App) deleteModal(c *ui.Context) {
	if !a.Skills.DeleteOpen {
		return
	}
	p := a.Palette()
	ui.Modal(c, &a.Skills.DeleteOpen, func() {
		panel := ui.Column(c).Width(460).Background(p.Elevated).Radius(12).Clip()
		panel.Children(func() {
			ui.Box(c).Padding(20).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Children(func() {
					ui.Text(c, a.deleteTitle()).FontSize(16).Bold().Grow(1).MinWidth(0)
					if ui.Icon(c, iconSVG("x.svg")).Size(30, 30).TextColor(p.Text).
						Background(p.Raised).Radius(8).Cursor(ui.CursorPointer).
						Key("close-delete-modal").Clicked() {
						a.Skills.DeleteOpen = false
						a.Skills.DeleteSkills = nil
						a.Skills.DeleteProjectSkill = nil
					}
				})
				ui.Text(c, a.deleteMessage()).FontSize(14).TextColor(p.Secondary).
					Margin(10, 0, 0, 0).Wrap()
				if location := a.deleteLocation(); location != "" {
					ui.Text(c, location).Font(FontMono).FontSize(12).TextColor(p.Secondary).
						Padding(9, 11).Radius(8).Background(p.Raised).Margin(10, 0, 0, 0).Wrap()
				}
				if a.Skills.DeleteProjectSkill != nil {
					ui.Text(c, a.T("选择要移除的位置", "Choose installations to remove")).
						FontSize(13).TextColor(p.Secondary).Margin(16, 0, 6, 0)
					unique := uniqueInstallationPaths(a.Skills.DeleteProjectSkill.Installations)
					for _, inst := range a.Skills.DeleteProjectSkill.Installations {
						if !containsString(unique, inst.Path) {
							continue
						}
						path := inst.Path
						checked := a.Skills.DeleteSelected[path]
						check := ui.Icon(c, iconSVG("check.svg")).Size(16, 16).Shrink(0).TextColor(p.Accent)
						if !checked {
							check.Invisible()
						}
						row := ui.Row(c).MinHeight(48).Padding(0, 11).Margin(6, 0, 0, 0).
							Radius(12).Border(1, p.Border).Background(p.Surface).
							AlignItems(ui.Center).Cursor(ui.CursorPointer).
							Key("delete-path-" + path)
						row.Children(func() {
							ui.Column(c).MinWidth(0).Margin(0, 0, 0, 10).Children(func() {
								ui.Text(c, agents.TargetDirectory(inst.Target)).FontSize(13)
								ui.Text(c, displayPath(path)).Font(FontMono).FontSize(11).
									TextColor(p.Muted).SingleLine()
							})
						})
						if row.Clicked() {
							if a.Skills.DeleteSelected[path] {
								delete(a.Skills.DeleteSelected, path)
							} else {
								a.Skills.DeleteSelected[path] = true
							}
						}
					}
				}
			})
			ui.Row(c).Height(60).Padding(0, 18).AlignItems(ui.Center).Justify(ui.End).Gap(8).
				Border(1, p.Border).Children(func() {
				if ui.Text(c, a.T("取消", "Cancel")).Height(34).Padding(0, 16).Radius(8).
					FontSize(14).Cursor(ui.CursorPointer).Key("cancel-delete").Clicked() {
					a.Skills.DeleteOpen = false
					a.Skills.DeleteSkills = nil
					a.Skills.DeleteProjectSkill = nil
				}
				canConfirm := !a.Skills.DeleteBusy &&
					(a.Skills.DeleteProjectSkill == nil || len(a.Skills.DeleteSelected) > 0)
				label := a.deleteAction()
				btn := ui.Text(c, label).Height(34).Padding(0, 16).Radius(8).FontSize(14).
					Key("confirm-delete")
				if canConfirm {
					btn.Background(p.Danger).TextColor(p.OnAccent).Cursor(ui.CursorPointer)
					if btn.Clicked() {
						if a.Skills.DeleteProjectSkill != nil {
							a.deleteProjectSkill(c)
						} else {
							a.deleteLibrarySkills(c)
						}
					}
				} else {
					btn.Background(p.DangerSoft).TextColor(p.Muted)
				}
			})
		})
	})
}

// deleteTitle/Message/Action mirror delete_confirmation_modal.
func (a *App) deleteTitle() string {
	if a.Skills.DeleteProjectSkill != nil {
		global := a.Skills.DeleteProject != "" && a.Skills.DeleteProject == homeDir()
		name := a.Skills.DeleteProjectSkill.Name
		switch {
		case global && a.UsesEnglish():
			return fmt.Sprintf("Remove “%s” from user-level locations?", name)
		case global:
			return fmt.Sprintf("从用户级目录中移除「%s」？", name)
		case a.UsesEnglish():
			return fmt.Sprintf("Remove “%s” from this project?", name)
		default:
			return fmt.Sprintf("从这个项目中移除「%s」？", name)
		}
	}
	var labels []string
	for _, skill := range a.Skills.DeleteSkills {
		labels = append(labels, skill.Record.Name)
	}
	label := strings.Join(labels, ", ")
	if len(labels) == 1 {
		if a.UsesEnglish() {
			return fmt.Sprintf("Delete “%s”?", label)
		}
		return fmt.Sprintf("删除「%s」？", label)
	}
	if a.UsesEnglish() {
		return fmt.Sprintf("Delete %d skills?", len(labels))
	}
	return fmt.Sprintf("删除 %d 个技能？", len(labels))
}

func (a *App) deleteMessage() string {
	if a.Skills.DeleteProjectSkill != nil {
		global := a.Skills.DeleteProject != "" && a.Skills.DeleteProject == homeDir()
		var kinds []project.RemovalKind
		var selected []model.ProjectSkillInstallation
		for i := range a.Skills.DeleteProjectSkill.Installations {
			inst := &a.Skills.DeleteProjectSkill.Installations[i]
			if a.Skills.DeleteSelected[inst.Path] {
				kinds = append(kinds, project.RemovalKindOf(inst))
				selected = append(selected, *inst)
			}
		}
		deletesFiles := false
		for _, k := range kinds {
			if k == project.RemovalSourceFiles {
				deletesFiles = true
			}
		}
		external := uniqueExternalSources(kinds, selected)
		switch {
		case deletesFiles && global:
			return a.T("所选用户级安装中包含直接保存的技能，确认后这些文件会被删除，且无法恢复。",
				"Some selected user-level installations are stored directly there. Their files will be deleted and cannot be restored.")
		case deletesFiles:
			return a.T("所选位置中包含直接保存在项目里的技能，确认后这些文件会被删除，且无法恢复。",
				"Some selected installations are stored directly in the project. Their files will be deleted and cannot be restored.")
		case len(external) > 0 && global:
			return a.T("只会移除选中的用户级安装，原始技能文件不会受到影响。",
				"Only the selected user-level installations will be removed. The original skill files will not be affected.")
		case len(external) > 0:
			return a.T("只会移除这个项目中的安装，原始技能文件不会受到影响。",
				"Only the project installations will be removed. The original skill files will not be affected.")
		case global:
			return a.T("只会移除选中的用户级安装。Kitter 中保存的技能不会被删除，你之后仍可以再次安装。",
				"Only the selected user-level installations will be removed. The skill saved in Kitter will remain available to install again.")
		default:
			return a.T("只会移除这个项目中的安装。Kitter 中保存的技能不会被删除，你之后仍可以再次安装。",
				"Only the project installations will be removed. The skill saved in Kitter will remain available to install again.")
		}
	}
	linked := 0
	for _, skill := range a.Skills.DeleteSkills {
		if a.Library.IsLinkedSource(SkillStorageName(&skill)) {
			linked++
		}
	}
	total := len(a.Skills.DeleteSkills)
	switch {
	case linked == total && total > 0:
		return a.T("仅移除 Kitter 托管与安装链接，原始源目录保留。",
			"Remove Kitter entries and installation links. Original source folders are kept.")
	case linked > 0:
		return a.T("托管副本与安装将被删除；链接来源的原始目录保留。",
			"Managed copies and installations will be removed. Linked source folders are kept.")
	default:
		return a.T("这会从 Kitter 中删除这些技能，同时移除它们在项目中的安装。以下位置的文件也会被删除，且无法恢复：",
			"This removes these skills from Kitter and from projects where they are installed. Files at these locations will also be deleted and cannot be restored:")
	}
}

func (a *App) deleteLocation() string {
	if a.Skills.DeleteProjectSkill != nil {
		var kinds []project.RemovalKind
		var selected []model.ProjectSkillInstallation
		for i := range a.Skills.DeleteProjectSkill.Installations {
			inst := &a.Skills.DeleteProjectSkill.Installations[i]
			if a.Skills.DeleteSelected[inst.Path] {
				kinds = append(kinds, project.RemovalKindOf(inst))
				selected = append(selected, *inst)
			}
		}
		return strings.Join(uniqueExternalSources(kinds, selected), "\n")
	}
	var paths []string
	for _, skill := range a.Skills.DeleteSkills {
		paths = append(paths, displayPath(skill.Path))
	}
	return strings.Join(paths, "\n")
}

func (a *App) deleteAction() string {
	if a.Skills.DeleteProjectSkill != nil {
		for i := range a.Skills.DeleteProjectSkill.Installations {
			inst := &a.Skills.DeleteProjectSkill.Installations[i]
			if a.Skills.DeleteSelected[inst.Path] &&
				project.RemovalKindOf(inst) == project.RemovalSourceFiles {
				return a.T("删除", "Delete")
			}
		}
		return a.T("移除", "Remove")
	}
	return a.T("删除", "Delete")
}

func containsString(list []string, item string) bool {
	for _, s := range list {
		if s == item {
			return true
		}
	}
	return false
}

// iconSVG wraps IconSVG for a friendlier call site.
func iconSVG(name string) *ui.SVG { return IconSVG(name) }
