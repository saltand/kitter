// adoption_list.go ports the virtualized adoption list row model from
// src/ui/adoption_list.rs: header/candidate/disclosure/reference rows
// over a flat slice, and the expand/collapse splice logic.
package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/saltand/kitter/native/core/adoption"
	"github.com/saltand/kitter/native/core/model"
)

// adoptionRowKind mirrors AdoptionRow.
type adoptionRowKind int

const (
	adoptionRowHeader adoptionRowKind = iota
	adoptionRowCandidate
	adoptionRowDisclosure
	adoptionRowReference
)

// adoptionRow is a flattened list row. index is the candidate index;
// ref is set for reference rows.
type adoptionRow struct {
	kind  adoptionRowKind
	index int
	ref   model.SkillReference
}

// buildAdoptionRows rebuilds the flat row list, as rows_for does.
func (a *App) buildAdoptionRows() {
	scan := a.AddFlow.AdoptionScan
	if scan == nil {
		a.AddFlow.adoptionRows = nil
		return
	}
	rows := []adoptionRow{}
	prevSource := ""
	expanded := a.AddFlow.adoptionExpanded
	if expanded == nil {
		expanded = map[string]bool{}
		a.AddFlow.adoptionExpanded = expanded
	}
	for i, c := range scan.Candidates {
		key := c.Origin.Source().Key()
		if key != prevSource {
			prevSource = key
			rows = append(rows, adoptionRow{kind: adoptionRowHeader, index: i})
		}
		rows = append(rows, adoptionRow{kind: adoptionRowCandidate, index: i})
		if len(c.References) > 0 {
			rows = append(rows, adoptionRow{kind: adoptionRowDisclosure, index: i})
			if expanded[c.ID] {
				for _, ref := range c.References {
					rows = append(rows, adoptionRow{kind: adoptionRowReference, index: i, ref: ref})
				}
			}
		}
	}
	a.AddFlow.adoptionRows = rows
}

// adoptionRowCandidate resolves a row's candidate.
func (a *App) adoptionRowCandidate(r adoptionRow) *adoption.AdoptionCandidate {
	if a.AddFlow.AdoptionScan == nil || r.index >= len(a.AddFlow.AdoptionScan.Candidates) {
		return nil
	}
	return a.AddFlow.AdoptionScan.Candidates[r.index]
}

// toggleAdoptionExpanded flips expanded state and rebuilds the row
// list (the `splice` calls in adoption_list.rs).
func (a *App) toggleAdoptionExpanded(id string) {
	if a.AddFlow.adoptionExpanded == nil {
		a.AddFlow.adoptionExpanded = map[string]bool{}
	}
	a.AddFlow.adoptionExpanded[id] = !a.AddFlow.adoptionExpanded[id]
	a.buildAdoptionRows()
}

// adoptionRow renders one row, as KitterApp::adoption_row does.
func (a *App) adoptionRow(c *ui.Context, pos int) ui.Element {
	if pos >= len(a.AddFlow.adoptionRows) {
		return ui.Box(c)
	}
	row := a.AddFlow.adoptionRows[pos]
	candidate := a.adoptionRowCandidate(row)
	if candidate == nil {
		return ui.Box(c)
	}
	p := a.Palette()
	switch row.kind {
	case adoptionRowHeader:
		return ui.Row(c).Height(32).AlignItems(ui.Center).Padding(4, 8).
			Children(func() {
				ui.Text(c, a.sourceLabel(&candidate.Origin)).
					FontSize(12).TextColor(p.Secondary).Bold()
			})

	case adoptionRowDisclosure:
		n := len(candidate.References)
		label := a.T(fmt.Sprintf("查看 %d 条引用", n), fmt.Sprintf("%d references", n))
		expanded := a.AddFlow.adoptionExpanded[candidate.ID]
		return ui.Row(c).Height(32).AlignItems(ui.Center).Padding(4, 8).
			Children(func() {
				ui.Text(c, chevron(expanded)).FontSize(12).TextColor(p.Secondary)
				ui.Box(c).Width(6).Shrink(0)
				ui.Text(c, label).FontSize(12).TextColor(p.Secondary)
			})

	case adoptionRowReference:
		return ui.Row(c).Height(32).AlignItems(ui.Center).Padding(4, 8).
			Children(func() {
				ui.Text(c, "  ").FontSize(12)
				ui.Text(c, adoptionReferenceLabel(row.ref)).FontSize(12).TextColor(p.Secondary)
			})

	default: // adoptionRowCandidate
		id := candidate.ID
		selectable := a.AddFlow.AdoptionScan.SelectableIDs()[id]
		check := "[ ]"
		if a.AddFlow.Selected[id] {
			check = "[x]"
		}
		if !selectable {
			check = "[-]"
		}
		conflict := a.AddFlow.AdoptionScan.HasConflict(candidate.Identity())
		badges := ""
		if conflict {
			badges = a.T("存在冲突", "Conflict")
		}
		if !selectable {
			badges = a.T("已在库中", "Already in library")
		}
		row := ui.Row(c).Height(32).AlignItems(ui.Center).Padding(4, 8).
			Children(func() {
				ui.Text(c, check).FontSize(13)
				ui.Box(c).Width(6).Shrink(0)
				ui.Text(c, candidate.Name).FontSize(13).Grow(1).MinWidth(0).SingleLine()
				if badges != "" {
					ui.Text(c, badges).FontSize(11).TextColor(p.Secondary)
					ui.Box(c).Width(6).Shrink(0)
				}
				ui.Text(c, shortenSource(candidate.Source)).FontSize(11).TextColor(p.Secondary)
				ui.Box(c).Width(6).Shrink(0)
				ui.Text(c, "Reveal").FontSize(11).TextColor(p.Accent)
			})
		if row.Clicked() {
			if !a.addBusy() {
				a.AddFlow.AdoptionScan.Select(a.AddFlow.Selected, id)
			}
		}
		return row
	}
}

// adoptionReferenceLabel is reference_label in add_flow.rs.
func adoptionReferenceLabel(ref model.SkillReference) string {
	suffix := ""
	switch ref.Kind {
	case model.ReferenceDirect:
		suffix = " · direct"
	case model.ReferencePlugin:
		suffix = " · plugin"
	}
	return ref.Path + suffix
}

func chevron(expanded bool) string {
	if expanded {
		return "v"
	}
	return ">"
}

// shortenSource abbreviates the source path (home-relative, tail
// segments) for the secondary line.
func shortenSource(source string) string {
	if home := homeDir(); home != "" {
		if rel, err := filepath.Rel(home, source); err == nil && !strings.HasPrefix(rel, "..") {
			return "~/" + rel
		}
	}
	parts := strings.Split(source, "/")
	if len(parts) > 3 {
		return "…/" + strings.Join(parts[len(parts)-3:], "/")
	}
	return source
}
