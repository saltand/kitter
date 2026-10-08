// projects_page.go ports src/ui/projects_page.rs: the recent-project
// list (sorted by config.project_paths, filtered by search + tag
// filter), the Global row, project rows with tag chips and context
// menu, and the detail pane (project/global view with the
// effective-skills view).
package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/saltand/kitter/native/core/agents"
	"github.com/saltand/kitter/native/core/effective"
	"github.com/saltand/kitter/native/core/model"
	"github.com/saltand/kitter/native/core/tags"
)

// projectsPage is projects_page() in projects_page.rs.
func (a *App) projectsPage(c *ui.Context) {
	p := a.Palette()
	home := homeDir()

	// Visible projects: config order, filtered by search + tag filter.
	query := strings.ToLower(strings.TrimSpace(a.Projects.Search))
	var projects []string
	for _, path := range a.Library.Config.ProjectPaths() {
		matchesQuery := query == "" ||
			strings.Contains(strings.ToLower(filepath.Base(path)), query) ||
			strings.Contains(strings.ToLower(path), query)
		matchesTag := true
		if a.Projects.SelectedProjectTagFilter != 0 {
			matchesTag = a.ProjectTags().MatchesFilter(projectTagKey(path), a.Projects.SelectedProjectTagFilter)
		}
		if matchesQuery && matchesTag {
			projects = append(projects, path)
		}
	}

	// Roots whose snapshots/estimates the page needs.
	var roots []string
	if home != "" {
		roots = append(roots, home)
	}
	roots = append(roots, projects...)
	a.requestProjectSnapshots(c, roots)

	ui.Row(c).Fill().MinWidth(0).Children(func() {
		// Sidebar.
		ui.Column(c).Width(300).Shrink(0).Border(1, p.Border).BorderWidth(0, 0, 0, 1).Children(func() {
			ui.Row(c).Height(52).Shrink(0).Padding(0, 12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.T("项目", "Projects")).FontSize(14).Bold().Grow(1)
				if ui.Icon(c, iconSVG("folder.svg")).Size(28, 28).TextColor(p.Text).
					Label(a.T("打开项目文件夹", "Open project folder")).
					Tooltip(a.T("打开项目文件夹", "Open project folder")).
					Cursor(ui.CursorPointer).Clicked() {
					a.browseProject()
				}
			})
			ui.Divider(c)
			ui.Box(c).Padding(8, 10).Children(func() {
				ui.SearchField(c, &a.Projects.Search).FillWidth().Label(a.T("搜索项目", "Search projects"))
			})

			// Filter line: 全部 N + tag filter button.
			ui.Row(c).Padding(0, 16, 6, 16).AlignItems(ui.Center).Children(func() {
				label := fmt.Sprintf("%s  %d", a.T("全部", "All"), len(projects))
				if a.Projects.SelectedProjectTagFilter != 0 {
					if name, ok := a.ProjectTags().Path(tags.TagID(a.Projects.SelectedProjectTagFilter)); ok {
						label = fmt.Sprintf("#%s  %d", name, len(projects))
					}
				}
				ui.Text(c, label).FontSize(12).TextColor(p.Muted).SingleLine()
				ui.Spacer(c).Grow(1)
				iconColor := p.Muted
				if a.Projects.SelectedProjectTagFilter != 0 {
					iconColor = p.Accent
				}
				ui.Icon(c, iconSVG("hash.svg")).Size(28, 28).TextColor(iconColor).
					Label(a.T("筛选标签", "Filter by tag")).Tooltip(a.T("筛选标签", "Filter by tag")).
					Menu(func(m *ui.Menu) { a.projectTagFilterMenu(m) })
			})

			ui.Scroll(c).Grow(1).MinHeight(0).Padding(0, 8).Children(func() {
				if home != "" {
					a.globalProjectRow(c, home)
				}
				if len(projects) == 0 {
					ui.Text(c, a.emptyProjectsText()).FontSize(13).TextColor(p.Muted).
						Padding(30, 16, 0, 16)
				}
				for _, path := range projects {
					a.projectRow(c, path)
				}
			})
		})

		// Detail pane.
		ui.Column(c).Grow(1).MinWidth(0).Fill().Children(func() {
			if a.Projects.GlobalProjectView && home != "" {
				a.projectDetail(c, home, true)
			} else if a.Projects.OpenProject != "" {
				a.projectDetail(c, a.Projects.OpenProject, false)
			} else {
				ui.Column(c).Grow(1).Center().Children(func() {
					ui.Text(c, a.T("选择一个项目查看其技能", "Select a project to view its skills")).TextColor(p.Muted)
				})
			}
		})
	})
}

