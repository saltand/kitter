package app

import (
	"reflect"
	"testing"
)

// Mirrors ui/skill_selection.rs tests.

func keys(values ...string) []string { return values }

func opt(s string, ok bool) (string, bool) { return s, ok }

func TestReconcileDropsMissingKeysAndSelectsFirstSurvivor(t *testing.T) {
	selection := NewSkillSelection("first", true)
	selection.SelectAll(keys("first", "second"))

	if got, _ := selection.Reconcile(keys("second", "third")); got != "second" {
		t.Fatalf("reconcile → %q", got)
	}
	if got := selection.SelectedIn(keys("second", "third")); !reflect.DeepEqual(got, keys("second")) {
		t.Fatalf("selected_in %v", got)
	}
}

func TestToggleKeepsDetailOnASelectedSkill(t *testing.T) {
	order := keys("first", "second", "third")
	selection := NewSkillSelection("first", true)

	if got, _ := selection.Toggle("second", order); got != "first" {
		t.Fatalf("toggle → %q", got)
	}
	if got, _ := selection.Toggle("first", order); got != "second" {
		t.Fatalf("toggle → %q", got)
	}
	if !selection.IsMultiple() {
		t.Fatal("expected multiple mode")
	}
}

func TestFinishingBatchSelectionCollapsesToOneSkill(t *testing.T) {
	order := keys("first", "second", "third")
	selection := NewSkillSelection("first", true)
	selection.SelectAll(keys("second", "third"))

	if got, _ := selection.Finish(order); got != "second" {
		t.Fatalf("finish → %q", got)
	}
	if selection.Len() != 1 || selection.IsMultiple() {
		t.Fatal("finish must collapse to one")
	}
}

func TestContextClickPreservesAnExistingBatch(t *testing.T) {
	selection := NewSkillSelection("first", true)
	selection.SelectAll(keys("first", "second"))

	if got, _ := selection.SelectForContext("second"); got != "second" {
		t.Fatalf("select_for_context → %q", got)
	}
	if selection.Len() != 2 || !selection.Contains("first") {
		t.Fatal("batch lost")
	}
}

func TestShiftSelectionUsesTheVisibleOrderInBothDirections(t *testing.T) {
	visible := keys("first", "third", "fifth", "seventh")
	selection := NewSkillSelection("third", true)

	if got, _ := selection.SelectRange("seventh", visible); got != "seventh" {
		t.Fatalf("select_range → %q", got)
	}
	if got := selection.SelectedIn(visible); !reflect.DeepEqual(got, keys("third", "fifth", "seventh")) {
		t.Fatalf("selected_in %v", got)
	}

	if got, _ := selection.SelectRange("first", visible); got != "first" {
		t.Fatalf("select_range → %q", got)
	}
	if got := selection.SelectedIn(visible); !reflect.DeepEqual(got, keys("first", "third")) {
		t.Fatalf("selected_in %v", got)
	}
}

func TestReplacingSelectionResetsTheShiftAnchor(t *testing.T) {
	visible := keys("first", "second", "third", "fourth")
	selection := NewSkillSelection("first", true)

	selection.Replace(keys("third"))
	selection.SelectRange("fourth", visible)

	if got := selection.SelectedIn(visible); !reflect.DeepEqual(got, keys("third", "fourth")) {
		t.Fatalf("selected_in %v", got)
	}
}
