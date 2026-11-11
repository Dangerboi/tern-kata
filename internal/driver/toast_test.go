package driver_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// toastText is the text of the one toast the step drew, "" when it drew none.
func toastText(t *testing.T, out driver.Output) string {
	t.Helper()
	nodes := toastNodes(t, *out.View)
	if len(nodes) == 0 {
		return ""
	}
	text, _ := nodes[0].P["text"].(string)
	return text
}

// withoutToastWait is cmds less the timer that takes a toast away.
func withoutToastWait(cmds []model.Cmd) []model.Cmd {
	return slices.DeleteFunc(slices.Clone(cmds), func(c model.Cmd) bool { _, ok := c.(model.WaitToast); return ok })
}

// theToastWait is the one WaitToast among cmds.
func theToastWait(t *testing.T, cmds []model.Cmd) model.WaitToast {
	t.Helper()
	for _, c := range cmds {
		if w, ok := c.(model.WaitToast); ok {
			return w
		}
	}
	t.Fatalf("cmds %#v hold no WaitToast", cmds)
	return model.WaitToast{}
}

// toastNodes are the toast nodes v draws.
func toastNodes(t *testing.T, v tern.View) []tern.WireNode {
	t.Helper()
	var found []tern.WireNode
	for _, n := range wireNodes(t, v) {
		if n.K == "toast" {
			found = append(found, n)
		}
	}
	return found
}

func TestANoticeWithAKeyIsOneToastWhoseButtonDoesWhatTheKeyDoes(t *testing.T) {
	closed := closedAt("t1", 10, 1)
	closed.ProjectID, closed.QualifiedID = 7, "work#t1"
	afterDelete := step(t, step(t, typed(t, deleting(t), "work#a"), key("enter")).State, deleted("a"))
	for _, tc := range []struct {
		name               string
		raised             driver.Output
		text, label, chord string
	}{
		{"delete", afterDelete, "Deleted work#a. Press u to restore it.", "Undo", "u"},
		{"w on a closed Issue", step(t, agentPanes(t, closed)["list"], key("w")), model.ClosedIssueRefusal, "Reopen", "r"},
		{"move on a closed card", step(t, boardPane(t, closedAt("d", 10, 5)), key(">")), "d is closed; press r to reopen it.", "Reopen", "r"},
		{"move past the last Lane", step(t, boardPane(t, boardCard("a", "ready-for-human")), key(">")), "a is in the last Lane; press x to close it.", "Close", "x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nodes := toastNodes(t, *tc.raised.View)
			if len(nodes) != 1 || tc.raised.State.Model.Notice != "" {
				t.Fatalf("%d toasts, notice %q; want one toast and no Notice line", len(nodes), tc.raised.State.Model.Notice)
			}
			p, act := nodes[0].P, view.ToastClick(tc.raised.State.Model.Toast.Seq)
			if p["text"] != tc.text || p["label"] != tc.label || p["action"] != act {
				t.Fatalf("toast text %q label %q action %q, want %q, %q, %q", p["text"], p["label"], p["action"], tc.text, tc.label, act)
			}
			pressed := step(t, tc.raised.State, click(nodes[0].ID, act))
			keyed := step(t, tc.raised.State, key(tc.chord))
			if !reflect.DeepEqual(pressed.Cmds, keyed.Cmds) || pressed.State.Model.CurrentMode() != keyed.State.Model.CurrentMode() || pressed.State.Model.TopCard() != keyed.State.Model.TopCard() {
				t.Errorf("the button gave cmds %#v mode %s card %v; the key gave cmds %#v mode %s card %v",
					pressed.Cmds, pressed.State.Model.CurrentMode(), pressed.State.Model.TopCard(), keyed.Cmds, keyed.State.Model.CurrentMode(), keyed.State.Model.TopCard())
			}
			if len(pressed.Cmds) == 0 && pressed.State.Model.TopCard() == model.NoCard {
				t.Errorf("%s did nothing", tc.label)
			}
		})
	}
}

