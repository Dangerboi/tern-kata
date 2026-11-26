package driver_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strconv"
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
	for i, column := range s.Model.BoardColumns() {
		lane := node(t, nodes, laneID(i))
		if got, want := lane.P["drop"], "lane-drop:"+column.Lane; got != want {
			t.Errorf("column %d drop = %v, want %s", i, got, want)
		}
		if !containsWord(roleWords(lane), view.LaneRole) {
			t.Errorf("column %d role = %v, want it to hold %s", i, lane.P["role"], view.LaneRole)
		}
		list := node(t, nodes, laneID(i)+".cards")
		if got, want := list.P["drag"], "card-drag:"+column.Lane; got != want {
			t.Errorf("column %d card list drag = %v, want %s", i, got, want)
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

func TestOnlyTheSelectedCardHasAMoreButtonThatOpensItsMenu(t *testing.T) {
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
		t.Fatalf("more buttons = %v, want only the selected card's %v", more, want)
	}
	button := node(t, nodes, selected+".id.more")
	if button.P["variant"] != "quiet" || button.P["icon"] != "more" {
		t.Errorf("more button = %v, want a quiet … icon button", button.P)
	}
	actions, _ := button.P["actions"].(map[string]any)
	act, value, _ := strings.Cut(fmt.Sprint(actions["click"]), "=")
	if got, want := view.DecodeAction(button.ID, act, value), (model.CardMenuMsg{UID: "a", Open: true}); got != want {
		t.Errorf("more button click decodes to %#v, want %#v", got, want)
	}
}

// cardMenuRows reads the open card menu: its rows' labels in order, "-" for a separator, and each row's click ("" for none).
func cardMenuRows(t *testing.T, nodes []tern.WireNode) (labels, clicks []string) {
	t.Helper()
	for _, row := range node(t, nodes, "layer."+view.CardMenuKey).C {
		if row.K == "rule" {
			labels, clicks = append(labels, "-"), append(clicks, "")
			continue
		}
		actions, _ := row.P["actions"].(map[string]any)
		click, _ := actions["click"].(string)
		labels, clicks = append(labels, fmt.Sprint(row.P["label"])), append(clicks, click)
	}
	return labels, clicks
}

func TestTheCardMenuFloatsBesideTheButtonWithTheRightClickRows(t *testing.T) {
	s := dragPane(t)
	if hasNode(wireNodes(t, view.Render(s.Model, now)), "layer."+view.CardMenuKey) {
		t.Fatal("the card menu shows before the … button is pressed")
	}
	s = step(t, s, &tern.Action{ID: rowOf(t, s.Model, view.BoardID, "a") + ".id.more", Act: "card-menu", Value: "a"}).State
	nodes := wireNodes(t, view.Render(s.Model, now))
	overlay := node(t, nodes, "layer."+view.CardMenuKey)
	if modal, _ := overlay.P["modal"].(bool); modal {
		t.Error("the card menu is modal: Tern then sends no outside press, so a click beside it would not close it")
	}
	anchor, _ := json.Marshal(overlay.P["anchor"])
	if want := rowOf(t, s.Model, view.BoardID, "a") + ".id.more"; !strings.Contains(string(anchor), want) {
		t.Errorf("anchor = %s, want the … button %s", anchor, want)
	}
	actions, _ := overlay.P["actions"].(map[string]any)
	if got := view.DecodeAction(overlay.ID, fmt.Sprint(actions["outside"]), ""); got != (model.CardMenuMsg{}) {
		t.Errorf("outside press decodes to %#v, want the menu closed", got)
	}
	// The rows are the card's right-click menu, in order, with its separators.
	var want []string
	right, _ := node(t, nodes, rowOf(t, s.Model, view.BoardID, "a")).P["actions"].(map[string]any)
	raw, _ := json.Marshal(right["menu"])
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		var row struct{ Label string }
		if json.Unmarshal(item, &row) != nil {
			want = append(want, "-")
			continue
		}
		want = append(want, row.Label)
	}
	if labels, _ := cardMenuRows(t, nodes); !reflect.DeepEqual(labels, want) {
		t.Errorf("card menu rows = %q, want the right-click menu's %q", labels, want)
	}
}

