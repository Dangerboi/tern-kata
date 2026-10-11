package model

import "slices"

// ScrollBy is one scroll move, spelled as Tern's scroll op names it.
type ScrollBy string

const (
	ScrollPageUp   ScrollBy = "page-up"
	ScrollPageDown ScrollBy = "page-down"
	ScrollLineUp   ScrollBy = "line-up"
	ScrollLineDown ScrollBy = "line-down"
	ScrollToStart  ScrollBy = "start"
)

type Scroll struct {
	Column Column
	By     ScrollBy
}

func (Scroll) cmd() {}

// RevealActivityTab asks for the active Activity tab to be brought into view in
// the detail scroller: its last row, then the tab names. The Pane driver sends
// it only while the tabs are drawn.
type RevealActivityTab struct{}

func (RevealActivityTab) cmd() {}

// DetailDrawn reports whether the view draws the Activity tabs: the detail column
// is on screen and holds a loaded Issue, not a placeholder or an error line.
func (m Model) DetailDrawn() bool {
	return m.Shows(ColumnDetail) && m.SelectedUID != "" && m.DetailErr == nil && m.Detail != nil && m.Detail.Issue != nil
}

// Shows reports whether the view draws column c. It draws none while the first
// load or a daemon failure fills the Pane, under MinCols, or for the columns the
// width folds away. Tern answers an op on an unknown node (scroll, reveal) with
// an error event, which ends the app, so the Pane driver sends no op naming a column that is not shown.
// The help sheet only hides the columns (they stay mounted) and Tern accepts scroll and reveal on a hidden node, so HelpOpen does not matter here.
func (m Model) Shows(c Column) bool {
	return m.Err == nil && !m.FirstLoad() && slices.Contains(m.VisibleColumns(), c)
}

// scrollFor is the scroll an action asks of the focused column: the page keys in
// the Menu sidebar, and the arrows and page keys in the detail.
func scrollFor(focus Column, action Action) (ScrollBy, bool) {
	paged := focus == ColumnCollections || focus == ColumnDetail
	switch action {
	case PageUp:
		return ScrollPageUp, paged
	case PageDown:
		return ScrollPageDown, paged
	case ScrollUp:
		return ScrollLineUp, focus == ColumnDetail
	case ScrollDown:
		return ScrollLineDown, focus == ColumnDetail
	}
	return "", false
}

func updateNavigation(m Model, action Action) (Model, []Cmd) {
	switch action {
	case MoveDown, MoveUp, MoveFirst, MoveLast, PageUp, PageDown, DrillIn, DrillOut:
		m.PendingCreatedUID = ""
	}
	if by, ok := scrollFor(m.Focus, action); ok {
		return m, []Cmd{Scroll{Column: m.Focus, By: by}}
	}
	if action == RetryAction {
		return updateLive(m, Retry{})
	}
	if action == QuitAction {
		m.Quitting = true
		return m, nil
	}
	if action == DrillIn || action == DrillOut || action == ToggleDetail || action == FocusDetail || action == NextActivityTab || action == PreviousActivityTab || m.Focus != ColumnList {
		return updateDetailNavigation(m, action)
	}
	switch action {
	case ToggleExpansion, ExpandIssue, CollapseIssue, ToggleAllExpansion, ToggleIssueView:
		return updateHierarchy(m, action), nil
	}
	rows := m.VisibleRows()
	if len(rows) == 0 {
		return m, nil
	}
	index := 0
	for i, issue := range rows {
		if issue.UID == m.SelectedUID {
			index = i
			break
		}
	}
	switch action {
	case MoveDown:
		index = min(index+1, len(rows)-1)
	case MoveUp:
		index = max(index-1, 0)
	case MoveFirst:
		index = 0
	case MoveLast:
		index = len(rows) - 1
	case PageDown:
		index = min(index+pageRows, len(rows)-1)
	case PageUp:
		index = max(index-pageRows, 0)
	}
	m.SelectedUID = rows[index].UID
	return m, nil
}

// pageRows is how far Page Up/Page Down move the Issue list selection; the
// 'nearest' reveal then keeps it in view. Tern's scroll op does not drive that
// list's own scroller and the Pane's rows are unknown here.
// ponytail: a fixed step, not a measured page; if ResizeMsg ever carries rows, use rows minus the chrome.
const pageRows = 10
