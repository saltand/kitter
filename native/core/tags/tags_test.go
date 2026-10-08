package tags

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Mirrors tags.rs tests.

func TestEnforcesTwoLevelsAndBuildsPaths(t *testing.T) {
	tags := NewTagState()
	parent, err := tags.Add("开发", nil)
	if err != nil {
		t.Fatal(err)
	}
	child, err := tags.Add("调试", &parent)
	if err != nil {
		t.Fatal(err)
	}
	if path, _ := tags.Path(child); path != "开发/调试" {
		t.Fatalf("path %q", path)
	}
	if _, err := tags.Add("更深", &child); err == nil || err.Error() != "子标签下不能继续创建标签" {
		t.Fatalf("want nesting error, got %v", err)
	}
}

func TestParentFilterIncludesChildAssignments(t *testing.T) {
	tags := NewTagState()
	parent, _ := tags.Add("开发", nil)
	child, _ := tags.Add("调试", &parent)
	tags.ToggleAssignment("grilling", child)
	if !tags.MatchesFilter("grilling", parent) {
		t.Fatal("parent filter should match child assignment")
	}
	if !tags.MatchesFilter("grilling", child) {
		t.Fatal("child filter should match")
	}
	if tags.Count(parent) != 1 {
		t.Fatalf("count %d", tags.Count(parent))
	}
}

func TestAssignmentCanBeSetIdempotently(t *testing.T) {
	tags := NewTagState()
	tag, _ := tags.Add("开发", nil)
	if !tags.SetAssignment("grilling", tag, true) {
		t.Fatal("first set should change")
	}
	if tags.SetAssignment("grilling", tag, true) {
		t.Fatal("second set should be a no-op")
	}
	if !tags.IsAssigned("grilling", tag) {
		t.Fatal("should be assigned")
	}
	if !tags.SetAssignment("grilling", tag, false) {
		t.Fatal("clear should change")
	}
	if tags.SetAssignment("grilling", tag, false) {
		t.Fatal("second clear should be a no-op")
	}
	if tags.IsAssigned("grilling", tag) {
		t.Fatal("should be unassigned")
	}
}

func TestDeletingParentRemovesChildrenAndAssignments(t *testing.T) {
	tags := NewTagState()
	parent, _ := tags.Add("开发", nil)
	child, _ := tags.Add("调试", &parent)
	tags.ToggleAssignment("grilling", child)
	tags.Delete(parent)
	if len(tags.TagsList()) != 0 {
		t.Fatal("tags left behind")
	}
	if len(tags.AssignedTags("grilling")) != 0 {
		t.Fatal("assignments left behind")
	}
}

func TestMovesBeforeTargetWithoutCrossingLevels(t *testing.T) {
	tags := NewTagState()
	first, _ := tags.Add("开发", nil)
	second, _ := tags.Add("研究", nil)
	third, _ := tags.Add("设计", nil)
	child, _ := tags.Add("调试", &first)

	if !tags.MoveBefore(third, first) {
		t.Fatal("move_before failed")
	}
	var ids []TagID
	for _, tag := range tags.Roots() {
		ids = append(ids, tag.ID)
	}
	want := []TagID{third, first, second}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("roots %v want %v", ids, want)
		}
	}
	if !tags.MoveAfter(second, third) {
		t.Fatal("move_after failed")
	}
	ids = ids[:0]
	for _, tag := range tags.Roots() {
		ids = append(ids, tag.ID)
	}
	want = []TagID{third, second, first}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("roots %v want %v", ids, want)
		}
	}
	if tags.MoveBefore(child, first) {
		t.Fatal("cross-level move should fail")
	}
}

// Round-trip against a tags.json fixture shaped like serde output.
func TestTagsJSONRoundTrip(t *testing.T) {
	dir := t.TempDir()
	fixture := `{
  "skills": {
    "next_id": 3,
    "tags": [
      {"id": 1, "name": "开发", "parent": null},
      {"id": 2, "name": "调试", "parent": 1},
      {"id": 3, "name": "研究", "parent": null}
    ],
    "assignments": {"demo": [2]}
  },
  "projects": {"next_id": 0, "tags": [], "assignments": {}}
}`
	if err := os.WriteFile(filepath.Join(dir, "tags.json"), []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	skills, projects := LoadTagStatesFrom(dir)
	if len(skills.TagsList()) != 3 {
		t.Fatalf("got %d tags", len(skills.TagsList()))
	}
	if path, ok := skills.Path(2); !ok || path != "开发/调试" {
		t.Fatalf("path %q", path)
	}
	if !skills.IsAssigned("demo", 2) {
		t.Fatal("assignment lost")
	}
	if len(projects.TagsList()) != 0 {
		t.Fatal("projects should be empty")
	}

	// Save and reload: the document must keep its shape.
	if err := SaveTagStatesTo(dir, skills, projects); err != nil {
		t.Fatal(err)
	}
	var doc TagDocument
	data, err := os.ReadFile(filepath.Join(dir, "tags.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Skills.NextID != 3 || len(doc.Skills.Tags) != 3 {
		t.Fatalf("reloaded %s", data)
	}
	// BTreeSet serializes sorted.
	var check map[string][]TagID
	if err := json.Unmarshal(data, &check); err != nil {
		// top-level isn't the assignments map; verify through the doc
		_ = check
	}
	raw := struct {
		Skills struct {
			Assignments map[string][]TagID `json:"assignments"`
		} `json:"skills"`
	}{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if got := raw.Skills.Assignments["demo"]; len(got) != 1 || got[0] != 2 {
		t.Fatalf("assignments %v", got)
	}
}

func TestTagStateNeverSerializesNullCollections(t *testing.T) {
	state := &TagState{Assignments: map[string][]TagID{"s": nil}}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	raw := string(data)
	if strings.Contains(raw, `"tags":null`) || strings.Contains(raw, `"assignments":null`) || strings.Contains(raw, `"s":null`) {
		t.Fatalf("tag state contains null: %s", raw)
	}
	var decoded TagState
	if err := json.Unmarshal([]byte(`{"next_id":0,"tags":null,"assignments":{"a":null}}`), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Tags == nil || decoded.Assignments["a"] == nil {
		t.Fatal("nulls must normalize to empty")
	}
}
