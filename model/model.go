package model

import g "go.kenn.io/kata/pkg/client/generated"

type Issue = g.UIIssue
type Project = g.ProjectOut

type Model struct {
	LiveState
	Workspace           string
	OpenIssueRef         string
	OpenedIssueUID      string
	Project             Project
	Snapshot            g.UISnapshotResponseBody
	Issues              []Issue
	ETag                string
	SelectedUID         string
	Loading             bool
	Err                 error
	Cols                int
	Quitting            bool
	Collection          Collection
	Collections         []Collection
	CollectionCursor    int
	Focus               Column
	Groups              []IssueGroup
	LocalDate, TimeZone string
	HelpOpen            bool
	DetailHidden bool
	Detail *g.UISelectedAuthority
	DetailUID string
	DetailLoading bool
	DetailErr error
	DetailSection DetailSection
	DetailIndex int
	DetailBack []string
	Flat bool
	ExpandedUIDs map[string]bool
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
		m.OpenIssueRef = msg.IssueRef
		m.Loading = true
		m.Err = nil
		return m, []Cmd{LoadProject{Workspace: msg.Workspace}}
	case ProjectLoaded:
		m, _ = updateProject(m, msg)
		m.Cursor = msg.Snapshot.Cursor
		m, cmds := startStream(m, 0)
		if m.OpenIssueRef != "" {
			m, openCmds := openIssue(m, m.OpenIssueRef)
			return m, append(cmds, openCmds...)
		}
		return m, cmds
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
	case CollectionSelected:
		return updateCollection(m, msg)
	case CollectionLoaded:
		return collectionLoaded(m, msg)
	case CollectionPointerSelected:
		for _, collection := range m.Collections {
			key := collection.View
			if key == "project" {
				key += "." + collection.ProjectUID
			}
			if msg.ID == "main.columns.collections."+key {
				m.Focus = ColumnList
				return updateCollection(m, CollectionSelected{Collection: collection})
			}
		}
	case Failed:
		m.Loading = false
		m.Err = msg.Err
	case ActionMsg:
		if msg.Action == SelectIssueAction {
			return openIssue(m, msg.Value)
		}
	case IssueOpened:
		if msg.Ref == m.OpenIssueRef && msg.Selected != nil && msg.Selected.Issue != nil {
			m.OpenIssueRef = ""
			m.SelectedUID = msg.Selected.Issue.UID
			m.OpenedIssueUID = m.SelectedUID
			m.DetailUID = m.SelectedUID
			m.Detail = msg.Selected
			m.DetailLoading = false
			m.DetailErr = nil
			m.DetailHidden = false
			m.DetailBack = nil
			m.DetailSection, m.DetailIndex = SectionBody, 0
			m.Focus = ColumnDetail
		}
	case IssueOpenFailed:
		if msg.Ref == m.OpenIssueRef {
			m.OpenIssueRef = ""
			m.Err = msg.Err
		}
	case KeyMsg:
		if binding, ok := LookupBindingForMode(msg.Chord, m.CurrentMode()); ok {
			if binding.Action == ToggleHelp {
				m.HelpOpen = !m.HelpOpen
				return m, nil
			}
			if binding.Action == CloseHelp {
				m.HelpOpen = false
				return m, nil
			}
			if binding.Action == QuitAction {
				return updateNavigation(m, binding.Action)
			}
			if m.Focus == ColumnCollections {
				return updateCollectionNavigation(m, binding.Action)
			}
			return updateNavigation(m, binding.Action)
		}
	case SelectMsg:
		for _, issue := range m.VisibleRows() {
			if issue.UID == msg.UID {
				m.SelectedUID = msg.UID
				m.Focus = ColumnList
				m.DetailBack = nil
				break
			}
		}
	case ResizeMsg:
		m.Cols = msg.Cols
	default:
		return updateLive(m, msg)
	}
	return m, nil
}