// emptyProjectsText is the sidebar empty message.
func (a *App) emptyProjectsText() string {
	if len(a.Library.Config.RecentProjects) == 0 {
		return a.T("还没有项目", "No projects yet")
	}
	return a.T("没有匹配的项目", "No matching projects")
}

// globalProjectRow is the Global sidebar row.
func (a *App) globalProjectRow(c *ui.Context, home string) {
	p := a.Palette()
	selected := a.Projects.GlobalProjectView
	skillCount := len(a.projectSnapshot(c, home))
	row := ui.Column(c).Padding(12, 10).Radius(10).Margin(0, 0, 4, 0).
		Cursor(ui.CursorPointer).Key("project-global").Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Icon(c, iconSVG("globe.svg")).TextColor(p.Secondary)
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				ui.Text(c, a.T("全局", "Global")).FontSize(14).Bold().SingleLine()
				ui.Text(c, displayPath(home)).FontSize(12).Font(FontMono).TextColor(p.Muted).SingleLine()
			})
		})
		var line string
		if a.UsesEnglish() {
			line = fmt.Sprintf("%s available across all projects", Counted(skillCount, "global skill", "global skills"))
		} else {
			line = fmt.Sprintf("%d 个全局技能可用于所有项目", skillCount)
		}
		ui.Text(c, line).FontSize(11).TextColor(p.Muted).Margin(6, 0, 0, 0)
	})
	if selected {
		row.Background(p.Selected)
	}
	row.OnClick(func() {
		a.selectGlobalProject()
	})
}

// projectRow is a sidebar project row.
func (a *App) projectRow(c *ui.Context, path string) {
	p := a.Palette()
	selected := !a.Projects.GlobalProjectView && a.Projects.OpenProject == path
	skillCount := len(a.projectSnapshot(c, path))
	name := filepath.Base(path)
	row := ui.Column(c).Padding(12, 10).Radius(10).Margin(0, 0, 4, 0).
		Cursor(ui.CursorPointer).Key("project-" + path).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Icon(c, iconSVG("folder.svg")).TextColor(p.Secondary)
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				ui.Text(c, name).FontSize(14).Bold().SingleLine()
				ui.Text(c, displayPath(path)).FontSize(12).Font(FontMono).TextColor(p.Muted).SingleLine()
			})
			ui.Text(c, fmt.Sprint(skillCount)).Font(FontMono).FontSize(12).TextColor(p.Muted)
		})
		a.projectTagChips(c, path)
	})
	if selected {
		row.Background(p.Selected)
	}
	row.OnClick(func() {
		a.selectProject(path)
	})
	row.Menu(func(m *ui.Menu) {
		a.projectContextMenu(m, path)
	})
}

// projectTagChips renders the assigned-tag chips on a project row.
func (a *App) projectTagChips(c *ui.Context, path string) {
	assigned := a.ProjectTags().AssignedTags(projectTagKey(path))
	if len(assigned) == 0 {
		return
	}
	p := a.Palette()
	ui.Row(c).Wrap().Gap(4).Margin(4, 0, 0, 0).Children(func() {
		for i, tag := range assigned {
			name, ok := a.ProjectTags().Path(tag.ID)
			if !ok {
				continue
			}
			nameCopy := name
			ui.Text(c, "#"+nameCopy).FontSize(10).Font(FontMono).Padding(1, 5).Radius(6).
				Background(p.Raised).TextColor(p.Muted).
				Key(fmt.Sprintf("project-tag-%s-%d", path, i))
		}
	})
}

