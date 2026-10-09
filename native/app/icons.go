// icons.go embeds the SVG icons and PNG agent icons plus the JetBrains
// Mono fonts (assets.rs / rust-embed in the Rust build).
package app

import (
	"embed"
	"strings"

	"github.com/egoist/mygo/ui"
)

//go:embed icons/*.svg
var iconFS embed.FS

//go:embed fonts/*.ttf
var fontFS embed.FS

// IconSVG parses an embedded icon; the name is its path under icons/,
// e.g. "sparkle.svg", with or without the "icons/" prefix the agent
// tables carry.
func IconSVG(name string) *ui.SVG {
	data, err := iconFS.ReadFile("icons/" + strings.TrimPrefix(name, "icons/"))
	if err != nil {
		return nil
	}
	svg, err := ui.ParseSVG(data)
	if err != nil {
		return nil
	}
	return svg
}

// RegisterFonts registers the embedded JetBrains Mono family.
func RegisterFonts() error {
	for _, name := range []string{"JetBrainsMono-Regular.ttf", "JetBrainsMono-Bold.ttf"} {
		data, err := fontFS.ReadFile("fonts/" + name)
		if err != nil {
			return err
		}
		if err := ui.RegisterFont(data, "JetBrains Mono"); err != nil {
			return err
		}
	}
	return nil
}

// iconButton is an embedded icon drawn glyph points high, centered in a
// box-point square. MyGo sizes are border boxes, so the padding shrinks
// the glyph while the whole box stays the hit target, as the Rust build's
// fixed-size div around a 16px svg did.
func iconButton(c *ui.Context, name string, box, glyph float32) ui.Element {
	return ui.Icon(c, IconSVG(name)).Size(box, box).Padding((box - glyph) / 2)
}

// brandIcon is components.rs brand_icon: the multi-color agent marks keep
// their own colors, the rest draw in the text color. Some marks fill less
// of their viewBox and are scaled up to the same perceived size.
func brandIcon(c *ui.Context, path string, size float32) ui.Element {
	switch path {
	case "icons/provider-claude.svg":
		size *= 1.4
	case "icons/provider-codex.svg":
		size *= 1.25
	case "icons/provider-opencode.svg", "icons/provider-grok.svg":
		size *= 1.1
	}
	switch path {
	case "icons/provider-codex.svg", "icons/provider-claude.svg", "icons/provider-openclaw.svg",
		"icons/provider-amp.svg", "icons/provider-antigravity.svg", "icons/provider-trae.svg":
		return ui.Image(c, IconSVG(path)).Size(size, size).Shrink(0)
	}
	return ui.Icon(c, IconSVG(path)).Size(size, size).Shrink(0)
}

// textButton is a label vertically centered in a height-point row; MyGo
// draws a Text at the top of a taller box. Text styles (FontSize,
// TextColor) chained on the row reach the label.
func textButton(c *ui.Context, label string, height float32) ui.Element {
	return ui.Row(c).Height(height).AlignItems(ui.Center).Shrink(0).Children(func() {
		ui.Text(c, label).SingleLine()
	})
}
