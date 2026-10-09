// install_flow.go ports src/ui/install_flow.rs: the install modal
// (project select, global/project toggle, target grid) plus the
// install execution from add_actions.rs::install_selected.
package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/saltand/kitter/native/core/agents"
	"github.com/saltand/kitter/native/core/model"
	"github.com/saltand/kitter/native/core/project"
)

// openInstallDialog is open_install_dialog (flows.rs:89): modal on,
// global mirrors the current view, targets cleared (the user picks).
func (a *App) openInstallDialog() {
	a.InstallFlow.Modal = true
	a.InstallFlow.Global = a.Page == PageProjects && a.Projects.GlobalProjectView
	a.InstallFlow.SelectedTargets = map[model.InstallTarget]bool{}
}

// toggleInstallTarget is toggle_install_target.
func (a *App) toggleInstallTarget(target model.InstallTarget) {
	if a.InstallFlow.SelectedTargets[target] {
		delete(a.InstallFlow.SelectedTargets, target)
	} else {
		a.InstallFlow.SelectedTargets[target] = true
	}
}

// installHasTarget is the install button enabled check.
func (a *App) installHasTarget() bool {
	if len(a.InstallFlow.SelectedTargets) == 0 {
		return false
	}
	if a.InstallFlow.Global {
		return homeDir() != ""
	}
	return a.Projects.OpenProject != ""
}

// installSelected is add_actions.rs::install_selected.
func (a *App) installSelected(c *ui.Context) {
	var projectPath string
	if a.InstallFlow.Global {
		projectPath = homeDir()
	} else {
		projectPath = a.Projects.OpenProject
	}
	if projectPath == "" {
		a.showNotice(c, a.T("请先选择项目", "Choose a project first"))
		return
	}
	selected := a.Skills.Selection.SelectedIn(a.skillOrder())
	if len(selected) == 0 {
		return
	}
	var targets []model.InstallTarget
	for _, target := range agents.ProjectInstallTargets {
		if a.InstallFlow.SelectedTargets[target] {
			targets = append(targets, target)
		}
	}
	installed := 0
	var failures []string
	for _, storageName := range selected {
		var skill *model.SkillSummary
		for i := range a.Skills.Items {
			if SkillStorageName(&a.Skills.Items[i]) == storageName {
				skill = &a.Skills.Items[i]
				break
			}
		}
		if skill == nil {
			continue
		}
		if err := project.InstallFromPath(projectPath, skill.Path, skill.Record.Name, targets); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %s", skill.Record.Name, a.ErrorMessage(err)))
		} else {
			installed++
		}
	}
	if installed > 0 && !a.InstallFlow.Global {
		a.Library.Config.TouchProject(projectPath)
		_ = a.Library.Save()
	}
	if installed > 0 {
		// A global installation contributes to every project's effective
		// skills, so invalidate all derived scans before redrawing.
		a.clearContextEstimates()
	}
	a.InstallFlow.Modal = false
	var summary string
	if len(failures) == 0 {
		if a.UsesEnglish() {
			summary = fmt.Sprintf("Installed %s", Counted(installed, "skill", "skills"))
		} else if installed == 1 {
			summary = "已安装"
		} else {
			summary = fmt.Sprintf("已安装 %d 个技能", installed)
		}
	} else if a.UsesEnglish() {
		summary = fmt.Sprintf("Installed %d; %d failed", installed, len(failures))
	} else {
		summary = fmt.Sprintf("已安装 %d 个，%d 个失败", installed, len(failures))
	}
	a.refreshProjectAndEstimate(projectPath)
	a.ReloadSkills()
	if len(failures) == 0 {
		a.showNotice(c, summary)
	} else {
		a.showNotice(c, summary+"\n"+joinLines(failures))
	}
}

// installDialog is install_modal.
func (a *App) installDialog(c *ui.Context) {
	if !a.InstallFlow.Modal {
		return
	}
	p := a.Palette()
	home := homeDir()

	ui.Modal(c, &a.InstallFlow.Modal, func() {
		ui.Column(c).Width(440).Background(p.Elevated).Radius(12).Clip().Children(func() {
			// Header.
			ui.Row(c).Padding(14, 16).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.T("安装技能", "Install Skill")).FontSize(16).Bold().Grow(1)
				if iconButton(c, "x.svg", 30, 14).TextColor(p.Text).
					Background(p.Raised).Radius(8).Cursor(ui.CursorPointer).
					Key("install-close").Clicked() {
					a.InstallFlow.Modal = false
				}
			})
			ui.Box(c).Height(1).Background(p.Border)

			ui.Box(c).Padding(16).Children(func() {
				// Scope toggle.
				ui.Row(c).Gap(8).Children(func() {
					a.installScopeButton(c, false, a.T("项目", "Project"), "install-scope-project")
					a.installScopeButton(c, true, a.T("全局", "Global"), "install-scope-global")
				})
				ui.Spacer(c).Height(14)

				// Project select.
				ui.Text(c, a.T("项目", "Project")).FontSize(12).TextColor(p.Secondary)
				ui.Spacer(c).Height(6)
				a.installProjectRow(c, home)
				ui.Spacer(c).Height(14)

				// Target grid.
				ui.Text(c, a.T("安装到", "Install to")).FontSize(12).TextColor(p.Secondary)
				ui.Spacer(c).Height(8)
				a.installTargetGrid(c)
			})

			ui.Box(c).Height(1).Background(p.Border)
			// Footer.
			ui.Row(c).Padding(14, 16).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.T("全局安装会同步到所有项目视图", "Global installs apply to every project view")).
					FontSize(11).TextColor(p.Muted).Grow(1)
				if ui.Text(c, a.T("取消", "Cancel")).FontSize(12).Padding(6, 14).Radius(8).
					Border(1, p.Border).Cursor(ui.CursorPointer).Key("install-cancel").Clicked() {
					a.InstallFlow.Modal = false
				}
				enabled := a.installHasTarget()
				btn := ui.Text(c, a.T("安装", "Install")).FontSize(12).Padding(6, 16).Radius(8).
					Key("install-confirm")
				if enabled {
					btn.Background(p.Accent).TextColor(p.OnAccent).Cursor(ui.CursorPointer)
				} else {
					btn.Background(p.Raised).TextColor(p.Muted)
				}
				btn.OnClick(func() {
					if a.installHasTarget() {
						a.installSelected(c)
					}
				})
			})
		})
	})
}