// A toast's button acts on the Issue the toast names, in the mode it was raised in: the selection or mode the Pane has moved to by
// the tap does not change what it does.
func TestAToastButtonIgnoresTheSelectionAndModeThePaneHasMovedTo(t *testing.T) {
	raised := step(t, boardPane(t, closedAt("d", 10, 5), closedAt("e", 10, 4)), key(">"))
	if raised.State.Model.SelectedUID != "d" {
		t.Fatalf("selected %q when the toast was raised, want d", raised.State.Model.SelectedUID)
	}
	act := view.ToastClick(raised.State.Model.Toast.Seq)
	for _, tc := range []struct {
		name  string
		moved driver.State
	}{
		{"another card selected", feed(t, raised.State, key("j")).State},
		{"search line open", feed(t, raised.State, key("/")).State},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if m := tc.moved.Model; m.SelectedUID == "d" && m.CurrentMode() == model.ModeBoard {
				t.Fatalf("selection d and mode %s after the move, want either changed", m.CurrentMode())
			}
			pressed := step(t, tc.moved, click("", act))
			var reopened []string
			for _, c := range pressed.Cmds {
				if r, ok := c.(model.ReopenIssueCmd); ok {
					reopened = append(reopened, r.UID)
				}
			}
			if !slices.Equal(reopened, []string{"d"}) || pressed.State.Model.Filters.Search != tc.moved.Model.Filters.Search {
				t.Errorf("Reopen reopened %v and left search %q, want d reopened and search %q", reopened,
					pressed.State.Model.Filters.Search, tc.moved.Model.Filters.Search)
			}
		})
	}
	gone := step(t, raised.State, model.ToastExpired(theToastWait(t, raised.Cmds))).State
	if late := step(t, gone, click("", act)); len(withoutToastWait(late.Cmds)) != 0 {
		t.Errorf("a tap after the toast expired sent %#v", late.Cmds)
	}
}

func TestAToastStaysDrawnUntilItsOwnExpiry(t *testing.T) {
	first := step(t, boardPane(t, closedAt("d", 10, 5)), key(">"))
	raised := theToastWait(t, first.Cmds)
	nodes := toastNodes(t, *first.View)
	if len(nodes) != 1 {
		t.Fatal("the step that raised the toast did not draw it")
	}
	later := step(t, first.State, resize(150))
	if got := toastNodes(t, *later.View); len(got) != 1 || got[0].ID != nodes[0].ID {
		t.Errorf("a later step draws toasts %v, want the same toast still up", got)
	}
	expired := step(t, later.State, model.ToastExpired(raised))
	if got := toastNodes(t, *expired.View); len(got) != 0 {
		t.Errorf("after its expiry %d toasts are drawn", len(got))
	}
}

func TestAnOlderToastsExpiryLeavesANewerToastDrawn(t *testing.T) {
	first := step(t, boardPane(t, closedAt("d", 10, 5)), key(">"))
	second := step(t, first.State, key(">"))
	old := theToastWait(t, first.Cmds)
	after := step(t, second.State, model.ToastExpired(old))
	if got := toastNodes(t, *after.View); len(got) != 1 {
		t.Errorf("the older expiry left %d toasts, want the newer one", len(got))
	}
}

func TestTheSameRefusalTwiceRaisesTwoToastsWithDifferentIds(t *testing.T) {
	first := step(t, boardPane(t, closedAt("d", 10, 5)), key(">"))
	second := step(t, first.State, key(">"))
	a, b := toastNodes(t, *first.View), toastNodes(t, *second.View)
	if len(a) != 1 || len(b) != 1 {
		t.Fatalf("%d and %d toasts, want one each", len(a), len(b))
	}
	if a[0].ID == b[0].ID || a[0].P["text"] != b[0].P["text"] {
		t.Errorf("ids %q and %q for texts %q and %q, want a new id for the same text", a[0].ID, b[0].ID, a[0].P["text"], b[0].P["text"])
	}
}
