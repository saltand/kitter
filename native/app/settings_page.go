// settings_page.go mirrors ui/settings_page.rs — M1 placeholder: shows
// where the data lives and the current language/theme.
package app

import (
	"github.com/egoist/mygo/ui"
)

func (a *App) settingsPage(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Padding(24, 28).Gap(16).MaxWidth(768).Children(func() {
		ui.Text(c, a.T("技能库", "Skill library")).Bold()
		ui.Text(c, a.Library.Config.LibraryDir).Font(FontMono).FontSize(12).
			Padding(10, 12).Radius(8).Background(t.Surface).Selectable()

		ui.Text(c, a.T("语言", "Language")).Bold()
		ui.Text(c, a.T("当前跟随系统；语言与主题设置将在后续里程碑提供。",
			"Follows the system for now; language and theme settings arrive in a later milestone.")).
			FontSize(12).TextColor(t.TextMuted)
	})
}