// projectContextMenu is the project row context menu.
func (a *App) projectContextMenu(m *ui.Menu, path string) {
	if m.Item(a.T("打开", "Open")).Chosen() {
		a.selectProject(path)
	}
	if m.Item(a.T("移除", "Remove")).Chosen() {
		a.removeProject(nil, path)
	}
}

// projectDetail is the project/global detail pane.
func (a *App) projectDetail(c *ui.Context, path string, global bool) {
	estimates := a.contextEstimateSnapshot(path)
	snapshot := a.projectSnapshot(c, path)
	var rows []EffectiveSkillRow
	if global {
		rows = globalSkillRows(estimates, snapshot)
	} else {
		rows = skillRows(estimates, snapshot, a.Projects.SelectedProjectAgent)
	}
	plugins := pluginGroups(estimates, a.Projects.SelectedProjectAgent)

	ui.Column(c).Grow(1).MinWidth(0).Fill().Children(func() {
		// Header.
		ui.Row(c).Height(52).Shrink(0).Padding(0, 20).AlignItems(ui.Center).Children(func() {
			name := filepath.Base(path)
			if global {
				name = a.T("全局", "Global")
			}
			ui.Text(c, name).FontSize(14).Bold().SingleLine()
			ui.Spacer(c).Grow(1)
		})
		ui.Divider(c)

		// Context-estimate cards.
		a.contextEstimatePanel(c, path, estimates)

		// Tabs.
		skillCount := len(rows)
		pluginCount := 0
		for _, g := range plugins {
			pluginCount += len(g.Skills)
		}
		a.projectSkillsTabs(c, skillCount, pluginCount)

		// List.
		if a.Projects.ProjectSkillsTab == ProjectTabPlugins {
			a.effectivePluginsList(c, plugins)
		} else {
			a.effectiveSkillsList(c, path, global, rows)
		}
	})
}

// projectSkillsTabs is project_skills_tabs in components.rs.
func (a *App) projectSkillsTabs(c *ui.Context, skillCount, pluginCount int) {
	p := a.Palette()
	ui.Row(c).Height(43).Shrink(0).Padding(0, 28).AlignItems(ui.End).Gap(20).
		Border(1, p.Border).BorderWidth(0, 0, 1, 0).Children(func() {
		for _, tab := range []struct {
			tab   ProjectSkillsTab
			label string
			count int
			key   string
		}{
			{ProjectTabSkills, a.T("技能", "Skills"), skillCount, "project-skills-tab-skills"},
			{ProjectTabPlugins, a.T("插件", "Plugins"), pluginCount, "project-skills-tab-plugins"},
		} {
			tab := tab
			selected := a.Projects.ProjectSkillsTab == tab.tab
			el := ui.Row(c).Height(37).Padding(0, 2).AlignItems(ui.Center).Gap(6).
				Cursor(ui.CursorPointer).Key(tab.key).Children(func() {
				el2 := ui.Text(c, tab.label).FontSize(12).Bold()
				if selected {
					el2.TextColor(p.Text)
				} else {
					el2.TextColor(p.Muted)
				}
				ui.Text(c, fmt.Sprint(tab.count)).Font(FontMono).FontSize(10).TextColor(p.Muted)
			})
			if selected {
				el.Border(1, p.Accent).BorderWidth(0, 0, 1, 0)
			}
			el.OnClick(func() {
				a.Projects.ProjectSkillsTab = tab.tab
			})
		}
	})
}

