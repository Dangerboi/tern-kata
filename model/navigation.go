package model

import "slices"

type Scroll struct {
	Column Column
	Down   bool
	Line   bool // by a line, instead of a page
	Start  bool // to the top of the column, instead of a page
}

func (Scroll) cmd() {}

// Shows reports whether the view draws column c. It draws none while the first
// load or a daemon failure fills the Pane, under MinCols, or for the columns the
// width folds away. Tern answers an op on an unknown node (scroll, reveal) with
// an error event, which ends the app, so no op may name a column that is not shown.
func (m Model) Shows(c Column) bool {
	return m.Err == nil && !m.FirstLoad() && slices.Contains(m.VisibleColumns(), c)
}

// onScreen drops the Scrolls aimed at a column the view does not draw. A column
// drawn again later starts at its top, so nothing is lost.
func onScreen(m Model, cmds []Cmd) []Cmd {
	return slices.DeleteFunc(cmds, func(cmd Cmd) bool {
		scroll, ok := cmd.(Scroll)
		return ok && !m.Shows(scroll.Column)
	})
}

func updateNavigation(m Model, action Action) (Model, []Cmd) {
	switch action {
	case MoveDown, MoveUp, MoveFirst, MoveLast, PageUp, PageDown, DrillIn, DrillOut:
		m.PendingCreatedUID = ""
	}
	if (action == PageUp || action == PageDown) && m.Focus == ColumnCollections {
		return m, []Cmd{Scroll{Column: m.Focus, Down: action == PageDown}}
	}
	if m.Focus == ColumnDetail && (action == ScrollDown || action == ScrollUp || action == PageUp || action == PageDown) {
		return m, []Cmd{Scroll{Column: ColumnDetail, Down: action == ScrollDown || action == PageDown, Line: action == ScrollDown || action == ScrollUp}}
	}
	if action == RetryAction {
		return updateLive(m, Retry{})
	}
	if action == QuitAction {
		m.Quitting = true
		return m, []Cmd{Quit{}}
	}
	if action == DrillIn || action == DrillOut || action == ToggleDetail || action == NextActivityTab || action == PreviousActivityTab || m.Focus != ColumnList {
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
