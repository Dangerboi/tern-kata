package driver_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// laneViewHeader is the Lane view's header node, failing the test when none is drawn.
func laneViewHeader(t *testing.T, nodes []tern.WireNode) tern.WireNode {
	t.Helper()
	return node(t, nodes, view.LaneHeaderID)
}

func TestLaneViewOfACustomOrderedLaneSaysSoAndOffersResetOrder(t *testing.T) {
	s := laneBoard(t, 160)
	loaded := model.LanesLoaded{ProjectUID: "p", Request: s.Model.LanesRequest(), Order: model.LaneOrders{"needs-info": {"i1"}}}
	s = step(t, s, loaded).State
	s = feed(t, s, key("right")).State // i1, in Needs info
	s = step(t, s, key("enter")).State
	head := laneViewHeader(t, wireNodes(t, view.Render(s.Model, now)))
	if text := spansJoined(head); !strings.Contains(text, "custom order") {
		t.Errorf("Lane header = %s, want custom order", text)
	}
	actions, _ := head.P["actions"].(map[string]any)
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
		if got, want := view.DecodeAction(view.LaneHeaderID, act, value), (model.ResetLaneOrder{Lane: "needs-info"}); got != want {
			t.Errorf("%s row decodes to %#v, want %#v", kind, got, want)
		}
	}
}

func TestLaneViewOfALaneWithoutAnOrderShowsNoOrderNorMenu(t *testing.T) {
	s := laneBoard(t, 160)
	loaded := model.LanesLoaded{ProjectUID: "p", Request: s.Model.LanesRequest(), Order: model.LaneOrders{"needs-info": {"i1"}}}
	s = step(t, s, loaded).State
	s = step(t, s, key("enter")).State // t1, in Needs triage
	head := laneViewHeader(t, wireNodes(t, view.Render(s.Model, now)))
	actions, _ := head.P["actions"].(map[string]any)
	if text := spansJoined(head); !strings.Contains(text, "Needs triage") || strings.Contains(text, "custom order") || actions["menu"] != nil || actions["hold"] != nil {
		t.Errorf("Lane header = %s %v, want Needs triage with no custom order or menu", text, actions)
	}
}
