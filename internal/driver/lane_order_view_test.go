package driver_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

// orderedPane is dragPane with a second card d in Needs triage and a custom order saved for Needs triage only.
func orderedPane(t *testing.T) (s struct{ Model model.Model }) {
	t.Helper()
	pane := boardPane(t, boardCard("a", "needs-triage"), boardCard("d", "needs-triage"), boardCard("b", "needs-info"), boardCard("c"))
	loaded := model.LanesLoaded{ProjectUID: "p", Request: pane.Model.LanesRequest(), Order: model.LaneOrders{"needs-triage": {"d", "a"}}}
	s.Model = step(t, pane, loaded).State.Model
	return s
}

func TestOnlyACustomOrderedLanesCardsAreDropTargets(t *testing.T) {
	s := orderedPane(t)
	nodes := wireNodes(t, view.Render(s.Model, now))
	for i, column := range s.Model.BoardColumns() {
		for _, card := range column.Cards {
			got, _ := node(t, nodes, laneID(i)+".cards."+card.Issue.UID).P["drop"].(string)
			want := ""
			if column.Lane == "needs-triage" {
				want = view.CardDropAct
			}
			if got != want {
				t.Errorf("card %s in %q: drop = %q, want %q", card.Issue.UID, column.Lane, got, want)
			}
		}
	}
}

func TestACustomOrderedLaneSaysSoAndOffersResetOrder(t *testing.T) {
	s := orderedPane(t)
	nodes := wireNodes(t, view.Render(s.Model, now))
	for i, column := range s.Model.BoardColumns() {
		head := laneID(i) + ".head"
		text := spansJoined(node(t, nodes, head))
		actions, _ := node(t, nodes, head).P["actions"].(map[string]any)
		if column.Lane != "needs-triage" {
			if strings.Contains(text, "custom order") || actions["menu"] != nil || actions["hold"] != nil {
				t.Errorf("head of %q shows a custom order or menu: %s %v", column.Lane, text, actions)
			}
			continue
		}
		if !strings.Contains(text, "custom order") {
			t.Errorf("head of %q = %s, want custom order", column.Lane, text)
		}
		for _, kind := range []string{"menu", "hold"} {
			raw, _ := json.Marshal(actions[kind])
			var items []struct{ ID, Label string }
			if err := json.Unmarshal(raw, &items); err != nil || len(items) != 1 {
				t.Fatalf("%s = %s, want one row", kind, raw)
			}
			if items[0].Label != "Reset order" {
				t.Errorf("%s label = %q, want Reset order", kind, items[0].Label)
			}
			act, value, _ := strings.Cut(items[0].ID, "=")
			if got, want := view.DecodeAction(head, act, value), (model.ResetLaneOrder{Lane: "needs-triage"}); got != want {
				t.Errorf("%s row decodes to %#v, want %#v", kind, got, want)
			}
		}
	}
}

func TestResetOrderDecodesTheNoLaneKey(t *testing.T) {
	act, value, _ := strings.Cut(view.ResetLaneOrderClick(""), "=")
	if got, want := view.DecodeAction("x", act, value), (model.ResetLaneOrder{}); got != want {
		t.Errorf("decoded %#v, want %#v", got, want)
	}
}
