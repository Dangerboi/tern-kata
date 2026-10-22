package model

import (
	"maps"
	"slices"
	"time"
)

// The Board Page shows one Project's Issues in Lanes. The Board holds the screen's list column and hides the
// detail column, so the snapshot read carries no selected detail while it is shown. The Lane view is one Lane as the list's
// rows beside the detail column.

// Done's defaults: it reaches back DefaultDoneDays days of the Pane calendar and shows at most DefaultDoneCards cards.
const (
	DefaultDoneDays  = 14
	DefaultDoneCards = 20
)

// BoardMode is what the Board Page shows.
type BoardMode uint8

const (
	// BoardColumns is the Board: every Lane side by side.
	BoardColumns BoardMode = iota
	// BoardLane is the Lane view: the Lane named by BoardState.Lane as a flat list beside the detail.
	BoardLane
)

// BoardState is the Board Page's state. It is not saved: a Pane starts on the Issues Page. Mode is only meaningful while the
// Board Page is current; every way off the Board Page resets it to BoardColumns.
type BoardState struct {
	Mode BoardMode
	// Lanes is the Project's Lane list; empty means the default.
	Lanes []string
	// DoneDays and DoneCards are the Project's own Done limits; zero means the default.
	DoneDays, DoneCards int
	// Lane is the key of the Lane the Lane view shows.
	Lane string
	// fileErr is why the Lane list file could not be read; while it is set the file is never overwritten.
	fileErr error
	// loaded says the load for the current Project has answered; until then Lanes is the default, not the saved list.
	loaded bool
	// asked is the Project whose Lane list was last asked for; empty when the Collection is not a Project.
	asked string
	// detailHidden is the detail column's hidden flag from before the Board took its place.
	detailHidden bool
}

// OnBoard says the Board Page is current.
func (m Model) OnBoard() bool { return m.Page == PageBoard }

// BoardShown says the Board's Lane columns are on screen: the Board Page and not its Lane view.
func (m Model) BoardShown() bool { return m.Page == PageBoard && m.Board.Mode == BoardColumns }

// LaneViewShown says the Board Page shows its Lane view.
func (m Model) LaneViewShown() bool { return m.Page == PageBoard && m.Board.Mode == BoardLane }

// DoneLimits are how many days back Done reaches and how many cards it shows at most.
func (m Model) DoneLimits() (days, cards int) {
	days, cards = m.Board.DoneDays, m.Board.DoneCards
	if days <= 0 {
		days = DefaultDoneDays
	}
	if cards <= 0 {
		cards = DefaultDoneCards
	}
	return days, cards
}

// LaneList is the Lane list the Board draws.
func (m Model) LaneList() []string {
	if len(m.Board.Lanes) == 0 {
		return DefaultLanes()
	}
	return m.Board.Lanes
}

// BoardCard is one Issue as a card: what the view draws besides the Issue itself.
type BoardCard struct {
	Issue Issue
	// Labels are the Issue's labels that are not Lane labels.
	Labels []string
	// Also are the other Lane labels of an open Issue, after the one that placed it.
	Also []string
	// Reopen titles the Lane a Done card reopens into.
	Reopen string
	// Relationships are the Issue's Relationships from the Collection snapshot's links, every linked Issue counted whether the Board draws it or not.
	Relationships Relationships
}

// BoardColumn is one column of the Board: No lane, a Lane, or Done. Lane is its key, see LaneNone and LaneDone.
type BoardColumn struct {
	Lane  string
	Title string
	Cards []BoardCard
	// Total is how many cards qualify for Done, of which Cards holds the newest; zero in every other column.
	Total int
}

