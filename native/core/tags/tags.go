// Package tags mirrors src/tags.rs: two-level tags and per-skill
// assignments persisted in tags.json.
package tags

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/saltand/kitter/native/core/config"
)

// TagID is tags::TagId.
type TagID = uint64

// Tag mirrors tags::Tag.
type Tag struct {
	ID     TagID  `json:"id"`
	Name   string `json:"name"`
	Parent *TagID `json:"parent"`
}

// TagState mirrors tags::TagState. Note serde derives field order
// (next_id, tags, assignments) and emits empty maps/arrays, not null.
type TagState struct {
	NextID      TagID              `json:"next_id"`
	Tags        []Tag              `json:"tags"`
	Assignments map[string][]TagID `json:"assignments"` // BTreeMap<String, BTreeSet<TagId>>
}

// NewTagState returns a state that marshals like a fresh Rust TagState.
func NewTagState() *TagState {
	return &TagState{Tags: []Tag{}, Assignments: map[string][]TagID{}}
}

func (s *TagState) normalize() {
	if s.Tags == nil {
		s.Tags = []Tag{}
	}
	if s.Assignments == nil {
		s.Assignments = map[string][]TagID{}
	}
	// BTreeSet serializes as a sorted array.
	for k, ids := range s.Assignments {
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		s.Assignments[k] = ids
	}
}

// MarshalJSON emits sorted assignment arrays like a BTreeSet.
func (s TagState) MarshalJSON() ([]byte, error) {
	s.normalize()
	type alias TagState
	return json.Marshal(alias(s))
}

// UnmarshalJSON accepts missing fields (serde default).
func (s *TagState) UnmarshalJSON(data []byte) error {
	type alias TagState
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*s = TagState(a)
	s.normalize()
	return nil
}

// TagDocument mirrors the private TagDocument struct.
type TagDocument struct {
	Skills   TagState `json:"skills"`
	Projects TagState `json:"projects"`
}

// LoadTagStates reads tags.json from the default data directory.
func LoadTagStates() (*TagState, *TagState) {
	return LoadTagStatesFrom(config.AppDataDir())
}

// LoadTagStatesFrom is load_tag_states_from.
func LoadTagStatesFrom(dataDir string) (*TagState, *TagState) {
	path := filepath.Join(dataDir, "tags.json")
	bytes, err := os.ReadFile(path)
	if err != nil {
		return NewTagState(), NewTagState()
	}
	var doc TagDocument
	if err := json.Unmarshal(bytes, &doc); err != nil {
		return NewTagState(), NewTagState()
	}
	doc.Skills.normalize()
	doc.Projects.normalize()
	return &doc.Skills, &doc.Projects
}

// SaveTagStates writes tags.json to the default data directory.
func SaveTagStates(skills, projects *TagState) error {
	return SaveTagStatesTo(config.AppDataDir(), skills, projects)
}

// SaveTagStatesTo is save_tag_states_to.
func SaveTagStatesTo(dataDir string, skills, projects *TagState) error {
	return config.SaveJSON(filepath.Join(dataDir, "tags.json"), &TagDocument{
		Skills:   *skills,
		Projects: *projects,
	})
}

// TagsList is TagState::tags.
func (s *TagState) TagsList() []Tag { return s.Tags }

// GetTag is TagState::tag.
func (s *TagState) GetTag(id TagID) *Tag {
	for i := range s.Tags {
		if s.Tags[i].ID == id {
			return &s.Tags[i]
		}
	}
	return nil
}

// Roots is TagState::roots.
func (s *TagState) Roots() []*Tag {
	var out []*Tag
	for i := range s.Tags {
		if s.Tags[i].Parent == nil {
			out = append(out, &s.Tags[i])
		}
	}
	return out
}

// Children is TagState::children.
func (s *TagState) Children(parent TagID) []*Tag {
	var out []*Tag
	for i := range s.Tags {
		if s.Tags[i].Parent != nil && *s.Tags[i].Parent == parent {
			out = append(out, &s.Tags[i])
		}
	}
	return out
}

