// shell.go mirrors ui/mod.rs + ui/components.rs::sidebar: the hidden title
// bar, the vibrancy sidebar with three-page navigation, and the content
// pane switch.
package app

import (
	"github.com/egoist/mygo/ui"
)

// titleBarHeight matches the Rust traffic-light inset (px 16/17 → ~52pt
// bar) and mygo's TitleBarHiddenInset defaults.
const titleBarHeight = 52.0

// sidebarWidth matches layout::SIDEBAR_WIDTH.
const sidebarWidth = 180.0

// View is the root view function (KitterApp::render).
func (a *App) View(c *ui.Context) {
	p := a.Palette()

	// A vibrant window shows through wherever the view draws nothing:
	// make the root transparent and give the content pane a background.
	vibrant := c.Vibrancy()
	if vibrant {
		c.Root().Background(ui.Transparent)
	}
	bar := c.TitleBar()

	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		sidebar := ui.Column(c).Width(sidebarWidth).Shrink(0)
		if !vibrant {
			sidebar.Background(p.Sidebar)
		}
		sidebar.Children(func() { a.sidebar(c, vibrant) })
		ui.Box(c).Width(1).Shrink(0).Background(p.Border)
		a.content(c, vibrant)
	})

	// The window controls sit over the sidebar on macOS; reserve their
	// room whether the vibrancy material shows or not.
	_ = bar
}

func (a *App) sidebar(c *ui.Context, vibrant bool) {
	// Drag area under the traffic lights.
	ui.Box(c).Height(titleBarHeight).Shrink(0).DragWindow()

	list := ui.Sidebar(c, &a.sidebarSelected, func() {
		ui.SidebarItem(c, string(PageSkills), IconSVG("package.svg"), a.T("技能", "Skills"))
		ui.SidebarItem(c, string(PageProjects), IconSVG("folder.svg"), a.T("项目", "Projects"))
		ui.SidebarItem(c, string(PageSettings), IconSVG("settings.svg"), a.T("设置", "Settings"))
	})
	list.Grow(1).Label("Kitter")
	if vibrant {
		list.Background(ui.Transparent)
	}
	if list.Changed() {
		a.Page = Page(a.sidebarSelected)
	}
}

func (a *App) content(c *ui.Context, vibrant bool) {
	t := c.Theme()
	_ = vibrant // pane stays opaque; only the sidebar shows the material
	pane := ui.Column(c).Grow(1).MinWidth(0)
	pane.Background(t.Background)
	pane.Children(func() {
		// Inset title bar: the page title sits next to the traffic lights.
		bar := c.TitleBar()
		ui.Row(c).Height(titleBarHeight).Shrink(0).AlignItems(ui.Center).
			Padding(0, bar.Right+20, 0, bar.Left+20).DragWindow().Children(func() {
			ui.Text(c, a.pageTitle()).FontSize(15).Bold().SingleLine()
		})
		ui.Divider(c)
		if a.Page == PageSkills {
			// The skills page owns its own scrolling inside the split
			// panes, so it fills the content area directly.
			ui.Box(c).Grow(1).MinHeight(0).MinWidth(0).Children(func() {
				a.skillsPage(c)
			})
		} else {
			ui.Scroll(c).Grow(1).Children(func() {
				switch a.Page {
				case PageProjects:
					a.projectsPage(c)
				case PageSettings:
					a.settingsPage(c)
				}
			})
		}
	})
	// Dialogs render above the page, matching Rust's dialog overlay.
	a.deleteModal(c)
}

func (a *App) pageTitle() string {
	switch a.Page {
	case PageProjects:
		return a.T("项目", "Projects")
	case PageSettings:
		return a.T("设置", "Settings")
	default:
		return a.T("技能", "Skills")
	}
}
