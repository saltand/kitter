// theme.go mirrors src/ui/theme.rs: the Kitter palette for light and dark.
// MyGo supplies a Theme already; Palette keeps the exact Rust colors for
// the areas MyGo does not style (sidebar, badges, danger_soft…).
package app

import "github.com/egoist/mygo/ui"

// Font families used by the Rust UI (src/ui/theme.rs).
const (
	FontMono = "JetBrains Mono"
)

// Palette is src/ui/theme.rs::Palette.
type Palette struct {
	Window       ui.Color
	WindowBorder ui.Color
	Base         ui.Color
	Sidebar      ui.Color
	Surface      ui.Color
	Elevated     ui.Color
	Raised       ui.Color
	Hover        ui.Color
	Selected     ui.Color
	Border       ui.Color
	BorderStrong ui.Color
	Text         ui.Color
	SidebarText  ui.Color
	Secondary    ui.Color
	Muted        ui.Color
	Accent       ui.Color
	AccentFill   ui.Color
	OnAccent     ui.Color
	Success      ui.Color
	Warning      ui.Color
	Danger       ui.Color
	DangerSoft   ui.Color
	Overlay      ui.Color
}

// PaletteDark is Palette::dark().
func PaletteDark() Palette {
	return Palette{
		Window:       ui.RGB(0x14, 0x14, 0x14),
		WindowBorder: ui.RGBA(0xB0, 0xB0, 0xB0, 77.0/255.0),
		Base:         ui.RGB(0x18, 0x18, 0x18),
		Sidebar:      ui.RGB(0x18, 0x18, 0x18),
		Surface:      ui.RGB(0x18, 0x18, 0x18),
		Elevated:     ui.RGBA(0x36, 0x36, 0x36, 245.0/255.0),
		Raised:       ui.RGBA(0xFF, 0xFF, 0xFF, 13.0/255.0),
		Hover:        ui.RGBA(0xFF, 0xFF, 0xFF, 15.0/255.0),
		Selected:     ui.RGBA(0xFF, 0xFF, 0xFF, 15.0/255.0),
		Border:       ui.RGBA(0xFF, 0xFF, 0xFF, 21.0/255.0),
		BorderStrong: ui.RGBA(0xFF, 0xFF, 0xFF, 40.0/255.0),
		Text:         ui.RGB(0xDF, 0xDF, 0xDF),
		SidebarText:  ui.RGBA(0xFF, 0xFF, 0xFF, 217.0/255.0),
		Secondary:    ui.RGBA(0xFF, 0xFF, 0xFF, 181.0/255.0),
		Muted:        ui.RGBA(0xFF, 0xFF, 0xFF, 127.0/255.0),
		Accent:       ui.RGB(0x33, 0x9C, 0xFF),
		AccentFill:   ui.RGB(0x0D, 0x0D, 0x0D),
		OnAccent:     ui.RGB(0xFF, 0xFF, 0xFF),
		Success:      ui.RGB(0x3F, 0xB9, 0x50),
		Warning:      ui.RGB(0xE3, 0xB3, 0x41),
		Danger:       ui.RGB(0xFF, 0x67, 0x62),
		DangerSoft:   ui.RGBA(0xFF, 0x67, 0x62, 26.0/255.0),
		Overlay:      ui.RGBA(0x00, 0x00, 0x00, 34.0/255.0),
	}
}

// PaletteLight is Palette::light().
func PaletteLight() Palette {
	return Palette{
		Window:       ui.RGB(0xF5, 0xF5, 0xF5),
		WindowBorder: ui.RGBA(0x64, 0x64, 0x64, 89.0/255.0),
		Base:         ui.RGB(0xFF, 0xFF, 0xFF),
		Sidebar:      ui.RGBA(0xFF, 0xFF, 0xFF, 179.0/255.0),
		Surface:      ui.RGB(0xFF, 0xFF, 0xFF),
		Elevated:     ui.RGBA(0xFF, 0xFF, 0xFF, 245.0/255.0),
		Raised:       ui.RGBA(0x0D, 0x0D, 0x0D, 13.0/255.0),
		Hover:        ui.RGBA(0x0D, 0x0D, 0x0D, 14.0/255.0),
		Selected:     ui.RGBA(0x0D, 0x0D, 0x0D, 14.0/255.0),
		Border:       ui.RGBA(0x0D, 0x0D, 0x0D, 20.0/255.0),
		BorderStrong: ui.RGBA(0x0D, 0x0D, 0x0D, 30.0/255.0),
		Text:         ui.RGB(0x0D, 0x0D, 0x0D),
		SidebarText:  ui.RGBA(0x0D, 0x0D, 0x0D, 217.0/255.0),
		Secondary:    ui.RGBA(0x0D, 0x0D, 0x0D, 177.0/255.0),
		Muted:        ui.RGBA(0x0D, 0x0D, 0x0D, 126.0/255.0),
		Accent:       ui.RGB(0x01, 0x69, 0xCC),
		AccentFill:   ui.RGB(0x0D, 0x0D, 0x0D),
		OnAccent:     ui.RGB(0xFF, 0xFF, 0xFF),
		Success:      ui.RGB(0x1A, 0x7F, 0x37),
		Warning:      ui.RGB(0xC9, 0x87, 0x00),
		Danger:       ui.RGB(0xE0, 0x2E, 0x2A),
		DangerSoft:   ui.RGBA(0xE0, 0x2E, 0x2A, 26.0/255.0),
		Overlay:      ui.RGBA(0x00, 0x00, 0x00, 34.0/255.0),
	}
}

// Palette picks the palette for the current appearance.
func (a *App) Palette() Palette {
	if a.Dark() {
		return PaletteDark()
	}
	return PaletteLight()
}
