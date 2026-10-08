package model

import g "go.kenn.io/kata/pkg/client/generated"

type Issue = g.UIIssue
type Project = g.ProjectOut

type Model struct {
	Workspace   string
	Project     Project
	Snapshot    g.UISnapshotResponseBody
	Issues      []Issue
	ETag        string
	SelectedUID string
	Loading     bool
	Err         error
	Cols        int
	Quitting    bool
	Focus         Column
	DetailHidden  bool
	Detail        *g.UISelectedAuthority
	DetailUID     string
	DetailLoading bool
	DetailErr     error
	DetailSection DetailSection
	DetailIndex   int
	DetailBack    []string
}

func Update(m Model, msg Msg) (Model, []Cmd) {
	m, cmds := update(m, msg)
	m, detailCmds := EnsureDetail(m)
	return m, append(cmds, detailCmds...)
}

func update(m Model, msg Msg) (Model, []Cmd) {
	switch msg := msg.(type) {
	case Start:
		m.Workspace = msg.Workspace
		m.Loading = true
		m.Err = nil
		return m, []Cmd{LoadProject{Workspace: msg.Workspace}}
	case ProjectLoaded:
		return updateProject(m, msg)
	case DetailLoaded:
		if msg.UID == m.SelectedUID && msg.UID == m.DetailUID {
			m.Detail = msg.Selected
			m.DetailLoading = false
			m.DetailErr = nil
		}
	case DetailFailed:
		if msg.UID == m.SelectedUID && msg.UID == m.DetailUID {
			m.DetailLoading = false
			m.DetailErr = msg.Err
		}
	case JumpIssueMsg:
		return jumpIssue(m, msg.UID), nil
	case FocusColumnMsg:
		if msg.Column <= ColumnDetail {
			m.Focus = msg.Column
			if m.Focus == ColumnDetail { m.DetailHidden = false }
		}
	case DetailSectionMsg:
		if msg.Section <= SectionHistory {
			m.Focus = ColumnDetail
			m.DetailSection = msg.Section
			m.DetailIndex = 0
		}
	case DetailItemMsg:
		if msg.Section <= SectionHistory {
			m.Focus = ColumnDetail
			m.DetailSection = msg.Section
			for i := range m.DetailRowCount(msg.Section) {
				row, _ := m.DetailRow(msg.Section, i)
				if row.Key == msg.Key { m.DetailIndex = i; break }
			}
		}
	case Failed:
		m.Loading = false
		m.Err = msg.Err
	case KeyMsg:
		if binding, ok := LookupBinding(msg.Chord); ok {
			return updateNavigation(m, binding.Action)
		}
	case SelectMsg:
		for _, issue := range m.Issues {
			if issue.UID == msg.UID {
				m.SelectedUID = msg.UID
				m.Focus = ColumnList
				m.DetailBack = nil
				break
			}
		}
	case ResizeMsg:
		m.Cols = msg.Cols
	}
	return m, nil
}
