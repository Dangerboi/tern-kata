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
// Close card, and a closed card dropped on a Lane raises the same Reopen toast as the keys. A Move into a Lane the Pane sees
// with a custom order also places the card in it: before or after the card it was let go on, else last. That change is applied
// only where the store still holds an order for the Lane, so a drop never brings back an order another Pane reset.
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
	if !m.LaneOrderCustom(lane) {
		return m, cmds
	}
	m, save := m.saveOrder(lane, OrderChange{Shown: shownUIDs(columns[msg.Column]), Move: msg.UID, Anchor: msg.Target, After: msg.After, OnlyOrdered: true})
	return m, append(cmds, save...)
}

// shownUIDs are the uids of the cards the Lane shows, top first.
func shownUIDs(column BoardColumn) []string {
	uids := make([]string, len(column.Cards))
	for i, card := range column.Cards {
		uids[i] = card.Issue.UID
	}
	return uids
}

// reorderCard moves uid to index among the cards the Lane shows and saves the change. uid goes right after its new upper
// neighbour among the shown cards, or right before its new lower one when it is first, so the uids the Pane does not show keep
// their places. Nothing is saved when the shown order does not change.
func reorderCard(m Model, column BoardColumn, uid string, index int) (Model, []Cmd) {
	shown := shownUIDs(column)
	from := slices.Index(shown, uid)
	if from < 0 {
		return m, nil
	}
	moved := slices.Delete(slices.Clone(shown), from, from+1)
	index = max(0, min(index, len(moved)))
	if index == from {
		return m, nil
	}
	moved = slices.Insert(moved, index, uid)
	change := OrderChange{Shown: shown, Move: uid}
	if index == 0 {
		change.Anchor = moved[1]
	} else {
		change.Anchor, change.After = moved[index-1], true
	}
	return m.saveOrder(column.Lane, change)
}
