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
	if m.OnJobs() {
		return m.Shows(ColumnDetail) && m.SelectedJob() != nil
	}
	return m.Shows(ColumnDetail) && m.SelectedUID != "" && m.err == nil && m.authority != nil && m.authority.Issue != nil
}

// Shows reports whether the view draws column c. It draws none while the first
// load or a daemon failure fills the Pane, under MinCols, or for the columns the
// width folds away. Tern answers an op on an unknown node (scroll, reveal) with
// an error event, which ends the app, so the Pane driver sends no op naming a column that is not shown.
// The help sheet only hides the columns (they stay mounted) and Tern accepts scroll and reveal on a hidden node, so HelpOpen does not matter here.
func (m Model) Shows(c Column) bool {
	return m.Err == nil && !m.FirstLoad() && slices.Contains(m.VisibleColumns(), c)
}

// moveSelection moves the list selection by action: one row, to either end, or a page.
func moveSelection(m Model, action Action) Model {
	rows := m.VisibleRows()
	if len(rows) == 0 {
		return m
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
	return m
}

// pageRows is how far Page Up/Page Down move the Issue list selection; the
// 'nearest' reveal then keeps it in view. Tern's scroll op does not drive that
// list's own scroller and the Pane's rows are unknown here.
// ponytail: a fixed step, not a measured page; if ResizeMsg ever carries rows, use rows minus the chrome.
const pageRows = 10

// Position is the place of the selected row in the current Page's list, counted from one and zero when nothing is selected,
// and the number of rows: what the footer shows at its right.
func (m Model) Position() (selected, total int) {
	if m.OnJobs() {
		jobs := m.visibleJobs()
		return slices.IndexFunc(jobs, func(j CronJob) bool { return j.UID == m.Jobs.Selected }) + 1, len(jobs)
	}
	rows := m.VisibleRows()
	for i, issue := range rows {
		if issue.UID == m.SelectedUID {
			return i + 1, len(rows)
		}
	}
	return 0, len(rows)
}
