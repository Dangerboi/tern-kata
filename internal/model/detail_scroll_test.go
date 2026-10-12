package model_test

import (
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
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
		if got.ActiveTab() != detail.ActiveTab() || got.DetailIndex() != detail.DetailIndex() {
			t.Errorf("%s moved the row cursor to %v/%d", chord, got.ActiveTab(), got.DetailIndex())
		}
	}
}

// Whether a column is drawn is the Pane driver's call: the model asks for the scroll whatever the width.
func TestDetailScrollIntentsDoNotDependOnTheWidth(t *testing.T) {
	detail := arrowModes(t)["detail"]
	detail.Cols = 60
	if detail.Shows(model.ColumnDetail) {
		t.Fatal("fixture: the detail is on screen")
	}
	for chord, want := range map[string]model.ScrollBy{"down": model.ScrollLineDown, "up": model.ScrollLineUp, "page_down": model.ScrollPageDown, "page_up": model.ScrollPageUp} {
		_, cmds := model.Update(detail, model.KeyMsg{Chord: chord})
		if wantCmds := []model.Cmd{model.Scroll{Column: model.ColumnDetail, By: want}}; !reflect.DeepEqual(cmds, wantCmds) {
			t.Errorf("%s with the detail off screen: %#v, want %#v", chord, cmds, wantCmds)
		}
	}
}

func TestLetterKeysStillMoveTheDetailCursor(t *testing.T) {
	detail := arrowModes(t)["detail"]
	got, cmds := model.Update(detail, model.KeyMsg{Chord: "j"})
	if got.DetailIndex() != detail.DetailIndex()+1 || len(cmds) != 0 {
		t.Fatalf("j: index %d, commands %#v", got.DetailIndex(), cmds)
	}
}

// Switching Activity tab, by key or click, asks the app to bring the new tab into view; keeping the tab changes nothing.
func TestSwitchingActivityTabRevealsIt(t *testing.T) {
	reveal := []model.Cmd{model.RevealActivityTab{}}
	detail := arrowModes(t)["detail"]
	for name, msg := range map[string]model.Msg{
		"tab":       model.KeyMsg{Chord: "tab"},
		"shift+tab": model.KeyMsg{Chord: "shift+tab"},
		"click":     model.ActivityTabMsg{Tab: (detail.ActiveTab() + 1) % model.TabCount},
	} {
		if _, cmds := model.Update(detail, msg); !reflect.DeepEqual(cmds, reveal) {
			t.Errorf("%s: commands %#v, want %#v", name, cmds, reveal)
		}
	}
	if _, cmds := model.Update(detail, model.ActivityTabMsg{Tab: detail.ActiveTab()}); len(cmds) != 0 {
		t.Errorf("re-clicking the active tab: %#v", cmds)
	}
}

// Whether the tabs are drawn is the Pane driver's call: the model asks for the reveal even for a folded or unloaded detail.
func TestActivityTabRevealIntentDoesNotDependOnTheTabsBeingDrawn(t *testing.T) {
	folded := arrowModes(t)["detail"]
	folded.Cols = 60
	unloaded := withDetail(arrowModes(t)["detail"], nil)
	reveal := []model.Cmd{model.RevealActivityTab{}}
	for name, m := range map[string]model.Model{"folded": folded, "unloaded": unloaded} {
		for _, msg := range []model.Msg{model.KeyMsg{Chord: "tab"}, model.KeyMsg{Chord: "shift+tab"}, model.ActivityTabMsg{Tab: model.TabComments}} {
			if _, cmds := model.Update(m, msg); !reflect.DeepEqual(cmds, reveal) {
				t.Errorf("%s, %T: %#v, want %#v", name, msg, cmds, reveal)
			}
		}
	}
}

// The help sheet only hides the columns: Tern keeps them mounted and accepts a scroll or reveal on a hidden node,
// so a change under the sheet keeps its commands and the offsets are right when the sheet closes.
func TestDetailCommandsSurviveWhileHelpIsOpen(t *testing.T) {
	detail := arrowModes(t)["detail"]
	detail.HelpOpen = true
	_, cmds := model.Update(detail, model.ActivityTabMsg{Tab: (detail.ActiveTab() + 1) % model.TabCount})
	if want := []model.Cmd{model.RevealActivityTab{}}; !reflect.DeepEqual(cmds, want) {
		t.Fatalf("commands %#v, want %#v", cmds, want)
	}
}
