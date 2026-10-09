package model

import (
	"fmt"
	"time"

	g "go.kenn.io/kata/pkg/client/generated"
)

type ActivityTab uint8

// The Activity tabs under the fixed header; the active one is Model.ActivityTab.
const (
	TabComments ActivityTab = iota
	TabEvents
	TabLinks
)

type LoadDetail struct{ Workspace, UID string }

func (LoadDetail) cmd() {}

type DetailLoaded struct {
	UID      string
	Selected *g.UISelectedAuthority
}

func (DetailLoaded) msg() {}

type DetailFailed struct {
	UID string
	Err error
}

func (DetailFailed) msg() {}

type JumpIssueMsg struct{ UID string }

func (JumpIssueMsg) msg() {}

type FocusColumnMsg struct{ Column Column }

func (FocusColumnMsg) msg() {}

type ActivityTabMsg struct{ Tab ActivityTab }

func (ActivityTabMsg) msg() {}

type DetailItemMsg struct {
	Tab ActivityTab
	Key string
}

func (DetailItemMsg) msg() {}

func (s ActivityTab) String() string {
	return [...]string{"Comments", "Events", "Links"}[s]
}

// DetailRow is one row of an Activity tab. A history row also carries its
// sentence in parts.
type DetailRow struct {
	Key, Title, Body, ReferenceUID string
	History                        *HistorySentence
}

// HistorySentence is "<Actor> <Verb> <Object>" with a local When and the Verb's tone.
type HistorySentence struct {
	Actor, Verb, Object, When string
	Tone                      VerbTone
}

func (m Model) DetailRowCount(tab ActivityTab) int {
	if m.Detail == nil {
		return 0
	}
	switch tab {
	case TabLinks:
		return len(m.Detail.Links)
	case TabComments:
		return len(m.Detail.Comments)
	case TabEvents:
		return len(m.Detail.History)
	}
	return 0
}

func (m Model) DetailRow(tab ActivityTab, index int) (DetailRow, bool) {
	if index < 0 || index >= m.DetailRowCount(tab) {
		return DetailRow{}, false
	}
	switch tab {
	case TabLinks:
		link := m.Detail.Links[index]
		uid, ref := link.ToIssueUID, link.ToQualifiedID
		relation := link.Type
		if uid == m.DetailIssueUID() {
			uid, ref = link.FromIssueUID, link.FromQualifiedID
			switch relation {
			case "parent":
				relation = "child"
			case "child":
				relation = "parent"
			case "blocks":
				relation = "blocked_by"
			case "blocked_by":
				relation = "blocks"
			}
		}
		return DetailRow{Key: fmt.Sprint(link.ID), Title: relation + " " + ref, ReferenceUID: uid}, true
	case TabComments:
		comment := m.Detail.Comments[index]
		return DetailRow{Key: comment.UID, Title: comment.Author + " / " + comment.CreatedAt.Format(time.RFC3339), Body: comment.Body}, true
	case TabEvents:
		return historyRow(m.Detail.History[index]), true
	}
	return DetailRow{}, false
}

func jumpIssue(m Model, uid string) Model {
	if uid == "" || uid == m.SelectedUID {
		return m
	}
	m.PendingCreatedUID = ""
	m.DetailBack = append(m.DetailBack[:len(m.DetailBack):len(m.DetailBack)], m.SelectedUID)
	m.SelectedUID = uid
	m.Focus = ColumnDetail
	m.DetailHidden = false
	return m
}

// Width tiers, in Pane columns, matching kata's own TUI.
const (
	// MinCols is the narrowest Pane that lays out; below it nothing shows but a notice.
	MinCols = 80
	// SideBySideCols is the narrowest Pane that shows the list and detail together.
	SideBySideCols = 140
)

// VisibleColumns is every column from SideBySideCols, only the focused column
// from MinCols, and none below. A bound Project opens without the Collections
// column; the picker replaces it.
func (m Model) VisibleColumns() []Column {
	if m.Cols < MinCols {
		return nil
	}
	if m.Cols < SideBySideCols {
		return []Column{m.Focus}
	}
	columns := []Column{ColumnList}
	if !m.DetailHidden {
		columns = append(columns, ColumnDetail)
	}
	if !m.Bound {
		columns = append([]Column{ColumnCollections}, columns...)
	}
	return columns
}

func EnsureDetail(m Model) (Model, []Cmd) {
	if m.Quitting || m.DetailHidden || m.SelectedUID == m.DetailUID {
		return m, nil
	}
	m.DetailUID = m.SelectedUID
	if m.Detail == nil || m.Detail.Issue == nil || m.Detail.Issue.UID != m.SelectedUID {
		// A different Issue starts at its first row; the Activity tab stays. The previous detail
		// stays on screen until the new one arrives, unless nothing is selected.
		if m.SelectedUID == "" || m.DetailIssueUID() == "" {
			m.Detail = nil
		}
		m.DetailIndex = 0
	}
	m.DetailErr = nil
	m.DetailLoading = m.SelectedUID != ""
	if !m.DetailLoading {
		return m, nil
	}
	return m, []Cmd{LoadDetail{Workspace: m.Workspace, UID: m.SelectedUID}}
}

func updateDetailNavigation(m Model, action Action) (Model, []Cmd) {
	switch action {
	case DrillIn:
		if m.Focus == ColumnCollections {
			m.Focus = ColumnList
		} else if m.Focus == ColumnList && m.SelectedUID != "" {
			m.Focus = ColumnDetail
			m.DetailHidden = false
		} else if m.Focus == ColumnDetail {
			if row, ok := m.DetailRow(m.ActivityTab, m.DetailIndex); ok {
				m = jumpIssue(m, row.ReferenceUID)
			}
		}
	case DrillOut:
		if m.Focus == ColumnDetail {
			if len(m.DetailBack) > 0 {
				m.SelectedUID = m.DetailBack[len(m.DetailBack)-1]
				m.DetailBack = m.DetailBack[:len(m.DetailBack)-1]
				return m, nil
			}
			m.Focus = ColumnList
		} else if m.Focus == ColumnList {
			if m.Bound {
				return openPicker(m), nil
			}
			m.Focus = ColumnCollections
		}
	case ToggleDetail:
		m.DetailHidden = !m.DetailHidden
		if m.DetailHidden && m.Focus == ColumnDetail {
			m.Focus = ColumnList
		}
	case NextActivityTab, PreviousActivityTab:
		if m.Focus == ColumnDetail {
			if action == NextActivityTab {
				m.ActivityTab = (m.ActivityTab + 1) % 3
			} else {
				m.ActivityTab = (m.ActivityTab + 2) % 3
			}
			m.DetailIndex = 0
		}
	case MoveDown, MoveUp, MoveFirst, MoveLast, PageDown, PageUp:
		if m.Focus == ColumnDetail {
			count := m.DetailRowCount(m.ActivityTab)
			if count == 0 {
				return m, nil
			}
			switch action {
			case MoveDown:
				m.DetailIndex = min(m.DetailIndex+1, count-1)
			case MoveUp:
				m.DetailIndex = max(m.DetailIndex-1, 0)
			case MoveFirst:
				m.DetailIndex = 0
			case MoveLast:
				m.DetailIndex = count - 1
			case PageDown:
				m.DetailIndex = min(m.DetailIndex+pageRows, count-1)
			case PageUp:
				m.DetailIndex = max(m.DetailIndex-pageRows, 0)
			}
		}
	}
	return m, nil
}
