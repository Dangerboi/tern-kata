package view_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

func pressed(m model.Model, chords ...string) model.Model {
	for _, chord := range chords {
		m, _ = model.Update(m, model.KeyMsg{Chord: chord})
	}
	return m
}

func TestFilterFieldsDrawAClearButton(t *testing.T) {
	list := pickerLoaded(true)
	for _, tt := range []struct {
		name string
		m    model.Model
		id   string
	}{
		{"search line", pressed(list, "/", "a"), view.SearchID},
		{"search line showing results", pressed(list, "/", "a", "down"), view.SearchID},
		{"page search line", pressed(list, "2", "/", "a"), view.SearchID},
		{"Menu filter", menuOpened(list), view.LayerID(model.CardPicker) + ".input"},
		{"Move to Project filter", pressed(list, "shift+m"), view.LayerID(model.CardMove) + ".input"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			node, ok := pickerNodes(t, tt.m)[tt.id]
			if !ok {
				t.Fatalf("node %s is not drawn in mode %s", tt.id, tt.m.CurrentMode())
			}
			if node.P["clear"] != true {
				t.Errorf("props %v, want clear true", node.P)
			}
		})
	}
	if m := pressed(list, "/", "a", "down"); !m.Search.Results {
		t.Fatal("down in the search did not show results, so the read-only case is untested")
	}
}

func TestClearActionDecodesToTheClearInputChord(t *testing.T) {
	want := model.ButtonPressMsg{Chord: "ctrl+u"}
	if got := view.DecodeAction(view.SearchID, "clear", ""); got != want {
		t.Errorf("clear decodes to %#v, want %#v", got, want)
	}
}
