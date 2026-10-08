// serde_compat_test.go verifies that every persisted enum reads the
// exact JSON serde writes and rejects unknown variants the way serde's
// derived Deserialize does.
package model_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/saltand/kitter/native/core/model"
)

// roundTrip unmarshals Rust-shaped JSON, re-marshals, and checks the
// output is byte-identical.
func roundTrip[T any](t *testing.T, input string, doc *T) {
	t.Helper()
	if err := json.Unmarshal([]byte(input), doc); err != nil {
		t.Fatalf("unmarshal %s: %v", input, err)
	}
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(out) != input {
		t.Fatalf("round-trip mismatch:\n in: %s\nout: %s", input, out)
	}
}

func TestReferenceKindSerdeShape(t *testing.T) {
	// Rust: enum ReferenceKind { Link, Direct, Alias, Plugin } with NO
	// serde attribute → variant names verbatim.
	for _, input := range []string{
		`{"path":"/p","source":"/s","kind":"Link","original_target":"/s"}`,
		`{"path":"/p","source":"/s","kind":"Direct","original_target":null}`,
		`{"path":"/p","source":"/s","kind":"Alias","original_target":"/t"}`,
		`{"path":"/p","source":"/s","kind":"Plugin","original_target":null}`,
	} {
		var ref model.SkillReference
		roundTrip(t, input, &ref)
	}
}

func TestReferenceKindRejectsUnknown(t *testing.T) {
	for _, input := range []string{
		`{"path":"/p","source":"/s","kind":"link","original_target":null}`,
		`{"path":"/p","source":"/s","kind":"bogus","original_target":null}`,
		`{"path":"/p","source":"/s","kind":null,"original_target":null}`,
	} {
		var ref model.SkillReference
		if err := json.Unmarshal([]byte(input), &ref); err == nil {
			t.Fatalf("expected error for %s", input)
		}
	}
}

func TestInstallTargetSerdeShape(t *testing.T) {
	for _, v := range []string{
		"universal", "codex", "claude_code", "cursor", "open_code",
		"pi", "grok", "antigravity", "droid", "copilot",
	} {
		input := `{"target":"` + v + `","path":"/p","managed":true}`
		var inst model.ProjectSkillInstallation
		roundTrip(t, input, &inst)
	}
	var inst model.ProjectSkillInstallation
	if err := json.Unmarshal([]byte(`{"target":"ClaudeCode","path":"/p","managed":true}`), &inst); err == nil {
		t.Fatal("expected error for un-renamed variant")
	}
}

func TestSkillOriginSerdeShape(t *testing.T) {
	for _, input := range []string{
		`{"type":"builtin"}`,
		`{"type":"npx","repository":"o/r","skill":"s","source_hash":"h"}`,
		`{"type":"npx","repository":"o/r","skill":"s"}`,
		`{"type":"claude_marketplace","plugin":"p","skill":"s"}`,
		`{"type":"git","repository":"o/r","subdir":"sub"}`,
		`{"type":"git","repository":"o/r","subdir":null}`,
		`{"type":"local","path":"/p","source_root":"/r"}`,
		`{"type":"local","path":"/p"}`,
		`{"type":"unknown"}`,
	} {
		var o model.SkillOrigin
		roundTrip(t, input, &o)
	}
}

func TestSkillOriginRejectsBadTag(t *testing.T) {
	for _, input := range []string{
		`{"type":"bogus"}`,
		`{"repository":"o/r","skill":"s"}`, // missing tag
		`null`,
	} {
		var o model.SkillOrigin
		if err := json.Unmarshal([]byte(input), &o); err == nil {
			t.Fatalf("expected error for %s", input)
		}
	}
}

func TestSkillSourceSerdeShape(t *testing.T) {
	for _, input := range []string{
		`{"type":"builtin"}`,
		`{"type":"npx","repository":"o/r"}`,
		`{"type":"claude_marketplace","plugin":"p"}`,
		`{"type":"git","repository":"o/r","subdir":"sub"}`,
		`{"type":"git","repository":"o/r","subdir":null}`,
		`{"type":"local","path":"/p"}`,
		`{"type":"unknown"}`,
	} {
		var s model.SkillSource
		roundTrip(t, input, &s)
	}
	for _, input := range []string{
		`{"type":"bogus"}`,
		`{"repository":"o/r"}`, // missing tag
	} {
		var s model.SkillSource
		if err := json.Unmarshal([]byte(input), &s); err == nil {
			t.Fatalf("expected error for %s", input)
		}
	}
}

func TestSkillRecordSerdeDefaults(t *testing.T) {
	// Every field except name has #[serde(default)] or is skipped.
	var r model.SkillRecord
	if err := json.Unmarshal([]byte(`{"name":"demo"}`), &r); err != nil {
		t.Fatal(err)
	}
	if r.Origin.Type != "unknown" {
		t.Fatalf("missing origin should default to unknown, got %q", r.Origin.Type)
	}
	if r.Description != "" {
		t.Fatal("description must never deserialize (serde skip)")
	}
	// origin present and valid
	if err := json.Unmarshal([]byte(`{"name":"demo","origin":{"type":"local","path":"/p"}}`), &r); err != nil {
		t.Fatal(err)
	}
	if r.Origin.Type != "local" {
		t.Fatalf("origin %q", r.Origin.Type)
	}
	// origin present but invalid → error (serde does not default inside)
	if err := json.Unmarshal([]byte(`{"name":"demo","origin":{"type":"bogus"}}`), &r); err == nil {
		t.Fatal("expected unknown variant error")
	}
	if err := json.Unmarshal([]byte(`{"name":"demo","origin":null}`), &r); err == nil {
		t.Fatal("expected error for null origin")
	}
}

func TestSkillRecordRoundTrip(t *testing.T) {
	gid := "g1"
	r := model.SkillRecord{
		Name:            "demo",
		StorageName:     "demo",
		Description:     "never persisted",
		Origin:          model.OriginGit("https://github.com/o/r.git", nil),
		UpdateAvailable: true,
		GroupID:         &gid,
		LastOperatedAt:  42,
		KitterManual:    true,
	}
	out, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"name":"demo","storage_name":"demo","origin":{"type":"git","repository":"https://github.com/o/r.git","subdir":null},"update_available":true,"group_id":"g1","last_operated_at":42,"kitter_manual":true}`
	if string(out) != want {
		t.Fatalf("\n got: %s\nwant: %s", out, want)
	}
	if strings.Contains(string(out), "description") {
		t.Fatal("description must be skipped")
	}
	var back model.SkillRecord
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatal(err)
	}
	if !back.Origin.HasSubdir && back.Origin.Subdir != "" {
		t.Fatal("subdir should stay absent")
	}
}
