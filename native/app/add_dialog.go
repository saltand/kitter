// add_dialog.go renders the AddSkill modal, porting
// src/ui/add_flow.rs::render_add_dialog: kind selector, input row,
// scan results with per-skill checkboxes, grouping controls, the
// adoption list, error text and action buttons.
package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

func (a *App) addDialog(c *ui.Context) {
	if !a.AddFlow.Open {
		return
	}
	p := a.Palette()
	busy := a.addBusy()
	ui.Modal(c, &a.AddFlow.Open, func() {
		panel := ui.Column(c).Width(640).Height(560).Background(p.Elevated).Radius(12).Clip()
		panel.Children(func() {
			ui.Box(c).Padding(20).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Children(func() {
					ui.Text(c, a.T("添加技能", "Add Skill")).FontSize(16).Bold().Grow(1)
					if iconButton(c, "x.svg", 30, 14).TextColor(p.Text).
						Background(p.Raised).Radius(8).Cursor(ui.CursorPointer).
						Key("add-close").Clicked() {
						a.closeAddModal()
					}
				})
				ui.Spacer(c).Height(16)
				a.addKindSelector(c)
				ui.Spacer(c).Height(12)
				a.addInputRow(c)
				ui.Spacer(c).Height(8)
				a.addOptionsRow(c)
				ui.Spacer(c).Height(8)
				a.addResultArea(c)
				if a.AddFlow.Error != "" {
					ui.Text(c, a.AddFlow.Error).FontSize(12).TextColor(p.Danger).Wrap()
					ui.Spacer(c).Height(8)
				}
			})
			ui.Row(c).Height(60).Padding(0, 18).AlignItems(ui.Center).Justify(ui.End).Gap(8).
				Border(1, p.Border).Children(func() {
				if ui.Text(c, a.T("取消", "Cancel")).Height(34).Padding(0, 16).Radius(8).
					FontSize(14).Cursor(ui.CursorPointer).Key("add-cancel").Clicked() {
					a.closeAddModal()
				}
				label := a.T("添加", "Add")
				if a.AddFlow.Kind == AddExisting {
					label = a.T("托管", "Adopt")
				}
				enabled := !busy && len(a.AddFlow.Selected) > 0 &&
					(a.AddFlow.Scan != nil || a.AddFlow.AdoptionScan != nil)
				btn := ui.Text(c, label).Height(34).Padding(0, 16).Radius(8).FontSize(14).Key("add-confirm")
				if enabled {
					btn.Background(p.Accent).TextColor(p.OnAccent).Cursor(ui.CursorPointer)
					if btn.Clicked() {
						a.importScannedSkills()
					}
				} else {
					btn.Background(p.Raised).TextColor(p.Muted)
				}
			})
		})
	})
}

// addKindSelector is the horizontal kind tabs.
func (a *App) addKindSelector(c *ui.Context) {
	p := a.Palette()
	kinds := []struct {
		kind  AddKind
		label string
	}{
		{AddLocal, a.T("本地文件夹", "Local folder")},
		{AddNpx, "skills.sh / GitHub"},
		{AddClaude, a.T("Claude 插件", "Claude plugin")},
		{AddExisting, a.T("现有技能", "Existing skills")},
	}
	ui.Row(c).Gap(8).Children(func() {
		for _, k := range kinds {
			k := k
			active := a.AddFlow.Kind == k.kind
			tab := ui.Text(c, k.label).Height(30).Padding(0, 12).Radius(8).FontSize(13)
			if active {
				tab.Background(p.Accent).TextColor(p.OnAccent)
			} else {
				tab.Background(p.Raised).TextColor(p.Text).Cursor(ui.CursorPointer)
				if tab.Clicked() {
					a.setAddKind(k.kind)
				}
			}
		}
	})
}

