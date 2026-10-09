package model_test

import (
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/model"
)

// With the detail focused, the arrows scroll its viewport by a line and the page keys by a page, as in
// kata's TUI; the row cursor stays where it is (j/k move it).
func TestArrowAndPageKeysScrollTheDetailViewport(t *testing.T) {
	detail := arrowModes(t)["detail"]
	for chord, want := range map[string]model.Scroll{
		"down":      {Column: model.ColumnDetail, Down: true, Line: true},
		"up":        {Column: model.ColumnDetail, Line: true},
		"page_down": {Column: model.ColumnDetail, Down: true},
		"page_up":   {Column: model.ColumnDetail},
	} {
		got, cmds := model.Update(detail, model.KeyMsg{Chord: chord})
		if !reflect.DeepEqual(cmds, []model.Cmd{want}) {
			t.Errorf("%s: commands %#v, want %#v", chord, cmds, want)
		}
		if got.DetailSection != detail.DetailSection || got.DetailIndex != detail.DetailIndex {
			t.Errorf("%s moved the row cursor to %v/%d", chord, got.DetailSection, got.DetailIndex)
		}
	}
}

// Tern answers a scroll aimed at a node it does not draw with an error that ends the Pane's program.
func TestDetailScrollsAreSentOnlyWhileTheDetailIsOnScreen(t *testing.T) {
	detail := arrowModes(t)["detail"]
	detail.Cols = 60
	if detail.Shows(model.ColumnDetail) {
		t.Fatal("fixture: the detail is on screen")
	}
	for _, chord := range []string{"down", "up", "page_down", "page_up"} {
		if _, cmds := model.Update(detail, model.KeyMsg{Chord: chord}); len(cmds) != 0 {
			t.Errorf("%s with the detail off screen: %#v", chord, cmds)
		}
	}
}

func TestLetterKeysStillMoveTheDetailCursor(t *testing.T) {
	detail := arrowModes(t)["detail"]
	got, cmds := model.Update(detail, model.KeyMsg{Chord: "j"})
	if got.DetailIndex != detail.DetailIndex+1 || len(cmds) != 0 {
		t.Fatalf("j: index %d, commands %#v", got.DetailIndex, cmds)
	}
}
