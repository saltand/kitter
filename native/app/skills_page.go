// skills_page.go mirrors ui/skills_page.rs — M1 minimal version: a
// searchable list of the library's skills proving core ↔ UI plumbing.
package app

import (
	"fmt"
	"strings"

	"github.com/saltand/kitter/native/core/model"

	"github.com/egoist/mygo/ui"
)

func (a *App) skillsPage(c *ui.Context) {
	t := c.Theme()
	p := a.Palette()

	ui.Column(c).Padding(16, 20).Gap(12).FillWidth().Children(func() {
		ui.SearchField(c, &a.Skills.Search).Label(a.T("搜索技能", "Search skills")).FillWidth()
		if a.Skills.Err != nil {
			ui.Text(c, a.ErrorMessage(a.Skills.Err)).TextColor(p.Danger)
			return
		}
		items := a.filteredSkills()
		if len(items) == 0 {
			ui.Column(c).Grow(1).Center().Padding(40).Children(func() {
				ui.Text(c, a.T("这个文件夹中没有找到可用的技能", "No skills found in this folder")).TextColor(t.TextMuted)
			})
			return
		}
		a.skillList.Key = func(row int) any {
			if row < 0 || row >= len(items) {
				return row
			}
			return items[row].Record.StorageName
		}
		a.skillList.Label = func(row int) string {
			if row < 0 || row >= len(items) {
				return ""
			}
			return items[row].Record.Name
		}
		ui.List(c, &a.skillList, len(items), func(row int) {
			skill := items[row]
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(10).Padding(6, 10).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					name := skill.Record.Name
					if skill.Record.Origin.IsBuiltin() {
						name += " · Kitter"
					}
					ui.Text(c, name).Bold().SingleLine()
					if skill.Record.Description != "" {
						ui.Text(c, skill.Record.Description).FontSize(12).TextColor(t.TextMuted).SingleLine()
					}
				})
				if skill.ManualOnly {
					ui.Badge(c, a.T("手动", "Manual"))
				}
				if skill.InstalledProjects > 0 {
					ui.Badge(c, fmt.Sprint(skill.InstalledProjects))
				}
			})
		}).Grow(1)
	})
}

func (a *App) filteredSkills() []model.SkillSummary {
	query := strings.TrimSpace(strings.ToLower(a.Skills.Search))
	if query == "" {
		return a.Skills.Items
	}
	var out []model.SkillSummary
	for _, skill := range a.Skills.Items {
		if strings.Contains(strings.ToLower(skill.Record.Name), query) ||
			strings.Contains(strings.ToLower(skill.Record.Description), query) {
			out = append(out, skill)
		}
	}
	return out
}
