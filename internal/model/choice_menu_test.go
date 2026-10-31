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
	menu := m.OpenMenu(model.CardFilter, model.FilterRelationship)
	if !menu.Open || menu.Active != 0 {
		t.Fatalf("down on a closed select left menu %+v, want it open on the held choice", menu)
	}
	m = cardKeys(m, "down", "down")
	if got := m.OpenMenu(model.CardFilter, model.FilterRelationship).Active; got != 2 {
		t.Fatalf("two downs highlight %d, want 2", got)
	}
	m = cardKeys(m, "up", "up", "up")
	if got := m.OpenMenu(model.CardFilter, model.FilterRelationship).Active; got != len(model.FilterRelationships)-1 {
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
	if m.OpenMenu(model.CardFilter, model.FilterRelationship).Open || !m.FilterForm.Open || m.FilterForm.Field != model.FilterRelationship {
		t.Fatalf("after a pick: menu open %v, card open %v, field %q", m.Menu.Open, m.FilterForm.Open, m.FilterForm.Field)
	}
	m = cardKeys(m, "down", "escape")
	if m.OpenMenu(model.CardFilter, model.FilterRelationship).Open || !m.FilterForm.Open || m.FilterForm.Values.Relationship != "child" {
		t.Fatalf("escape closes the menu only: menu open %v, card open %v, value %q", m.Menu.Open, m.FilterForm.Open, m.FilterForm.Values.Relationship)
	}
	m = cardKeys(m, "escape")
	if m.FilterForm.Open {
		t.Fatal("escape with the menu closed did not cancel the card")
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
	if got := m.FilterForm.Values.Relationship; got != "parent" || m.Menu.Open {
		t.Fatalf("left in an open menu set %q (menu open %v), want parent and the menu closed", got, m.Menu.Open)
	}
	m = cardKeys(m, "down", "tab")
	if m.Menu.Open || m.FilterForm.Field == model.FilterRelationship {
		t.Fatalf("tab left menu open %v on field %q", m.Menu.Open, m.FilterForm.Field)
	}
}

func TestDownOnATabsFieldOpensNothing(t *testing.T) {
	m := cardKeys(filterCardOn(0, model.FilterStatus), "down")
	if m.Menu.Open {
		t.Fatal("down opened a menu on the Status tabs")
	}
}

func TestSelectPastADozenOptionsSearchesAndEnterPicksTheFirstHit(t *testing.T) {
	m := filterCardOn(13, model.FilterProject) // the empty choice and 13 Projects
	set, ok := m.CardChoices(model.CardFilter, model.FilterProject)
	if !ok || !set.Search() {
		t.Fatalf("project choices %+v (ok %v), want a searching select", set, ok)
	}
	m = cardKeys(m, "down", "p", "1", "1")
	menu := m.OpenMenu(model.CardFilter, model.FilterProject)
	if menu.Query != "p11" || set.Matches(menu.Query)[0] != menu.Active {
		t.Fatalf("typing p11 left menu %+v", menu)
	}
	m = cardKeys(m, "backspace")
	if got := m.OpenMenu(model.CardFilter, model.FilterProject).Query; got != "p1" {
		t.Fatalf("backspace left query %q, want p1", got)
	}
	m = cardKeys(m, "1", "enter")
	if got := m.FilterForm.Values.ProjectUID; got != "p11" {
		t.Fatalf("enter picked %q, want p11", got)
	}
	if m.Menu.Open || m.Menu.Query != "" {
		t.Fatalf("a pick left menu %+v", m.Menu)
	}
	m = cardKeys(m, "down", "z", "z", "escape")
	if m.Menu.Open || m.Menu.Query != "" || !m.FilterForm.Open {
		t.Fatalf("escape left menu %+v, card open %v", m.Menu, m.FilterForm.Open)
	}
}

func TestSelectWithAFewOptionsTypesNothing(t *testing.T) {
	m := cardKeys(filterCardOn(0, model.FilterRelationship), "down", "x")
	if m.Menu.Query != "" {
		t.Fatalf("a select without search took the text %q", m.Menu.Query)
	}
}

func TestPointerPickAndToggleDriveTheSameState(t *testing.T) {
	m := filterCardOn(0, model.FilterStatus)
	m, _ = model.Update(m, model.MenuToggleMsg{Field: model.FilterRelationship, Open: true})
	if menu := m.OpenMenu(model.CardFilter, model.FilterRelationship); !menu.Open || m.FilterForm.Field != model.FilterRelationship {
		t.Fatalf("the pointer's open left menu %+v on field %q", menu, m.FilterForm.Field)
	}
	m, _ = model.Update(m, model.ChooseMsg{Field: model.FilterRelationship, Index: 3})
	if got := m.FilterForm.Values.Relationship; got != "blocks" || m.Menu.Open {
		t.Fatalf("a pick set %q with the menu open %v", got, m.Menu.Open)
	}
	m, _ = model.Update(m, model.MenuToggleMsg{Field: model.FilterStatus, Open: true})
	if m.Menu.Open {
		t.Fatal("the pointer opened a menu on a tabs field")
	}
}

func TestMenuClosesWhenFocusMovesOrTheCardGoes(t *testing.T) {
	m := cardKeys(filterCardOn(0, model.FilterRelationship), "down")
	m, _ = model.Update(m, model.FocusFieldMsg{Field: model.FilterOwner})
	if m.Menu.Open {
		t.Fatal("focus on another field left the menu open")
	}
	m = cardKeys(filterCardOn(0, model.FilterRelationship), "down", "ctrl+o")
	if m.Menu.Open || m.FilterForm.Open {
		t.Fatalf("applying left menu open %v, card open %v", m.Menu.Open, m.FilterForm.Open)
	}
}