// Add is TagState::add.
func (s *TagState) Add(name string, parent *TagID) (TagID, error) {
	name, err := normalizeName(name)
	if err != nil {
		return 0, err
	}
	if parent != nil {
		parentTag := s.GetTag(*parent)
		if parentTag == nil {
			return 0, errors.New("父标签不存在")
		}
		if parentTag.Parent != nil {
			return 0, errors.New("子标签下不能继续创建标签")
		}
	}
	if s.siblingNameExists(name, parent, 0) {
		return 0, errors.New("同一级已经存在这个标签")
	}
	s.NextID++
	id := s.NextID
	s.Tags = append(s.Tags, Tag{ID: id, Name: name, Parent: parent})
	return id, nil
}

// Rename is TagState::rename.
func (s *TagState) Rename(id TagID, name string) error {
	name, err := normalizeName(name)
	if err != nil {
		return err
	}
	tag := s.GetTag(id)
	if tag == nil {
		return errors.New("标签不存在")
	}
	if s.siblingNameExists(name, tag.Parent, id) {
		return errors.New("同一级已经存在这个标签")
	}
	for i := range s.Tags {
		if s.Tags[i].ID == id {
			s.Tags[i].Name = name
		}
	}
	return nil
}

// Delete is TagState::delete.
func (s *TagState) Delete(id TagID) {
	removed := map[TagID]bool{id: true}
	for _, child := range s.Children(id) {
		removed[child.ID] = true
	}
	var kept []Tag
	for _, tag := range s.Tags {
		if !removed[tag.ID] {
			kept = append(kept, tag)
		}
	}
	s.Tags = kept
	for skill, ids := range s.Assignments {
		var keptIDs []TagID
		for _, tid := range ids {
			if !removed[tid] {
				keptIDs = append(keptIDs, tid)
			}
		}
		if len(keptIDs) == 0 {
			delete(s.Assignments, skill)
		} else {
			s.Assignments[skill] = keptIDs
		}
	}
}

// MoveBefore is TagState::move_before.
func (s *TagState) MoveBefore(id, targetID TagID) bool {
	return s.moveRelative(id, targetID, false)
}

// MoveAfter is TagState::move_after.
func (s *TagState) MoveAfter(id, targetID TagID) bool {
	return s.moveRelative(id, targetID, true)
}

func (s *TagState) moveRelative(id, targetID TagID, after bool) bool {
	if id == targetID {
		return false
	}
	source := s.GetTag(id)
	target := s.GetTag(targetID)
	if source == nil || target == nil {
		return false
	}
	if !sameParent(source.Parent, target.Parent) {
		return false
	}
	sourceIndex, targetIndex := -1, -1
	for i, tag := range s.Tags {
		if tag.ID == id {
			sourceIndex = i
		}
		if tag.ID == targetID {
			targetIndex = i
		}
	}
	if sourceIndex < 0 || targetIndex < 0 {
		return false
	}
	tag := s.Tags[sourceIndex]
	s.Tags = append(s.Tags[:sourceIndex], s.Tags[sourceIndex+1:]...)
	if sourceIndex < targetIndex {
		targetIndex--
	}
	insertionIndex := targetIndex
	if after {
		insertionIndex++
	}
	if insertionIndex > len(s.Tags) {
		insertionIndex = len(s.Tags)
	}
	s.Tags = append(s.Tags[:insertionIndex], append([]Tag{tag}, s.Tags[insertionIndex:]...)...)
	return true
}

func sameParent(a, b *TagID) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// ToggleAssignment is TagState::toggle_assignment.
func (s *TagState) ToggleAssignment(skill string, tagID TagID) {
	s.SetAssignment(skill, tagID, !s.IsAssigned(skill, tagID))
}

