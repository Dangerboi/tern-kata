package view_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

func TestDecodeDrag(t *testing.T) {
	const lane2 = "main.columns.list.board.lane2"
	const other = "main.columns.list.board.lane0.cards.u9"
	drag := func(lane string) string { return view.DragAct(view.CardDragAct, lane) }
	drop := func(lane string) string { return view.DragAct(view.LaneDropAct, lane) }
	cardDrop := func(lane string) string { return view.DragAct(view.CardDropAct, lane) }
	tests := []struct {
		name          string
		id, act, item string
		value         any
		want          model.Msg
	}{
		{"reorder", lane2 + ".cards", drag("needs-info"), lane2 + ".cards.u1", float64(3), model.CardDropped{UID: "u1", Column: 2, Lane: "needs-info", Reorder: true, Index: 3}},
		{"reorder index as int", lane2 + ".cards", drag("needs-info"), lane2 + ".cards.u1", 0, nil},
		{"reorder index as text", lane2 + ".cards", drag("needs-info"), lane2 + ".cards.u1", "3", nil},
		{"two digit lane", "main.columns.list.board.lane12.cards", drag("x"), "main.columns.list.board.lane12.cards.u1", float64(0), model.CardDropped{UID: "u1", Column: 12, Lane: "x", Reorder: true}},
		{"drop on a Lane", lane2, drop("needs-info"), other, "after", model.CardDropped{UID: "u9", Column: 2, Lane: "needs-info"}},
		{"drop on a label with a colon", lane2, drop("area:ui"), other, "after", model.CardDropped{UID: "u9", Column: 2, Lane: "area:ui"}},
		{"reorder in a label with a colon", lane2 + ".cards", drag("a:b:c"), lane2 + ".cards.u1", float64(1), model.CardDropped{UID: "u1", Column: 2, Lane: "a:b:c", Reorder: true, Index: 1}},
		{"drop on No lane", lane2, drop(model.LaneNone), other, "after", model.CardDropped{UID: "u9", Column: 2, Lane: model.LaneNone}},
		{"drop on Done", lane2, drop(model.LaneDone), other, "after", model.CardDropped{UID: "u9", Column: 2, Lane: " done"}},
		{"reorder in Done", lane2 + ".cards", drag(model.LaneDone), lane2 + ".cards.u1", float64(0), model.CardDropped{UID: "u1", Column: 2, Lane: " done", Reorder: true}},
		{"act without a Lane", lane2, view.LaneDropAct, other, "after", nil},
		{"reorder item from another list", lane2 + ".cards", drag("l"), "main.columns.list.board.lane1.cards.u1", float64(0), nil},
		{"reorder item is the list itself", lane2 + ".cards", drag("l"), lane2 + ".cards", float64(0), nil},
		{"reorder on the Lane column", lane2, drag("l"), lane2 + ".cards.u1", float64(0), nil},
		{"drop on the card list", lane2 + ".cards", drop("l"), lane2 + ".cards.u1", "after", nil},
		{"drop item without a card", lane2, drop("l"), "main.columns.list.board.lane0.cards", "after", nil},
		{"drop item that is not a card", lane2, drop("l"), "main.columns.list.board.lane0.cards.", "after", nil},
		{"drop item outside the Board", lane2, drop("l"), "elsewhere.cards.u1", "after", nil},
		{"lane not a number", "main.columns.list.board.laneX", drop("l"), other, "after", nil},
		{"id outside the Board", "other.lane2", drop("l"), other, "after", nil},
		{"other act", lane2, "press", other, "after", nil},
		{"card drop after", lane2 + ".cards.u2", cardDrop("needs-info"), other, "after", model.CardDropped{UID: "u9", Column: 2, Lane: "needs-info", Target: "u2", After: true}},
		{"card drop before", lane2 + ".cards.u2", cardDrop("area:ui"), "main.columns.list.board.lane1.cards.u9", "before", model.CardDropped{UID: "u9", Column: 2, Lane: "area:ui", Target: "u2"}},
		{"card drop on itself", lane2 + ".cards.u2", cardDrop("l"), lane2 + ".cards.u2", "after", nil},
		{"card drop without a side", lane2 + ".cards.u2", cardDrop("l"), other, "inside", nil},
		{"card drop side not text", lane2 + ".cards.u2", cardDrop("l"), other, float64(1), nil},
		{"card drop target is the list", lane2 + ".cards", cardDrop("l"), other, "after", nil},
		{"card drop target without a uid", lane2 + ".cards.", cardDrop("l"), other, "after", nil},
		{"card drop target on the Lane", lane2, cardDrop("l"), other, "after", nil},
		{"card drop item without a card", lane2 + ".cards.u2", cardDrop("l"), "main.columns.list.board.lane0.cards.", "after", nil},
		{"card drop item outside the Board", lane2 + ".cards.u2", cardDrop("l"), "elsewhere.cards.u9", "after", nil},
		{"card drop id outside the Board", "other.lane2.cards.u2", cardDrop("l"), other, "after", nil},
		{"card drop act without a Lane", lane2 + ".cards.u2", view.CardDropAct, other, "after", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := view.DecodeDrag(tt.id, tt.act, tt.item, tt.value); got != tt.want {
				t.Errorf("DecodeDrag = %#v, want %#v", got, tt.want)
			}
		})
	}
}