// installScopeButton is the Global/Project toggle in install_modal.
func (a *App) installScopeButton(c *ui.Context, global bool, label, key string) {
	p := a.Palette()
	selected := a.InstallFlow.Global == global
	el := ui.Text(c, label).FontSize(12).Padding(5, 12).Radius(8).Cursor(ui.CursorPointer).Key(key)
	if selected {
		el.Background(p.Selected).TextColor(p.Text)
	} else {
		el.Background(p.Raised).TextColor(p.Muted)
	}
	el.OnClick(func() {
		a.InstallFlow.Global = global
	})
}

// installProjectRow renders the project combobox equivalent: the
// current selection plus browse + recent choices.
func (a *App) installProjectRow(c *ui.Context, home string) {
	p := a.Palette()
	current := a.Projects.OpenProject
	ui.Row(c).Height(36).Padding(0, 10).Radius(8).Border(1, p.Border).
		AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Icon(c, iconSVG("folder.svg")).TextColor(p.Secondary).Shrink(0)
		label := a.T("选择项目", "Choose a project")
		if current != "" {
			label = displayPath(current)
		}
		ui.Text(c, label).FontSize(13).Font(FontMono).Grow(1).SingleLine().
			TextColor(p.Text)
		if iconButton(c, "ellipsis.svg", 20, 14).TextColor(p.Muted).
			Cursor(ui.CursorPointer).Key("install-browse-project").Clicked() {
			a.browseProject()
		}
	})
	ui.Spacer(c).Height(6)
	ui.Column(c).Children(func() {
		for _, path := range a.Library.Config.ProjectPaths() {
			path := path
			selected := path == current
			row := ui.Row(c).Height(28).Padding(0, 10).AlignItems(ui.Center).Gap(8).
				Cursor(ui.CursorPointer).Radius(6).
				Key("install-project-" + path).
				Children(func() {
					ui.Text(c, displayPath(path)).FontSize(12).Font(FontMono).Grow(1).SingleLine()
					if selected {
						ui.Icon(c, iconSVG("check.svg")).TextColor(p.Accent)
					}
				})
			if selected {
				row.Background(p.Selected)
			}
			row.OnClick(func() {
				a.Projects.OpenProject = path
			})
		}
	})
}

// installTargetGrid is target_group: the 3-column AGENT_ICON_ORDER
// grid filtered by supports_target(Universal).
func (a *App) installTargetGrid(c *ui.Context) {
	p := a.Palette()
	var flat []agents.AgentIconInfo
	for _, info := range agents.AgentIconOrder {
		if info.SupportsTarget(model.TargetUniversal) {
			flat = append(flat, info)
		}
	}
	const cols = 3
	for i := 0; i < len(flat); i += cols {
		end := i + cols
		if end > len(flat) {
			end = len(flat)
		}
		row := flat[i:end]
		ui.Row(c).Gap(6).Margin(0, 0, 6, 0).Children(func() {
			for _, info := range row {
				info := info
				target := info.InstallTargets[0]
				selected := a.InstallFlow.SelectedTargets[target]
				tile := ui.Column(c).Basis(0).Grow(1).Padding(8, 10).Gap(4).
					Radius(10).Border(1, p.Border).Cursor(ui.CursorPointer).
					Key("install-target-" + string(target)).
					Children(func() {
						ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
							ui.Icon(c, iconSVG(info.IconPath)).TextColor(p.Text)
							ui.Text(c, info.Name).FontSize(12).Bold().Shrink(0)
						})
						var dir string
						if a.InstallFlow.Global {
							dir = displayPath(agents.GlobalTargetRoot(homeDir(), target))
						} else {
							dir = agents.TargetDirectory(target)
						}
						ui.Text(c, dir).FontSize(10).Font(FontMono).TextColor(p.Muted).SingleLine()
					})
				if selected {
					tile.BorderColor(p.Accent)
				}
				tile.OnClick(func() {
					a.toggleInstallTarget(target)
				})
			}
		})
	}
}

// joinLines joins failure messages with newlines.
func joinLines(lines []string) string {
	out := ""
	for i, line := range lines {
		if i > 0 {
			out += "\n"
		}
		out += line
	}
	return out
}
