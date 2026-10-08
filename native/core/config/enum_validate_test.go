package config

import (
	"encoding/json"
	"testing"
)

// Serde unit enums reject unknown variants; a config with a typo must
// fail to load rather than silently pick a zero value.
func TestLanguageRejectsUnknown(t *testing.T) {
	var c AppConfig
	for _, input := range []string{
		`{"language":"bogus","library_dir":"/l"}`,
		`{"language":"EN","library_dir":"/l"}`,
	} {
		if err := json.Unmarshal([]byte(input), &c); err == nil {
			t.Fatalf("expected error for %s", input)
		}
	}
}

func TestThemeRejectsUnknown(t *testing.T) {
	var c AppConfig
	if err := json.Unmarshal([]byte(`{"theme":"Bogus","library_dir":"/l"}`), &c); err == nil {
		t.Fatal("expected error for unknown theme")
	}
}

func TestLanguageThemeRoundTrip(t *testing.T) {
	for _, input := range []string{
		`{"language":"zh_cn","theme":"dark","library_dir":"/l","recent_projects":[],"project_activity":{},"collapsed_skill_groups":[]}`,
		`{"language":"en","theme":"light","library_dir":"/l","recent_projects":[],"project_activity":{},"collapsed_skill_groups":[]}`,
		`{"language":"system","theme":"system","library_dir":"/l","recent_projects":[],"project_activity":{},"collapsed_skill_groups":[]}`,
	} {
		var c AppConfig
		if err := json.Unmarshal([]byte(input), &c); err != nil {
			t.Fatalf("unmarshal %s: %v", input, err)
		}
		out, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		if string(out) != input {
			t.Fatalf("\n got: %s\nwant: %s", out, input)
		}
	}
}
