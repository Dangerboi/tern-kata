package model

import (
	"slices"
	"time"
)

// The Board Page shows one Project's Issues in Lanes. The Board holds the screen's list column and hides the
// detail column, so the snapshot read carries no selected detail while it is shown. The Lane view is one Lane as the list's
// rows beside the detail column.

const (
	DefaultDoneDays  = 14
	DefaultDoneCards = 20
)

type BoardMode uint8

const (
	BoardColumns BoardMode = iota
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
	// Shared says Lanes and the limits are the Project's shared list (its tern.lanes metadata), not the Lane list file's.
	Shared bool
	Lane   string
	// fileErr is why the Lane list file could not be read; while it is set the file is never overwritten.
	fileErr error
	// projectID and revision are the Project as read with the list, which a shared write needs; projectID is 0 when it was not
	// read, and sharedErr then or for a shared value that is not a Lane list says why. reported is the text of the last such
	// error a Notice gave, so a refresh that finds it unchanged says nothing.
	projectID, revision int64
	sharedErr           error
	archived            bool
	reported            string
	// loaded says the load for the current Project has answered; until then Lanes is the default, not the saved list.
	loaded bool
	// asked is the Project whose Lane list was last asked for; empty when the Collection is not a Project.
	asked string
	// reads numbers the Lane list reads, so an answer sent before a save took effect is dropped.
	reads Latest
	// kept is the Issues Page selection the Board's own selection replaced on entry, and replaced is what it was replaced by;
	// leaving the Board puts kept back unless the selection moved on.
	kept, replaced string
}

func (m Model) LanesRequest() int64 { return m.Board.reads.Newest() }

func (m Model) OnBoard() bool { return m.Page == PageBoard }

func (m Model) BoardShown() bool { return m.Page == PageBoard && m.Board.Mode == BoardColumns }

func (m Model) LaneViewShown() bool { return m.Page == PageBoard && m.Board.Mode == BoardLane }

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

func (m Model) LaneList() []string {
	if len(m.Board.Lanes) == 0 {
		return DefaultLanes()
	}
	return m.Board.Lanes
}

func (m Model) LaneSource() string {
	switch {
	case m.Board.Shared:
		return "shared with the Project"
	case len(m.Board.Lanes) > 0 || m.Board.DoneDays > 0 || m.Board.DoneCards > 0:
		return "this machine"
	}
	return "default"
}

type BoardCard struct {
	Issue  Issue
	Labels []string
	// Also are the other Lane labels of an open Issue, after the one that placed it.
	Also   []string
	Reopen string
	// Relationships are the Issue's Relationships from the Collection snapshot's links, every linked Issue counted whether the Board draws it or not.
	Relationships Relationships
}

type BoardColumn struct {
	Lane  string
	Title string
	Cards []BoardCard
	// Total is how many cards qualify for Done, of which Cards holds the newest; zero in every other column.
	Total int
}

// BoardColumns are the Board's columns, left to right: No lane, one per Lane, Done. Cards follow the list's order, Done's
// newest closed first. Done holds only Issues closed today or in the DoneLimits days before it, by the Pane calendar, at most DoneLimits cards.
// On the Board Page the slice is shared with the kept columns; see collectionRows.
func (m Model) BoardColumns() []BoardColumn {
	if m.OnBoard() && m.collection.answers(m) {
		return m.collection.columns
	}
	return m.boardColumns()
}

func (m Model) boardColumns() []BoardColumn {
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

func (m Model) boardRows() []IssueRow { return m.boardRowsOf(m.BoardColumns()) }

func (m Model) boardRowsOf(columns []BoardColumn) []IssueRow {
	var rows []IssueRow
	for _, column := range columns {
		if m.Board.Mode == BoardLane && column.Lane != m.Board.Lane {
			continue
		}
		for _, card := range column.Cards {
			rows = append(rows, IssueRow{Issue: card.Issue})
		}
	}
	return rows
}

func toggleBoard(m Model) (Model, []Cmd) {
	if m.OnBoard() {
		return showPage(PageIssues)(m)
	}
	return showPage(PageBoard)(m)
}

// enterBoard is the Board Page's own work on entry (see switchPage). A Pane too narrow for the Board opens the
// Lane view of the selected Issue's Lane.
func enterBoard(m Model) (Model, []Cmd) {
	m.Board.Mode = BoardColumns
	before := m.SelectedUID
	m = m.WithVisibleSelection()
	m.Board.kept, m.Board.replaced = "", ""
	if before != "" && m.SelectedUID != before {
		m.Board.kept, m.Board.replaced = before, m.SelectedUID
	}
	if m.Cols < SideBySideCols {
		m = m.laneView()
	}
	return m, nil
}

func CardAt(columns []BoardColumn, uid string) (col, row int) {
	for c, column := range columns {
		if i := slices.IndexFunc(column.Cards, func(card BoardCard) bool { return card.Issue.UID == uid }); i >= 0 {
			return c, i
		}
	}
	return -1, -1
}

func neighbour(columns []BoardColumn, from, step int) int {
	for c := from + step; c >= 0 && c < len(columns); c += step {
		if len(columns[c].Cards) > 0 {
			return c
		}
	}
	return -1
}

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

func laneBack(m Model) (Model, []Cmd) {
	if back, cmds, ok := m.backAcross(); ok {
		return back, cmds
	}
	if m.Cols < SideBySideCols {
		return drillOut(m)
	}
	m.Board.Mode = BoardColumns
	m.hidden = true
	m.Focus = ColumnList
	return m.WithVisibleSelection(), nil
}

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

func openLane(m Model) Model {
	if m.SelectedUID == "" {
		return m
	}
	return m.laneView()
}

func clearBoard(m Model) Model {
	m.Board.Mode = BoardColumns
	if m.Board.kept != "" && m.SelectedUID == m.Board.replaced {
		m.SelectedUID = m.Board.kept
	}
	m.Board.kept, m.Board.replaced = "", ""
	m.lanesWanted = false
	return m
}

func (m Model) settleBoard() Model {
	switch {
	case !m.OnBoard():
	case !m.Collection.IsProject():
		return m.leavePage()
	case m.Board.Mode == BoardColumns && m.Cols < SideBySideCols:
		return m.laneView()
	case m.Board.Mode == BoardColumns && m.Focus != ColumnList:
		return m.leavePage()
	}
	return m
}

func (m Model) revealInList(uid string) Model {
	m.ExpandedUIDs = m.expandedCopy()
	parents, _ := m.issueHierarchy()
	for parent := parents[uid]; parent != ""; parent = parents[parent] {
		m.ExpandedUIDs[parent] = true
	}
	return m
}

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

func updateBoard(m Model, msg Msg) (Model, []Cmd, bool) {
	switch msg := msg.(type) {
	case LanesLoaded:
		m, cmds := lanesLoaded(m, msg)
		return m, cmds, true
	case LanesSaved:
		return lanesSaved(m, msg), nil, true
	case LanesShared:
		m, cmds := lanesShared(m, msg)
		return m, cmds, true
	}
	return m, nil, false
}
