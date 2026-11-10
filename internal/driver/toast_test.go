package driver_test

import (
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

func toastText(m model.Model) string {
	if m.Toast == nil {
		return ""
	}
	return m.Toast.Text
}

// toastNodes are the toast nodes the Pane draws for s.
func toastNodes(t *testing.T, s driver.State) []tern.WireNode {
	t.Helper()
	var found []tern.WireNode
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if n.K == "toast" {
			found = append(found, n)
		}
	}
	return found
}

func TestANoticeWithAKeyIsOneToastWhoseButtonDoesWhatTheKeyDoes(t *testing.T) {
	closed := closedAt("t1", 10, 1)
	closed.ProjectID, closed.QualifiedID = 7, "work#t1"
	afterDelete := step(t, step(t, typed(t, deleting(t), "work#a"), key("enter")).State, deleted("a")).State
	for _, tc := range []struct {
		name               string
		s                  driver.State
		text, label, chord string
	}{
		{"delete", afterDelete, "Deleted work#a. Press u to restore it.", "Undo", "u"},
		{"w on a closed Issue", step(t, agentPanes(t, closed)["list"], key("w")).State, model.ClosedIssueRefusal, "Reopen", "r"},
		{"move on a closed card", step(t, boardPane(t, closedAt("d", 10, 5)), key(">")).State, "d is closed; press r to reopen it.", "Reopen", "r"},
		{"move past the last Lane", step(t, boardPane(t, boardCard("a", "ready-for-human")), key(">")).State, "a is in the last Lane; press x to close it.", "Close", "x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nodes := toastNodes(t, tc.s)
			if len(nodes) != 1 || tc.s.Model.Notice != "" {
				t.Fatalf("%d toasts, notice %q; want one toast and no Notice line", len(nodes), tc.s.Model.Notice)
			}
			p, act := nodes[0].P, view.ButtonClick(tc.chord)
			if p["text"] != tc.text || p["label"] != tc.label || p["action"] != act {
				t.Fatalf("toast text %q label %q action %q, want %q, %q, %q", p["text"], p["label"], p["action"], tc.text, tc.label, act)
			}
			pressed := step(t, tc.s, click(nodes[0].ID, act))
			keyed := step(t, tc.s, key(tc.chord))
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

func TestTheSameRefusalTwiceRaisesTwoToastsWithDifferentIds(t *testing.T) {
	s := boardPane(t, closedAt("d", 10, 5))
	first := step(t, s, key(">")).State
	second := step(t, first, key(">")).State
	a, b := toastNodes(t, first), toastNodes(t, second)
	if len(a) != 1 || len(b) != 1 {
		t.Fatalf("%d and %d toasts, want one each", len(a), len(b))
	}
	if a[0].ID == b[0].ID || a[0].P["text"] != b[0].P["text"] {
		t.Errorf("ids %q and %q for texts %q and %q, want a new id for the same text", a[0].ID, b[0].ID, a[0].P["text"], b[0].P["text"])
	}
}
