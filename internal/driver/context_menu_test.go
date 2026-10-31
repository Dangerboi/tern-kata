package driver_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// menuPick is the Action input Tern sends when a context menu row with id is chosen on node nodeID: the row's id is the act, split at its first `=`.
func menuPick(nodeID, rowID string) *tern.Action { return click(nodeID, rowID) }

// A pick on an unselected Issue's menu selects that Issue, then does what the row's key does on it.
func TestContextMenuPickSelectsTheRowThenPressesItsKey(t *testing.T) {
	first, second := boardCard("one", "needs-triage"), boardCard("two", "needs-triage")
	first, second = updatedAt(first, 9), updatedAt(second, 8)
	cases := []struct {
		name  string
		state func(t *testing.T) driver.State
		node  func(t *testing.T, s driver.State) string
		chord string
	}{
		{"Issue row", func(t *testing.T) driver.State { return projectPane(t, 160, first, second) },
			func(t *testing.T, s driver.State) string { return view.IssueID("two") }, "x"},
		{"Lane row", func(t *testing.T) driver.State {
			s := boardPane(t, first, second)
			return feed(t, s, key("enter")).State
		}, func(t *testing.T, s driver.State) string { return view.IssueID("two") }, "x"},
		{"Board card", func(t *testing.T) driver.State { return boardPane(t, first, second) },
			func(t *testing.T, s driver.State) string { return issueRow(t, s.Model, view.BoardID, second) }, "x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := tc.state(t)
			if s.Model.SelectedUID != "one" {
				t.Fatalf("selected %q before the pick, want one", s.Model.SelectedUID)
			}
			out := step(t, s, menuPick(tc.node(t, s), view.MenuPickClick("two", tc.chord)))
			if got := out.State.Model.SelectedUID; got != "two" {
				t.Errorf("selected %q after the pick, want two", got)
			}
			if form := out.State.Model.CloseForm; !form.Open || form.UID != "two" {
				t.Errorf("Close card = %+v, want open on two", form)
			}
		})
	}
}

// The menu's ids are the key presses of the bound chords.
func TestContextMenuIdsDecodeToTheirChords(t *testing.T) {
	got := view.DecodeAction("any", "pick", "two:shift+m")
	if want := (model.MenuPickMsg{UID: "two", Chord: "shift+m"}); got != want {
		t.Errorf("decoded %+v, want %+v", got, want)
	}
	if got := view.DecodeAction("any", "pick", "no-chord"); got != nil {
		t.Errorf("a pick without a chord decoded to %+v", got)
	}
}