// contextEstimatePanel is context_estimate_panel.
func (a *App) contextEstimatePanel(c *ui.Context, path string, estimates []effective.AgentContextEstimate) {
	p := a.Palette()
	if len(estimates) == 0 {
		return
	}
	isGlobal := homeDir() == path
	var ordered []effective.AgentContextEstimate
	for _, est := range estimates {
		globalOnly := false
		for _, agent := range agents.AgentIconOrder {
			if agent.ID == est.Agent.ID() && agent.GlobalOnly {
				globalOnly = true
			}
		}
		if isGlobal || !globalOnly {
			ordered = append(ordered, est)
		}
	}
	// Sort by AGENT_ICON_ORDER position.
	for i := 0; i < len(ordered); i++ {
		for j := i + 1; j < len(ordered); j++ {
			if agentIconOrder(ordered[j].Agent) < agentIconOrder(ordered[i].Agent) {
				ordered[i], ordered[j] = ordered[j], ordered[i]
			}
		}
	}
	const (
		tokenWarning = 2000
		tokenDanger  = 5000
		countWarning = 20
		countDanger  = 50
	)
	canExpand := len(ordered) > 8
	shown := ordered
	if canExpand && !a.Projects.ProjectAgentsExpanded {
		shown = shown[:8]
	}
	ui.Box(c).Margin(8, 8, 0, 8).Border(1, p.Border).Radius(10).Clip().
		Background(p.Surface).Children(func() {
		ui.Row(c).Wrap().Padding(8, 8, 8, 42).Children(func() {
			for _, est := range shown {
				est := est
				a.estimateCard(c, est, tokenWarning, tokenDanger, countWarning, countDanger)
			}
		})
		if canExpand {
			expanded := a.Projects.ProjectAgentsExpanded
			label := a.T("展开", "Show more")
			icon := "chevron-down.svg"
			if expanded {
				label = a.T("收起", "Show less")
				icon = "chevron-up.svg"
			}
			ui.Row(c).Height(30).Justify(ui.Center).Children(func() {
				if ui.Icon(c, iconSVG(icon)).Size(28, 28).TextColor(p.Muted).
					Tooltip(label).Label("toggle-project-agents").Cursor(ui.CursorPointer).Clicked() {
					a.Projects.ProjectAgentsExpanded = !a.Projects.ProjectAgentsExpanded
				}
			})
		}
		// Refresh button (top-right).
		ui.Row(c).Absolute().Top(7).Right(7).Children(func() {
			if ui.Icon(c, iconSVG("rotate-cw.svg")).Size(28, 28).TextColor(p.Secondary).
				Tooltip(a.T("刷新 token 扫描", "Refresh token scan")).
				Label(a.T("刷新 token 扫描", "Refresh token scan")).
				Key("refresh-context-" + path).Cursor(ui.CursorPointer).Clicked() {
				a.refreshContextEstimate(path)
			}
		})
	})
}

