// projects_page.go mirrors ui/projects_page.rs — M1 placeholder.
package app

import (
	"github.com/egoist/mygo/ui"
)

func (a *App) projectsPage(c *ui.Context) {
	t := c.Theme()
	projects := a.Library.Config.ProjectPaths()
	ui.Column(c).Padding(24, 28).Gap(12).FillWidth().Children(func() {
		if len(projects) == 0 {
			ui.Text(c, a.T("还没有项目", "No projects yet")).TextColor(t.TextMuted)
			ui.Text(c, a.T("从技能页安装技能后，项目会出现在这里。", "Projects appear here after you install skills into them.")).
				FontSize(12).TextColor(t.TextMuted)
			return
		}
		for _, project := range projects {
			ui.Text(c, project).SingleLine()
		}
	})
}
