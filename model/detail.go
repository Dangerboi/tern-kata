package model

import (
	"fmt"
	"slices"
	"strings"
	"time"

	g "go.kenn.io/kata/pkg/client/generated"
)

type ActivityTab uint8

// The Activity tabs under the fixed header; the active one is Model.ActivityTab.
const (
	TabComments ActivityTab = iota
	TabEvents
	TabLinks
	// TabCount is the number of tabs.
	TabCount
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

// DetailRow is one row of an Activity tab. A Link row has Relation and Ref
// instead of a Title; a history row also carries its sentence in parts.
type DetailRow struct {
	Key, Title, Body, ReferenceUID string
	// Ref is the referenced Issue's id as shown (#<short_id> in the detail Issue's Project), and Relation how it relates to the
	// detail's Issue. RefTitle is that Issue's title when it is loaded.
	Relation, Ref, RefTitle string
	History                 *HistorySentence
}

// HistorySentence is "<Actor> <Verb> <Object>" with a local When and the Verb's
// tone. Object stops before the row's Ref when the sentence names the referenced Issue.
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
		if project, short, ok := strings.Cut(ref, "#"); ok && m.Detail.Issue != nil && project == m.Detail.Issue.ProjectName {
			ref = "#" + short
		}
		row := DetailRow{Key: fmt.Sprint(link.ID), Relation: relation, Ref: ref, ReferenceUID: uid}
		if i := slices.IndexFunc(m.Issues, func(issue Issue) bool { return issue.UID == uid }); i >= 0 {
			row.RefTitle = m.Issues[i].Title
		}
		return row, true
	case TabComments:
		comment := m.Detail.Comments[index]
		return DetailRow{Key: comment.UID, Title: comment.Author + " / " + comment.CreatedAt.Format(time.RFC3339), Body: comment.Body}, true
	case TabEvents:
		return historyRow(m.Detail.History[index]), true
	}
	return DetailRow{}, false
}

// ChildRow is one row of the Children table. Key is the Link's, which is daemon-wide unique, so a
// row's id never ends in the child's UID as the list row's does. A child that is not loaded has only its UID, Key and Ref.
type ChildRow struct {
	Key, UID, Ref, Title, Status string
	UpdatedAt                    time.Time
	Loaded                       bool
}

// DetailChildren are the detail Issue's children: the loaded ones in the list's order, then
// the ones not loaded in the Link order. Titles, statuses and times come from the loaded Issues.
func (m Model) DetailChildren() []ChildRow {
	links := make(map[string]DetailRow)
	var uids []string
	for i := range m.DetailRowCount(TabLinks) {
		if link, _ := m.DetailRow(TabLinks, i); link.Relation == "child" {
			links[link.ReferenceUID] = link
			uids = append(uids, link.ReferenceUID)
		}
	}
	rows := make([]ChildRow, 0, len(uids))
	for _, issue := range m.Issues {
		if link, ok := links[issue.UID]; ok {
			rows = append(rows, ChildRow{Key: link.Key, UID: issue.UID, Ref: link.Ref, Title: issue.Title, Status: issue.Status, UpdatedAt: issue.UpdatedAt, Loaded: true})
			delete(links, issue.UID)
		}
	}
	for _, uid := range uids {
		if link, ok := links[uid]; ok {
			rows = append(rows, ChildRow{Key: link.Key, UID: uid, Ref: link.Ref})
			delete(links, uid)
		}
	}
	return rows
}

// DetailCursorCount is the rows the detail's j/k cursor walks: the children, then the active tab's rows.
func (m Model) DetailCursorCount() int {
	return len(m.DetailChildren()) + m.DetailRowCount(m.ActivityTab)
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
			if children := m.DetailChildren(); m.DetailIndex < len(children) {
				m = jumpIssue(m, children[m.DetailIndex].UID)
			} else if row, ok := m.DetailRow(m.ActivityTab, m.DetailIndex-len(children)); ok {
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
				m.ActivityTab = (m.ActivityTab + 1) % TabCount
			} else {
				m.ActivityTab = (m.ActivityTab + TabCount - 1) % TabCount
			}
			m.DetailIndex = len(m.DetailChildren())
		}
	case MoveDown, MoveUp, MoveFirst, MoveLast:
		if m.Focus == ColumnDetail {
			count := m.DetailCursorCount()
			if count == 0 {
				return m, nil
			}
			switch action {
			case MoveDown:
				if m.DetailIndex < count-1 {
					m.DetailIndex++
				}
			case MoveUp:
				m.DetailIndex = max(m.DetailIndex-1, 0)
			case MoveFirst:
				m.DetailIndex = 0
			case MoveLast:
				m.DetailIndex = count - 1
			}
		}
	}
	return m, nil
}
