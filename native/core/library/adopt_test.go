package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/saltand/kitter/native/core/adoption"
	"github.com/saltand/kitter/native/core/config"
	"github.com/saltand/kitter/native/core/model"
	"github.com/saltand/kitter/native/core/project"
)

// adoption_fixture mirrors the Rust #[cfg(unix)] adoption_fixture: a
// library under data/skills plus a separate home directory.
func adoptionFixture(t *testing.T) (*SkillLibrary, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "home")
	libraryDir := filepath.Join(root, "data", "skills")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(libraryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.LibraryDir = libraryDir
	lib := &SkillLibrary{
		Config:   cfg,
		Registry: newRegistry(),
		DataDir:  filepath.Join(root, "data"),
	}
	return lib, home
}

// discover mirrors the Rust discover() helper.
func discover(t *testing.T, lib *SkillLibrary, home string) *adoption.AdoptionScan {
	t.Helper()
	skills, err := lib.List()
	if err != nil {
		t.Fatal(err)
	}
	scan, err := adoption.Scan(context.Background(), home, nil, lib.Config.LibraryDir, skills)
	if err != nil {
		t.Fatal(err)
	}
	return scan
}

// TestAdoptionLinksLibrary mirrors
// adoption_links_library_and_all_references_without_copying_or_deleting_source.
func TestAdoptionLinksLibrary(t *testing.T) {
	lib, home := adoptionFixture(t)
	source := filepath.Join(home, "source")
	fixtureSkill(t, source, "demo")
	if err := os.MkdirAll(filepath.Join(home, ".claude", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, ".claude", "skills", "demo")
	if err := os.Symlink("../../source", link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, filepath.Join(home, "custom-alias")); err != nil {
		t.Fatal(err)
	}
	scan := discover(t, lib, home)
	candidate := scan.Candidates[0]
	storage, err := lib.Adopt(candidate, scan.ReferencesFor(candidate))
	if err != nil {
		t.Fatal(err)
	}
	if !lib.IsLinkedSource(storage) {
		t.Fatal("not a linked source")
	}
	target, _ := os.Readlink(link)
	// The reference must still resolve to the source; Rust compares the
	// raw read_link result, which equals the absolute source there.
	resolved, err := filepath.EvalSymlinks(link)
	if err != nil || resolved != source {
		t.Fatalf("link resolves %q (raw %q)", resolved, target)
	}
	projects, err := project.List(home, lib.Config.LibraryDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) == 0 || !projects[0].Installations[0].Managed {
		t.Fatalf("installations %+v", projects)
	}
	// Re-adopting an already-managed source is a no-op.
	scan = discover(t, lib, home)
	if _, err := lib.Adopt(scan.Candidates[0], scan.ReferencesFor(scan.Candidates[0])); err != nil {
		t.Fatal(err)
	}
	if skills, _ := lib.List(); len(skills) != 1 {
		t.Fatalf("skills %d", len(skills))
	}
	if err := lib.RemoveByStorage(storage); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(source, "keep.txt")); err != nil || string(data) != "original source" {
		t.Fatal("source modified")
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatal("reference link not removed")
	}
	if resolved, err := filepath.EvalSymlinks(filepath.Join(home, "custom-alias")); err != nil || resolved != source {
		t.Fatal("outside alias touched")
	}
}

