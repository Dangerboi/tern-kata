package model

import "slices"

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

func (m Model) DetailDrawn() bool {
	if m.OnJobs() {
		return m.Shows(ColumnDetail) && m.SelectedJob() != nil
	}
	if m.OnRuns() {
		return m.Shows(ColumnDetail) && m.SelectedRun() != nil
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

func moveSelection(m Model, action Action) Model {
	rows := m.VisibleRows()
	if len(rows) == 0 {
		return m
	}
	at := slices.IndexFunc(rows, func(issue IssueRow) bool { return issue.UID == m.SelectedUID })
	m.SelectedUID = rows[movedIndex(at, len(rows), m.issuePage(), action)].UID
	return m
}

func (m Model) Position() (selected, total int) {
	if m.OnConfig() {
		return min(m.ConfigPage.Cursor+1, len(m.ConfigPage.Files)), len(m.ConfigPage.Files)
	}
	if m.OnDaemons() {
		return min(m.Daemons.Cursor+1, len(m.Daemons.Entries)), len(m.Daemons.Entries)
	}
	if m.OnFederation() {
		cursor, n := m.federationCursorCount()
		return min(cursor+1, n), n
	}
	if m.OnAttention() {
		return m.attentionIndex() + 1, len(m.Attention.Requests)
	}
	if m.OnCredentials() {
		return min(m.Credentials.Cursor+1, len(m.Credentials.Tokens)), len(m.Credentials.Tokens)
	}
	if m.OnRuns() {
		runs := m.searchedRuns()
		return slices.IndexFunc(runs, func(r Run) bool { return r.UID == m.Runs.Selected }) + 1, len(runs)
	}
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
