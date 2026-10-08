// drag.go ports the drag/drop model from ui/mod.rs + organize_flows.rs
// + skills_page.rs: SkillDrag, GroupDrag, TagDrag, the drop_target
// state, and the drop-position computation.
//
// mygo's ui.Drag + ui.Drop[T]/ui.DragOver[T] are the direct analogs of
// gpui's on_drag/on_drop/drag_over: Drag attaches a typed value to a
// row, Drop reads it on release, DragOver reads it while hovering. The
// drop-position rule (upper half = Before, lower half = After, plus
// scope/parent/self filters) lives in pure functions so tests can drive
// them without pointer math.
package app

import (
	"github.com/saltand/kitter/native/core/tags"
)

// GroupDragScope is ui::GroupDragScope.
type GroupDragScope int

const (
	GroupDragManagement GroupDragScope = iota
	GroupDragList
)

// skillDrag is ui::SkillDrag: the storage name of the dragged skill.
type skillDrag struct {
	Name string
}

// groupDrag is ui::GroupDrag.
type groupDrag struct {
	Scope GroupDragScope
	ID    string
	Name  string
}

// tagDrag is ui::TagDrag.
type tagDrag struct {
	Scope  TagScope
	Parent *tags.TagID
	ID     tags.TagID
	Name   string
}

// tagDropPosition is ui::TagDropPosition.
type tagDropPosition int

const (
	tagDropBefore tagDropPosition = iota
	tagDropAfter
)

// tagDropTarget is ui::TagDropTarget.
type tagDropTarget struct {
	Scope    TagScope
	TagID    tags.TagID
	Position tagDropPosition
}

// groupDropTarget is the (scope, id, position) tuple Rust stores in
// groups_flow.drop_target.
type groupDropTarget struct {
	Scope    GroupDragScope
	ID       string
	Position tagDropPosition
}

// dropHalf returns the position for a pointer at pointerY inside a row
// whose vertical span is [rowTop, rowBottom): upper half → Before.
func dropHalf(rowTop, rowBottom, pointerY float32) tagDropPosition {
	if pointerY < rowTop+(rowBottom-rowTop)/2 {
		return tagDropBefore
	}
	return tagDropAfter
}

// tagDropAllowed is the DragMoveEvent<TagDrag> validity filter: same
// scope, same parent level, not self.
func tagDropAllowed(drag *tagDrag, targetScope TagScope, targetParent *tags.TagID, targetID tags.TagID) bool {
	if drag.Scope != targetScope || drag.ID == targetID {
		return false
	}
	if drag.Parent == nil && targetParent == nil {
		return true
	}
	if drag.Parent != nil && targetParent != nil {
		return *drag.Parent == *targetParent
	}
	return false
}

// groupDropAllowed is the DragMoveEvent<GroupDrag> filter: same scope,
// not self.
func groupDropAllowed(drag *groupDrag, targetScope GroupDragScope, targetID string) bool {
	return drag.Scope == targetScope && drag.ID != targetID
}

// tagDropPoint computes the drop target for a tag row from the row's
// top/height and the pointer's y. Returns nil when the drop is invalid.
func tagDropPoint(drag *tagDrag, scope TagScope, parent *tags.TagID, id tags.TagID, rowTop, rowBottom, pointerY float32) *tagDropTarget {
	if !tagDropAllowed(drag, scope, parent, id) {
		return nil
	}
	return &tagDropTarget{Scope: scope, TagID: id, Position: dropHalf(rowTop, rowBottom, pointerY)}
}

// groupDropPoint computes the drop target for a group row.
func groupDropPoint(drag *groupDrag, scope GroupDragScope, id string, rowTop, rowBottom, pointerY float32) *groupDropTarget {
	if !groupDropAllowed(drag, scope, id) {
		return nil
	}
	return &groupDropTarget{Scope: scope, ID: id, Position: dropHalf(rowTop, rowBottom, pointerY)}
}

// applyTagDrop performs the reorder (move_before/move_after) +
// persist, mirroring the on_drop handler.
func (a *App) applyTagDrop(drag *tagDrag, target *tagDropTarget) {
	if !tagDropAllowed(drag, target.Scope, a.tagsFor(target.Scope).GetTag(target.TagID).Parent, target.TagID) {
		return
	}
	state := a.tagsFor(target.Scope)
	var moved bool
	if target.Position == tagDropBefore {
		moved = state.MoveBefore(drag.ID, target.TagID)
	} else {
		moved = state.MoveAfter(drag.ID, target.TagID)
	}
	if moved {
		a.persistTags()
	}
}

// applyGroupDrop performs move_group for the same scope.
func (a *App) applyGroupDrop(drag *groupDrag, target *groupDropTarget) {
	if !groupDropAllowed(drag, target.Scope, target.ID) {
		return
	}
	if _, err := a.Library.MoveGroup(drag.ID, target.ID, target.Position == tagDropAfter); err != nil {
		a.notice(a.ErrorMessage(err))
	}
}