// addInputRow is the source input (Local path + Browse, or text input +
// Scan, or the Existing trigger).
func (a *App) addInputRow(c *ui.Context) {
	p := a.Palette()
	busy := a.addBusy()
	switch a.AddFlow.Kind {
	case AddLocal:
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.TextInput(c, &a.AddFlow.PrimaryInput).
				Grow(1).Height(34).FontSize(13).Label("add-local-path")
			browse := ui.Text(c, a.T("浏览…", "Browse…")).Height(34).Padding(0, 12).Radius(8).
				FontSize(13).Key("add-browse")
			if !busy {
				browse.Background(p.Raised).TextColor(p.Text).Cursor(ui.CursorPointer)
				if browse.Clicked() {
					a.browseAddLocal()
				}
			} else {
				browse.Background(p.Raised).TextColor(p.Muted)
			}
		})
	case AddNpx, AddClaude:
		placeholder := a.T("skills.sh 或 GitHub 地址", "skills.sh or GitHub URL")
		if a.AddFlow.Kind == AddClaude {
			placeholder = a.T("Claude 插件名称", "Claude plugin name")
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			input := ui.TextInput(c, &a.AddFlow.PrimaryInput).
				Grow(1).Height(34).FontSize(13).Label("add-source-input")
			input.Placeholder(placeholder)
			scan := ui.Text(c, a.T("扫描", "Scan")).Height(34).Padding(0, 12).Radius(8).
				FontSize(13).Key("add-scan")
			if !busy {
				scan.Background(p.Raised).TextColor(p.Text).Cursor(ui.CursorPointer)
				if scan.Clicked() {
					a.scanAddSource()
				}
			} else {
				scan.Background(p.Raised).TextColor(p.Muted)
			}
		})
	case AddExisting:
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.T("扫描现有技能来源", "Scan existing skill sources")).
				FontSize(13).TextColor(p.Secondary).Grow(1)
			label := a.T("扫描", "Scan")
			if a.AddFlow.AdoptionRoot != "" {
				label = a.T("重新扫描", "Rescan")
			}
			btn := ui.Text(c, label).Height(34).Padding(0, 12).Radius(8).FontSize(13).Key("add-existing-scan")
			if !busy {
				btn.Background(p.Raised).TextColor(p.Text).Cursor(ui.CursorPointer)
				if btn.Clicked() {
					a.scanExistingSkills()
				}
			} else {
				btn.Background(p.Raised).TextColor(p.Muted)
			}
			if a.AddFlow.AdoptionScan != nil || a.AddFlow.AdoptionRoot != "" {
				browse := ui.Text(c, a.T("更改来源…", "Change source…")).Height(34).Padding(0, 12).
					Radius(8).FontSize(13).Key("add-existing-browse")
				if !busy {
					browse.Background(p.Raised).TextColor(p.Text).Cursor(ui.CursorPointer)
					if browse.Clicked() {
						a.browseAdoptionRoot()
					}
				} else {
					browse.Background(p.Raised).TextColor(p.Muted)
				}
			}
		})
	}
}

// addOptionsRow is the grouping controls (source kinds only).
func (a *App) addOptionsRow(c *ui.Context) {
	if a.AddFlow.Kind == AddExisting {
		return
	}
	busy := a.addBusy()
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Checkbox(c, &a.AddFlow.GroupEnabled, a.T("放入分组", "Group skills")).
			Disabled(busy)
		input := ui.TextInput(c, &a.AddFlow.GroupName).Grow(1).Height(30).
			FontSize(13).Label("add-group-name")
		input.Placeholder(a.T("分组名称", "Group name"))
		if !a.AddFlow.GroupEnabled || busy {
			input.Invisible()
		}
	})
}

// addResultArea is the scrollable scan-result/adoption-list region.
func (a *App) addResultArea(c *ui.Context) {
	p := a.Palette()
	if a.AddFlow.Kind == AddExisting {
		a.adoptionListView(c)
		return
	}
	if a.AddFlow.Scan == nil {
		return
	}
	skills := a.AddFlow.Scan.Skills()
	ui.Row(c).AlignItems(ui.Center).Margin(0, 0, 4, 0).Children(func() {
		ui.Text(c, fmt.Sprintf("%d", len(skills))).FontSize(12).TextColor(p.Muted)
		ui.Spacer(c).Width(6)
		ui.Text(c, a.T("找到技能", "Found skills")).FontSize(12).TextColor(p.Secondary)
	})
	ui.Box(c).MaxHeight(240).Grow(1).MinHeight(0).Children(func() {
		ui.Scroll(c).Children(func() {
			for i := range skills {
				skill := skills[i]
				row := ui.Row(c).Height(30).AlignItems(ui.Center).Gap(8)
				row.Children(func() {
					check := "[ ]"
					if a.AddFlow.Selected[skill.Name] {
						check = "[x]"
					}
					ui.Text(c, check).FontSize(13)
					ui.Text(c, skill.Name).FontSize(13).Grow(1).MinWidth(0).SingleLine()
					ui.Text(c, shortenSource(skill.Path)).FontSize(11).TextColor(p.Muted).SingleLine()
				})
				if row.Clicked() {
					if a.AddFlow.Selected[skill.Name] {
						delete(a.AddFlow.Selected, skill.Name)
					} else {
						a.AddFlow.Selected[skill.Name] = true
					}
				}
			}
		})
	})
}

// adoptionListView renders the virtualized adoption list.
func (a *App) adoptionListView(c *ui.Context) {
	rows := a.AddFlow.adoptionRows
	ui.List(c, &a.AddFlow.adoptionList, len(rows), func(pos int) {
		a.adoptionRow(c, pos)
	}).Grow(1).MinHeight(0)
}
