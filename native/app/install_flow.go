// install_flow.go ports src/ui/install_flow.rs: the install modal
// (project select, global/project toggle, target grid) plus the
// install execution from add_actions.rs::install_selected.
package app

import (
	"fmt"
	"path/filepath"

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

// installDialog is install_modal: "Install {name}", a project dropdown
// (Global first) with a browse button, the target group in a scrolling
// body, and a 60-point footer.
func (a *App) installDialog(c *ui.Context) {
	if !a.InstallFlow.Modal {
		return
	}
	p := a.Palette()
	keys := a.selectedSkillKeys()
	name := a.T("技能", "Skill")
	if len(keys) > 1 {
		if a.UsesEnglish() {
			name = Counted(len(keys), "skill", "skills")
		} else {
			name = fmt.Sprintf("%d 个技能", len(keys))
		}
	} else if targets := a.selectedLibraryTargets(); len(targets) == 1 {
		name = targets[0].Record.Name
	}
	title := a.T("安装 ", "Install ") + name

	a.dialog(c, &a.InstallFlow.Modal, func() {
		ui.Column(c).Width(560).Background(p.Elevated).Radius(RadiusModal).Clip().Children(func() {
			ui.Row(c).Padding(20, 20, 14, 20).AlignItems(ui.Center).Children(func() {
				ui.Text(c, title).FontSize(16).FontWeight(600).Grow(1).MinWidth(0).SingleLine()
				if iconButton(c, "x.svg", ControlHeight, 14).TextColor(p.Text).
					Background(p.Raised).Radius(RadiusControl).Cursor(ui.CursorPointer).
					Label(a.T("关闭", "Close")).Key("install-close").Clicked() {
					a.InstallFlow.Modal = false
				}
			})
			ui.Scroll(c).MaxHeight(500).Padding(0, 20, 20, 20).Children(func() {
				ui.Text(c, a.T("安装到", "Install to")).FontSize(14).TextColor(p.Muted).Margin(0, 0, 7, 0)
				ui.Row(c).Gap(8).Children(func() {
					a.installProjectSelect(c)
					if iconButton(c, "folder.svg", ControlHeight, 16).TextColor(p.Text).
						Background(p.Surface).Border(1, p.Border).Radius(RadiusControl).
						Cursor(ui.CursorPointer).Label(a.T("浏览", "Browse")).
						Key("install-browse-project").Clicked() {
						a.browseProject()
						a.InstallFlow.Global = false
					}
				})
				ui.Text(c, a.T("安装位置", "Install location")).FontSize(13).TextColor(p.Secondary).
					Margin(18, 0, 7, 0)
				a.installTargetGrid(c)
			})
			ui.Row(c).Height(60).Padding(0, 20).Gap(8).AlignItems(ui.Center).Justify(ui.End).
				BorderWidth(1, 0, 0, 0).BorderColor(p.Border).FontSize(14).Children(func() {
				if textButton(c, a.T("取消", "Cancel"), DialogControlHeight).Padding(0, 16).
					Radius(RadiusControl).Cursor(ui.CursorPointer).Key("install-cancel").Clicked() {
					a.InstallFlow.Modal = false
				}
				enabled := a.installHasTarget()
				btn := textButton(c, a.T("安装", "Install"), DialogControlHeight).Padding(0, 16).
					Radius(RadiusControl).Border(1, p.Border).Key("install-confirm")
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

// installProjectSelect is the install-project-select dropdown: Global
// (the home folder) first, then every known project.
func (a *App) installProjectSelect(c *ui.Context) {
	label := a.T("全局", "Global")
	if !a.InstallFlow.Global {
		label = a.T("选择一个项目", "Choose a project")
		if a.Projects.OpenProject != "" {
			label = filepath.Base(a.Projects.OpenProject)
		}
	}
	home := homeDir()
	a.dropdownButton(c, label, "install-project-select").Grow(1).MinWidth(0).Menu(func(m *ui.Menu) {
		if home != "" && m.Item(a.T("全局", "Global")+"  "+displayPath(home)).Checked(a.InstallFlow.Global).Chosen() {
			a.InstallFlow.Global = true
		}
		for _, path := range a.Library.Config.ProjectPaths() {
			if path == home {
				continue
			}
			path := path
			active := !a.InstallFlow.Global && a.Projects.OpenProject == path
			if m.Item(filepath.Base(path) + "  " + displayPath(path)).Checked(active).Chosen() {
				a.InstallFlow.Global = false
				a.Projects.OpenProject = path
			}
		}
	})
}

// installTargetGrid is target_group: the Universal card (.agents/skills,
// with the agents that read it in a 3-column grid) and the
// agent-specific targets below it.
func (a *App) installTargetGrid(c *ui.Context) {
	p := a.Palette()
	universal := a.InstallFlow.SelectedTargets[model.TargetUniversal]
	var shared []agents.AgentIconInfo
	for _, info := range agents.AgentIconOrder {
		if !info.GlobalOnly && info.SupportsTarget(model.TargetUniversal) {
			shared = append(shared, info)
		}
	}
	universalDir := ".agents/skills"
	if a.InstallFlow.Global {
		universalDir = "~/.agents/skills"
	}
	ui.Column(c).Radius(RadiusCard).Border(1, p.Border).Background(p.Surface).Clip().Children(func() {
		head := a.targetRow(c, "install-target-universal", universal, func() {
			ui.Text(c, a.T("通用", "Universal")).FontSize(14).Bold().Margin(0, 0, 0, 9)
		}, universalDir)
		head.OnClick(func() { a.toggleInstallTarget(model.TargetUniversal) })
		const cols = 3
		for i := 0; i < len(shared); i += cols {
			ui.Row(c).Children(func() {
				for j := i; j < i+cols; j++ {
					cell := ui.Row(c).Height(48).Basis(0).Grow(1).Padding(0, 11).Gap(8).
						AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(p.Border)
					if j%cols < cols-1 {
						cell.BorderWidth(1, 1, 0, 0)
					}
					if j >= len(shared) {
						continue
					}
					info := shared[j]
					cell.Children(func() {
						brandTile(c, p, info.IconPath)
						ui.Text(c, info.Name).FontSize(12).TextColor(p.Secondary).SingleLine()
					})
				}
			})
		}
	})
	ui.Box(c).Height(10).Shrink(0)
	ui.Column(c).Radius(RadiusCard).Border(1, p.Border).Background(p.Surface).Clip().Children(func() {
		ui.Row(c).Height(42).Padding(0, 13).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.T("独立安装", "Agent-specific")).FontSize(14).Bold()
		})
		for _, info := range agents.IndependentInstallTargets {
			info := info
			var dir string
			if a.InstallFlow.Global {
				dir = displayPath(agents.GlobalTargetRoot(homeDir(), info.Target))
			} else {
				dir = agents.TargetDirectory(info.Target)
			}
			row := a.targetRow(c, "install-target-"+string(info.Target), a.InstallFlow.SelectedTargets[info.Target], func() {
				ui.Box(c).Margin(0, 0, 0, 9).Children(func() { brandTile(c, p, info.IconPath) })
				ui.Text(c, info.Name).FontSize(13).Margin(0, 0, 0, 9).SingleLine()
			}, dir)
			row.BorderWidth(1, 0, 0, 0).BorderColor(p.Border)
			row.OnClick(func() { a.toggleInstallTarget(info.Target) })
		}
	})
}

// targetRow is a 48-point selectable row: a check box, the label built by
// label, and the target directory right-aligned in mono.
func (a *App) targetRow(c *ui.Context, key string, active bool, label func(), dir string) ui.Element {
	p := a.Palette()
	row := ui.Row(c).Height(48).Padding(0, 13).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Key(key)
	if active {
		row.Background(p.Selected)
	}
	return row.Children(func() {
		checkMark(c, p, active)
		label()
		ui.Spacer(c).Grow(1)
		ui.Text(c, dir).FontSize(12).Font(FontMono).TextColor(p.Muted).SingleLine().Shrink(1)
	})
}

// checkMark draws a 16-point check box, filled with the accent when on.
func checkMark(c *ui.Context, p Palette, on bool) {
	box := ui.Box(c).Size(16, 16).Radius(4).Center().Shrink(0)
	if on {
		box.Background(p.Accent).Children(func() {
			ui.Icon(c, iconSVG("check.svg")).Size(12, 12).TextColor(p.OnAccent)
		})
	} else {
		box.Border(1, p.BorderStrong)
	}
}

// brandTile is the 28-point rounded tile around an agent's brand icon.
func brandTile(c *ui.Context, p Palette, iconPath string) {
	tile := ui.Box(c).Size(28, 28).Radius(7).Center().Shrink(0)
	if iconPath != "icons/provider-codex.svg" {
		tile.Background(p.Raised)
	}
	tile.Children(func() { brandIcon(c, iconPath, 18).TextColor(p.Text) })
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
