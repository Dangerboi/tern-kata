package driver_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// dragPane is a Board with a in Needs triage, b in Needs info and c in No lane, with a selected.
func dragPane(t *testing.T) driver.State {
	t.Helper()
	s := boardPane(t, boardCard("a", "needs-triage"), boardCard("b", "needs-info"), boardCard("c"))
	return step(t, s, model.SelectMsg{UID: "a"}).State
}

func laneID(i int) string { return fmt.Sprintf("%s.lane%d", view.BoardID, i) }

func roleWords(n tern.WireNode) []string {
	role, _ := n.P["role"].(string)
	return strings.Fields(role)
}

func TestEveryLaneColumnTakesDropsAndEveryCardListDrags(t *testing.T) {
	s := dragPane(t)
	nodes := wireNodes(t, view.Render(s.Model, now))
	for i := range s.Model.BoardColumns() {
		lane := node(t, nodes, laneID(i))
		if got := lane.P["drop"]; got != view.LaneDropAct {
			t.Errorf("column %d drop = %v, want %s", i, got, view.LaneDropAct)
		}
		if !containsWord(roleWords(lane), view.LaneRole) {
			t.Errorf("column %d role = %v, want it to hold %s", i, lane.P["role"], view.LaneRole)
		}
		list := node(t, nodes, laneID(i)+".cards")
		if got := list.P["drag"]; got != view.CardDragAct {
			t.Errorf("column %d card list drag = %v, want %s", i, got, view.CardDragAct)
		}
		if !containsWord(roleWords(list), view.FillRole) {
			t.Errorf("column %d card list role = %v, want it to keep %s", i, list.P["role"], view.FillRole)
		}
		if _, ok := list.P["drop"]; ok {
			t.Errorf("column %d card list takes drops: the Lane is the only target", i)
		}
	}
}

func containsWord(words []string, want string) bool {
	for _, w := range words {
		if w == want {
			return true
		}
	}
	return false
}

func TestADropTheModelIgnoresStillChangesTheCardListNodeAndOnlyItsRole(t *testing.T) {
	s := dragPane(t)
	before := node(t, wireNodes(t, view.Render(s.Model, now)), laneID(1)+".cards")
	// a is the only card of its Lane, so a Reorder is a drop the Board has nothing to do for.
	after := step(t, s, model.CardDropped{UID: "a", Column: 1, Reorder: true, Index: 0}).State
	changed := node(t, wireNodes(t, view.Render(after.Model, now)), laneID(1)+".cards")
	if reflect.DeepEqual(before.P["role"], changed.P["role"]) {
		t.Errorf("card list role stayed %v after a drop, so Tern would not put the card back at once", before.P["role"])
	}
	if !containsWord(roleWords(changed), view.FillRole) {
		t.Errorf("card list role = %v lost %s", changed.P["role"], view.FillRole)
	}
	was, got := withoutRole(before.P), withoutRole(changed.P)
	if !reflect.DeepEqual(was, got) {
		t.Errorf("card list props changed beyond the role: %v became %v", was, got)
	}
}

func withoutRole(p map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range p {
		if k != "role" {
			out[k] = v
		}
	}
	return out
}

func TestOnlyTheSelectedCardHasAMoreMenuWithItsRightClickEntries(t *testing.T) {
	s := dragPane(t)
	nodes := wireNodes(t, view.Render(s.Model, now))
	var more []string
	for _, n := range nodes {
		if strings.HasSuffix(n.ID, ".id.more") {
			more = append(more, n.ID)
		}
	}
	selected := rowOf(t, s.Model, view.BoardID, "a")
	if want := []string{selected + ".id.more"}; !reflect.DeepEqual(more, want) {
		t.Fatalf("more menus = %v, want only the selected card's %v", more, want)
	}
	button := node(t, nodes, selected+".id.more")
	if got := button.P["trigger"]; got != "quiet" {
		t.Errorf("trigger = %v, want quiet", got)
	}
	if _, ok := button.P["text"]; ok {
		t.Errorf("the button has a label %v, want Tern's square … button", button.P["text"])
	}
	actions, _ := node(t, nodes, selected).P["actions"].(map[string]any)
	if len(actions) == 0 || actions["menu"] == nil {
		t.Fatalf("the card has no right-click menu: %v", actions)
	}
	if !reflect.DeepEqual(button.P["items"], actions["menu"]) {
		t.Errorf("more menu items = %v, want the right-click menu's %v", button.P["items"], actions["menu"])
	}
}

// A Menu row sends its whole id as the act, with no value; the Close row of the selected card's more menu still opens the Close card.
func TestAMoreMenuRowSentAsAWholeActOpensTheCloseCard(t *testing.T) {
	s := dragPane(t)
	more := rowOf(t, s.Model, view.BoardID, "a") + ".id.more"
	out := step(t, s, &tern.Action{ID: more, Act: view.ContextMenuPickClick("a", model.ModeBoard, "", "x")})
	if form := out.State.Model.CloseForm; !form.Open || form.UID != "a" {
		t.Errorf("Close card = %+v, want open on a", form)
	}
}
