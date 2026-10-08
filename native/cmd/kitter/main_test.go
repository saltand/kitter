// Golden tests for the kitter CLI. Each case runs the real `run`
// dispatcher against a fresh KITTER_HOME with a seeded library and
// compares stdout to testdata/*.golden. The goldens are the JSON/human
// contract the builtin SKILL.md tells agents to parse.
package main

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/saltand/kitter/native/core/library"
	"github.com/saltand/kitter/native/core/model"
	"github.com/saltand/kitter/native/core/tags"
)

var update = flag.Bool("update", false, "regenerate golden files")

// seedLibrary writes one imported skill into the test data dir so list/
// show/group/tag commands have deterministic content.
func seedLibrary(t *testing.T, dataDir string) {
	t.Helper()
	lib, err := library.OpenIn(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dataDir, "source", "demo-skill")
	writeFile(t, filepath.Join(src, "SKILL.md"),
		"---\nname: demo-skill\ndescription: Demo description\n---\nbody\n")
	writeFile(t, filepath.Join(src, "notes.md"), "notes\n")
	if err := lib.Import(src, model.SkillRecord{
		Name:        "demo-skill",
		Description: "Demo description",
		Origin:      model.SkillOrigin{Type: "local", Path: src, HasSourceRt: true, SourceRoot: filepath.Dir(src)},
	}); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// normalize replaces the machine-specific data dir, temp paths and
// generated ids (group-*, tag ids) in output so goldens are stable
// across machines.
func normalize(out, dataDir string, extra map[string]string) string {
	out = strings.ReplaceAll(out, dataDir, "<DATA>")
	for k, v := range extra {
		out = strings.ReplaceAll(out, v, k)
	}
	// Generated ids: "group-<digits>", "id:<digits>" for tags.
	out = regexp.MustCompile(`group-\d+`).ReplaceAllString(out, "group-<N>")
	out = regexp.MustCompile(`id:\d+`).ReplaceAllString(out, "id:<N>")
	return out
}

func runCLI(t *testing.T, dataDir string, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	err := run(args, &buf)
	return normalize(buf.String(), dataDir, nil), err
}

// golden compares output to testdata/name.golden, regenerating with
// -update.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden %s (run with -update): %s", path, got)
	}
	if got != string(want) {
		t.Fatalf("output mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func TestListJSON(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("KITTER_HOME", dataDir)
	seedLibrary(t, dataDir)
	out, err := runCLI(t, dataDir, "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "list_json", out)
}

func TestListText(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("KITTER_HOME", dataDir)
	seedLibrary(t, dataDir)
	out, err := runCLI(t, dataDir, "list")
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "list_text", out)
}

func TestShowJSON(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("KITTER_HOME", dataDir)
	seedLibrary(t, dataDir)
	out, err := runCLI(t, dataDir, "show", "demo-skill", "--json")
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "show_json", out)
}

func TestShowText(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("KITTER_HOME", dataDir)
	seedLibrary(t, dataDir)
	out, err := runCLI(t, dataDir, "show", "demo-skill")
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "show_text", out)
}

func TestFilesAndRead(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("KITTER_HOME", dataDir)
	seedLibrary(t, dataDir)
	out, err := runCLI(t, dataDir, "files", "demo-skill")
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "files", out)
	out, err = runCLI(t, dataDir, "read", "demo-skill", "notes.md")
	if err != nil {
		t.Fatal(err)
	}
	if out != "notes\n" {
		t.Fatalf("read output %q", out)
	}
}

