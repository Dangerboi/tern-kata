package view

import (
	"strconv"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// Board drag and drop (Tern 0.9.32). Each Lane's card list on the columns layout is a drag container with CardDragAct, and
// each Lane column a drop target with LaneDropAct. Tern names the dragged card by its full node id (CardID) in the event's
// item, and the container or target by its own id. Tern splits an act at its first "=" and uses the event's value for the
// drop side or index, so each act carries the key of the Lane its frame drew after a ":" (the act name holds none): a drop
// from a frame drawn before the Lane list changed then names the Lane the user saw, and the model refuses it if the column
// is another Lane now.
const (
	CardDragAct = "card-drag"
	LaneDropAct = "lane-drop"
	// CardDropAct is the drop target of each card in a Lane with a custom order: a card released on it lands before or after it.
	CardDropAct = "card-drop"
	// LaneRole marks a Lane column of the columns layout, for BoardDropCSS.
	LaneRole = "tern-kata.lane"
	// DragGenRole prefixes the drag generation in the role of each Lane's card list; see model.Model.BoardDragGen.
	DragGenRole = "tern-kata.drag-"
)

// DragAct is act (CardDragAct, LaneDropAct or CardDropAct) for the Lane with the key lane. The key is kept whole after the
// first ":", so No lane (the empty key) and Done (" done") round-trip.
func DragAct(act, lane string) string { return act + ":" + lane }

// DecodeDrag turns a drag or drop on the Board into a CardDropped: a CardDragAct on a Lane's card list (value the new 0-based
// index, a number) is a Reorder, a LaneDropAct on a Lane column (value "before" or "after") a drop on that Lane. Anything else,
// or an id or item that names no Lane and card, is nil.
func DecodeDrag(id, fullAct, item string, value any) model.Msg {
	act, lane, ok := strings.Cut(fullAct, ":")
	if !ok {
		return nil
	}
	if act == CardDropAct {
		return decodeCardDrop(id, lane, item, value)
	}
	column, rest, ok := laneOf(id)
	if !ok {
		return nil
	}
	switch act {
	case CardDragAct:
		index, ok := dragIndex(value)
		if !ok || rest != ".cards" {
			return nil
		}
		if uid := strings.TrimPrefix(item, id+"."); uid != "" && uid != item {
			return model.CardDropped{UID: uid, Column: column, Lane: lane, Reorder: true, Index: index}
		}
	case LaneDropAct:
		if rest != "" {
			return nil
		}
		if _, tail, ok := laneOf(item); ok {
			if uid, ok := strings.CutPrefix(tail, ".cards."); ok && uid != "" {
				return model.CardDropped{UID: uid, Column: column, Lane: lane}
			}
		}
	}
	return nil
}

// decodeCardDrop reads a card released on another card of a custom-ordered Lane: value "before" or "after" says on which side
// of the target it landed. A card on itself, or an id or item that names no Lane and card, is nil.
func decodeCardDrop(id, lane, item string, value any) model.Msg {
	column, rest, ok := laneOf(id)
	if !ok {
		return nil
	}
	target, ok := strings.CutPrefix(rest, ".cards.")
	if !ok || target == "" {
		return nil
	}
	_, tail, ok := laneOf(item)
	if !ok {
		return nil
	}
	uid, ok := strings.CutPrefix(tail, ".cards.")
	if !ok || uid == "" || uid == target {
		return nil
	}
	side, _ := value.(string)
	if side != "before" && side != "after" {
		return nil
	}
	return model.CardDropped{UID: uid, Column: column, Lane: lane, Target: target, After: side == "after"}
}

// laneOf splits a node id inside the Board's Lane columns into the column index and what follows the Lane's own id.
func laneOf(id string) (column int, rest string, ok bool) {
	tail, ok := strings.CutPrefix(id, BoardID+".lane")
	if !ok {
		return 0, "", false
	}
	digits := 0
	for digits < len(tail) && tail[digits] >= '0' && tail[digits] <= '9' {
		digits++
	}
	column, err := strconv.Atoi(tail[:digits])
	if err != nil {
		return 0, "", false
	}
	return column, tail[digits:], true
}

// dragIndex reads the new index Tern sends with a Reorder: a JSON number, which decodes as a float64.
func dragIndex(value any) (int, bool) {
	f, ok := value.(float64)
	return int(f), ok
}
