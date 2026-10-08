package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolvesChineseLocaleVariants(t *testing.T) {
	for _, locale := range []string{"zh-CN", "zh-Hans-CN", "zh_TW", "ZH-hant"} {
		if got := LanguageFromLocale(locale); got != LanguageZhCn {
			t.Fatalf("%s → %s, want zh_cn", locale, got)
		}
	}
}

func TestDefaultsNonChineseLocalesToEnglish(t *testing.T) {
	for _, locale := range []string{"en-US", "ja-JP", "", "C"} {
		if got := LanguageFromLocale(locale); got != LanguageEn {
			t.Fatalf("%s → %s, want en", locale, got)
		}
	}
}

func TestProjectsSortByActivityAndKeepPathOrderAsTieBreaker(t *testing.T) {
	cfg := Default()
	first, second, third := "/tmp/first", "/tmp/second", "/tmp/third"
	cfg.RecentProjects = []string{first, second, third}
	cfg.ProjectActivity[first] = 10
	cfg.ProjectActivity[second] = 30
	cfg.ProjectActivity[third] = 20

	want := []string{second, third, first}
	got := cfg.ProjectPaths()
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestTouchingProjectUpdatesActivityWithoutReordering(t *testing.T) {
	cfg := Default()
	first, second := "/tmp/first", "/tmp/second"
	cfg.RememberProject(first)
	cfg.RememberProject(second)
	before := cfg.ProjectActivity[first]
	cfg.TouchProject(first)

	if cfg.ProjectActivity[first] <= before {
		t.Fatal("activity did not increase")
	}
	got := cfg.ProjectPaths()
	if got[0] != first || got[1] != second {
		t.Fatalf("got %v", got)
	}
}

func TestProjectActivityIsOptionalInOlderConfigsAndRoundTrips(t *testing.T) {
	cfg := Default()
	project := "/tmp/project"
	cfg.TouchProject(project)

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var restored AppConfig
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.ProjectActivity[project] != cfg.ProjectActivity[project] {
		t.Fatal("project_activity did not round-trip")
	}

	older := `{"language":"system","theme":"system","library_dir":"/tmp/skills","recent_projects":["` + project + `"]}`
	var legacy AppConfig
	if err := json.Unmarshal([]byte(older), &legacy); err != nil {
		t.Fatal(err)
	}
	if len(legacy.ProjectActivity) != 0 {
		t.Fatal("project_activity should default to empty")
	}
}

func TestSaveJSONIsAtomicAndPretty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config.json")
	cfg := Default()
	cfg.LibraryDir = "/tmp/skills"
	if err := SaveJSON(path, &cfg); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// serde_json::to_vec_pretty uses two-space indents, no trailing newline.
	if data[len(data)-1] == '\n' {
		t.Fatal("trailing newline present")
	}
	if len(data) < 4 || string(data[:4]) != "{\n  " {
		t.Fatalf("not pretty-printed: %q", data[:min(20, len(data))])
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("temp file left behind")
	}
	// HTML-unsafe text must not be escaped (serde_json does not escape).
	path2 := filepath.Join(dir, "x.json")
	if err := SaveJSON(path2, map[string]string{"k": "a<b>&c"}); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path2)
	if !json.Valid(data) || string(data) != "{\n  \"k\": \"a<b>&c\"\n}" {
		t.Fatalf("unexpected output: %s", data)
	}
}

func TestAppDataDirPrefersKitterHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KITTER_HOME", dir)
	if got := AppDataDir(); got != dir {
		t.Fatalf("got %s want %s", got, dir)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LibraryDir != filepath.Join(dir, "skills") {
		t.Fatalf("library_dir %s", cfg.LibraryDir)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestConfigNeverSerializesNullCollections(t *testing.T) {
	// serde #[serde(default)] reads a missing key but not a null.
	cfg := AppConfig{}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	raw := string(data)
	if strings.Contains(raw, `"recent_projects":null`) || strings.Contains(raw, `"project_activity":null`) {
		t.Fatalf("config contains null: %s", raw)
	}
	var decoded AppConfig
	if err := json.Unmarshal([]byte(`{"language":"system","theme":"system","library_dir":"/x","recent_projects":null,"project_activity":null}`), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.RecentProjects == nil || decoded.ProjectActivity == nil {
		t.Fatal("nulls must normalize to empty")
	}
}

// feat/persist-collapsed: collapsed_skill_groups round-trips and defaults
// empty for older config files.
func TestCollapsedSkillGroupsRoundTripAndDefault(t *testing.T) {
	cfg := Default()
	cfg.CollapsedSkillGroups["group-a"] = true
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var restored AppConfig
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if !restored.CollapsedSkillGroups["group-a"] {
		t.Fatal("group lost")
	}
	older := `{"language":"system","theme":"system","library_dir":"/tmp/skills"}`
	var legacy AppConfig
	if err := json.Unmarshal([]byte(older), &legacy); err != nil {
		t.Fatal(err)
	}
	if len(legacy.CollapsedSkillGroups) != 0 {
		t.Fatal("must default empty")
	}
	// The BTreeSet serializes as a sorted array.
	if !strings.Contains(string(data), `"collapsed_skill_groups":["group-a"]`) {
		t.Fatalf("want array encoding: %s", data)
	}
}
