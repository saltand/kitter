package model_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/saltand/kitter/native/core/model"
)

// Rust's serde deserializes Option<T> from either a missing key or an
// explicit null, so only fields the Rust side declares as Option may be
// null on the wire: SkillOrigin.git.subdir (and inside SkillSource.git)
// and AdoptedSource.previous_library. #[serde(default)] fields must come
// as a value or not at all.
//
// allowedNullFields lists the JSON field names that may serialize as null.
var allowedNullFields = map[string]bool{
	"subdir":           true, // Option<String> without skip_serializing_if
	"previous_library": true, // Option<PathBuf>
	"parent":           true, // tags::Tag.parent, Option<TagId>
}

// collectNulls walks decoded JSON and returns every "path → null" pair.
func collectNulls(value any, path string, out *[]string) {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			next := path + "." + key
			if path == "" {
				next = key
			}
			collectNulls(child, next, out)
		}
	case []any:
		for i, child := range v {
			collectNulls(child, fmt.Sprintf("%s[%d]", path, i), out)
		}
	case nil:
		*out = append(*out, path)
	}
}

func assertOnlyAllowedNulls(t *testing.T, doc any, label string) {
	t.Helper()
	var nulls []string
	collectNulls(doc, "", &nulls)
	for _, path := range nulls {
		field := path[strings.LastIndex(path, ".")+1:]
		field = strings.TrimSuffix(field, "]")
		if idx := strings.LastIndex(field, "["); idx >= 0 {
			field = field[:idx]
		}
		if !allowedNullFields[field] {
			t.Errorf("%s: unexpected null at %s (not a serde Option field)", label, path)
		}
	}
}

func TestSerializedShapeHasNoUnexpectedNulls(t *testing.T) {
	// A nil-bearing Go value must still marshal as serde expects.
	type doc struct {
		Record   model.SkillRecord       `json:"record"`
		Source   model.SkillSourceRecord `json:"source"`
		Registry struct {
			Skills  map[string]model.SkillRecord       `json:"skills"`
			Sources map[string]model.SkillSourceRecord `json:"sources"`
			Groups  []model.SkillGroup                 `json:"groups"`
		} `json:"registry"`
	}
	d := doc{
		Record: model.SkillRecord{
			Name:   "demo",
			Origin: model.OriginGit("https://github.com/o/r", nil), // subdir → null (allowed)
		},
		Source: model.SkillSourceRecord{ // nil slices → []
			Source: model.SourceUnknown(),
		},
	}
	d.Registry.Skills = map[string]model.SkillRecord{"demo": d.Record}
	d.Registry.Sources = map[string]model.SkillSourceRecord{"s": d.Source}
	d.Registry.Groups = []model.SkillGroup{} // the persisted Registry type normalizes nil to []

	data, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	assertOnlyAllowedNulls(t, decoded, "doc")

	// The null-containing variants must keep the explicit null (serde Option).
	raw := string(data)
	if !strings.Contains(raw, `"subdir":null`) {
		t.Fatalf("git subdir must serialize as explicit null: %s", raw)
	}
	if !strings.Contains(raw, `"discovered_skills":[]`) || !strings.Contains(raw, `"added_skills":[]`) {
		t.Fatalf("nil slices must serialize as []: %s", raw)
	}
}

func TestNullCollectionsAreAcceptedOnRead(t *testing.T) {
	// Rust #[serde(default)] accepts a missing key; the Go side must also
	// tolerate an explicit null so hand-edited or intermediate files load.
	var record model.SkillSourceRecord
	if err := json.Unmarshal([]byte(`{"source":{"type":"npx","repository":"r"},"discovered_skills":null,"added_skills":null}`), &record); err != nil {
		t.Fatal(err)
	}
	if len(record.DiscoveredSkills) != 0 || len(record.AddedSkills) != 0 {
		t.Fatal("null slices must normalize to empty")
	}
}