// estimateCard is one card inside context_estimate_panel.
func (a *App) estimateCard(c *ui.Context, est effective.AgentContextEstimate, tokenWarning, tokenDanger, countWarning, countDanger int) {
	p := a.Palette()
	selected := a.Projects.SelectedProjectAgent != nil && *a.Projects.SelectedProjectAgent == est.Agent
	automatic := est.ModelVisibleCount - est.NameOnlyCount
	if automatic < 0 {
		automatic = 0
	}
	severity := p.Success
	if est.EstimatedTokens >= tokenDanger || automatic >= countDanger {
		severity = p.Danger
	} else if est.EstimatedTokens > tokenWarning || automatic > countWarning {
		severity = p.Warning
	}
	var icon *ui.SVG
	for _, agent := range agents.AgentIconOrder {
		if agent.ID == est.Agent.ID() {
			icon = iconSVG(agent.IconPath)
		}
	}
	if icon == nil {
		icon = iconSVG("package.svg")
	}
	card := ui.Column(c).Width(176).Height(90).Padding(10, 12).Radius(12).
		Cursor(ui.CursorPointer).Shrink(0).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Icon(c, icon).TextColor(p.Text).Shrink(0)
			ui.Text(c, est.Agent.Label()).FontSize(11).TextColor(p.Secondary).SingleLine()
		})
		ui.Text(c, fmt.Sprintf("≈ %d tokens", est.EstimatedTokens)).Font(FontMono).
			FontSize(14).Bold().TextColor(severity).Margin(7, 0, 0, 0)
		ui.Row(c).Margin(2, 0, 0, 0).AlignItems(ui.Center).Children(func() {
			var auto string
			if a.UsesEnglish() {
				auto = fmt.Sprintf("%d automatic", automatic)
			} else {
				auto = fmt.Sprintf("%d 自动", automatic)
			}
			ui.Text(c, auto).FontSize(11).TextColor(severity)
			var detail string
			if a.UsesEnglish() {
				detail = fmt.Sprintf(" · %d manual", est.ManualOnlyCount)
				if est.ConditionalCount > 0 {
					detail += fmt.Sprintf(" · %d conditional", est.ConditionalCount)
				}
				if est.NameOnlyCount > 0 {
					detail += fmt.Sprintf(" · %d name-only", est.NameOnlyCount)
				}
			} else {
				detail = fmt.Sprintf(" · %d 手动", est.ManualOnlyCount)
				if est.ConditionalCount > 0 {
					detail += fmt.Sprintf(" · %d 条件", est.ConditionalCount)
				}
				if est.NameOnlyCount > 0 {
					detail += fmt.Sprintf(" · %d 仅名称", est.NameOnlyCount)
				}
			}
			ui.Text(c, detail).FontSize(11).TextColor(p.Muted)
		})
	})
	if selected {
		card.Background(p.Selected)
	}
	card.Key("project-agent-filter-" + est.Agent.ID()).OnClick(func() {
		if a.Projects.SelectedProjectAgent != nil && *a.Projects.SelectedProjectAgent == est.Agent {
			a.Projects.SelectedProjectAgent = nil
		} else {
			agent := est.Agent
			a.Projects.SelectedProjectAgent = &agent
		}
	})
}

// effectiveSkillsList renders the skills rows for the detail pane.
func (a *App) effectiveSkillsList(c *ui.Context, path string, global bool, rows []EffectiveSkillRow) {
	p := a.Palette()
	ui.Scroll(c).Grow(1).MinHeight(0).Padding(0, 28, 28, 28).Children(func() {
		if len(rows) == 0 {
			ui.Column(c).Margin(70, 0, 0, 0).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconSVG("package.svg")).TextColor(p.Muted)
				ui.Text(c, a.T("此项目中没有检测到技能", "No skills detected in this project")).
					FontSize(14).TextColor(p.Muted).Margin(10, 0, 0, 0)
			})
			return
		}
		for _, row := range rows {
			row := row
			a.effectiveSkillRow(c, path, global, row)
		}
	})
}

