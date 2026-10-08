package adoption

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/saltand/kitter/native/core/model"
)

func writeSkillAt(t *testing.T, path, name string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	content := fmt.Sprintf("---\nname: %s\ndescription: fixture\n---\nbody", name)
	if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func scanHome(t *testing.T, home string) *AdoptionScan {
	t.Helper()
	scan, err := Scan(context.Background(), home, nil, filepath.Join(home, "kitter-library"), nil)
	if err != nil {
		t.Fatal(err)
	}
	return scan
}

// TestMarkerDefinesIdentity mirrors
// marker_not_directory_name_defines_identity_and_aliases_are_merged.
func TestMarkerDefinesIdentity(t *testing.T) {
	temp := t.TempDir()
	home, _ := filepath.EvalSymlinks(temp)
	source := filepath.Join(home, "sources", "not-the-name")
	writeSkillAt(t, source, "actual-name")
	mustMkdir(t, filepath.Join(home, ".agents", "skills"))
	mustMkdir(t, filepath.Join(home, "project", ".claude", "skills"))
	mustSymlink(t, source, filepath.Join(home, ".agents", "skills", "alias-one"))
	mustSymlink(t, "../../../sources/not-the-name", filepath.Join(home, "project", ".claude", "skills", "alias-two"))
	mustMkdir(t, filepath.Join(home, "looks-like-a-skill"))
	if err := os.WriteFile(filepath.Join(home, "looks-like-a-skill", "SKILL.md"), []byte("# no metadata"), 0o644); err != nil {
		t.Fatal(err)
	}
	scan := scanHome(t, home)
	if len(scan.Candidates) != 1 {
		t.Fatalf("candidates %d", len(scan.Candidates))
	}
	if scan.Candidates[0].Name != "actual-name" {
		t.Fatalf("name %q", scan.Candidates[0].Name)
	}
	if len(scan.Candidates[0].References) != 2 {
		t.Fatalf("references %d", len(scan.Candidates[0].References))
	}
}

// TestSameNameDifferentLocalSources mirrors
// same_name_different_local_sources_coexist.
func TestSameNameDifferentLocalSources(t *testing.T) {
	temp := t.TempDir()
	writeSkillAt(t, filepath.Join(temp, ".agents", "skills", "a"), "same")
	writeSkillAt(t, filepath.Join(temp, ".agents", "skills", "b"), "same")
	scan := scanHome(t, temp)
	if len(scan.Candidates) != 2 {
		t.Fatalf("candidates %d", len(scan.Candidates))
	}
	if len(scan.DefaultSelection()) != 2 {
		t.Fatalf("default selection %d", len(scan.DefaultSelection()))
	}
	if scan.Candidates[0].Identity() == scan.Candidates[1].Identity() {
		t.Fatal("identities should differ")
	}
}

// TestOnlyDirectInstallations mirrors
// only_direct_agent_installations_are_discovered.
func TestOnlyDirectInstallations(t *testing.T) {
	temp := t.TempDir()
	home, _ := filepath.EvalSymlinks(temp)
	roots := []string{
		".agents/skills",
		".claude/skills",
		".codex/skills",
		".pi/skills",
		".pi/agent/skills",
		".config/opencode/skills",
		"project/.cursor/skills",
	}
	for i, root := range roots {
		writeSkillAt(t, filepath.Join(home, root, "folder"), fmt.Sprintf("direct-%d", i))
		writeSkillAt(t, filepath.Join(home, root, "folder", "examples", "nested"), "nested-example")
		writeSkillAt(t, filepath.Join(home, root, "collection", "inside"), "nested-collection")
	}
	for _, path := range []string{
		"checkout/skills/loose",
		"project/docs/example",
		".pi/agent/extensions/pkg/.claude/skills/hidden",
		".codex/plugins/cache/pkg/.agents/skills/hidden",
	} {
		writeSkillAt(t, filepath.Join(home, path), "not-direct")
	}
	scan := scanHome(t, home)
	if len(scan.Candidates) != len(roots) {
		t.Fatalf("candidates %d, want %d", len(scan.Candidates), len(roots))
	}
	for _, c := range scan.Candidates {
		if len(c.Name) < 7 || c.Name[:7] != "direct-" {
			t.Fatalf("candidate %q", c.Name)
		}
	}
}

// TestAliasedSkillsRoot mirrors
// selecting_an_aliased_skills_root_keeps_its_logical_installation_paths.
func TestAliasedSkillsRoot(t *testing.T) {
	temp := t.TempDir()
	home, _ := filepath.EvalSymlinks(temp)
	writeSkillAt(t, filepath.Join(home, "source", "demo"), "demo")
	mustMkdir(t, filepath.Join(home, ".claude"))
	mustSymlink(t, filepath.Join(home, "source"), filepath.Join(home, ".claude", "skills"))
	scan, err := ScanRoots(context.Background(), home,
		[]string{filepath.Join(home, ".claude", "skills")},
		filepath.Join(home, "library"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(scan.Candidates) != 1 {
		t.Fatalf("candidates %d", len(scan.Candidates))
	}
	if scan.Candidates[0].References[0].Path != filepath.Join(home, ".claude", "skills", "demo") {
		t.Fatalf("reference path %q", scan.Candidates[0].References[0].Path)
	}
	if scan.Candidates[0].References[0].Kind != model.ReferenceAlias {
		t.Fatalf("kind %q", scan.Candidates[0].References[0].Kind)
	}
}

// TestLargeSelection mirrors
// large_selection_uses_identity_indexes_and_reindexes_after_filtering.
func TestLargeSelection(t *testing.T) {
	origin := model.OriginNpx("fixture/repo", "same-name", nil)
	var candidates []*AdoptionCandidate
	for i := 0; i < 10000; i++ {
		path := fmt.Sprintf("/fixture/%d", i)
		o := model.OriginLocal(path, nil)
		if i < 2 {
			o = origin
		}
		candidates = append(candidates, FixtureCandidate(path, o))
	}
	scan := NewAdoptionScan(candidates, nil)
	if len(scan.SelectableIDs()) != 9998 {
		t.Fatalf("selectable %d", len(scan.SelectableIDs()))
	}
	selected := scan.DefaultSelection()
	scan.Select(selected, "/fixture/0")
	scan.Select(selected, "/fixture/1")
	if selected["/fixture/0"] {
		t.Fatal("0 should be deselected by 1's select (same identity)")
	}
	if !selected["/fixture/1"] {
		t.Fatal("1 should be selected")
	}
	scan.Retain(func(c *AdoptionCandidate) bool { return c.ID != "/fixture/0" })
	if len(scan.SelectableIDs()) != 9999 {
		t.Fatalf("selectable after retain %d", len(scan.SelectableIDs()))
	}
	if scan.HasConflict(origin.IdentityKey("same-name")) {
		t.Fatal("conflict should clear after retain")
	}
}

// TestGitSource mirrors
// git_source_records_the_repository_and_skill_subdirectory, with
// GitRunner faked instead of running git init/config.
func TestGitSource(t *testing.T) {
	temp := t.TempDir()
	home, _ := filepath.EvalSymlinks(temp)
	root := filepath.Join(home, "checkout")
	writeSkillAt(t, filepath.Join(root, "skills", "not-the-name"), "review")
	// A .git dir marks the checkout; GitRunner returns the remote URL.
	mustMkdir(t, filepath.Join(root, ".git"))
	old := GitRunner
	GitRunner = func(ctx context.Context, r string) (string, error) {
		if r != root {
			return "", fmt.Errorf("unexpected root %s", r)
		}
		return "https://github.com/fixture/skills.git", nil
	}
	defer func() { GitRunner = old }()
	mustMkdir(t, filepath.Join(home, ".agents", "skills"))
	mustSymlink(t, filepath.Join(root, "skills", "not-the-name"), filepath.Join(home, ".agents", "skills", "review"))
	scan := scanHome(t, home)
	if len(scan.Candidates) != 1 {
		t.Fatalf("candidates %d", len(scan.Candidates))
	}
	o := scan.Candidates[0].Origin
	if o.Type != "git" || o.Repository != "https://github.com/fixture/skills.git" ||
		!o.HasSubdir || o.Subdir != "skills/not-the-name" {
		t.Fatalf("origin %+v", o)
	}
}

// TestSameNpxIdentity mirrors
// same_npx_identity_requires_choice_and_keeps_all_references.
func TestSameNpxIdentity(t *testing.T) {
	temp := t.TempDir()
	home, _ := filepath.EvalSymlinks(temp)
	for _, project := range []string{"a", "b"} {
		root := filepath.Join(home, project)
		writeSkillAt(t, filepath.Join(root, ".agents", "skills", "demo"), "demo")
		mustMkdir(t, filepath.Join(root, ".claude", "skills"))
		mustSymlink(t, "../../.agents/skills/demo", filepath.Join(root, ".claude", "skills", "demo"))
		if err := os.WriteFile(filepath.Join(root, "skills-lock.json"),
			[]byte(`{"version":1,"skills":{"demo":{"source":"owner/repo","sourceType":"github","computedHash":"x"}}}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	scan := scanHome(t, home)
	if len(scan.Candidates) != 2 {
		t.Fatalf("candidates %d", len(scan.Candidates))
	}
	if len(scan.DefaultSelection()) != 0 {
		t.Fatalf("default selection %d", len(scan.DefaultSelection()))
	}
	selected := map[string]bool{}
	scan.Select(selected, scan.Candidates[0].ID)
	scan.Select(selected, scan.Candidates[1].ID)
	if len(selected) != 1 {
		t.Fatalf("selected %d", len(selected))
	}
	if !selected[scan.Candidates[1].ID] {
		t.Fatal("second candidate should be selected")
	}
	if got := len(scan.ReferencesFor(scan.Candidates[1])); got != 4 {
		t.Fatalf("references %d", got)
	}
}

// TestExcludes mirrors
// excludes_media_packages_caches_and_does_not_follow_privacy_aliases.
func TestExcludes(t *testing.T) {
	temp := t.TempDir()
	home, _ := filepath.EvalSymlinks(temp)
	for _, name := range []string{
		"Pictures/photo",
		"Music/music",
		"Library/private",
		"stuff/foo.photoslibrary/hidden",
		"node_modules/package",
		"target/debug",
		"kitter-library/owned",
		".codex/skills/.system/builtin",
	} {
		writeSkillAt(t, filepath.Join(home, name), "hidden")
	}
	mustSymlink(t, filepath.Join(home, "Pictures", "photo"), filepath.Join(home, "innocent-link"))
	writeSkillAt(t, filepath.Join(home, ".claude", "skills", "visible"), "visible")
	scan := scanHome(t, home)
	if len(scan.Candidates) != 1 {
		t.Fatalf("candidates %d", len(scan.Candidates))
	}
	if scan.Candidates[0].Name != "visible" {
		t.Fatalf("name %q", scan.Candidates[0].Name)
	}
}

// TestParentAlias mirrors
// parent_alias_is_an_observation_not_a_replaceable_directory.
func TestParentAlias(t *testing.T) {
	temp := t.TempDir()
	home, _ := filepath.EvalSymlinks(temp)
	writeSkillAt(t, filepath.Join(home, ".agents", "skills", "demo"), "demo")
	mustMkdir(t, filepath.Join(home, ".claude"))
	mustSymlink(t, "../.agents/skills", filepath.Join(home, ".claude", "skills"))
	scan := scanHome(t, home)
	if len(scan.Candidates) != 1 {
		t.Fatalf("candidates %d", len(scan.Candidates))
	}
	found := false
	for _, r := range scan.Candidates[0].References {
		if r.Kind == model.ReferenceAlias {
			found = true
		}
	}
	if !found {
		t.Fatal("no alias reference")
	}
}

// TestPluginExclusions mirrors
// plugin_registries_caches_and_direct_links_into_plugins_are_excluded.
func TestPluginExclusions(t *testing.T) {
	temp := t.TempDir()
	home, _ := filepath.EvalSymlinks(temp)
	root := filepath.Join(home, ".claude", "plugins", "cache", "market", "plugin", "v2")
	writeSkillAt(t, filepath.Join(root, "skills", "wrong-dir"), "actual")
	writeSkillAt(t, filepath.Join(home, ".claude", "plugins", "cache", "market", "plugin", "v1", "skills", "old"), "stale")
	if err := os.WriteFile(filepath.Join(home, ".claude", "plugins", "installed_plugins.json"),
		[]byte(`{"plugins":{"plugin@market":[{"scope":"user","installPath":"`+root+`"}]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	mustMkdir(t, filepath.Join(home, ".codex", "skills"))
	mustSymlink(t, filepath.Join(root, "skills", "wrong-dir"), filepath.Join(home, ".codex", "skills", "external-plugin-link"))
	mustSymlink(t, filepath.Join(home, ".claude", "plugins", "cache", "market", "plugin", "v1", "skills", "old"), filepath.Join(home, "stale-plugin-link"))
	scan := scanHome(t, home)
	if len(scan.Candidates) != 0 {
		t.Fatalf("candidates %d", len(scan.Candidates))
	}
}

// TestInvalidMarkers mirrors
// invalid_markers_never_fall_back_to_directory_names.
func TestInvalidMarkers(t *testing.T) {
	temp := t.TempDir()
	for i, marker := range []string{
		"---\ndescription: missing name\n---",
		"---\nname: unclosed",
		"---\nname: ../escape\n---",
		"---\nname: [invalid]\n---",
	} {
		path := filepath.Join(temp, ".agents", "skills", fmt.Sprintf("folder-%d", i))
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte(marker), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(scanHome(t, temp).Candidates); got != 0 {
		t.Fatalf("candidates %d", got)
	}
}

// TestCancellationAndDrift mirrors
// cancellation_and_marker_drift_are_detected.
func TestCancellationAndDrift(t *testing.T) {
	temp := t.TempDir()
	writeSkillAt(t, filepath.Join(temp, ".agents", "skills", "demo"), "demo")
	scan := scanHome(t, temp)
	if err := os.WriteFile(filepath.Join(temp, ".agents", "skills", "demo", "SKILL.md"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := scan.Candidates[0].Verify(); err == nil {
		t.Fatal("verify should fail after drift")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Scan(ctx, temp, nil, filepath.Join(temp, "lib"), nil); err == nil {
		t.Fatal("cancelled scan should error")
	}
}

// TestStaleLink mirrors stale_link_is_not_overwritten.
func TestStaleLink(t *testing.T) {
	temp := t.TempDir()
	home, _ := filepath.EvalSymlinks(temp)
	writeSkillAt(t, filepath.Join(home, "source"), "demo")
	writeSkillAt(t, filepath.Join(home, "other"), "other")
	mustMkdir(t, filepath.Join(home, ".agents", "skills"))
	mustSymlink(t, filepath.Join(home, "source"), filepath.Join(home, ".agents", "skills", "link"))
	scan := scanHome(t, home)
	var reference *model.SkillReference
	for _, c := range scan.Candidates {
		if c.Name == "demo" {
			reference = &c.References[0]
		}
	}
	if reference == nil {
		t.Fatal("demo reference missing")
	}
	if err := ReplaceLink(filepath.Join(home, ".agents", "skills", "link"), filepath.Join(home, "other")); err != nil {
		t.Fatal(err)
	}
	if err := ValidateReference(reference); err == nil {
		t.Fatal("validate should fail after repoint")
	}
	resolved, _ := filepath.EvalSymlinks(filepath.Join(home, ".agents", "skills", "link"))
	if resolved != filepath.Join(home, "other") {
		t.Fatalf("resolved %q", resolved)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

var _ = sort.Strings
