// skill_selection.go ports ui/skill_selection.rs: the skill-list
// selection model, owned independently from MyGo rendering state.
//
// primary drives the detail pane, while keys is the complete action
// selection. Keeping both here prevents list gestures, refreshes, and
// batch actions from implementing slightly different reconciliation
// rules.
package app

// SkillSelection is ui::skill_selection::SkillSelection.
type SkillSelection struct {
	primary  string
	hasPrim  bool
	anchor   string
	hasAnchr bool
	multiple bool
	keys     map[string]bool
}

// NewSkillSelection is SkillSelection::new.
func NewSkillSelection(primary string, hasPrimary bool) SkillSelection {
	s := SkillSelection{keys: map[string]bool{}}
	if hasPrimary {
		s.primary, s.hasPrim = primary, true
		s.anchor, s.hasAnchr = primary, true
		s.keys[primary] = true
	}
	return s
}

// ensureKeys lazily creates the key set so the zero value works.
func (s *SkillSelection) ensureKeys() {
	if s.keys == nil {
		s.keys = map[string]bool{}
	}
}

// Primary is SkillSelection::primary.
func (s *SkillSelection) Primary() (string, bool) { return s.primary, s.hasPrim }

// SetPrimary is SkillSelection::set_primary.
func (s *SkillSelection) SetPrimary(primary string, has bool) {
	s.primary, s.hasPrim = primary, has
}

// IsMultiple is SkillSelection::is_multiple.
func (s *SkillSelection) IsMultiple() bool { return s.multiple }

// Contains is SkillSelection::contains.
func (s *SkillSelection) Contains(key string) bool { return s.keys[key] }

// Len is SkillSelection::len.
func (s *SkillSelection) Len() int { return len(s.keys) }

// SelectedIn is SkillSelection::selected_in.
func (s *SkillSelection) SelectedIn(order []string) []string {
	var selected []string
	for _, key := range order {
		if s.keys[key] {
			selected = append(selected, key)
		}
	}
	if len(selected) == 0 && s.hasPrim && strIn(order, s.primary) {
		return []string{s.primary}
	}
	return selected
}

// Reconcile is SkillSelection::reconcile.
func (s *SkillSelection) Reconcile(order []string) (string, bool) {
	s.ensureKeys()
	existing := strSet(order)
	for key := range s.keys {
		if !existing[key] {
			delete(s.keys, key)
		}
	}
	if len(s.keys) == 0 && s.hasPrim && existing[s.primary] {
		s.keys[s.primary] = true
	}
	if !s.hasPrim || !existing[s.primary] || !s.keys[s.primary] {
		s.hasPrim = false
		for _, key := range order {
			if s.keys[key] {
				s.primary, s.hasPrim = key, true
				break
			}
		}
		if !s.hasPrim && len(order) > 0 {
			s.primary, s.hasPrim = order[0], true
		}
	}
	if s.hasAnchr && !existing[s.anchor] {
		s.anchor, s.hasAnchr = s.primary, s.hasPrim
	}
	if len(s.keys) == 0 && s.hasPrim {
		s.keys[s.primary] = true
	}
	return s.primary, s.hasPrim
}

// Replace is SkillSelection::replace.
func (s *SkillSelection) Replace(keys []string) (string, bool) {
	s.keys = strSet(keys)
	if !s.hasPrim || !s.keys[s.primary] {
		s.hasPrim = len(keys) > 0
		if s.hasPrim {
			s.primary = keys[0]
		}
	}
	s.anchor, s.hasAnchr = s.primary, s.hasPrim
	return s.primary, s.hasPrim
}

// Remove is SkillSelection::remove.
func (s *SkillSelection) Remove(removed []string, order []string) (string, bool) {
	gone := strSet(removed)
	for key := range s.keys {
		if gone[key] {
			delete(s.keys, key)
		}
	}
	if s.hasPrim && gone[s.primary] {
		s.hasPrim = false
	}
	return s.Reconcile(order)
}

// Finish is SkillSelection::finish.
func (s *SkillSelection) Finish(order []string) (string, bool) {
	s.ensureKeys()
	var primary string
	has := false
	for _, key := range order {
		if s.keys[key] {
			primary, has = key, true
			break
		}
	}
	if !has && s.hasPrim && strIn(order, s.primary) {
		primary, has = s.primary, true
	}
	if !has && len(order) > 0 {
		primary, has = order[0], true
	}
	s.multiple = false
	s.keys = map[string]bool{}
	if has {
		s.keys[primary] = true
	}
	s.anchor, s.hasAnchr = primary, has
	s.primary, s.hasPrim = primary, has
	return primary, has
}

// Clear is SkillSelection::clear.
func (s *SkillSelection) Clear() {
	s.multiple = false
	s.keys = map[string]bool{}
	s.hasAnchr = false
	s.hasPrim = false
}

// SelectAll is SkillSelection::select_all.
func (s *SkillSelection) SelectAll(visible []string) (string, bool) {
	s.ensureKeys()
	s.multiple = true
	s.keys = strSet(visible)
	if !s.hasPrim || !s.keys[s.primary] {
		s.hasPrim = len(visible) > 0
		if s.hasPrim {
			s.primary = visible[0]
		}
	}
	s.anchor, s.hasAnchr = s.primary, s.hasPrim
	return s.primary, s.hasPrim
}

// Toggle is SkillSelection::toggle.
func (s *SkillSelection) Toggle(key string, order []string) (string, bool) {
	s.ensureKeys()
	s.multiple = true
	if s.keys[key] {
		delete(s.keys, key)
	} else {
		s.keys[key] = true
	}
	if len(s.keys) == 0 {
		s.hasPrim = false
	} else if !s.hasPrim || !s.keys[s.primary] {
		s.hasPrim = false
		for _, candidate := range order {
			if s.keys[candidate] {
				s.primary, s.hasPrim = candidate, true
				break
			}
		}
	}
	s.anchor, s.hasAnchr = key, true
	return s.primary, s.hasPrim
}

// SelectRange is SkillSelection::select_range.
func (s *SkillSelection) SelectRange(key string, visible []string) (string, bool) {
	s.ensureKeys()
	anchorIdx := -1
	anchor := s.anchor
	if !s.hasAnchr && s.hasPrim {
		anchor = s.primary
	}
	if s.hasAnchr || s.hasPrim {
		anchorIdx = indexOf(visible, anchor)
	}
	target := indexOf(visible, key)
	if anchorIdx < 0 || target < 0 {
		return s.SelectOne(key)
	}
	start, end := anchorIdx, target
	if start > end {
		start, end = end, start
	}
	s.multiple = true
	s.keys = strSet(visible[start : end+1])
	s.primary, s.hasPrim = key, true
	return key, true
}

// SelectOne is SkillSelection::select_one.
func (s *SkillSelection) SelectOne(key string) (string, bool) {
	s.multiple = false
	s.keys = map[string]bool{key: true}
	s.anchor, s.hasAnchr = key, true
	s.primary, s.hasPrim = key, true
	return key, true
}

// SelectForContext is SkillSelection::select_for_context.
func (s *SkillSelection) SelectForContext(key string) (string, bool) {
	s.ensureKeys()
	if !s.keys[key] {
		s.multiple = false
		s.keys = map[string]bool{key: true}
	}
	s.primary, s.hasPrim = key, true
	return key, true
}

func strSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, v := range values {
		set[v] = true
	}
	return set
}

func strIn(values []string, needle string) bool { return indexOf(values, needle) >= 0 }

func indexOf(values []string, needle string) int {
	for i, v := range values {
		if v == needle {
			return i
		}
	}
	return -1
}
