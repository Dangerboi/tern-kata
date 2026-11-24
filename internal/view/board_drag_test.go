package view_test

import (
	"encoding/json"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

func TestDecodeDrag(t *testing.T) {
	const lane2 = "main.columns.list.board.lane2"
	tests := []struct {
		name          string
		id, act, item string
		value         any
		want          model.Msg
	}{
		{"reorder", lane2 + ".cards", view.CardDragAct, lane2 + ".cards.u1", float64(3), model.CardDropped{UID: "u1", Column: 2, Reorder: true, Index: 3}},
		{"reorder index as int", lane2 + ".cards", view.CardDragAct, lane2 + ".cards.u1", 0, model.CardDropped{UID: "u1", Column: 2, Reorder: true}},
		{"reorder index as json number", lane2 + ".cards", view.CardDragAct, lane2 + ".cards.u1", json.Number("1"), model.CardDropped{UID: "u1", Column: 2, Reorder: true, Index: 1}},
		{"two digit lane", "main.columns.list.board.lane12.cards", view.CardDragAct, "main.columns.list.board.lane12.cards.u1", float64(0), model.CardDropped{UID: "u1", Column: 12, Reorder: true}},
		{"drop on a Lane", lane2, view.LaneDropAct, "main.columns.list.board.lane0.cards.u9", "after", model.CardDropped{UID: "u9", Column: 2}},
		{"reorder without a number", lane2 + ".cards", view.CardDragAct, lane2 + ".cards.u1", "3", nil},
		{"reorder item from another list", lane2 + ".cards", view.CardDragAct, "main.columns.list.board.lane1.cards.u1", float64(0), nil},
		{"reorder item is the list itself", lane2 + ".cards", view.CardDragAct, lane2 + ".cards", float64(0), nil},
		{"reorder on the Lane column", lane2, view.CardDragAct, lane2 + ".cards.u1", float64(0), nil},
		{"drop on the card list", lane2 + ".cards", view.LaneDropAct, lane2 + ".cards.u1", "after", nil},
		{"drop item without a card", lane2, view.LaneDropAct, "main.columns.list.board.lane0.cards", "after", nil},
		{"drop item that is not a card", lane2, view.LaneDropAct, "main.columns.list.board.lane0.cards.", "after", nil},
		{"drop item outside the Board", lane2, view.LaneDropAct, "elsewhere.cards.u1", "after", nil},
		{"lane not a number", "main.columns.list.board.laneX", view.LaneDropAct, "main.columns.list.board.lane0.cards.u1", "after", nil},
		{"id outside the Board", "other.lane2", view.LaneDropAct, "main.columns.list.board.lane0.cards.u1", "after", nil},
		{"other act", lane2, "press", "main.columns.list.board.lane0.cards.u1", "after", nil},
		{"card drop after", lane2 + ".cards.u2", view.CardDropAct, "main.columns.list.board.lane0.cards.u9", "after", model.CardDropped{UID: "u9", Column: 2, Target: "u2", After: true}},
		{"card drop before", lane2 + ".cards.u2", view.CardDropAct, "main.columns.list.board.lane1.cards.u9", "before", model.CardDropped{UID: "u9", Column: 2, Target: "u2"}},
		{"card drop on itself", lane2 + ".cards.u2", view.CardDropAct, lane2 + ".cards.u2", "after", nil},
		{"card drop without a side", lane2 + ".cards.u2", view.CardDropAct, "main.columns.list.board.lane0.cards.u9", "inside", nil},
		{"card drop side not text", lane2 + ".cards.u2", view.CardDropAct, "main.columns.list.board.lane0.cards.u9", float64(1), nil},
		{"card drop target is the list", lane2 + ".cards", view.CardDropAct, "main.columns.list.board.lane0.cards.u9", "after", nil},
		{"card drop target without a uid", lane2 + ".cards.", view.CardDropAct, "main.columns.list.board.lane0.cards.u9", "after", nil},
		{"card drop target on the Lane", lane2, view.CardDropAct, "main.columns.list.board.lane0.cards.u9", "after", nil},
		{"card drop item without a card", lane2 + ".cards.u2", view.CardDropAct, "main.columns.list.board.lane0.cards.", "after", nil},
		{"card drop item outside the Board", lane2 + ".cards.u2", view.CardDropAct, "elsewhere.cards.u9", "after", nil},
		{"card drop id outside the Board", "other.lane2.cards.u2", view.CardDropAct, "main.columns.list.board.lane0.cards.u9", "after", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := view.DecodeDrag(tt.id, tt.act, tt.item, tt.value); got != tt.want {
				t.Errorf("DecodeDrag = %#v, want %#v", got, tt.want)
			}
		})
	}
}
