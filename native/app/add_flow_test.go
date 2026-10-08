// add_flow_test.go ports the AddSkill flow tests: local scan -> select
// -> import, scan error text, and the Existing adoption flow including
// cancellation.
package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/saltand/kitter/native/core/adoption"
	"github.com/saltand/kitter/native/core/model"
)

func TestAddLocalScanImport(t *testing.T) {
	app := newTestApp(t)
	app.SetDark(false)
	dir := t.TempDir()
	fixtureSkill(t, filepath.Join(dir, "alpha"), "alpha", "Alpha skill")
	fixtureSkill(t, filepath.Join(dir, "beta"), "beta", "Beta skill")

	// Stub the directory picker to return our folder.
	old := PickDirectory
	PickDirectory = func(string) ([]string, error) { return []string{dir}, nil }
	defer func() { PickDirectory = old }()

	tt := ui.NewTester(app.View, 1200, 720)
	app.openAddModal(AddLocal)
	tt.Frame()
	if !tt.HasText("Add Skill") {
		t.Fatal("add dialog not shown")
	}

	// Trigger browse -> scan.
	if err := tt.Click("Browse…"); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, app)
	tt.Frame()

	if app.AddFlow.Scan == nil {
		t.Fatal("scan did not run")
	}
	if len(app.AddFlow.Scan.Skills()) != 2 {
		t.Fatalf("expected 2 skills, got %v", app.AddFlow.Scan.Skills())
	}
	// Select only "alpha".
	app.AddFlow.Selected = map[string]bool{"alpha": true}
	tt.Frame()
	if err := tt.Click("Add"); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, app)
	tt.Frame()

	if !skillPresent(app, "alpha") {
		t.Fatal("alpha not imported")
	}
	if skillPresent(app, "beta") {
		t.Fatal("beta should not be imported")
	}
	if app.AddFlow.Open {
		t.Fatal("dialog should have closed")
	}
}

func TestAddLocalScanError(t *testing.T) {
	app := newTestApp(t)
	app.SetDark(false)
	tt := ui.NewTester(app.View, 1200, 720)
	app.openAddModal(AddLocal)
	tt.Frame()

	// Scan a nonexistent folder directly.
	app.scanLocalFolder(filepath.Join(t.TempDir(), "missing"))
	waitIdle(t, app)
	tt.Frame()

	if app.AddFlow.Error == "" {
		t.Fatal("expected an error")
	}
	if !tt.HasText(app.AddFlow.Error) {
		t.Fatalf("error text %q not visible", app.AddFlow.Error)
	}
}

func TestAddExistingOpenAndCancel(t *testing.T) {
	app := newTestApp(t)
	app.SetDark(false)
	tt := ui.NewTester(app.View, 1200, 720)

	app.openAddModal(AddExisting)
	tt.Frame()
	if !tt.HasText("Scan") {
		t.Fatal("existing kind not shown")
	}

	// Closing cancels the flow and clears the cancel func.
	app.AddFlow.AdoptionCancel = func() {} // pretend a scan is running
	app.closeAddModal()
	tt.Frame()
	if app.AddFlow.Open {
		t.Fatal("dialog should be closed")
	}
	if app.AddFlow.AdoptionCancel != nil {
		t.Fatal("cancel func should be cleared")
	}
}

func TestAddAdoptionListRows(t *testing.T) {
	app := newTestApp(t)
	// Two candidates sharing one source header, one with references.
	c1 := adoption.FixtureCandidate("/ext/one", model.OriginLocal("/ext/one", nil))
	c1.Name = "one"
	c1.Source = "/ext/one"
	c1.References = []model.SkillReference{
		{Path: "/proj/.agents/skills/one", Source: "/ext/one", Kind: model.ReferenceLink},
	}
	c2 := adoption.FixtureCandidate("/ext/two", model.OriginLocal("/ext/two", nil))
	c2.Name = "two"
	c2.Source = "/ext/two"
	scan := adoption.NewAdoptionScan([]*adoption.AdoptionCandidate{c1, c2}, nil)

	app.AddFlow.AdoptionScan = scan
	app.buildAdoptionRows()

	// header + candidate + disclosure + header + candidate = 5 rows
	// collapsed (two different sources -> two headers).
	if len(app.AddFlow.adoptionRows) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(app.AddFlow.adoptionRows))
	}
	if app.AddFlow.adoptionRows[0].kind != adoptionRowHeader {
		t.Fatal("first row should be a header")
	}
	// Expand c1 -> adds 1 reference row.
	app.toggleAdoptionExpanded(c1.ID)
	if len(app.AddFlow.adoptionRows) != 6 {
		t.Fatalf("expected 6 rows after expand, got %d", len(app.AddFlow.adoptionRows))
	}
	var sawRef bool
	for _, r := range app.AddFlow.adoptionRows {
		if r.kind == adoptionRowReference {
			sawRef = true
		}
	}
	if !sawRef {
		t.Fatal("expected a reference row")
	}
	// Collapse restores.
	app.toggleAdoptionExpanded(c1.ID)
	if len(app.AddFlow.adoptionRows) != 5 {
		t.Fatalf("expected 5 rows after collapse, got %d", len(app.AddFlow.adoptionRows))
	}
}

// TestAdoptContextCancel verifies ctx cancellation aborts ScanRoots.
func TestAdoptContextCancel(t *testing.T) {
	app := newTestApp(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := adoption.ScanRoots(ctx, t.TempDir(), []string{t.TempDir()}, app.Library.Config.LibraryDir, nil)
	if err == nil || !strings.Contains(err.Error(), "取消") {
		t.Fatalf("expected cancellation error, got %v", err)
	}
}

func skillPresent(app *App, name string) bool {
	for i := range app.Skills.Items {
		if app.Skills.Items[i].Record.Name == name {
			return true
		}
	}
	return false
}