// effectiveSkillRow is one effective_skills row.
func (a *App) effectiveSkillRow(c *ui.Context, projectPath string, global bool, row EffectiveSkillRow) {
	p := a.Palette()
	ui.Column(c).Padding(12, 4).Border(1, p.Border).BorderWidth(0, 0, 1, 0).Children(func() {
		ui.Row(c).AlignItems(ui.Start).Gap(10).Children(func() {
			ui.Icon(c, iconSVG("package.svg")).TextColor(p.Secondary).Margin(2, 0, 0, 0).Shrink(0)
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
					ui.Text(c, row.Name).Font(FontMono).FontSize(13).Bold().Selectable().Shrink(0)
					if row.BuiltIn {
						ui.Text(c, a.T("内置", "Built-in")).FontSize(10).Padding(1, 5).Radius(5).
							Background(p.Raised).TextColor(p.Muted).Shrink(0)
					}
					if row.ManualOnly {
						ui.Text(c, a.T("手动", "Manual")).FontSize(10).Padding(1, 5).Radius(5).
							Background(p.Raised).TextColor(p.Warning).Shrink(0)
					}
					if global {
						ui.Text(c, a.T("用户级", "User-level")).FontSize(10).Padding(1, 5).Radius(5).
							Background(p.Raised).TextColor(p.Muted).Shrink(0)
					}
				})
				if row.Description != "" {
					ui.Text(c, row.Description).FontSize(12).TextColor(p.Secondary).Margin(4, 0, 0, 0).Wrap()
				}
				// Locations.
				for _, loc := range row.Locations {
					ui.Text(c, displayEffectiveRoot(loc, projectPath)).FontSize(11).Font(FontMono).
						TextColor(p.Muted).Margin(3, 0, 0, 0).SingleLine()
				}
				// Installations.
				for _, inst := range row.DirectInstallations {
					inst := inst
					ui.Row(c).AlignItems(ui.Center).Gap(8).Margin(6, 0, 0, 0).Children(func() {
						ui.Icon(c, iconSVG("folder.svg")).TextColor(p.Muted).Shrink(0)
						ui.Text(c, displayEffectiveRoot(inst.Path, projectPath)).FontSize(11).Font(FontMono).
							TextColor(p.Muted).Grow(1).SingleLine()
						ui.Text(c, string(inst.Target)).FontSize(10).Font(FontMono).TextColor(p.Muted)
						if ui.Icon(c, iconSVG("trash.svg")).Size(24, 24).TextColor(p.Danger).
							Background(p.DangerSoft).Radius(6).Cursor(ui.CursorPointer).
							Label(a.T("移除", "Remove")).Tooltip(a.T("移除", "Remove")).
							Key("remove-install-" + inst.Path).Clicked() {
							skill := model.ProjectSkill{Name: row.Name, Installations: row.DirectInstallations}
							a.openProjectDelete(projectPath, skill)
						}
					})
				}
			})
			a.effectiveAgentBadges(c, row.Agents)
		})
	})
}

