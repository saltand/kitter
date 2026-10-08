package source

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saltand/kitter/native/core/library"
	"github.com/saltand/kitter/native/core/model"
)

func writeSkill(t *testing.T, path, name string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	content := fmt.Sprintf("---\nname: %s\ndescription: fixture\n---\n", name)
	if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeRunner installs a Runner that serves canned npx behaviour: it
// counts invocations and copies a prepared snapshot into the working
// directory, exactly like the Rust test's stub `npx` script — but
// in-process, replacing the Rust test's env::current_exe child run.
func fakeNpxRunner(t *testing.T, snapshot string) *int {
	t.Helper()
	calls := new(int)
	old := Runner
	Runner = func(ctx context.Context, c Command) error {
		*calls++
		if len(c.Args) < 4 || c.Args[2] != "add" {
			return fmt.Errorf("unexpected args %v", c.Args)
		}
		// The stub requires the '*' skill argument like the Rust fixture.
		skill := c.Args[5]
		if skill != "*" {
			return fmt.Errorf("exit 91")
		}
		return copyDir(snapshot, c.WorkDir)
	}
	t.Cleanup(func() { Runner = old })
	return calls
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

// TestNpxScanIsTheOnlyDownload mirrors
// npx_scan_is_the_only_download_even_after_partial_import_and_deletion:
// one npx call per scan, none during import, even after a partial
// import and deletion. The Rust test runs the fixture in a child
// process via env::current_exe; Go replaces the child with an
// in-process fake Runner because the Kitter CLI does not exist until M6.
func TestNpxScanIsTheOnlyDownload(t *testing.T) {
	temp := t.TempDir()
	snapshot := filepath.Join(temp, "snapshot")
	for i := 1; i <= 33; i++ {
		name := fmt.Sprintf("skill-%02d", i)
		writeSkill(t, filepath.Join(snapshot, ".agents", "skills", name), name)
	}
	entries := map[string]map[string]string{}
	for i := 1; i <= 33; i++ {
		entries[fmt.Sprintf("skill-%02d", i)] = map[string]string{"computedHash": fmt.Sprintf("hash-%d", i)}
	}
	lock, _ := json.Marshal(map[string]any{"skills": entries})
	if err := os.WriteFile(filepath.Join(snapshot, "skills-lock.json"), lock, 0o644); err != nil {
		t.Fatal(err)
	}
	calls := fakeNpxRunner(t, snapshot)
	dataDir := filepath.Join(temp, "data")
	oldDir := NpxDataDir
	NpxDataDir = func() string { return dataDir }
	t.Cleanup(func() { NpxDataDir = oldDir })

	repository := "https://github.com/fixture/import-test"
	ctx := context.Background()

	scan, err := ScanNpx(ctx, repository)
	if err != nil {
		t.Fatal(err)
	}
	selected := map[string]bool{}
	for _, s := range scan.Skills() {
		selected[s.Name] = true
	}
	// A failed eighteenth item leaves the same 17 persisted skills as an
	// interrupted import, without killing a worker or touching real data.
	if err := os.RemoveAll(scan.Skills()[17].Path); err != nil {
		t.Fatal(err)
	}
	lib, err := library.OpenIn(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scan.ImportSelected(lib, selected, "fixture"); err == nil {
		t.Fatal("expected import error")
	}
	skills, err := lib.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 18 { // 17 imported + built-in
		t.Fatalf("expected 18 skills, got %d", len(skills))
	}
	var group *model.SkillGroup
	for i, g := range lib.Groups() {
		if g.Name == "fixture" {
			group = &lib.Groups()[i]
			break
		}
	}
	if group == nil {
		t.Fatal("fixture group missing")
	}
	removed, err := lib.DeleteGroup(group.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 17 {
		t.Fatalf("expected 17 removed, got %d", len(removed))
	}
	if skills, _ := lib.List(); len(skills) != 1 {
		t.Fatalf("expected only builtin, got %d", len(skills))
	}

	scan, err = ScanNpx(ctx, repository)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := scan.ImportSelected(lib, selected, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if summary.Added != 33 || summary.Skipped != 0 {
		t.Fatalf("summary %+v", summary)
	}
	record, err := lib.Record("skill-18")
	if err != nil {
		t.Fatal(err)
	}
	if record.Origin.Type != "npx" || !record.Origin.HasSourceHsh || record.Origin.SourceHash != "hash-18" {
		t.Fatalf("origin %+v", record.Origin)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "npx-sources")); !os.IsNotExist(err) {
		t.Fatal("npx-sources workspace should not exist after import")
	}
	if *calls != 2 {
		t.Fatalf("exactly one download per scan, none during import: %d calls", *calls)
	}
}

// TestLocalBatchSkipsExisting mirrors
// local_batch_skips_existing_identity_and_imports_the_rest.
func TestLocalBatchSkipsExisting(t *testing.T) {
	temp := t.TempDir()
	sourceRoot := filepath.Join(temp, "source")
	writeSkill(t, filepath.Join(sourceRoot, "alpha"), "alpha")
	lib, err := library.OpenIn(filepath.Join(temp, "data"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := ScanLocal(sourceRoot)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := first.ImportSelected(lib, map[string]bool{"alpha": true}, "")
	if err != nil {
		t.Fatal(err)
	}
	if summary.Added != 1 || summary.Skipped != 0 {
		t.Fatalf("summary %+v", summary)
	}
	writeSkill(t, filepath.Join(sourceRoot, "beta"), "beta")
	second, err := ScanLocal(sourceRoot)
	if err != nil {
		t.Fatal(err)
	}
	summary, err = second.ImportSelected(lib, map[string]bool{"alpha": true, "beta": true}, "")
	if err != nil {
		t.Fatal(err)
	}
	if summary.Added != 1 || summary.Skipped != 1 {
		t.Fatalf("summary %+v", summary)
	}
	skills, _ := lib.List()
	names := map[string]bool{}
	for _, s := range skills {
		names[s.Record.Name] = true
	}
	if !names["alpha"] || !names["beta"] {
		t.Fatalf("names %v", names)
	}
}

// TestSkippedBatchNoEmptyGroup mirrors
// skipped_batch_does_not_create_an_empty_group.
func TestSkippedBatchNoEmptyGroup(t *testing.T) {
	temp := t.TempDir()
	sourceRoot := filepath.Join(temp, "source")
	writeSkill(t, filepath.Join(sourceRoot, "alpha"), "alpha")
	lib, err := library.OpenIn(filepath.Join(temp, "data"))
	if err != nil {
		t.Fatal(err)
	}
	selected := map[string]bool{"alpha": true}
	scan, err := ScanLocal(sourceRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scan.ImportSelected(lib, selected, ""); err != nil {
		t.Fatal(err)
	}
	scan, err = ScanLocal(sourceRoot)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := scan.ImportSelected(lib, selected, "owner/repository")
	if err != nil {
		t.Fatal(err)
	}
	if summary.Added != 0 || summary.Skipped != 1 {
		t.Fatalf("summary %+v", summary)
	}
	if len(lib.Groups()) != 0 {
		t.Fatalf("groups %v", lib.Groups())
	}
}

// TestNpxImportPreservesContent mirrors
// npx_import_preserves_the_scan_content_and_lock_hash.
func TestNpxImportPreservesContent(t *testing.T) {
	temp := t.TempDir()
	scanTemp := t.TempDir()
	scannedSkill := filepath.Join(scanTemp, ".agents", "skills", "alpha")
	if err := os.MkdirAll(scannedSkill, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scannedSkill, "SKILL.md"),
		[]byte("---\nname: alpha\ndescription: fresh\n---\nfresh scan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scanTemp, ".agents", ".skill-lock.json"),
		[]byte(`{"skills":{"alpha":{"skillFolderHash":"fresh-hash"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	scan := &SkillScan{
		origin: scanOrigin{kind: "npx", repository: "owner/repository"},
		skills: []ScannedSkill{{Name: "alpha", Description: "fresh", Path: scannedSkill}},
		temp:   scanTemp,
	}
	lib, err := library.OpenIn(filepath.Join(temp, "data"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scan.ImportSelected(lib, map[string]bool{"alpha": true}, ""); err != nil {
		t.Fatal(err)
	}
	path, err := lib.SkillPath("alpha")
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(path, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "fresh scan") {
		t.Fatalf("content %q", content)
	}
	record, err := lib.Record("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if record.Origin.Type != "npx" || !record.Origin.HasSourceHsh || record.Origin.SourceHash != "fresh-hash" {
		t.Fatalf("origin %+v", record.Origin)
	}
}
