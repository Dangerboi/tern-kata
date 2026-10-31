package driver_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

// A pick on an unselected Issue's menu selects that Issue, then does what the row's key does on it.
func TestContextMenuPickSelectsTheRowThenPressesItsKey(t *testing.T) {
	first, second := boardCard("one", "needs-triage"), boardCard("two", "needs-triage")
	first, second = updatedAt(first, 9), updatedAt(second, 8)
	cases := []struct {
		name  string
		state func(t *testing.T) driver.State
		node  func(t *testing.T, s driver.State) string
		mode  model.Mode
	}{
		{"Issue row", func(t *testing.T) driver.State { return projectPane(t, 160, first, second) },
			func(t *testing.T, s driver.State) string { return view.IssueID("two") }, model.ModeList},
		{"Lane row", func(t *testing.T) driver.State {
			s := boardPane(t, first, second)
			return feed(t, s, key("enter")).State
		}, func(t *testing.T, s driver.State) string { return view.IssueID("two") }, model.ModeLane},
		{"Board card", func(t *testing.T) driver.State { return boardPane(t, first, second) },
			func(t *testing.T, s driver.State) string { return issueRow(t, s.Model, view.BoardID, second) }, model.ModeBoard},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := tc.state(t)
			if s.Model.SelectedUID != "one" {
				t.Fatalf("selected %q before the pick, want one", s.Model.SelectedUID)
			}
			out := step(t, s, click(tc.node(t, s), view.ContextMenuPickClick("two", tc.mode, "x")))
			if got := out.State.Model.SelectedUID; got != "two" {
				t.Errorf("selected %q after the pick, want two", got)
			}
			if form := out.State.Model.CloseForm; !form.Open || form.UID != "two" {
				t.Errorf("Close card = %+v, want open on two", form)
			}
		})
	}
}

// A pick runs what its key does in the mode the menu was built for, not in the mode the Pane is in when it arrives: with the search
// line open the Close card opens and the search text stays as typed.
func TestContextMenuPickIgnoresTheModeThePaneHasMovedTo(t *testing.T) {
	first, second := boardCard("one", "needs-triage"), boardCard("two", "needs-triage")
	first, second = updatedAt(first, 9), updatedAt(second, 8)
	for _, tc := range []struct {
		name  string
		state func(t *testing.T) driver.State
		mode  model.Mode
	}{
		{"Issue list", func(t *testing.T) driver.State { return projectPane(t, 160, first, second) }, model.ModeList},
		{"Board", func(t *testing.T) driver.State { return boardPane(t, first, second) }, model.ModeBoard},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := feed(t, tc.state(t), key("/"), key("o")).State
			if got := s.Model.CurrentMode(); got != model.ModeSearch {
				t.Fatalf("mode %q before the pick, want search", got)
			}
			out := step(t, s, click("", view.ContextMenuPickClick("two", tc.mode, "x")))
			got := out.State.Model
			if !got.CloseForm.Open || got.CloseForm.UID != "two" {
				t.Errorf("Close card = %+v, want open on two", got.CloseForm)
			}
			if got.Filters.Search != s.Model.Filters.Search {
				t.Errorf("search text %q after the pick, want %q", got.Filters.Search, s.Model.Filters.Search)
			}
		})
	}
}

// The menu's ids carry the mode and the chord they run.
func TestContextMenuIdsDecodeToTheirModeAndChord(t *testing.T) {
	got := view.DecodeAction("any", "pick", "list:two:shift+m")
	if want := (model.ContextMenuPickMsg{UID: "two", Mode: model.ModeList, Chord: "shift+m"}); got != want {
		t.Errorf("decoded %+v, want %+v", got, want)
	}
	if got := view.DecodeAction("any", "pick", "no-chord"); got != nil {
		t.Errorf("a pick without a chord decoded to %+v", got)
	}
}
