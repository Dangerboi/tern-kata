package model_test

import (
	"fmt"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func cardKeys(m model.Model, chords ...string) model.Model {
	for _, chord := range chords {
		msg := model.KeyMsg{Chord: chord}
		if len([]rune(chord)) == 1 {
			msg.Text = chord
		}
		m, _ = model.Update(m, msg)
	}
	return m
}

// filterCardOn is the Filter card opened over a Project Collection of n Projects, with focus on field.
func filterCardOn(n int, field model.Field) model.Model {
	m := model.Model{Collection: model.Collection{View: "all-open"}}
	for i := range n {
		uid := fmt.Sprintf("p%02d", i)
		m.Snapshot.Catalog = append(m.Snapshot.Catalog, model.CatalogProject{Project: model.Project{UID: uid, Name: "Project " + uid}})
	}
	m = cardKeys(m, "f")
	m, _ = model.Update(m, model.FocusFieldMsg{Field: field})
	return m
}

func TestSelectOpensWithDownAndHighlightsWithTheArrows(t *testing.T) {
	m := filterCardOn(0, model.FilterRelationship)
	m = cardKeys(m, "down")
	dropdown := m.OpenDropdown(model.CardFilter, model.FilterRelationship)
	if !dropdown.Open || dropdown.Active != 0 {
		t.Fatalf("down on a closed select left dropdown %+v, want it open on the held choice", dropdown)
	}
	m = cardKeys(m, "down", "down")
	if got := m.OpenDropdown(model.CardFilter, model.FilterRelationship).Active; got != 2 {
		t.Fatalf("two downs highlight %d, want 2", got)
	}
	m = cardKeys(m, "up", "up", "up")
	if got := m.OpenDropdown(model.CardFilter, model.FilterRelationship).Active; got != len(model.FilterRelationships)-1 {
		t.Fatalf("up from the first wraps to %d, want the last", got)
	}
	if m.FilterForm.Values.Relationship != "" {
		t.Fatalf("moving the highlight set %q", m.FilterForm.Values.Relationship)
	}
}

func TestSelectEnterPicksTheHighlightAndEscapeKeepsTheCard(t *testing.T) {
	m := cardKeys(filterCardOn(0, model.FilterRelationship), "down", "down", "down", "enter")
	if got := m.FilterForm.Values.Relationship; got != "child" {
		t.Fatalf("enter picked %q, want child", got)
	}
	if m.OpenDropdown(model.CardFilter, model.FilterRelationship).Open || !m.FilterForm.Open || m.FilterForm.Field != model.FilterRelationship {
		t.Fatalf("after a pick: dropdown open %v, card open %v, field %q", m.Dropdown.Open, m.FilterForm.Open, m.FilterForm.Field)
	}
	m = cardKeys(m, "down", "escape")
	if m.OpenDropdown(model.CardFilter, model.FilterRelationship).Open || !m.FilterForm.Open || m.FilterForm.Values.Relationship != "child" {
		t.Fatalf("escape closes the dropdown only: dropdown open %v, card open %v, value %q", m.Dropdown.Open, m.FilterForm.Open, m.FilterForm.Values.Relationship)
	}
	m = cardKeys(m, "escape")
	if m.FilterForm.Open {
		t.Fatal("escape with the dropdown closed did not cancel the card")
	}
}

func TestKeysThatSetAChoiceStillSetItWhileTheMenuIsClosedOrOpen(t *testing.T) {
	m := cardKeys(filterCardOn(0, model.FilterRelationship), "right")
	if got := m.FilterForm.Values.Relationship; got != "parent" {
		t.Fatalf("right set %q, want parent", got)
	}
	m = cardKeys(m, "space")
	if got := m.FilterForm.Values.Relationship; got != "child" {
		t.Fatalf("space set %q, want child", got)
	}
	m = cardKeys(m, "down", "left")
	if got := m.FilterForm.Values.Relationship; got != "parent" || m.Dropdown.Open {
		t.Fatalf("left in an open dropdown set %q (dropdown open %v), want parent and the dropdown closed", got, m.Dropdown.Open)
	}
	m = cardKeys(m, "down", "tab")
	if m.Dropdown.Open || m.FilterForm.Field == model.FilterRelationship {
		t.Fatalf("tab left dropdown open %v on field %q", m.Dropdown.Open, m.FilterForm.Field)
	}
}

func TestDownOnATabsFieldOpensNothing(t *testing.T) {
	m := cardKeys(filterCardOn(0, model.FilterStatus), "down")
	if m.Dropdown.Open {
		t.Fatal("down opened a dropdown on the Status tabs")
	}
}

func TestSelectPastADozenOptionsSearchesAndEnterPicksTheFirstHit(t *testing.T) {
	m := filterCardOn(13, model.FilterProject) // the empty choice and 13 Projects
	set, ok := m.CardChoices(model.CardFilter, model.FilterProject)
	if !ok || !set.Search() {
		t.Fatalf("project choices %+v (ok %v), want a searching select", set, ok)
	}
	m = cardKeys(m, "down", "p", "1", "1")
	dropdown := m.OpenDropdown(model.CardFilter, model.FilterProject)
	if dropdown.Query != "p11" || set.Matches(dropdown.Query)[0] != dropdown.Active {
		t.Fatalf("typing p11 left dropdown %+v", dropdown)
	}
	m = cardKeys(m, "backspace")
	if got := m.OpenDropdown(model.CardFilter, model.FilterProject).Query; got != "p1" {
		t.Fatalf("backspace left query %q, want p1", got)
	}
	m = cardKeys(m, "1", "enter")
	if got := m.FilterForm.Values.ProjectUID; got != "p11" {
		t.Fatalf("enter picked %q, want p11", got)
	}
	if m.Dropdown.Open || m.Dropdown.Query != "" {
		t.Fatalf("a pick left dropdown %+v", m.Dropdown)
	}
	m = cardKeys(m, "down", "z", "z", "escape")
	if m.Dropdown.Open || m.Dropdown.Query != "" || !m.FilterForm.Open {
		t.Fatalf("escape left dropdown %+v, card open %v", m.Dropdown, m.FilterForm.Open)
	}
}

func TestSelectWithAFewOptionsTypesNothing(t *testing.T) {
	m := cardKeys(filterCardOn(0, model.FilterRelationship), "down", "x")
	if m.Dropdown.Query != "" {
		t.Fatalf("a select without search took the text %q", m.Dropdown.Query)
	}
}

func TestPointerPickAndToggleDriveTheSameState(t *testing.T) {
	m := filterCardOn(0, model.FilterStatus)
	m, _ = model.Update(m, model.DropdownToggleMsg{Field: model.FilterRelationship, Open: true})
	if dropdown := m.OpenDropdown(model.CardFilter, model.FilterRelationship); !dropdown.Open || m.FilterForm.Field != model.FilterRelationship {
		t.Fatalf("the pointer's open left dropdown %+v on field %q", dropdown, m.FilterForm.Field)
	}
	m, _ = model.Update(m, model.ChooseMsg{Field: model.FilterRelationship, Index: 3})
	if got := m.FilterForm.Values.Relationship; got != "blocks" || m.Dropdown.Open {
		t.Fatalf("a pick set %q with the dropdown open %v", got, m.Dropdown.Open)
	}
	m, _ = model.Update(m, model.DropdownToggleMsg{Field: model.FilterStatus, Open: true})
	if m.Dropdown.Open {
		t.Fatal("the pointer opened a dropdown on a tabs field")
	}
}

func TestMenuClosesWhenFocusMovesOrTheCardGoes(t *testing.T) {
	m := cardKeys(filterCardOn(0, model.FilterRelationship), "down")
	m, _ = model.Update(m, model.FocusFieldMsg{Field: model.FilterOwner})
	if m.Dropdown.Open {
		t.Fatal("focus on another field left the dropdown open")
	}
	m = cardKeys(filterCardOn(0, model.FilterRelationship), "down", "ctrl+o")
	if m.Dropdown.Open || m.FilterForm.Open {
		t.Fatalf("applying left dropdown open %v, card open %v", m.Dropdown.Open, m.FilterForm.Open)
	}
}

func TestShiftedLettersTypeIntoTheSearchLine(t *testing.T) {
	m := cardKeys(filterCardOn(13, model.FilterProject), "down")
	m, _ = model.Update(m, model.KeyMsg{Chord: "shift+p", Text: "P"})
	if got := m.OpenDropdown(model.CardFilter, model.FilterProject); !got.Open || got.Query != "P" {
		t.Fatalf("a capital letter left the dropdown %+v, want it open on the query P", got)
	}
}

// The key line follows the keys the open dropdown takes: esc closes the choices, not the card.
func TestTheOpenDropdownHasItsOwnKeyLine(t *testing.T) {
	m := cardKeys(filterCardOn(3, model.FilterRelationship), "down")
	if got := m.CurrentMode(); got != model.ModeDropdown {
		t.Fatalf("mode %s with the dropdown open, want %s", got, model.ModeDropdown)
	}
	var esc string
	for b := range model.BindingsForMode(m.CurrentMode(), true) {
		if b.Chord == "escape" {
			esc = b.Help
		}
	}
	if esc != "close choices" {
		t.Errorf("esc reads %q in the key line, want close choices", esc)
	}
	m = cardKeys(m, "escape")
	if got := m.CurrentMode(); got != model.ModeFilterForm {
		t.Errorf("mode %s after esc, want %s", got, model.ModeFilterForm)
	}
}

// An open select keeps its highlight on the same choice when a live refresh reorders or grows the choices.
func TestOpenSelectKeepsItsHighlightedChoiceAcrossALiveRefresh(t *testing.T) {
	catalog := func(names ...string) []model.CatalogProject {
		var out []model.CatalogProject
		for _, n := range names {
			out = append(out, model.CatalogProject{Project: model.Project{UID: "uid-" + n, Name: n}})
		}
		return out
	}
	m := model.Model{Collection: model.Collection{View: "all-open"}}
	m.Snapshot.Catalog = catalog("bravo", "delta", "foxtrot")
	m = cardKeys(m, "f")
	m, _ = model.Update(m, model.FocusFieldMsg{Field: model.FilterProject})
	m = cardKeys(m, "down", "down", "down")
	m, cmds := model.Update(m, model.StreamEvent{Generation: m.StreamGeneration, ID: 99, Type: "project.created"})
	for _, cmd := range cmds {
		if read, ok := cmd.(model.ReadSnapshot); ok {
			m, _ = model.Update(m, model.SnapshotLoaded{Request: read.Request, Query: read.Query, Snapshot: model.Snapshot{Catalog: catalog("alpha", "bravo", "delta", "foxtrot")}, LocalDate: "2026-10-08", TimeZone: "UTC"})
		}
	}
	if !m.OpenDropdown(model.CardFilter, model.FilterProject).Open {
		t.Fatal("the refresh closed the dropdown")
	}
	m = cardKeys(m, "enter")
	if got := m.FilterForm.Values.Value(model.FilterProject); got != "uid-delta" {
		t.Errorf("enter picked %q, want the highlighted delta", got)
	}
}