// effectiveAgentBadges is effective_agent_badges (agent icons on a
// skills row).
func (a *App) effectiveAgentBadges(c *ui.Context, kinds []effective.AgentKind) {
	var icons []agents.AgentIconInfo
	for _, agent := range agents.AgentIconOrder {
		for _, kind := range kinds {
			if kind.ID() == agent.ID {
				icons = append(icons, agent)
				break
			}
		}
	}
	if len(icons) == 0 {
		return
	}
	p := a.Palette()
	ui.Row(c).Shrink(0).AlignItems(ui.Center).Children(func() {
		hidden := 0
		shown := icons
		if len(shown) > 5 {
			hidden = len(shown) - 5
			shown = shown[:5]
		}
		for i, agent := range shown {
			icon := ui.Icon(c, iconSVG(agent.IconPath)).Size(25, 25).
				TextColor(p.Text).Tooltip(agent.Name).Label(agent.Name)
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

// effectivePluginsList is effective_plugins_list.
func (a *App) effectivePluginsList(c *ui.Context, groups []EffectivePluginGroup) {
	p := a.Palette()
	ui.Scroll(c).Grow(1).MinHeight(0).Padding(0, 28, 28, 28).Children(func() {
		if len(groups) == 0 {
			ui.Column(c).Margin(70, 0, 0, 0).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconSVG("package.svg")).TextColor(p.Muted)
				ui.Text(c, a.T("没有检测到插件加载的技能", "No plugin-provided skills detected")).
					FontSize(14).TextColor(p.Muted).Margin(10, 0, 0, 0)
			})
			return
		}
		for _, group := range groups {
			group := group
			expanded := a.Projects.ExpandedProjectPlugins[group.Key]
			ui.Column(c).Border(1, p.Border).BorderWidth(0, 0, 1, 0).Children(func() {
				ui.Row(c).Height(58).Padding(0, 4).AlignItems(ui.Center).Gap(9).
					Cursor(ui.CursorPointer).Key("plugin-" + group.Key).Children(func() {
					ui.Icon(c, iconSVG("package.svg")).TextColor(p.Secondary).Shrink(0)
					ui.Column(c).Grow(1).MinWidth(0).Children(func() {
						ui.Text(c, group.DisplayName).FontSize(13).Bold().SingleLine()
						var line string
						if a.UsesEnglish() {
							line = fmt.Sprintf("%s · %s", group.Agent.Label(), Counted(len(group.Skills), "skill", "skills"))
						} else {
							line = fmt.Sprintf("%s · %d 个技能", group.Agent.Label(), len(group.Skills))
						}
						ui.Text(c, line).FontSize(11).TextColor(p.Muted).SingleLine()
					})
					icon := "chevron-right.svg"
					if expanded {
						icon = "chevron-down.svg"
					}
					ui.Icon(c, iconSVG(icon)).TextColor(p.Muted).Shrink(0)
				}).OnClick(func() {
					if expanded {
						delete(a.Projects.ExpandedProjectPlugins, group.Key)
					} else {
						a.Projects.ExpandedProjectPlugins[group.Key] = true
					}
				})
				if expanded {
					ui.Column(c).Padding(0, 4, 8, 4).Children(func() {
						for _, name := range group.Skills {
							ui.Text(c, name).FontSize(12).Font(FontMono).Padding(3, 0).SingleLine()
						}
					})
				}
			})
		}
	})
}

// globalSkillRows is global_skill_rows (mod.rs): skills from
// global-scope agents.
func globalSkillRows(estimates []effective.AgentContextEstimate, snapshot []model.ProjectSkill) []EffectiveSkillRow {
	var entries []effective.GroupEntry
	for i := range estimates {
		est := &estimates[i]
		globalOnly := false
		for _, agent := range agents.AgentIconOrder {
			if agent.ID == est.Agent.ID() && agent.GlobalOnly {
				globalOnly = true
			}
		}
		if !globalOnly {
			continue
		}
		for j := range est.Skills {
			skill := &est.Skills[j]
			if skill.IsPlugin() {
				continue
			}
			entries = append(entries, effective.GroupEntry{Agent: est.Agent, Skill: skill})
		}
	}
	groups := effective.GroupEffectiveSkills(entries)
	rows := make([]EffectiveSkillRow, 0, len(groups))
	for _, group := range groups {
		if len(group.Entries) == 0 {
			continue
		}
		first := group.Entries[0].Skill
		row := EffectiveSkillRow{Name: first.Name, Description: first.Description}
		for _, entry := range group.Entries {
			skill := entry.Skill
			if row.Description == "" && skill.Description != "" {
				row.Description = skill.Description
			}
			if skill.RootPath != "" && !containsString(row.Locations, skill.RootPath) {
				row.Locations = append(row.Locations, skill.RootPath)
			}
			if skill.Source.Kind == effective.SourceBuiltin {
				row.BuiltIn = true
			}
			if !agentKindContains(row.Agents, entry.Agent) {
				row.Agents = append(row.Agents, entry.Agent)
			}
			if skill.Visibility == effective.VisibilityManualOnly {
				row.ManualOnly = true
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// projectTagFilterMenu is the sidebar tag filter menu for projects.
func (a *App) projectTagFilterMenu(m *ui.Menu) {
	allLabel := a.T("全部项目", "All projects")
	if m.Item(allLabel).Checked(a.Projects.SelectedProjectTagFilter == 0).Chosen() {
		a.Projects.SelectedProjectTagFilter = 0
	}
	for _, tag := range a.ProjectTags().Roots() {
		a.projectTagFilterItem(m, tag)
		for _, child := range a.ProjectTags().Children(tag.ID) {
			a.projectTagFilterItem(m, child)
		}
	}
}

func (a *App) projectTagFilterItem(m *ui.Menu, tag *tags.Tag) {
	label := fmt.Sprintf("#%s  %d", tag.Name, a.ProjectTags().Count(tag.ID))
	if m.Item(label).Checked(a.Projects.SelectedProjectTagFilter == tag.ID).Chosen() {
		a.Projects.SelectedProjectTagFilter = tag.ID
	}
}