// BoardColumns are the Board's columns, left to right: No lane, one per Lane, Done. Cards follow the list's order, Done's
// newest closed first. Done holds only Issues closed in the last DoneLimits days of the Pane calendar, at most DoneLimits cards.
func (m Model) BoardColumns() []BoardColumn {
	lanes := m.LaneList()
	doneDays, doneCards := m.DoneLimits()
	keys := append([]string{LaneNone}, lanes...)
	keys = append(keys, LaneDone)
	columns := make([]BoardColumn, len(keys))
	for i, key := range keys {
		columns[i] = BoardColumn{Lane: key, Title: LaneTitle(key)}
	}
	for _, issue := range m.Issues {
		lane := LaneOf(issue, lanes)
		if lane == LaneDone && !m.Calendar.closedWithin(issue, doneDays) {
			continue
		}
		card := BoardCard{Issue: issue, Relationships: m.Relationships.Of(issue.UID)}
		for _, label := range issue.Labels {
			if !slices.Contains(lanes, label) {
				card.Labels = append(card.Labels, label)
			}
		}
		if lane == LaneDone {
			card.Reopen = LaneTitle(LabelLane(issue, lanes))
		} else {
			if labelled := LaneLabels(issue, lanes); len(labelled) > 1 {
				card.Also = labelled[1:]
			}
		}
		i := slices.Index(keys, lane)
		columns[i].Cards = append(columns[i].Cards, card)
	}
	done := &columns[len(columns)-1]
	slices.SortStableFunc(done.Cards, func(a, b BoardCard) int { return b.Issue.ClosedAt.Compare(*a.Issue.ClosedAt) })
	done.Total = len(done.Cards)
	done.Cards = done.Cards[:min(done.Total, doneCards)]
	return columns
}

// closedWithin says issue was closed on one of the last days days up to the calendar's date; without a date there is no window.
func (c PaneCalendar) closedWithin(issue Issue, days int) bool {
	today, err := time.Parse("2006-01-02", c.Date)
	if err != nil || issue.ClosedAt == nil {
		return false
	}
	return c.DayOf(*issue.ClosedAt) >= today.AddDate(0, 0, -days).Format("2006-01-02")
}

// boardRows are the Board's cards in reading order, or the Lane view's Lane's cards: the rows the selection moves over while the Board Page is shown.
func (m Model) boardRows() []IssueRow {
	var rows []IssueRow
	for _, column := range m.BoardColumns() {
		if m.Board.Mode == BoardLane && column.Lane != m.Board.Lane {
			continue
		}
		for _, card := range column.Cards {
			rows = append(rows, IssueRow{Issue: card.Issue})
		}
	}
	return rows
}

// toggleBoard is `b`: the Board Page from the Issues Page, the Issues Page back with the same Issue selected.
func toggleBoard(m Model) (Model, []Cmd) {
	if m.OnBoard() {
		return showPage(PageIssues)(m)
	}
	return showPage(PageBoard)(m)
}

// enterBoard is the Board Page's entry, on the Pane's Project (see enterProjectPage). A Pane too narrow for the Board opens the
// Lane view of the selected Issue's Lane.
func enterBoard(m Model) (Model, []Cmd) {
	m, cmds, ok := enterProjectPage(m, "Board")
	if !ok {
		return m, nil
	}
	m.Page = PageBoard
	m.Board.Mode = BoardColumns
	m.Board.detailHidden = m.DetailHidden()
	m.hidden = true
	m.Focus = ColumnList
	m = m.WithVisibleSelection()
	if m.Cols < SideBySideCols {
		m = m.laneView()
	}
	return m, cmds
}

// CardAt is the column and row of the card of uid, -1 and -1 when no column holds it.
func CardAt(columns []BoardColumn, uid string) (col, row int) {
	for c, column := range columns {
		if i := slices.IndexFunc(column.Cards, func(card BoardCard) bool { return card.Issue.UID == uid }); i >= 0 {
			return c, i
		}
	}
	return -1, -1
}

// neighbour is the nearest column from step away from from, towards step, that has cards; -1 when there is none.
func neighbour(columns []BoardColumn, from, step int) int {
	for c := from + step; c >= 0 && c < len(columns); c += step {
		if len(columns[c].Cards) > 0 {
			return c
		}
	}
	return -1
}

// laneView shows the Lane of the selected card (else the first Lane with cards, else No lane) as the Lane view, with the detail beside it.
func (m Model) laneView() Model {
	columns := m.BoardColumns()
	m.Board.Lane = LaneNone
	if col, _ := CardAt(columns, m.SelectedUID); col >= 0 {
		m.Board.Lane = columns[col].Lane
	} else if col := neighbour(columns, -1, 1); col >= 0 {
		m.Board.Lane = columns[col].Lane
	}
	m.Board.Mode = BoardLane
	m.hidden = false
	m.Focus = ColumnList
	return m.WithVisibleSelection()
}