// SetAssignment is TagState::set_assignment.
func (s *TagState) SetAssignment(skill string, tagID TagID, assigned bool) bool {
	tag := s.GetTag(tagID)
	if tag == nil {
		return false
	}
	parent := tag.Parent
	var children []TagID
	if parent == nil {
		for _, child := range s.Children(tagID) {
			children = append(children, child.ID)
		}
	}
	s.normalize()
	set := map[TagID]bool{}
	for _, id := range s.Assignments[skill] {
		set[id] = true
	}
	var changed bool
	if assigned {
		_, had := set[tagID]
		set[tagID] = true
		changed = !had
		if parent != nil {
			delete(set, *parent)
		} else {
			for _, child := range children {
				delete(set, child)
			}
		}
	} else {
		_, had := set[tagID]
		delete(set, tagID)
		changed = had
	}
	if len(set) == 0 {
		delete(s.Assignments, skill)
	} else {
		ids := make([]TagID, 0, len(set))
		for id := range set {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		s.Assignments[skill] = ids
	}
	return changed
}

// IsAssigned is TagState::is_assigned.
func (s *TagState) IsAssigned(skill string, tagID TagID) bool {
	for _, id := range s.Assignments[skill] {
		if id == tagID {
			return true
		}
	}
	return false
}

// AssignedTags is TagState::assigned_tags.
func (s *TagState) AssignedTags(skill string) []*Tag {
	assigned := map[TagID]bool{}
	for _, id := range s.Assignments[skill] {
		assigned[id] = true
	}
	var out []*Tag
	for i := range s.Tags {
		if assigned[s.Tags[i].ID] {
			out = append(out, &s.Tags[i])
		}
	}
	return out
}

// Path is TagState::path.
func (s *TagState) Path(id TagID) (string, bool) {
	tag := s.GetTag(id)
	if tag == nil {
		return "", false
	}
	if tag.Parent != nil {
		if parent := s.GetTag(*tag.Parent); parent != nil {
			return parent.Name + "/" + tag.Name, true
		}
	}
	return tag.Name, true
}

// FindPath is TagState::find_path.
func (s *TagState) FindPath(path string) (TagID, bool) {
	path = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(path), "#"))
	for _, tag := range s.Tags {
		if candidate, ok := s.Path(tag.ID); ok && strings.EqualFold(candidate, path) {
			return tag.ID, true
		}
	}
	return 0, false
}

// Count is TagState::count.
func (s *TagState) Count(id TagID) int {
	descendants := map[TagID]bool{}
	if tag := s.GetTag(id); tag != nil && tag.Parent == nil {
		for _, child := range s.Children(id) {
			descendants[child.ID] = true
		}
	}
	count := 0
	for _, assigned := range s.Assignments {
		for _, tid := range assigned {
			if tid == id || descendants[tid] {
				count++
				break
			}
		}
	}
	return count
}

// MatchesFilter is TagState::matches_filter.
func (s *TagState) MatchesFilter(skill string, filter TagID) bool {
	assigned, ok := s.Assignments[skill]
	if !ok {
		return false
	}
	for _, id := range assigned {
		if id == filter {
			return true
		}
	}
	tag := s.GetTag(filter)
	if tag == nil || tag.Parent != nil {
		return false
	}
	for _, child := range s.Children(filter) {
		for _, id := range assigned {
			if id == child.ID {
				return true
			}
		}
	}
	return false
}

func (s *TagState) siblingNameExists(name string, parent *TagID, except TagID) bool {
	for _, tag := range s.Tags {
		if tag.ID != except && sameParent(tag.Parent, parent) && strings.EqualFold(tag.Name, name) {
			return true
		}
	}
	return false
}

func normalizeName(name string) (string, error) {
	name = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(name), "#"))
	if name == "" {
		return "", errors.New("请输入标签名称")
	}
	if strings.Contains(name, "/") {
		return "", fmt.Errorf("标签名称不能包含 / ")
	}
	return name, nil
}
