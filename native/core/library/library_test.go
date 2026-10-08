package library

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saltand/kitter/native/core/config"
	"github.com/saltand/kitter/native/core/effective"
	"github.com/saltand/kitter/native/core/model"
	"github.com/saltand/kitter/native/core/skillfile"
)

// Mirrors library.rs tests; adoption tests (#[cfg(unix)] block) are M3.

func fixtureSkill(t *testing.T, path, name string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: test\n---\n"
	if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "keep.txt"), []byte("original source"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuiltinSkillIsManualPinnedAndProtected(t *testing.T) {
	temp := t.TempDir()
	dataDir := filepath.Join(temp, "data")
	libraryDir := filepath.Join(dataDir, "skills")
	if err := os.MkdirAll(libraryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fixtureSkill(t, filepath.Join(libraryDir, "another-skill"), "another-skill")
	if err := EnsureBuiltinSkill(libraryDir); err != nil {
		t.Fatal(err)
	}

	builtinPath := filepath.Join(libraryDir, KitterSkillStorage)
	if !effective.IsManualSkill(builtinPath) {
		t.Fatal("builtin skill must be manual-only")
	}
	skillMD, err := os.ReadFile(filepath.Join(builtinPath, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		"kitter install",
		"Get-Command kitter",
		"disable-model-invocation: true",
		"autoinvoke: false",
	} {
		if !strings.Contains(string(skillMD), needle) {
			t.Fatalf("SKILL.md missing %q", needle)
		}
	}
	installCLI, _ := os.ReadFile(filepath.Join(builtinPath, "references", "install-cli.md"))
	if !strings.Contains(string(installCLI), "gh release download") {
		t.Fatal("install-cli.md missing content")
	}
	openaiYaml, _ := os.ReadFile(filepath.Join(builtinPath, "agents", "openai.yaml"))
	if !strings.Contains(string(openaiYaml), "allow_implicit_invocation: false") {
		t.Fatal("openai.yaml missing policy")
	}
	if _, err := os.Stat(filepath.Join(builtinPath, "bin")); !os.IsNotExist(err) {
		t.Fatal("unexpected bin directory")
	}

	registry := newRegistry()
	registry.Skills[KitterSkillStorage] = model.SkillRecord{
		Name:           kitterSkillName,
		StorageName:    KitterSkillStorage,
		Origin:         model.OriginBuiltin(),
		LastOperatedAt: ^uint64(0),
	}
	library := &SkillLibrary{
		Config:   config.AppConfig{LibraryDir: libraryDir},
		Registry: registry,
		DataDir:  dataDir,
	}
	skills, err := library.List()
	if err != nil {
		t.Fatal(err)
	}
	if skills[0].Record.Name != kitterSkillName {
		t.Fatalf("first skill %q", skills[0].Record.Name)
	}
	if !skills[0].Record.Origin.IsBuiltin() {
		t.Fatal("first skill not builtin")
	}
	if err := library.RemoveByStorage(KitterSkillStorage); err == nil {
		t.Fatal("builtin removal must fail")
	}
	if err := library.AssignGroupByStorage(KitterSkillStorage, nil); err == nil {
		t.Fatal("builtin group assignment must fail")
	}
}

func TestReadsLiteralBlockDescriptions(t *testing.T) {
	temp := t.TempDir()
	path := filepath.Join(temp, "SKILL.md")
	content := "---\nname: multiline\ndescription: |\n  First line\n  Second line\n---\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	name, desc, err := skillfile.ReadFrontmatter(path)
	if err != nil {
		t.Fatal(err)
	}
	if name != "multiline" {
		t.Fatalf("name %q", name)
	}
	if desc != "First line\nSecond line" {
		t.Fatalf("description %q", desc)
	}
}

func TestListReadsCurrentDescriptionWithoutPersistingIt(t *testing.T) {
	temp := t.TempDir()
	libraryDir := filepath.Join(temp, "skills")
	skillDir := filepath.Join(libraryDir, "demo")
	os.MkdirAll(skillDir, 0o755)
	skillFile := filepath.Join(skillDir, "SKILL.md")
	os.WriteFile(skillFile, []byte("---\nname: demo\ndescription: first\n---\n"), 0o644)

	library := &SkillLibrary{
		Config:   config.AppConfig{LibraryDir: libraryDir},
		Registry: newRegistry(),
		DataDir:  filepath.Join(temp, "data"),
	}
	skills, err := library.List()
	if err != nil {
		t.Fatal(err)
	}
	if skills[0].Record.Description != "first" {
		t.Fatalf("description %q", skills[0].Record.Description)
	}
	os.WriteFile(skillFile, []byte("---\nname: demo\ndescription: second\n---\n"), 0o644)
	skills, _ = library.List()
	if skills[0].Record.Description != "second" {
		t.Fatalf("description %q", skills[0].Record.Description)
	}
	data, err := json.Marshal(skills[0].Record)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "description") {
		t.Fatal("description must not be persisted")
	}
}

func TestGroupOrderIsPersistedWithoutChangingIdentity(t *testing.T) {
	temp := t.TempDir()
	library, err := OpenIn(temp)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := library.CreateGroup("A")
	b, _ := library.CreateGroup("B")
	c, _ := library.CreateGroup("C")
	moved, err := library.MoveGroup(a.ID, c.ID, true)
	if err != nil || !moved {
		t.Fatalf("move %v %v", moved, err)
	}
	var names []string
	for _, g := range library.Groups() {
		names = append(names, g.Name)
	}
	if strings.Join(names, ",") != "B,C,A" {
		t.Fatalf("order %v", names)
	}
	if moved, _ := library.MoveGroup(a.ID, b.ID, false); !moved {
		t.Fatal("expected move")
	}
	if moved, _ := library.MoveGroup(a.ID, b.ID, false); moved {
		t.Fatal("unexpected second move")
	}
	if moved, _ := library.MoveGroup(a.ID, a.ID, true); moved {
		t.Fatal("self move should fail")
	}
	if moved, _ := library.MoveGroup("missing", a.ID, true); moved {
		t.Fatal("missing move should fail")
	}
	reopened, err := OpenIn(temp)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, g := range reopened.Groups() {
		ids = append(ids, g.ID)
	}
	if strings.Join(ids, ",") != strings.Join([]string{a.ID, b.ID, c.ID}, ",") {
		t.Fatalf("ids %v", ids)
	}
}

func TestGroupNamesPreserveSourceStyleSeparators(t *testing.T) {
	temp := t.TempDir()
	dataDir := filepath.Join(temp, "data")
	library, err := OpenIn(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	group, err := library.CreateGroup("owner/repository")
	if err != nil {
		t.Fatal(err)
	}
	if group.Name != "owner/repository" {
		t.Fatalf("name %q", group.Name)
	}
	if err := library.RenameGroup(group.ID, `team\skills`); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenIn(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Groups()[0].Name != `team\skills` {
		t.Fatalf("name %q", reopened.Groups()[0].Name)
	}
}

func localRecord(name, path string) model.SkillRecord {
	return model.SkillRecord{
		Name:   name,
		Origin: model.OriginLocal(path, nil),
	}
}

func TestKitterManualWritesFrontmatterAndSurvivesUpdates(t *testing.T) {
	temp := t.TempDir()
	library, err := OpenIn(filepath.Join(temp, "data"))
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(temp, "source")
	fixtureSkill(t, source, "demo")
	if err := library.Import(source, localRecord("demo", source)); err != nil {
		t.Fatal(err)
	}
	if err := library.SetKitterManualByStorage("demo", true); err != nil {
		t.Fatal(err)
	}
	skillPath, _ := library.SkillPathByStorage("demo")
	skillMD := filepath.Join(skillPath, "SKILL.md")
	marked, _ := os.ReadFile(skillMD)
	if !strings.Contains(string(marked), "disable-model-invocation: true") {
		t.Fatal("flag not written")
	}
	if !strings.Contains(string(marked), "name: demo") {
		t.Fatal("name lost")
	}
	record, _ := library.RecordByStorage("demo")
	if !record.KitterManual {
		t.Fatal("kitter_manual not recorded")
	}
	skills, _ := library.List()
	found := false
	for _, skill := range skills {
		if skill.Record.StorageName == "demo" && skill.ManualOnly && skill.Record.KitterManual {
			found = true
		}
	}
	if !found {
		t.Fatal("manual_only flag missing")
	}

	updated := filepath.Join(temp, "updated")
	fixtureSkill(t, updated, "demo")
	os.WriteFile(
		filepath.Join(updated, "SKILL.md"),
		[]byte("---\nname: demo\ndescription: newer\n---\nupdated body\n"),
		0o644,
	)
	record, _ = library.RecordByStorage("demo")
	if err := library.ReplaceByStorage(updated, "demo", record); err != nil {
		t.Fatal(err)
	}
	restored, _ := os.ReadFile(skillMD)
	if !strings.Contains(string(restored), "disable-model-invocation: true") {
		t.Fatal("flag lost after update")
	}
	if !strings.Contains(string(restored), "description: newer") ||
		!strings.Contains(string(restored), "updated body") {
		t.Fatal("update content missing")
	}
	record, _ = library.RecordByStorage("demo")
	if !record.KitterManual {
		t.Fatal("kitter_manual lost after update")
	}

	if err := library.SetKitterManualByStorage("demo", false); err != nil {
		t.Fatal(err)
	}
	cleared, _ := os.ReadFile(skillMD)
	if strings.Contains(string(cleared), "disable-model-invocation") {
		t.Fatal("flag not removed")
	}
	if !strings.Contains(string(cleared), "description: newer") {
		t.Fatal("content lost")
	}
	record, _ = library.RecordByStorage("demo")
	if record.KitterManual {
		t.Fatal("kitter_manual still set")
	}
}

func TestOriginallyManualSkillsCannotBeToggledByKitter(t *testing.T) {
	temp := t.TempDir()
	library, err := OpenIn(filepath.Join(temp, "data"))
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(temp, "source")
	os.MkdirAll(source, 0o755)
	os.WriteFile(
		filepath.Join(source, "SKILL.md"),
		[]byte("---\nname: orig\ndescription: test\ndisable-model-invocation: true\n---\n"),
		0o644,
	)
	if err := library.Import(source, localRecord("orig", source)); err != nil {
		t.Fatal(err)
	}
	err = library.SetKitterManualByStorage("orig", true)
	if err == nil || !strings.Contains(err.Error(), "本身就是仅手动触发") {
		t.Fatalf("want already-manual error, got %v", err)
	}
	record, _ := library.RecordByStorage("orig")
	if record.KitterManual {
		t.Fatal("kitter_manual set")
	}
	if err := library.SetKitterManualByStorage("orig", false); err == nil {
		t.Fatal("clearing must fail")
	}
	if err := library.SetKitterManualByStorage(KitterSkillStorage, true); err == nil {
		t.Fatal("builtin toggle must fail")
	}
}

func TestKitterManualOverrideClearsWhenUpdateIsAlreadyManual(t *testing.T) {
	temp := t.TempDir()
	library, err := OpenIn(filepath.Join(temp, "data"))
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(temp, "source")
	fixtureSkill(t, source, "demo")
	if err := library.Import(source, localRecord("demo", source)); err != nil {
		t.Fatal(err)
	}
	if err := library.SetKitterManualByStorage("demo", true); err != nil {
		t.Fatal(err)
	}
	updated := filepath.Join(temp, "updated")
	os.MkdirAll(updated, 0o755)
	os.WriteFile(
		filepath.Join(updated, "SKILL.md"),
		[]byte("---\nname: demo\ndescription: upstream manual\ndisable-model-invocation: true\n---\n"),
		0o644,
	)
	record, _ := library.RecordByStorage("demo")
	if err := library.ReplaceByStorage(updated, "demo", record); err != nil {
		t.Fatal(err)
	}
	record, _ = library.RecordByStorage("demo")
	if record.KitterManual {
		t.Fatal("kitter_manual should clear")
	}
	path, _ := library.SkillPathByStorage("demo")
	if !effective.HasDisableModelInvocation(path) {
		t.Fatal("upstream manual flag missing")
	}
}

func TestDisableModelInvocationPreservesNestedFrontmatter(t *testing.T) {
	original := "---\nname: demo\ndescription: |\n  First line\n  Second line\nmetadata:\n  opencode:\n    autoinvoke: false\n---\nBody\n"
	enabled := setDisableModelInvocationMarkdown(original, true)
	if !strings.Contains(enabled, "disable-model-invocation: true") {
		t.Fatal("flag not added")
	}
	if !strings.Contains(enabled, "  First line") || !strings.Contains(enabled, "    autoinvoke: false") {
		t.Fatal("nested frontmatter damaged")
	}
	disabled := setDisableModelInvocationMarkdown(enabled, false)
	if strings.Contains(disabled, "disable-model-invocation") {
		t.Fatal("flag not removed")
	}
	if !strings.Contains(disabled, "  First line") {
		t.Fatal("content lost")
	}
	got := setDisableModelInvocationMarkdown(
		"---\nname: demo\ndisable-model-invocation: false\n---\n", true)
	want := "---\nname: demo\ndisable-model-invocation: true\n---\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// Registry round-trip: a fixture written in the serde shape must load and
// re-save byte-comparably in structure.
func TestRegistryJSONRoundTrip(t *testing.T) {
	dir := t.TempDir()
	fixture := `{
  "skills": {
    "demo": {
      "name": "demo",
      "storage_name": "demo",
      "origin": {"type": "npx", "repository": "owner/repo", "skill": "demo", "source_hash": "abc"},
      "update_available": true,
      "group_id": "group-1",
      "last_operated_at": 42,
      "kitter_manual": true
    }
  },
  "sources": {
    "npx:owner/repo": {
      "source": {"type": "npx", "repository": "owner/repo"},
      "discovered_skills": ["demo"],
      "added_skills": ["demo"]
    }
  },
  "groups": [{"id": "group-1", "name": "frontend", "created_at": 7}],
  "source_groups_migrated": true,
  "adopted_sources": {
    "demo": {
      "source": "/tmp/external",
      "references": [{"path": "/p/.claude/skills/demo", "source": "/tmp/external", "kind": "Link", "original_target": "/tmp/external"}],
      "previous_library": null
    }
  }
}`
	if err := os.WriteFile(filepath.Join(dir, "registry.json"), []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	library, err := OpenIn(dir)
	if err != nil {
		t.Fatal(err)
	}
	record := library.Registry.Skills["demo"]
	if record.Name != "demo" || !record.UpdateAvailable || !record.KitterManual {
		t.Fatalf("record %+v", record)
	}
	if record.Origin.Type != "npx" || record.Origin.Repository != "owner/repo" || record.Origin.SourceHash != "abc" {
		t.Fatalf("origin %+v", record.Origin)
	}
	if record.GroupID == nil || *record.GroupID != "group-1" {
		t.Fatal("group lost")
	}
	if _, ok := library.Registry.AdoptedSources["demo"]; !ok {
		t.Fatal("adopted source lost")
	}
	if err := library.Save(); err != nil {
		t.Fatal(err)
	}
	// Loading again must produce the same effective registry.
	library2, err := OpenIn(dir)
	if err != nil {
		t.Fatal(err)
	}
	if library2.Registry.Skills["demo"].Origin.Type != "npx" {
		t.Fatal("re-save corrupted registry")
	}
}
