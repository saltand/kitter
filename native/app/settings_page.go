// settings_page.go ports ui/settings_page.rs + settings_actions.rs:
// language dropdown, theme choices, data dir display + browse, plus
// the update-check row.
package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/saltand/kitter/native/core/config"
)

// settingsPage is settings_page in settings_page.rs.
func (a *App) settingsPage(c *ui.Context) {
	ui.Scroll(c).Fill().Padding(20).Children(func() {
		ui.Column(c).MaxWidth(768).Children(func() {
			// Preferences.
			ui.Text(c, a.T("偏好设置", "Preferences")).FontSize(14).Bold().Margin(0, 0, 10, 0)
			a.settingsCard(c, func() {
				a.settingsRow(c,
					a.T("语言", "Language"),
					a.T("选择 Kitter 界面的显示语言。", "Choose the language used by Kitter."),
					func() { a.languageControl(c) })
			})

			ui.Text(c, a.T("外观", "Appearance")).FontSize(14).Bold().Margin(40, 0, 10, 0)
			a.settingsCard(c, func() {
				a.settingsRow(c,
					a.T("主题", "Theme"),
					a.T("跟随系统，或固定使用浅色与深色外观。", "Follow the system or use a fixed light or dark appearance."),
					func() { a.themeControl(c) })
			})

			ui.Text(c, a.T("技能库", "Skill library")).FontSize(14).Bold().Margin(40, 0, 10, 0)
			a.settingsCard(c, func() {
				a.settingsRow(c,
					a.T("技能存放位置", "Skills location"),
					a.T("Kitter 用来保存所有技能的文件夹。", "The folder where Kitter keeps all skills."),
					func() { a.libraryDirControl(c) })
			})

			ui.Text(c, a.T("更新", "Updates")).FontSize(14).Bold().Margin(40, 0, 10, 0)
			a.settingsCard(c, func() {
				a.settingsRow(c,
					a.T("检查更新", "Check for updates"),
					a.T("扫描所有技能的远端是否有新版本。", "Scan every skill's remote source for new versions."),
					func() { a.updateControl(c) })
			})
		})
	})
}

// settingsCard wraps a group of rows in a bordered card.
func (a *App) settingsCard(c *ui.Context, children func()) {
	p := a.Palette()
	ui.Column(c).FillWidth().Radius(10).Border(1, p.Border).Background(p.Surface).Clip().
		Children(children)
}

// settingsRow is the shared title+description row.
func (a *App) settingsRow(c *ui.Context, title, description string, control func()) {
	p := a.Palette()
	ui.Row(c).MinHeight(60).Padding(12, 16).AlignItems(ui.Center).Gap(24).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, title).FontSize(14)
			ui.Text(c, description).FontSize(12).TextColor(p.Muted).Margin(2, 0, 0, 0)
		})
		control()
	})
}

// languageControl is the language dropdown.
func (a *App) languageControl(c *ui.Context) {
	p := a.Palette()
	var label string
	switch a.Library.Config.Language {
	case config.LanguageZhCn:
		label = "简体中文"
	case config.LanguageEn:
		label = "English"
	default:
		label = a.T("跟随系统", "System")
	}
	ui.Row(c).Width(180).Height(30).Padding(0, 10).Radius(7).Border(1, p.Border).
		AlignItems(ui.Center).Cursor(ui.CursorPointer).Key("language-select").Children(func() {
		ui.Text(c, label).FontSize(13).Grow(1)
		ui.Icon(c, iconSVG("chevron-down.svg")).TextColor(p.Muted)
	}).Menu(func(m *ui.Menu) {
		options := []struct {
			lang  config.Language
			label string
		}{
			{config.LanguageSystem, a.T("跟随系统", "System")},
			{config.LanguageZhCn, "简体中文"},
			{config.LanguageEn, "English"},
		}
		for _, opt := range options {
			opt := opt
			item := m.Item(opt.label).Checked(a.Library.Config.Language == opt.lang)
			if item.Chosen() {
				a.setLanguage(opt.lang)
			}
		}
	})
}

// setLanguage is set_language: persists + refreshes placeholders.
func (a *App) setLanguage(lang config.Language) {
	a.Library.Config.Language = lang
	_ = a.Library.Save()
}

// themeControl is the System/Light/Dark choice row.
func (a *App) themeControl(c *ui.Context) {
	p := a.Palette()
	for _, opt := range []struct {
		theme config.Theme
		label string
		key   string
	}{
		{config.ThemeSystem, a.T("跟随系统", "System"), "theme-system"},
		{config.ThemeLight, a.T("浅色", "Light"), "theme-light"},
		{config.ThemeDark, a.T("深色", "Dark"), "theme-dark"},
	} {
		opt := opt
		selected := a.Library.Config.Theme == opt.theme
		el := ui.Text(c, opt.label).FontSize(12).Padding(5, 10).Radius(7).Cursor(ui.CursorPointer).
			Key(opt.key)
		if selected {
			el.Background(p.Selected).TextColor(p.Text)
		} else {
			el.Background(p.Raised).TextColor(p.Muted)
		}
		el.Margin(0, 0, 0, 2).OnClick(func() {
			a.setTheme(opt.theme)
		})
	}
}

// setTheme persists the theme choice (the shell reads it on the next
// frame and applies it).
func (a *App) setTheme(theme config.Theme) {
	a.Library.Config.Theme = theme
	_ = a.Library.Save()
}

// libraryDirControl shows the data dir + a browse button.
func (a *App) libraryDirControl(c *ui.Context) {
	p := a.Palette()
	ui.Row(c).Width(320).AlignItems(ui.Center).Gap(7).Children(func() {
		ui.Text(c, displayPath(a.Library.Config.LibraryDir)).Font(FontMono).FontSize(12).
			TextColor(p.Secondary).Grow(1).SingleLine().Selectable()
		if iconButton(c, "folder.svg", 30, 16).TextColor(p.Text).
			Background(p.Raised).Radius(7).Cursor(ui.CursorPointer).
			Label(a.T("选择", "Choose")).Tooltip(a.T("选择", "Choose")).
			Key("browse-library").Clicked() {
			a.browseLibrary()
		}
	})
}

// browseLibrary is browse_library.
func (a *App) browseLibrary() {
	a.spawn(func() {
		paths, err := PickDirectory(a.T("选择", "Choose"))
		if err != nil || len(paths) == 0 {
			return
		}
		path := paths[0]
		a.Apply(func() {
			a.Library.Config.LibraryDir = path
			_ = a.Library.Save()
			a.ReloadSkills()
		})
	})
}

// updateControl is the check-updates row (button + busy state).
func (a *App) updateControl(c *ui.Context) {
	p := a.Palette()
	if a.CheckingUpdates {
		ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Icon(c, iconSVG("rotate-cw.svg")).TextColor(p.Accent)
			ui.Text(c, a.T("正在检查…", "Checking…")).FontSize(12).TextColor(p.Secondary)
		})
		return
	}
	label := a.T("检查更新", "Check")
	if a.UpdateCount > 0 {
		label = fmt.Sprintf("%s (%d)", a.T("可更新", "Updates"), a.UpdateCount)
	}
	if ui.Text(c, label).FontSize(13).Padding(6, 14).Radius(8).
		Background(p.Accent).TextColor(p.OnAccent).Cursor(ui.CursorPointer).
		Key("check-updates").Clicked() {
		a.checkAllUpdates()
	}
}
