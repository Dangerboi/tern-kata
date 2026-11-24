package model

import "slices"

// CardDropped is a Board card the user dragged and let go of on the columns layout (Tern 0.9.32 drag and drop). Column is the
// index into BoardColumns of the Lane it was let go on, and Lane that Lane's key as the frame the user dragged in drew it, so a
// drop sent from a frame drawn before the Lane list changed is not taken for another Lane. Reorder says it was let go inside
// its own Lane's card list, at Index (0-based, among the cards that Lane shows). Target is the card it was let go on, when it
// was let go on a card of another Lane (a custom-ordered Lane's cards are drop targets), and After says it landed below that
// card rather than above.
type CardDropped struct {
	UID     string
	Column  int
	Lane    string
	Reorder bool
	Index   int
	Target  string
	After   bool
}

func (CardDropped) msg() {}

// BoardDragGen counts the cards dropped on the Board. The view writes it into each Lane's card list, so every drop changes the
// list and Tern puts a card the Pane did not move back in place at once (an unchanged frame would leave it for seconds).
func (m Model) BoardDragGen() int { return m.Board.dragGen }

// cardDropped is the model's answer to a card let go on the Board. Every drop counts in dragGen first, so the next frame
// differs and Tern restores a card the Pane did not move. A drop inside the card's own Lane's card list reorders that Lane, a
// drop on its own Lane's empty space changes nothing. A drop on another Lane is the Move the keys make, a drop on Done is the
// Close card, and a closed card dropped on a Lane raises the same Reopen toast as the keys. A Move into a Lane with a custom
// order also places the card in it: before or after the card it was let go on, else last.
func cardDropped(m Model, msg CardDropped) (Model, []Cmd) {
	m.Board.dragGen++
	if !m.OnBoard() || m.Board.Mode != BoardColumns {
		return m, nil
	}
	columns := m.BoardColumns()
	if msg.Column < 0 || msg.Column >= len(columns) {
		return m, nil
	}
	from, _ := CardAt(columns, msg.UID)
	if from < 0 {
		return m, nil
	}
	lane := columns[msg.Column].Lane
	if msg.Lane != lane {
		return m, nil // the drop was sent from a frame drawn before the Lane list changed
	}
	if msg.Reorder {
		if from != msg.Column || !LaneOrderable(lane) {
			return m, nil
		}
		return reorderCard(m, columns[msg.Column], msg.UID, msg.Index)
	}
	if from == msg.Column {
		return m, nil
	}
	m.SelectedUID = msg.UID
	if lane == LaneDone {
		return openClose(m), nil
	}
	m, at, cmds, ok := movable(m, msg.UID)
	if !ok {
		return m, cmds
	}
	m, cmds = moveCardTo(m, at, lane)
	if !LaneOrderable(lane) || !m.LaneOrderCustom(lane) {
		return m, cmds
	}
	order := placeInOrder(laneBase(m, lane, columns[msg.Column]), msg.UID, msg.Target, msg.After)
	m, save := m.saveOrder(lane, order)
	return m, append(cmds, save...)
}

// reorderCard moves uid to index among the cards the Lane shows and saves the new order.
func reorderCard(m Model, column BoardColumn, uid string, index int) (Model, []Cmd) {
	visible := make([]string, len(column.Cards))
	for i, card := range column.Cards {
		visible[i] = card.Issue.UID
	}
	order := reorderLane(laneBase(m, column.Lane, column), visible, uid, index)
	if order == nil {
		return m, nil
	}
	return m.saveOrder(column.Lane, order)
}

// laneBase is the Lane's stored order as a write starts from it: without the Issues now closed or in another Lane, and with the
// cards the order does not name first, as the Lane shows them.
func laneBase(m Model, lane string, column BoardColumn) []string {
	lanes := m.LaneList()
	var base []string
	for _, card := range column.Cards {
		if !slices.Contains(m.Board.order[lane], card.Issue.UID) {
			base = append(base, card.Issue.UID)
		}
	}
	for _, uid := range m.Board.order[lane] {
		if m.leftLane(uid, lane, lanes) {
			continue
		}
		base = append(base, uid)
	}
	return base
}

// reorderLane is the stored order after uid is dragged to index among the visible uids (the Lane's shown cards, top first).
// base is the order as laneBase gives it. uid goes right after its new upper neighbour among the visible cards, or right before
// its new lower one when it is first, so the uids the Pane does not show keep their places. It is nil when the visible order
// does not change.
func reorderLane(base, visible []string, uid string, index int) []string {
	from := slices.Index(visible, uid)
	if from < 0 {
		return nil
	}
	moved := slices.Clone(visible)
	moved = slices.Delete(moved, from, from+1)
	index = max(0, min(index, len(moved)))
	if index == from {
		return nil
	}
	moved = slices.Insert(moved, index, uid)
	if index == 0 {
		return placeInOrder(base, uid, moved[1], false)
	}
	return placeInOrder(base, uid, moved[index-1], true)
}

// placeInOrder is order with uid put right before anchor, or right after it when after; last when anchor is not in it.
func placeInOrder(order []string, uid, anchor string, after bool) []string {
	placed := slices.DeleteFunc(slices.Clone(order), func(u string) bool { return u == uid })
	at := slices.Index(placed, anchor)
	switch {
	case at < 0:
		return append(placed, uid)
	case after:
		at++
	}
	return slices.Insert(placed, at, uid)
}
