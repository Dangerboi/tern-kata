package model

import (
	"fmt"
	"time"

	g "go.kenn.io/kata/pkg/client/generated"
)

type DetailSection uint8

const (
	SectionProperties DetailSection = iota
	SectionLabels
	SectionBody
	SectionLinks
	SectionComments
	SectionHistory
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

type DetailSectionMsg struct{ Section DetailSection }

func (DetailSectionMsg) msg() {}

type DetailItemMsg struct {
	Section DetailSection
	Key     string
}

func (DetailItemMsg) msg() {}

func (s DetailSection) String() string {
	return [...]string{"Properties", "Labels", "Body", "Links", "Comments", "Change history"}[s]
}

type DetailRow struct{ Key, Title, Body, ReferenceUID string }

func (m Model) DetailRowCount(section DetailSection) int {
	if m.Detail == nil {
		return 0
	}
	switch section {
	case SectionLinks:
		return len(m.Detail.Links)
	case SectionComments:
		return len(m.Detail.Comments)
	case SectionHistory:
		return len(m.Detail.History)
	}
	return 0
}

func (m Model) DetailRow(section DetailSection, index int) (DetailRow, bool) {
	if index < 0 || index >= m.DetailRowCount(section) {
		return DetailRow{}, false
	}
	switch section {
	case SectionLinks:
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
	case SectionComments:
		comment := m.Detail.Comments[index]
		return DetailRow{Key: comment.UID, Title: comment.Author + " / " + comment.CreatedAt.Format(time.RFC3339), Body: comment.Body}, true
	case SectionHistory:
		event := m.Detail.History[index]
		row := DetailRow{Key: event.UID, Title: event.Actor + " / " + event.CreatedAt.Format(time.RFC3339) + " / " + event.Type, Body: event.Payload}
		if event.RelatedIssueUID != nil {
			row.ReferenceUID = *event.RelatedIssueUID
			if event.RelatedIssueShortID != nil {
				row.Title += " / " + *event.RelatedIssueShortID
			}
		}
		return row, true
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

// VisibleColumns folds to the focused column on narrow Panes. A bound Project
// opens without the Collections column; the picker replaces it.
func (m Model) VisibleColumns() []Column {
	if m.Cols < 100 {
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
		// A different Issue starts at its first section. The previous detail
		// stays on screen until the new one arrives, unless nothing is selected.
		if m.SelectedUID == "" || m.DetailIssueUID() == "" {
			m.Detail = nil
		}
		m.DetailSection = SectionProperties
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
			if row, ok := m.DetailRow(m.DetailSection, m.DetailIndex); ok {
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
	case NextDetailSection, PreviousDetailSection:
		if m.Focus == ColumnDetail {
			if action == NextDetailSection {
				m.DetailSection = (m.DetailSection + 1) % 6
			} else {
				m.DetailSection = (m.DetailSection + 5) % 6
			}
			m.DetailIndex = 0
		}
	case MoveDown, MoveUp, MoveFirst, MoveLast:
		if m.Focus == ColumnDetail {
			count := m.DetailRowCount(m.DetailSection)
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
			}
		}
	}
	return m, nil
}