// TestAdoptionRollsBack mirrors
// adoption_rolls_back_links_and_registration_when_save_fails.
func TestAdoptionRollsBack(t *testing.T) {
	lib, home := adoptionFixture(t)
	fixtureSkill(t, filepath.Join(home, "source"), "demo")
	if err := os.MkdirAll(filepath.Join(home, ".claude", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../source", filepath.Join(home, ".claude", "skills", "alias")); err != nil {
		t.Fatal(err)
	}
	scan := discover(t, lib, home)
	// Point DataDir at a file so Save fails.
	lib.DataDir = filepath.Join(home, "source", "SKILL.md")
	if _, err := lib.Adopt(scan.Candidates[0], scan.ReferencesFor(scan.Candidates[0])); err == nil {
		t.Fatal("expected adopt failure")
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(home, ".claude", "skills", "alias"))
	if err != nil || resolved != filepath.Join(home, "source") {
		t.Fatal("link not rolled back")
	}
	if skills, _ := lib.List(); len(skills) != 0 {
		t.Fatalf("skills %d", len(skills))
	}
	if len(lib.Registry.Skills) != 0 {
		t.Fatal("registry not restored")
	}
	if _, err := os.Stat(filepath.Join(home, "source", "keep.txt")); err != nil {
		t.Fatal("source removed")
	}
}

// TestAdoptedSameNames mirrors
// adopted_same_names_keep_distinct_source_identities_and_storage.
func TestAdoptedSameNames(t *testing.T) {
	lib, home := adoptionFixture(t)
	fixtureSkill(t, filepath.Join(home, ".agents", "skills", "a"), "same")
	fixtureSkill(t, filepath.Join(home, ".agents", "skills", "b"), "same")
	scan := discover(t, lib, home)
	for _, candidate := range scan.Candidates {
		if _, err := lib.Adopt(candidate, nil); err != nil {
			t.Fatal(err)
		}
	}
	skills, err := lib.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 2 {
		t.Fatalf("skills %d", len(skills))
	}
	if skills[0].Record.IdentityKey() == skills[1].Record.IdentityKey() {
		t.Fatal("identities should differ")
	}
	if skills[0].Record.StorageName == skills[1].Record.StorageName {
		t.Fatal("storage should differ")
	}
	if _, err := lib.ResolveSkill("same"); err == nil {
		t.Fatal("ambiguous resolve should fail")
	}
	for _, skill := range skills {
		resolved, err := lib.ResolveSkill("id:" + skill.Record.StorageName)
		if err != nil {
			t.Fatal(err)
		}
		if resolved.Path != skill.Path {
			t.Fatal("path mismatch")
		}
	}
}

// TestSwitchingIdentity mirrors
// switching_an_identity_updates_chained_and_library_references.
func TestSwitchingIdentity(t *testing.T) {
	for _, failSave := range []bool{false, true} {
		lib, home := adoptionFixture(t)
		for _, project := range []string{"a", "b"} {
			root := filepath.Join(home, project)
			fixtureSkill(t, filepath.Join(root, ".agents", "skills", "alias"), "demo")
			if err := os.WriteFile(filepath.Join(root, "skills-lock.json"),
				[]byte(`{"skills":{"alias":{"source":"owner/repo","sourceType":"github"}}}`), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		a := filepath.Join(home, "a", ".agents", "skills", "alias")
		b := filepath.Join(home, "b", ".agents", "skills", "alias")
		scan := discover(t, lib, home)
		var first *adoption.AdoptionCandidate
		for _, c := range scan.Candidates {
			if c.Source == a {
				first = c
			}
		}
		if first == nil {
			t.Fatal("first candidate missing")
		}
		storage, err := lib.Adopt(first, nil)
		if err != nil {
			t.Fatal(err)
		}
		libraryPath := filepath.Join(lib.Config.LibraryDir, storage)
		if err := os.MkdirAll(filepath.Join(home, ".codex", "skills"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(libraryPath, filepath.Join(home, ".codex", "skills", "library-ref")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(a, filepath.Join(home, ".codex", "skills", "first-ref")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(home, ".codex", "skills", "first-ref"), filepath.Join(home, ".codex", "skills", "chained-ref")); err != nil {
			t.Fatal(err)
		}
		scan = discover(t, lib, home)
		var second *adoption.AdoptionCandidate
		for _, c := range scan.Candidates {
			if c.Source == b {
				second = c
			}
		}
		if second == nil {
			t.Fatal("second candidate missing")
		}
		if got := len(scan.ReferencesFor(second)); got != 5 {
			t.Fatalf("references %d", got)
		}
		if failSave {
			lib.DataDir = filepath.Join(home, "a", ".agents", "skills", "alias", "SKILL.md")
		}
		_, err = lib.Adopt(second, scan.ReferencesFor(second))
		if (err != nil) != failSave {
			t.Fatalf("adopt err %v, failSave %v", err, failSave)
		}
		expected := b
		if failSave {
			expected = a
		}
		for _, reference := range []string{"library-ref", "first-ref", "chained-ref"} {
			resolved, err := filepath.EvalSymlinks(filepath.Join(home, ".codex", "skills", reference))
			if err != nil || resolved != expected {
				t.Fatalf("%s resolved %q want %q", reference, resolved, expected)
			}
		}
		if resolved, _ := filepath.EvalSymlinks(libraryPath); resolved != expected {
			t.Fatalf("library path %q want %q", resolved, expected)
		}
		if _, err := os.Stat(filepath.Join(a, "keep.txt")); err != nil {
			t.Fatal("a source removed")
		}
		if _, err := os.Stat(filepath.Join(b, "keep.txt")); err != nil {
			t.Fatal("b source removed")
		}
		if skills, _ := lib.List(); len(skills) != 1 {
			t.Fatalf("skills %d", len(skills))
		}
	}
}

// TestRemovalKeepsRepointed mirrors
// removal_does_not_delete_a_repointed_reference_or_a_direct_source_directory.
func TestRemovalKeepsRepointed(t *testing.T) {
	lib, home := adoptionFixture(t)
	source := filepath.Join(home, ".agents", "skills", "demo")
	fixtureSkill(t, source, "demo")
	fixtureSkill(t, filepath.Join(home, "other"), "other")
	if err := os.MkdirAll(filepath.Join(home, ".claude", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, filepath.Join(home, ".claude", "skills", "alias")); err != nil {
		t.Fatal(err)
	}
	scan := discover(t, lib, home)
	var candidate *adoption.AdoptionCandidate
	for _, c := range scan.Candidates {
		if c.Name == "demo" {
			candidate = c
		}
	}
	storage, err := lib.Adopt(candidate, scan.ReferencesFor(candidate))
	if err != nil {
		t.Fatal(err)
	}
	projects, err := project.List(home, lib.Config.LibraryDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) == 0 || projects[0].Installations[0].Managed {
		t.Fatalf("installations %+v", projects)
	}
	if err := adoption.ReplaceLink(filepath.Join(home, ".claude", "skills", "alias"), filepath.Join(home, "other")); err != nil {
		t.Fatal(err)
	}
	if err := lib.RemoveByStorage(storage); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(source, "keep.txt")); err != nil {
		t.Fatal("source removed")
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(home, ".claude", "skills", "alias"))
	if err != nil || resolved != filepath.Join(home, "other") {
		t.Fatalf("resolved %q", resolved)
	}
}

// TestDirectInstallSkipsPlugins mirrors
// direct_install_scan_does_not_adopt_or_modify_plugins.
func TestDirectInstallSkipsPlugins(t *testing.T) {
	lib, home := adoptionFixture(t)
	root := filepath.Join(home, ".claude", "plugins", "cache", "market", "demo", "v1")
	fixtureSkill(t, filepath.Join(root, "skills", "demo"), "demo")
	registryPath := filepath.Join(home, ".claude", "plugins", "installed_plugins.json")
	bytes := []byte(`{"plugins":{"demo@market":[{"scope":"user","installPath":"` + root + `"}]}}`)
	if err := os.MkdirAll(filepath.Dir(registryPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(registryPath, bytes, 0o644); err != nil {
		t.Fatal(err)
	}
	scan := discover(t, lib, home)
	if len(scan.Candidates) != 0 {
		t.Fatalf("candidates %d", len(scan.Candidates))
	}
	if data, _ := os.ReadFile(registryPath); string(data) != string(bytes) {
		t.Fatal("registry modified")
	}
	if _, err := os.Stat(filepath.Join(root, "skills", "demo", "keep.txt")); err != nil {
		t.Fatal("plugin files touched")
	}
}

var _ = model.ReferenceLink
