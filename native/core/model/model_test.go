package model

import (
	"encoding/json"
	"testing"
)

// The fixtures below are hand-written to match what serde_json emits for
// the Rust types in src/model.rs (field order and null vs absent matter).

func TestSkillOriginJSONShapes(t *testing.T) {
	cases := []struct {
		name   string
		origin SkillOrigin
		json   string
	}{
		{"builtin", OriginBuiltin(), `{"type":"builtin"}`},
		{"unknown", OriginUnknown(), `{"type":"unknown"}`},
		{
			"npx no hash",
			OriginNpx("owner/repo", "skill-a", nil),
			`{"type":"npx","repository":"owner/repo","skill":"skill-a"}`,
		},
		{
			"npx with hash",
			OriginNpx("owner/repo", "skill-a", StrPtr("abc123")),
			`{"type":"npx","repository":"owner/repo","skill":"skill-a","source_hash":"abc123"}`,
		},
		{
			"claude marketplace",
			OriginClaudeMarketplace("plugin@market", "skill-a"),
			`{"type":"claude_marketplace","plugin":"plugin@market","skill":"skill-a"}`,
		},
		{
			"git without subdir serializes null",
			OriginGit("https://github.com/owner/repo.git", nil),
			`{"type":"git","repository":"https://github.com/owner/repo.git","subdir":null}`,
		},
		{
			"git with subdir",
			OriginGit("https://github.com/owner/repo", StrPtr("skills/demo")),
			`{"type":"git","repository":"https://github.com/owner/repo","subdir":"skills/demo"}`,
		},
		{
			"local without source_root",
			OriginLocal("/tmp/skills/demo", nil),
			`{"type":"local","path":"/tmp/skills/demo"}`,
		},
		{
			"local with source_root",
			OriginLocal("/tmp/skills/demo", StrPtr("/tmp/skills")),
			`{"type":"local","path":"/tmp/skills/demo","source_root":"/tmp/skills"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.origin)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != tc.json {
				t.Fatalf("marshal: got %s want %s", data, tc.json)
			}
			var back SkillOrigin
			if err := json.Unmarshal([]byte(tc.json), &back); err != nil {
				t.Fatal(err)
			}
			again, _ := json.Marshal(back)
			if string(again) != tc.json {
				t.Fatalf("round-trip: got %s want %s", again, tc.json)
			}
		})
	}
}

func TestSkillSourceJSONShapes(t *testing.T) {
	cases := []struct {
		source SkillSource
		json   string
	}{
		{SkillSource{Type: "builtin"}, `{"type":"builtin"}`},
		{SkillSource{Type: "unknown"}, `{"type":"unknown"}`},
		{SkillSource{Type: "npx", Repository: "owner/repo"}, `{"type":"npx","repository":"owner/repo"}`},
		{SkillSource{Type: "claude_marketplace", Plugin: "p@m"}, `{"type":"claude_marketplace","plugin":"p@m"}`},
		{SkillSource{Type: "git", Repository: "r"}, `{"type":"git","repository":"r","subdir":null}`},
		{SkillSource{Type: "git", Repository: "r", Subdir: "s", HasSubdir: true}, `{"type":"git","repository":"r","subdir":"s"}`},
		{SkillSource{Type: "local", Path: "/p"}, `{"type":"local","path":"/p"}`},
	}
	for _, tc := range cases {
		data, err := json.Marshal(tc.source)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != tc.json {
			t.Fatalf("marshal: got %s want %s", data, tc.json)
		}
		var back SkillSource
		if err := json.Unmarshal([]byte(tc.json), &back); err != nil {
			t.Fatal(err)
		}
		again, _ := json.Marshal(back)
		if string(again) != tc.json {
			t.Fatalf("round-trip: got %s want %s", again, tc.json)
		}
	}
}

func TestInstallTargetSnakeCase(t *testing.T) {
	cases := map[InstallTarget]string{
		TargetUniversal:   "universal",
		TargetCodex:       "codex",
		TargetClaudeCode:  "claude_code",
		TargetCursor:      "cursor",
		TargetOpenCode:    "open_code",
		TargetPi:          "pi",
		TargetGrok:        "grok",
		TargetAntigravity: "antigravity",
		TargetDroid:       "droid",
		TargetCopilot:     "copilot",
	}
	for target, want := range cases {
		data, _ := json.Marshal(target)
		if string(data) != `"`+want+`"` {
			t.Fatalf("%v marshals as %s, want %q", target, data, want)
		}
		var back InstallTarget
		if err := json.Unmarshal(data, &back); err != nil {
			t.Fatal(err)
		}
		if back != target {
			t.Fatalf("round-trip of %v gives %v", target, back)
		}
	}
}

func TestSkillRecordSkipsAndDefaults(t *testing.T) {
	// storage_name: skip_serializing_if String::is_empty; description:
	// serde(skip); group_id: skip_serializing_if None; the rest serialize
	// even when zero-valued.
	record := SkillRecord{
		Name:   "demo",
		Origin: OriginUnknown(),
	}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"name":"demo","origin":{"type":"unknown"},"update_available":false,"last_operated_at":0,"kitter_manual":false}`
	if string(data) != want {
		t.Fatalf("got %s want %s", data, want)
	}

	// Older registries may carry no origin/update fields at all.
	var older SkillRecord
	if err := json.Unmarshal([]byte(`{"name":"demo"}`), &older); err != nil {
		t.Fatal(err)
	}
	if older.Origin.Type != "" && older.Origin.Type != "unknown" {
		t.Fatalf("unexpected origin %q", older.Origin.Type)
	}
	if older.GroupID != nil || older.UpdateAvailable || older.KitterManual {
		t.Fatal("defaults not applied")
	}
}

func TestSkillSourceKeysAndLabels(t *testing.T) {
	if got := (SkillSource{Type: "builtin"}).Key(); got != "builtin" {
		t.Fatal(got)
	}
	if got := (SkillSource{Type: "npx", Repository: "vercel-labs/skills"}).Label(); got != "Vercel Skills" {
		t.Fatal(got)
	}
	if got := (SkillSource{Type: "npx", Repository: "git@github.com:owner/repo.git"}).Label(); got != "owner/repo" {
		t.Fatal(got)
	}
	if got := (SkillSource{Type: "git", Repository: "https://github.com/a/b/", Subdir: "s", HasSubdir: true}).Key(); got != "git:https://github.com/a/b/:s" {
		t.Fatal(got)
	}
	if got := (SkillSource{Type: "local", Path: "/tmp/my-skills"}).Label(); got != "my-skills" {
		t.Fatal(got)
	}
	if got := (SkillSource{Type: "local", Path: "/"}).Label(); got != "本地导入" {
		t.Fatal(got)
	}
	if got := (SkillSource{Type: "unknown"}).Label(); got != "本地技能" {
		t.Fatal(got)
	}
}

func TestOriginSourceMapping(t *testing.T) {
	origin := OriginNpx("owner/repo", "skill-a", nil)
	if got := origin.Source().Key(); got != "npx:owner/repo" {
		t.Fatal(got)
	}
	if got := origin.IdentityKey("demo"); got != "npx:owner/repo::demo" {
		t.Fatal(got)
	}
	// Local origins prefer source_root for the identity path.
	local := OriginLocal("/a/b", StrPtr("/a"))
	if got := local.Source().Key(); got != "local:/a" {
		t.Fatal(got)
	}
}
