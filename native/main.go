// Kitter desktop, Go + MyGo native UI port of the Rust GPUI app.
package main

import (
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	kitterapp "github.com/saltand/kitter/native/app"
	"github.com/saltand/kitter/native/core/config"
)

func main() {
	fixPathEnv()
	if err := kitterapp.RegisterFonts(); err != nil {
		log.Printf("kitter: register fonts: %v", err)
	}
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Role: mygo.RoleFileMenu},
		{Role: mygo.RoleEditMenu},
		{Role: mygo.RoleViewMenu},
		{Role: mygo.RoleWindowMenu},
	}))
	mygo.App.WhenReady(func() {
		app, err := kitterapp.NewApp(config.AppDataDir())
		if err != nil {
			log.Fatalf("kitter: open library: %v", err)
		}
		applyTheme(app)
		app.SetDark(mygo.Theme.IsDark())
		mygo.Theme.OnUpdated(func() { app.SetDark(mygo.Theme.IsDark()) })
		app.ShellWin = mygo.NewWindow(mygo.WindowOptions{
			Title:         "Kitter",
			Width:         1200,
			Height:        720,
			MinWidth:      940,
			MinHeight:     600,
			StateKey:      "main",
			TitleBarStyle: mygo.TitleBarHiddenInset,
			Vibrancy:      mygo.VibrancySidebar,
			Content:       ui.View(app.View),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// applyTheme maps the saved config theme onto MyGo's appearance override.
func applyTheme(app *kitterapp.App) {
	switch app.Library.Config.Theme {
	case config.ThemeLight:
		mygo.Theme.SetSource(mygo.ThemeLight)
	case config.ThemeDark:
		mygo.Theme.SetSource(mygo.ThemeDark)
	default:
		mygo.Theme.SetSource(mygo.ThemeSystem)
	}
}

// fixPathEnv merges the login shell's PATH into this process, as
// fix-path-env does in the Rust build, so skill sources that spawn git or
// npx work when the app is launched from Finder.
func fixPathEnv() {
	shell := os.Getenv("SHELL")
	if shell == "" {
		return
	}
	out, err := exec.Command(shell, "-ilc", `printf %s "$PATH"`).Output()
	if err != nil {
		return // best effort, like fix-path-env
	}
	login := strings.TrimSpace(string(out))
	if login == "" {
		return
	}
	os.Setenv("PATH", login)
}
