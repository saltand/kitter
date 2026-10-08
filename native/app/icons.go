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
