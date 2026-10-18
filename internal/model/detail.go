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

// DetailRow is one row of an Activity tab (Type is a history event's kata type). A Comment or Event row has its Author and Time (Model.DetailTime); a Link row has Relation and Ref
// instead; a history row also carries its sentence in parts.
type DetailRow struct {
	Key, Type, Author, Time, Body, ReferenceUID string
	// Ref is the referenced Issue's id as shown (#<short_id> in the detail Issue's Project), and Relation how it relates to the
	// detail's Issue. RefTitle is that Issue's title when it is loaded.
	Relation, Ref, RefTitle string
	History                 *HistorySentence
}

// HistorySentence is "<Verb> <Object>" (the row's Author did it) with the Verb's
// tone. Object stops before the row's Ref when the sentence names the referenced Issue.
type HistorySentence struct {
	Verb, Object string
	Tone         VerbTone
}

func (m Model) DetailRowCount(tab ActivityTab) int {
	if m.authority == nil {
		return 0
	}
	switch tab {
	case TabLinks:
		return len(m.authority.Links)
	case TabComments:
		return len(m.authority.Comments)
	case TabEvents:
		return len(m.authority.History)
	}
	return 0
}

func (m Model) DetailRow(tab ActivityTab, index int) (DetailRow, bool) {
	if index < 0 || index >= m.DetailRowCount(tab) {
		return DetailRow{}, false
	}
	switch tab {
	case TabLinks:
		link := m.authority.Links[index]
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
		if project, short, ok := strings.Cut(ref, "#"); ok && m.authority.Issue != nil && project == m.authority.Issue.ProjectName {
			ref = "#" + short
		}
		row := DetailRow{Key: fmt.Sprint(link.ID), Relation: relation, Ref: ref, ReferenceUID: uid}
		if i := slices.IndexFunc(m.Issues, func(issue Issue) bool { return issue.UID == uid }); i >= 0 {
			row.RefTitle = m.Issues[i].Title
		}
		return row, true
	case TabComments:
		comment := m.authority.Comments[index]
		return DetailRow{Key: comment.UID, Author: comment.Author, Time: m.DetailTime(comment.CreatedAt), Body: comment.Body}, true
	case TabEvents:
		// The snapshot lists History newest first; rows read oldest first, like Comments.
		return m.historyRow(m.authority.History[len(m.authority.History)-1-index]), true
	}
	return DetailRow{}, false
}

// ChildRow is one row of the Children table. Key is the Link's, which is daemon-wide unique, so a
// row's id never ends in the child's UID as the list row's does. A child that is not loaded has only its UID, Key and Ref.
type ChildRow struct {
	Key, UID, Ref, Title, Status, Owner string
	UpdatedAt                           time.Time
	Loaded, Closed                      bool
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
			owner := ""
			if issue.Owner != nil {
				owner = *issue.Owner
			}
			rows = append(rows, ChildRow{Key: link.Key, UID: issue.UID, Ref: link.Ref, Title: issue.Title, Status: issue.Status, Owner: owner, UpdatedAt: issue.UpdatedAt, Loaded: true, Closed: IsClosed(issue)})
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

// cursorChildUID is the child under the detail cursor, or "" when the cursor is on an Activity row.
func (m Model) cursorChildUID() string {
	if children := m.DetailChildren(); m.Focus == ColumnDetail && m.cursor < len(children) {
		return children[m.cursor].UID
	}
	return ""
}

// DetailCursorCount is the rows the detail's j/k cursor walks: the children, then the active tab's rows.
func (m Model) DetailCursorCount() int {
	return len(m.DetailChildren()) + m.DetailRowCount(m.tab)
}

// Width tiers, in Pane columns, matching kata's own TUI.
const (
	// MinCols is the narrowest Pane that lays out; below it nothing shows but a notice.
	MinCols = 80
	// SideBySideCols is the narrowest Pane that shows the list and detail together.
	SideBySideCols = 140
)

// SidebarShown reports whether the Collections sidebar is a column: an unbound
// directory shows it while a fixed Collection is current and drops it once a Project
// is entered. Everywhere else the picker replaces it.
func (m Model) SidebarShown() bool {
	return !m.Bound && !m.Collection.IsProject()
}

// VisibleColumns is every column from SideBySideCols, only the focused column
// from MinCols, and none below. See SidebarShown for the Collections column.
func (m Model) VisibleColumns() []Column {
	if m.Cols < MinCols {
		return nil
	}
	if m.Cols < SideBySideCols {
		return []Column{m.Focus}
	}
	columns := []Column{ColumnList}
	if !m.hidden {
		columns = append(columns, ColumnDetail)
	}
	if m.SidebarShown() {
		columns = append([]Column{ColumnCollections}, columns...)
	}
	return columns
}

// CanFocusDetail is whether list-mode tab moves to the detail: the list and detail
// are both visible and an Issue is selected.
func (m Model) CanFocusDetail() bool {
	return m.SelectedUID != "" && slices.Contains(m.VisibleColumns(), ColumnList) && slices.Contains(m.VisibleColumns(), ColumnDetail)
}

// drillIn is enter on whichever column has focus: the Menu hands focus to the list, the list to the
// detail of the selected Issue, the detail opens the row under its cursor.
func drillIn(m Model) Model {
	switch {
	case m.Focus == ColumnCollections:
		m.Focus = ColumnList
	case m.Focus == ColumnList && m.SelectedUID != "":
		m = m.focusDetail()
	case m.Focus == ColumnDetail:
		m = m.openCursorRow()
	}
	return m
}

// drillOut is esc or left: the detail goes back through the jump history or to the list, the list
// to the Menu.
func drillOut(m Model) (Model, []Cmd) {
	switch m.Focus {
	case ColumnDetail:
		if back, ok := m.jumpBack(); ok {
			return back, nil
		}
		m.Focus = ColumnList
	case ColumnList:
		if !m.SidebarShown() {
			return openPicker(m), nil
		}
		m.Focus = ColumnCollections
	}
	return m, nil
}
