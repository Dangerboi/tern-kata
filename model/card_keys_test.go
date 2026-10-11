package model_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/model"
)

// cardStates is every popup card state that owns the keyboard, with whether ctrl+c quits from it.
func cardStates() []struct {
	name  string
	open  func() model.Model
	quits bool
} {
	asking := func(open func() model.Model, body func(model.Model) model.Model) func() model.Model {
		return func() model.Model {
			m, _ := clickPress(typed(body(open()), "x"), "escape")
			return m
		}
	}
	same := func(m model.Model) model.Model { return m }
	draft := draftCards()[1]
	return []struct {
		name  string
		open  func() model.Model
		quits bool
	}{
		{"Create", openedCreate, false},
		{"Create asking to discard", asking(openedCreate, same), false},
		{"Draft editor", draft.open, false},
		{"Draft editor asking to discard", asking(draft.open, same), false},
		{"Draft editor with a conflict", func() model.Model {
			m := draft.open()
			m, _ = model.Update(m, model.DraftConflict{UID: m.EditorUID, Issue: model.Issue{UID: m.EditorUID, Revision: 9}})
			return m
		}, false},
		{"single-field form", opened("+"), false},
		{"Filter form", opened("f"), false},
		{"Close", opened("x"), true},
		{"Menu", opened("p"), true},
		{"New Project", func() model.Model {
			m, _ := model.Update(clickAt(model.ColumnList), model.CollectionPointerSelected{Key: model.NewProjectKey})
			return m
		}, true},
	}
}

func TestInterruptPerCard(t *testing.T) {
	for _, c := range cardStates() {
		t.Run(c.name, func(t *testing.T) {
			m := c.open()
			if mode := m.CurrentMode(); mode == model.ModeList || mode == model.ModeDetail {
				t.Fatalf("card did not open: mode %s", mode)
			}
			if m, _ = clickPress(m, "ctrl+c"); m.Quitting != c.quits {
				t.Fatalf("ctrl+c: quitting=%v, want %v", m.Quitting, c.quits)
			}
		})
	}
}

// A card lists a global key (quit, help) only if pressing it does what it says.
func TestCardHintsListNoSwallowedGlobalKey(t *testing.T) {
	for _, c := range cardStates() {
		t.Run(c.name, func(t *testing.T) {
			mode := c.open().CurrentMode()
			listed := false
			for binding := range model.BindingsForMode(mode, false) {
				if binding.Mode != "" {
					continue
				}
				m, _ := clickPress(c.open(), binding.Chord)
				if !m.Quitting && !m.HelpOpen {
					t.Errorf("%s lists %s (%s), which the card swallows", mode, binding.Chord, binding.Help)
				}
				listed = listed || binding.Chord == "ctrl+c"
			}
			if listed != c.quits {
				t.Errorf("%s lists ctrl+c = %v, but it quits = %v", mode, listed, c.quits)
			}
		})
	}
}
