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
		"down":      {Column: model.ColumnDetail, By: model.ScrollLineDown},
		"up":        {Column: model.ColumnDetail, By: model.ScrollLineUp},
		"page_down": {Column: model.ColumnDetail, By: model.ScrollPageDown},
		"page_up":   {Column: model.ColumnDetail, By: model.ScrollPageUp},
	} {
		got, cmds := model.Update(detail, model.KeyMsg{Chord: chord})
		if !reflect.DeepEqual(cmds, []model.Cmd{want}) {
			t.Errorf("%s: commands %#v, want %#v", chord, cmds, want)
		}
		if got.ActivityTab != detail.ActivityTab || got.DetailIndex != detail.DetailIndex {
			t.Errorf("%s moved the row cursor to %v/%d", chord, got.ActivityTab, got.DetailIndex)
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

// Switching Activity tab, by key or click, asks the app to bring the new tab into view; keeping the tab changes nothing.
func TestSwitchingActivityTabRevealsIt(t *testing.T) {
	reveal := []model.Cmd{model.RevealActivityTab{}}
	detail := arrowModes(t)["detail"]
	for name, msg := range map[string]model.Msg{
		"tab":       model.KeyMsg{Chord: "tab"},
		"shift+tab": model.KeyMsg{Chord: "shift+tab"},
		"click":     model.ActivityTabMsg{Tab: (detail.ActivityTab + 1) % model.TabCount},
	} {
		if _, cmds := model.Update(detail, msg); !reflect.DeepEqual(cmds, reveal) {
			t.Errorf("%s: commands %#v, want %#v", name, cmds, reveal)
		}
	}
	if _, cmds := model.Update(detail, model.ActivityTabMsg{Tab: detail.ActivityTab}); len(cmds) != 0 {
		t.Errorf("re-clicking the active tab: %#v", cmds)
	}
}

// Tern ends the program on a reveal aimed at a node it does not draw: the detail folded away, or not yet loaded.
func TestActivityTabRevealIsSentOnlyWhileTheTabsAreDrawn(t *testing.T) {
	folded := arrowModes(t)["detail"]
	folded.Cols = 60
	unloaded := arrowModes(t)["detail"]
	unloaded.Detail = nil
	for name, m := range map[string]model.Model{"folded": folded, "unloaded": unloaded} {
		for _, msg := range []model.Msg{model.KeyMsg{Chord: "tab"}, model.KeyMsg{Chord: "shift+tab"}, model.ActivityTabMsg{Tab: model.TabComments}} {
			if _, cmds := model.Update(m, msg); len(cmds) != 0 {
				t.Errorf("%s, %T: %#v", name, msg, cmds)
			}
		}
	}
}
