package model

import (
	"maps"
	"slices"
)

// collectionRows keeps the list's rows and the Board's columns between steps. Working them out walks every loaded Issue and
// Link, and one step reads them several times (the move, the view, the footer's position), while most steps (a selection
// move, a keystroke in a card) change none of what they are worked out from. Update fills it last, and WithVisibleSelection
// as soon as a snapshot lands; VisibleRows and BoardColumns answer from it only while its inputs still match the Model, so a
// Model changed since works them out afresh.
// The slices are shared by every copy of the Model: callers read them and never write to them.
type collectionRows struct {
	filled   bool
	inputs   collectionInputs
	expanded map[string]bool
	lanes    []string
	rows     []IssueRow
	columns  []BoardColumn
}

// collectionInputs are the comparable inputs of the rows and columns. The loaded Issues and the snapshot's Links are told
// apart by their first element and length: the model never writes to either in place, it replaces them (Relationships is
// rebuilt from the Links whenever they are).
type collectionInputs struct {
	firstIssue          *Issue
	issueN              int
	firstLink           *Link
	linkN               int
	flat, graphOrder    bool
	onBoard             bool
	boardMode           BoardMode
	boardLane           string
	doneDays, doneCards int
	calendar            PaneCalendar
}

func (m Model) collectionInputs() collectionInputs {
	in := collectionInputs{firstIssue: first(m.Issues), issueN: len(m.Issues), firstLink: first(m.Snapshot.Links), linkN: len(m.Snapshot.Links),
		flat: m.Flat, graphOrder: m.GraphOrder, onBoard: m.OnBoard(), boardMode: m.Board.Mode, boardLane: m.Board.Lane, calendar: m.Calendar}
	in.doneDays, in.doneCards = m.DoneLimits()
	return in
}

// first is the first element of s, nil when it is empty: with len(s) it tells a replaced slice from the one seen before.
func first[T any](s []T) *T {
	if len(s) == 0 {
		return nil
	}
	return &s[0]
}

func (c collectionRows) answers(m Model) bool {
	return c.filled && c.inputs == m.collectionInputs() && maps.Equal(c.expanded, m.ExpandedUIDs) && slices.Equal(c.lanes, m.Board.Lanes)
}

func (m Model) withCollectionRows() Model {
	if m.collection.answers(m) {
		return m
	}
	c := collectionRows{filled: true, inputs: m.collectionInputs(), expanded: maps.Clone(m.ExpandedUIDs), lanes: slices.Clone(m.Board.Lanes)}
	if m.OnBoard() {
		c.columns = m.boardColumns()
		c.rows = m.boardRowsOf(c.columns)
	} else {
		c.rows = m.listRows()
	}
	m.collection = c
	return m
}
