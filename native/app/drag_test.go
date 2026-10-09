// drag_test.go covers the drop-position pure functions and one
// end-to-end drag through ui.NewTester's Press/Move/Release.
package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/saltand/kitter/native/core/tags"
)

func ptr(v tags.TagID) *tags.TagID { return &v }

// TestDropHalfTable is the boundary split: upper half → Before,
// lower half → After.
func TestDropHalfTable(t *testing.T) {
	for _, tc := range []struct {
		top, bottom, y float32
		want           tagDropPosition
	}{
		{0, 34, 0, tagDropBefore},
		{0, 34, 16, tagDropBefore},
		{0, 34, 17, tagDropAfter},
		{0, 34, 33, tagDropAfter},
		{100, 134, 115, tagDropBefore},
		{100, 134, 118, tagDropAfter},
	} {
		if got := dropHalf(tc.top, tc.bottom, tc.y); got != tc.want {
			t.Fatalf("dropHalf(%v,%v,%v) = %v, want %v", tc.top, tc.bottom, tc.y, got, tc.want)
		}
	}
}

// TestTagDropAllowedTable covers the scope/parent/self filters.
func TestTagDropAllowedTable(t *testing.T) {
	a, b := tags.TagID(1), tags.TagID(2)
	for _, tc := range []struct {
		name   string
		drag   *tagDrag
		scope  TagScope
		parent *tags.TagID
		target tags.TagID
		want   bool
	}{
		{"same level root", &tagDrag{Scope: TagScopeSkills, ID: a}, TagScopeSkills, nil, b, true},
		{"self", &tagDrag{Scope: TagScopeSkills, ID: a}, TagScopeSkills, nil, a, false},
		{"cross scope", &tagDrag{Scope: TagScopeSkills, ID: a}, TagScopeProjects, nil, b, false},
		{"same child level", &tagDrag{Scope: TagScopeSkills, Parent: ptr(9), ID: a}, TagScopeSkills, ptr(9), b, true},
		{"different child level", &tagDrag{Scope: TagScopeSkills, Parent: ptr(9), ID: a}, TagScopeSkills, ptr(10), b, false},
		{"root onto child level", &tagDrag{Scope: TagScopeSkills, ID: a}, TagScopeSkills, ptr(9), b, false},
	} {
		if got := tagDropAllowed(tc.drag, tc.scope, tc.parent, tc.target); got != tc.want {
			t.Fatalf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestGroupDropAllowedTable covers scope + self filters.
func TestGroupDropAllowedTable(t *testing.T) {
	drag := &groupDrag{Scope: GroupDragList, ID: "a"}
	if !groupDropAllowed(drag, GroupDragList, "b") {
		t.Fatal("same-scope group must be droppable")
	}
	if groupDropAllowed(drag, GroupDragList, "a") {
		t.Fatal("self must be rejected")
	}
	if groupDropAllowed(drag, GroupDragManagement, "b") {
		t.Fatal("cross-scope must be rejected")
	}
}

// TestGroupReorderEndToEnd drags one group row onto another in the
// skills-page list and asserts the registry order changed.
func TestGroupReorderEndToEnd(t *testing.T) {
	app := newTestApp(t)
	g1, err := app.Library.CreateGroup("alpha")
	if err != nil {
		t.Fatal(err)
	}
	g2, err := app.Library.CreateGroup("beta")
	if err != nil {
		t.Fatal(err)
	}
	app.ReloadSkills()

	tt := ui.NewTester(app.View, 1200, 720)
	from, ok := tt.Find("skill-group-" + g1.ID)
	if !ok {
		t.Fatalf("group row for %s not found: %q", g1.ID, tt.Texts())
	}
	to, ok := tt.Find("skill-group-" + g2.ID)
	if !ok {
		t.Fatalf("group row for %s not found: %q", g2.ID, tt.Texts())
	}
	// Drag g1's row onto the lower half of g2's row (After).
	x, y := from.X+60, from.Y+from.H/2
	tx, ty := to.X+60, to.Y+to.H-4
	tt.Press(x, y)
	tt.Move(x+4, y+4) // pass the drag-start threshold
	for i := 1; i <= 4; i++ {
		f := float32(i) / 4
		tt.Move(x+(tx-x)*f, y+(ty-y)*f)
	}
	tt.Release(tx, ty)
	waitIdle(t, app)

	// Registry order should be beta, alpha now (alpha moved after beta).
	groups := app.Library.Groups()
	if len(groups) < 2 || groups[0].ID != g2.ID || groups[1].ID != g1.ID {
		t.Fatalf("order %v, want [%s %s]", groups, g2.ID, g1.ID)
	}
}

// TestSkillDropToGroupEndToEnd drags a skill row onto a group header
// and asserts the skill moved into the group.
func TestSkillDropToGroupEndToEnd(t *testing.T) {
	app := newTestApp(t)
	skill := importSkill(t, app, "drag-me", "demo")
	_ = skill
	group, err := app.Library.CreateGroup("target")
	if err != nil {
		t.Fatal(err)
	}
	app.ReloadSkills()

	tt := ui.NewTester(app.View, 1200, 720)
	from, ok := tt.Find("drag-me")
	if !ok {
		t.Fatalf("skill row not found: %q", tt.Texts())
	}
	to, ok := tt.Find("target")
	if !ok {
		t.Fatalf("group row not found: %q", tt.Texts())
	}
	x, y := from.X+60, from.Y+from.H/2
	tx, ty := to.X+60, to.Y+to.H/2
	tt.Press(x, y)
	tt.Move(x+4, y+4)
	for i := 1; i <= 4; i++ {
		f := float32(i) / 4
		tt.Move(x+(tx-x)*f, y+(ty-y)*f)
	}
	tt.Release(tx, ty)
	waitIdle(t, app)

	if got := findSkill(t, app, "drag-me").Record.GroupID; got == nil || *got != group.ID {
		t.Fatalf("skill group %v, want %s", got, group.ID)
	}
}