// laneBack is Esc in the Lane view: the Board with the same card selected, or the Menu where there is no Board to go back to.
func laneBack(m Model) (Model, []Cmd) {
	if m.Cols < SideBySideCols {
		return drillOut(m)
	}
	m.Board.Mode = BoardColumns
	m.hidden = true
	m.Focus = ColumnList
	return m.WithVisibleSelection(), nil
}

// switchLane is `←` and `→` in the Lane view: the neighbouring Lane that has cards, on the same row where it can.
func switchLane(m Model, step int) Model {
	columns := m.BoardColumns()
	at := slices.IndexFunc(columns, func(c BoardColumn) bool { return c.Lane == m.Board.Lane })
	col, row := CardAt(columns, m.SelectedUID)
	if col != at {
		row = 0
	}
	if c := neighbour(columns, at, step); at >= 0 && c >= 0 {
		m.Board.Lane = columns[c].Lane
		m.SelectedUID = columns[c].Cards[min(row, len(columns[c].Cards)-1)].Issue.UID
	}
	return m
}

// laneCardMove is `<` and `>` in the Lane view: a Move that leaves the selection on the card that takes the moved card's place.
func laneCardMove(dir int) func(Model) (Model, []Cmd) {
	return func(m Model) (Model, []Cmd) {
		at := slices.IndexFunc(m.boardRows(), func(row IssueRow) bool { return row.UID == m.SelectedUID })
		moved, cmds := MoveCard(m, m.SelectedUID, dir)
		if len(cmds) == 0 {
			return moved, nil
		}
		moved.SelectedUID = ""
		if rows := moved.boardRows(); len(rows) > 0 {
			moved.SelectedUID = rows[min(at, len(rows)-1)].UID
		}
		return moved, cmds
	}
}

// openLane is Enter on the Board: the Lane view of the selected card's Lane.
func openLane(m Model) Model {
	if m.SelectedUID == "" {
		return m
	}
	return m.laneView()
}

// boardOff leaves the Board Page for the Issues Page and gives the detail column back, unless something already focused it.
func (m Model) boardOff() Model {
	m.Page, m.Board.Mode = PageIssues, BoardColumns
	if m.Focus != ColumnDetail {
		m.hidden = m.Board.detailHidden
	}
	return m.revealInList(m.SelectedUID).WithVisibleSelection()
}

// settleBoard keeps the Board Page consistent: it ends without a Project Collection, and the Board gives way to the Lane view
// where the Pane is too narrow for it or the list lost focus.
func (m Model) settleBoard() Model {
	switch {
	case !m.OnBoard():
	case !m.Collection.IsProject():
		return m.boardOff()
	case m.Board.Mode == BoardColumns && m.Cols < SideBySideCols:
		return m.laneView()
	case m.Board.Mode == BoardColumns && m.Focus != ColumnList:
		return m.boardOff()
	}
	return m
}

// revealInList unfolds the parents of uid so the nested list shows it.
func (m Model) revealInList(uid string) Model {
	m.ExpandedUIDs = maps.Clone(m.ExpandedUIDs)
	if m.ExpandedUIDs == nil {
		m.ExpandedUIDs = map[string]bool{}
	}
	parents, _ := m.issueHierarchy()
	for parent := parents[uid]; parent != ""; parent = parents[parent] {
		m.ExpandedUIDs[parent] = true
	}
	return m
}

// moveOnBoard moves the selected card up or down its Lane, or to the neighbouring Lane that has cards, keeping its row where it can.
func moveOnBoard(m Model, action Action) Model {
	columns := m.BoardColumns()
	col, row := CardAt(columns, m.SelectedUID)
	if col < 0 {
		return m.WithVisibleSelection()
	}
	switch action {
	case MoveDown:
		row = min(row+1, len(columns[col].Cards)-1)
	case MoveUp:
		row = max(row-1, 0)
	case PreviousLane, NextLane:
		step := 1
		if action == PreviousLane {
			step = -1
		}
		if c := neighbour(columns, col, step); c >= 0 {
			col = c
			row = min(row, len(columns[c].Cards)-1)
		}
	}
	m.SelectedUID = columns[col].Cards[row].Issue.UID
	return m
}