func TestCheckJSON(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("KITTER_HOME", dataDir)
	seedLibrary(t, dataDir)
	out, err := runCLI(t, dataDir, "check", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if out != "{\"updates\":0}\n" {
		t.Fatalf("check --json = %q", out)
	}
}

func TestGroupFlow(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("KITTER_HOME", dataDir)
	seedLibrary(t, dataDir)
	must := func(args ...string) string {
		out, err := runCLI(t, dataDir, args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return out
	}
	must("group", "create", "frontend")
	out := must("group", "assign", "frontend", "demo-skill")
	if out != "" {
		t.Fatalf("assign output %q", out)
	}
	golden(t, "group_list_json", must("group", "list", "--json"))
	must("group", "rename", "frontend", "web")
	golden(t, "group_list_text", must("group", "list"))
	must("group", "clear", "demo-skill")
	must("group", "delete", "web")
	if out := must("group", "list"); !strings.Contains(out, "No groups") {
		t.Fatalf("expected empty groups: %q", out)
	}
}

func TestTagFlow(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("KITTER_HOME", dataDir)
	seedLibrary(t, dataDir)
	must := func(args ...string) string {
		out, err := runCLI(t, dataDir, args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return out
	}
	must("tag", "create", "testing")
	must("tag", "create", "e2e", "--parent", "testing")
	must("tag", "assign", "e2e", "demo-skill")
	golden(t, "tag_list_json", must("tag", "list", "--json"))
	golden(t, "tag_list_text", must("tag", "list"))
	// list --tag filters through MatchesFilter (parent tags include
	// children).
	out := must("list", "--tag", "testing")
	if !strings.Contains(out, "demo-skill") {
		t.Fatalf("list --tag testing missing skill: %q", out)
	}
	must("tag", "unassign", "e2e", "demo-skill")
	out = must("list", "--tag", "testing")
	if strings.Contains(out, "demo-skill") {
		t.Fatalf("unassigned skill still listed: %q", out)
	}
	must("tag", "rename", "e2e", "e2e-renamed")
	// Cross-level move fails (e2e-renamed is a child of testing).
	if _, err := runCLI(t, dataDir, "tag", "move", "e2e-renamed", "--before", "testing"); err == nil {
		t.Fatal("cross-level move should fail")
	}
}

func TestTagMoveSameLevel(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("KITTER_HOME", dataDir)
	must := func(args ...string) {
		if _, err := runCLI(t, dataDir, args...); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	must("tag", "create", "alpha")
	must("tag", "create", "beta")
	// alpha before beta → already before; move beta before alpha works.
	must("tag", "move", "beta", "--before", "alpha")
	out, err := runCLI(t, dataDir, "tag", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"name": "beta"`) ||
		strings.Index(out, `"name": "beta"`) > strings.Index(out, `"name": "alpha"`) {
		t.Fatalf("beta should sort before alpha: %s", out)
	}
	// Cross-parent move fails.
	must("tag", "create", "child", "--parent", "alpha")
	if _, err := runCLI(t, dataDir, "tag", "move", "child", "--after", "beta"); err == nil {
		t.Fatal("cross-level move should fail")
	}
}

func TestLibrarySetAndShow(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("KITTER_HOME", dataDir)
	out, err := runCLI(t, dataDir, "library")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "<DATA>/skills") {
		t.Fatalf("library output %q", out)
	}
	newDir := filepath.Join(dataDir, "newlib")
	out, err = runCLI(t, dataDir, "library", "--set", newDir)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "<DATA>/newlib" {
		t.Fatalf("library --set output %q", out)
	}
	if _, err := runCLI(t, dataDir, "library", "--set", "relative/path"); err == nil {
		t.Fatal("relative --set should fail")
	}
}

func TestRemove(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("KITTER_HOME", dataDir)
	seedLibrary(t, dataDir)
	out, err := runCLI(t, dataDir, "remove", "demo-skill")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Removed 1 skill") {
		t.Fatalf("remove output %q", out)
	}
}

func TestUnknownSelectorErrors(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("KITTER_HOME", dataDir)
	seedLibrary(t, dataDir)
	if _, err := runCLI(t, dataDir, "show", "nonexistent"); err == nil {
		t.Fatal("unknown skill should error")
	}
	if _, err := runCLI(t, dataDir, "tag", "assign", "nonexistent-tag", "demo-skill"); err == nil {
		t.Fatal("unknown tag should error")
	}
	if _, err := runCLI(t, dataDir, "group", "assign", "nonexistent-group", "demo-skill"); err == nil {
		t.Fatal("unknown group should error")
	}
}

func TestUpdateRequiresSelector(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("KITTER_HOME", dataDir)
	if _, err := runCLI(t, dataDir, "update"); err == nil ||
		!strings.Contains(err.Error(), "请指定至少一个 skill") {
		t.Fatalf("bare update should fail like Rust: %v", err)
	}
	if _, err := runCLI(t, dataDir, "update", "--all", "extra"); err == nil {
		t.Fatal("--all with skills should conflict")
	}
}

func TestArgParsingMatchesClap(t *testing.T) {
	// Mirrors the Rust cli parse tests.
	dataDir := t.TempDir()
	t.Setenv("KITTER_HOME", dataDir)
	seedLibrary(t, dataDir)
	projectDir := t.TempDir()

	// Batch install: flags may follow positionals like clap.
	out, err := runCLI(t, dataDir, "install", "demo-skill",
		"--project", projectDir, "--target", "universal", "--target", "codex")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Installed 1 skill") {
		t.Fatalf("install output %q", out)
	}
	// project --view plugins --agent codex parses.
	if _, err := runCLI(t, dataDir, "project", projectDir,
		"--agent", "codex", "--view", "plugins", "--json"); err != nil {
		t.Fatal(err)
	}
	// tag assign with multiple items.
	if _, err := runCLI(t, dataDir, "tag", "create", "client"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, dataDir, "tag", "assign", "client", "demo-skill"); err != nil {
		t.Fatal(err)
	}
	// config set-theme is not a command (Rust test).
	if _, err := runCLI(t, dataDir, "config", "set-theme", "dark"); err == nil {
		t.Fatal("config should not be a command")
	}
}

func TestUninstallFlow(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("KITTER_HOME", dataDir)
	seedLibrary(t, dataDir)
	projectDir := t.TempDir()
	if _, err := runCLI(t, dataDir, "install", "demo-skill",
		"--project", projectDir, "--target", "universal"); err != nil {
		t.Fatal(err)
	}
	out, err := runCLI(t, dataDir, "uninstall", "demo-skill", "--project", projectDir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Removed 1 installation") {
		t.Fatalf("uninstall output %q", out)
	}
	// --path form.
	if _, err := runCLI(t, dataDir, "install", "demo-skill",
		"--project", projectDir, "--target", "universal"); err != nil {
		t.Fatal(err)
	}
	out, err = runCLI(t, dataDir, "uninstall",
		"--project", projectDir, "--path", filepath.Join(".agents", "skills", "demo-skill"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Removed 1 installation") {
		t.Fatalf("uninstall --path output %q", out)
	}
}

func TestProjectJSON(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("KITTER_HOME", dataDir)
	seedLibrary(t, dataDir)
	projectDir := t.TempDir()
	if _, err := runCLI(t, dataDir, "install", "demo-skill",
		"--project", projectDir, "--target", "universal"); err != nil {
		t.Fatal(err)
	}
	out, err := runCLI(t, dataDir, "project", projectDir, "--json")
	if err != nil {
		t.Fatal(err)
	}
	out = normalize(out, dataDir, map[string]string{"<PROJECT>": projectDir})
	golden(t, "project_json", out)
}

func TestTagsResolveLikeRust(t *testing.T) {
	// Mirrors tags_use_names_and_only_require_ids_for_duplicates.
	state := tags.NewTagState()
	work, err := state.Add("work", nil)
	if err != nil {
		t.Fatal(err)
	}
	personal, err := state.Add("personal", nil)
	if err != nil {
		t.Fatal(err)
	}
	workClient, err := state.Add("client", &work)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.Add("client", &personal); err != nil {
		t.Fatal(err)
	}
	if got, err := resolveTag(state, "work"); err != nil || got != work {
		t.Fatalf("resolveTag(work) = %v, %v", got, err)
	}
	if _, err := resolveTag(state, "client"); err == nil {
		t.Fatal("ambiguous client should error")
	}
	got, err := resolveTag(state, "id:"+strconv.FormatUint(uint64(workClient), 10))
	if err != nil || got != workClient {
		t.Fatalf("resolveTag(id:) = %v, %v", got, err)
	}
}
