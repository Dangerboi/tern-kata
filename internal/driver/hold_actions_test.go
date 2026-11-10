package driver_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// entryActs is the act of the entry labelled label in the actions menu and hold lists of node id, as the Pane draws it for m.
func entryActs(t *testing.T, m model.Model, id, label string) (menu, hold string) {
	t.Helper()
	ops, err := tern.NewReconciler("test").Diff(view.Render(m, now))
	if err != nil {
		t.Fatal(err)
	}
	var find func(n tern.WireNode) bool
	find = func(n tern.WireNode) bool {
		if n.ID == id {
			actions, _ := n.P["actions"].(map[string]any)
			pick := func(list string) string {
				items, _ := actions[list].([]any)
				for _, item := range items {
					if fields := item.(map[string]any); fields["label"] == label {
						return fields["id"].(string)
					}
				}
				return ""
			}
			menu, hold = pick("menu"), pick("hold")
			return true
		}
		for _, child := range n.C {
			if find(child) {
				return true
			}
		}
		return false
	}
	for _, op := range ops {
		if op[0] == "add" && find(op[4].(tern.WireNode)) {
			return menu, hold
		}
	}
	t.Fatalf("no node %s", id)
	return "", ""
}

// A long-press pick reaches the Pane as the same action event a right-click pick does, so it runs what the menu entry runs.
func TestHoldPickRunsWhatTheSameMenuPickRuns(t *testing.T) {
	first, second := boardCard("one", "needs-triage"), boardCard("two", "needs-triage")
	first, second = updatedAt(first, 9), updatedAt(second, 8)
	cases := []struct {
		name  string
		state func(t *testing.T) driver.State
		node  func(t *testing.T, s driver.State) string
	}{
		{"Issue row", func(t *testing.T) driver.State { return projectPane(t, 160, first, second) },
			func(t *testing.T, s driver.State) string { return view.IssueID("two") }},
		{"Board card", func(t *testing.T) driver.State { return boardPane(t, first, second) },
			func(t *testing.T, s driver.State) string { return issueRow(t, s.Model, view.BoardID, second) }},
	}
	for _, tc := range cases {
		for _, label := range []string{"Close", "Priority…"} {
			t.Run(tc.name+" "+label, func(t *testing.T) {
				s := tc.state(t)
				id := tc.node(t, s)
				menuAct, holdAct := entryActs(t, s.Model, id, label)
				if holdAct == "" || holdAct != menuAct {
					t.Fatalf("hold act %q, menu act %q, want the same non-empty act", holdAct, menuAct)
				}
				viaMenu := step(t, s, click(id, menuAct)).State.Model
				viaHold := step(t, s, click(id, holdAct)).State.Model
				if viaHold.SelectedUID != "two" {
					t.Errorf("selected %q after the hold pick, want two", viaHold.SelectedUID)
				}
				if viaHold.TopCard() == model.NoCard || viaHold.TopCard() != viaMenu.TopCard() || viaHold.CurrentMode() != viaMenu.CurrentMode() {
					t.Errorf("hold pick left card %v mode %v, menu pick card %v mode %v", viaHold.TopCard(), viaHold.CurrentMode(), viaMenu.TopCard(), viaMenu.CurrentMode())
				}
			})
		}
	}
}
