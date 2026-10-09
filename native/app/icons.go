// icons.go embeds the SVG icons and PNG agent icons plus the JetBrains
// Mono fonts (assets.rs / rust-embed in the Rust build).
package app

import (
	"embed"

	"github.com/egoist/mygo/ui"
)

//go:embed icons/*.svg
var iconFS embed.FS

//go:embed fonts/*.ttf
var fontFS embed.FS

// IconSVG parses an embedded icon; the name is its path under icons/,
// e.g. "sparkle.svg".
func IconSVG(name string) *ui.SVG {
	data, err := iconFS.ReadFile("icons/" + name)
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
