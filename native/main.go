// Kitter desktop, Go + MyGo native UI port of the Rust GPUI app.
package main

import (
	"context"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

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
			// kitter-desktop.rs panics on an unreadable library; a modal
			// error tells the user before the app exits.
			mygo.Dialog.Error("Kitter", "无法打开 Kitter："+err.Error())
			os.Exit(1)
		}
		applyTheme(app)
		app.SetDark(mygo.Theme.IsDark())
		mygo.Theme.OnUpdated(func() {
			app.Apply(func() { app.SetDark(mygo.Theme.IsDark()) })
		})
		app.InitPost(mygo.NewWindow(mygo.WindowOptions{
			Title:         "Kitter",
			Width:         1200,
			Height:        720,
			MinWidth:      940,
			MinHeight:     600,
			StateKey:      "main",
			TitleBarStyle: mygo.TitleBarHiddenInset,
			Vibrancy:      mygo.VibrancySidebar,
			Content:       ui.View(app.View),
		}))
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

const (
	pathMarkStart = "__KITTER_PATH_START__"
	pathMarkEnd   = "__KITTER_PATH_END__"
)

// fixPathEnv merges the login shell's PATH into this process, as
// fix-path-env does in the Rust build, so skill sources that spawn git or
// npx work when the app is launched from Finder. An interactive shell may
// print banners or motd before our output, so the PATH is wrapped in
// markers and parsed back out of the noise.
func fixPathEnv() {
	shell := os.Getenv("SHELL")
	if shell == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, shell, "-ilc",
		`printf '%s' "`+pathMarkStart+`$PATH`+pathMarkEnd+`"`)
	cmd.Stdin = nil // no tty input: some shells block reading stdin
	out, err := cmd.Output()
	if err != nil {
		return // best effort, like fix-path-env
	}
	login := parseLoginPath(string(out))
	if login == "" {
		return
	}
	os.Setenv("PATH", login)
}

// parseLoginPath extracts the marked PATH value from login-shell output,
// ignoring anything before or after the markers (motd, banner, prompt
// noise). It returns "" without markers or for an empty value.
func parseLoginPath(out string) string {
	start := strings.Index(out, pathMarkStart)
	if start < 0 {
		return ""
	}
	rest := out[start+len(pathMarkStart):]
	end := strings.Index(rest, pathMarkEnd)
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:end])
}