// A row of the card menu is a node click: Tern splits its act at the "=", and the Close row opens the Close card on the
// card and closes the menu.
func TestACardMenuRowRunsItsKeyAndClosesTheMenu(t *testing.T) {
	s := dragPane(t)
	s = step(t, s, model.CardMenuMsg{UID: "a", Open: true}).State
	labels, clicks := cardMenuRows(t, wireNodes(t, view.Render(s.Model, now)))
	i := slices.Index(labels, "Close…")
	if i < 0 {
		t.Fatalf("card menu rows = %q, want a Close… row", labels)
	}
	act, value, _ := strings.Cut(clicks[i], "=")
	out := step(t, s, &tern.Action{ID: "layer." + view.CardMenuKey + ".row" + strconv.Itoa(i), Act: act, Value: value})
	if form := out.State.Model.CloseForm; !form.Open || form.UID != "a" {
		t.Errorf("Close card = %+v, want open on a", form)
	}
	if _, open := out.State.Model.CardMenu(); open {
		t.Error("the card menu stays open after a pick")
	}
}

func TestAPressOutsideOrAKeyClosesTheCardMenu(t *testing.T) {
	open := step(t, dragPane(t), model.CardMenuMsg{UID: "a", Open: true}).State
	if _, ok := open.Model.CardMenu(); !ok {
		t.Fatal("the card menu did not open")
	}
	for _, c := range []struct {
		name string
		in   any
		// moves says the message still does its own work after closing the menu.
		moves bool
	}{
		{name: "outside press", in: &tern.Action{ID: "layer." + view.CardMenuKey, Act: view.CardMenuCloseClick}},
		{name: "esc", in: key("escape")},
		{name: "a move key", in: key("right"), moves: true},
	} {
		m := step(t, open, c.in).State.Model
		if _, ok := m.CardMenu(); ok {
			t.Errorf("%s: the card menu stays open", c.name)
		}
		still := m.BoardShown() && m.SelectedUID == "a" && m.TopCard() == model.NoCard && m.Focus == open.Model.Focus
		if still == c.moves {
			t.Errorf("%s: Board shown %v, selected %q, top card %v, focus %v; want the key's own work %v", c.name, m.BoardShown(), m.SelectedUID, m.TopCard(), m.Focus, c.moves)
		}
	}
	// While the menu is open the … button closes it: its press is outside the menu, so a button that opened it would open it
	// again on the release.
	nodes := wireNodes(t, view.Render(open.Model, now))
	actions, _ := node(t, nodes, rowOf(t, open.Model, view.BoardID, "a")+".id.more").P["actions"].(map[string]any)
	if got := view.DecodeAction("", fmt.Sprint(actions["click"]), ""); got != (model.CardMenuMsg{}) {
		t.Errorf("the … button of an open menu decodes to %#v, want the menu closed", got)
	}
	// The menu belongs to the selected card: one asked for another card does not open.
	if _, ok := step(t, dragPane(t), model.CardMenuMsg{UID: "b", Open: true}).State.Model.CardMenu(); ok {
		t.Error("a card menu opened for a card that is not selected")
	}
}

func TestTheSelectedCardsMenuIsTheLastChildOfTheWrappingBadgeRowWithItsRole(t *testing.T) {
	s := dragPane(t)
	nodes := wireNodes(t, view.Render(s.Model, now))
	selected := rowOf(t, s.Model, view.BoardID, "a")
	row := node(t, nodes, selected+".id")
	if got := row.P["wrap"]; got != true {
		t.Errorf("first row wrap = %v, want true", got)
	}
	var ids []string
	for _, c := range row.C {
		ids = append(ids, c.ID)
	}
	if want := []string{selected + ".id.short", selected + ".id.more"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("first row children = %v, want %v", ids, want)
	}
	if !containsWord(roleWords(node(t, nodes, selected+".id.more")), view.CardMoreRole) {
		t.Errorf("more role = %v, want it to hold %s", node(t, nodes, selected+".id.more").P["role"], view.CardMoreRole)
	}
	if hasNode(nodes, selected+".id.fill") {
		t.Errorf("the first row still has a fill spacer")
	}
	// A card that is not selected has no menu.
	if hasNode(nodes, rowOf(t, s.Model, view.BoardID, "b")+".id.more") {
		t.Errorf("an unselected card has a more menu")
	}
}

// A Tern menu row sends its whole id as the act, with no value; the Close row of the right-click menu still opens the Close card.
func TestAMenuRowSentAsAWholeActOpensTheCloseCard(t *testing.T) {
	s := dragPane(t)
	out := step(t, s, &tern.Action{ID: rowOf(t, s.Model, view.BoardID, "a"), Act: view.ContextMenuPickClick("a", model.ModeBoard, "", "x")})
	if form := out.State.Model.CloseForm; !form.Open || form.UID != "a" {
		t.Errorf("Close card = %+v, want open on a", form)
	}
}
