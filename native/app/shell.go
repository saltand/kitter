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
	if a.pendingNotice != "" {
		c.Toast(a.pendingNotice)
		a.pendingNotice = ""
	}
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
			// Palette.Sidebar is translucent over the window color.
			sidebar.Background(p.Sidebar.Over(p.Window))
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
	// Drag area under the traffic lights (46 + 8 in components.rs).
	ui.Box(c).Height(titleBarHeight + 2).Shrink(0).DragWindow()

	// Pages also change outside the sidebar (opening a project from a
	// skill, tests); keep the highlight on the current page.
	a.sidebarSelected = string(a.Page)
	list := ui.Sidebar(c, &a.sidebarSelected, func() {
		ui.SidebarItem(c, string(PageSkills), IconSVG("package.svg"), a.T("技能", "Skills"))
		ui.SidebarItem(c, string(PageProjects), IconSVG("folder.svg"), a.T("项目", "Projects"))
		ui.SidebarItem(c, string(PageSettings), IconSVG("settings.svg"), a.T("设置", "Settings"))
	})
	list.Grow(1).Label("Kitter").Background(ui.Transparent)
	if list.Changed() {
		a.Page = Page(a.sidebarSelected)
	}
}

func (a *App) content(c *ui.Context, vibrant bool) {
	_ = vibrant // pane stays opaque; only the sidebar shows the material
	pane := ui.Column(c).Grow(1).MinWidth(0)
	pane.Background(a.Palette().Base)
	pane.Children(func() {
		// No page title bar: as in the Rust build, each page's own 52-point
		// headers sit at the top of the window and drag it.
		// Each page owns its scrolling, so it fills the content area.
		ui.Box(c).Grow(1).MinHeight(0).MinWidth(0).Children(func() {
			switch a.Page {
			case PageSettings:
				a.settingsPage(c)
			case PageProjects:
				a.projectsPage(c)
			default:
				a.skillsPage(c)
			}
		})
	})
	// Dialogs render above the page, matching Rust's dialog overlay.
	a.deleteModal(c)
	a.addDialog(c)
	a.installDialog(c)
	a.tagsDialog(c)
	a.assignTagsDialog(c)
	a.groupsDialog(c)
	a.deleteGroupDialog(c)
	a.moveGroupDialog(c)
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
