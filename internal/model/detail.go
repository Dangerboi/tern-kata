package model

import "slices"

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
	Selected *IssueDetailData
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
	// Reply says what a comment answers ("confirms dana's comment in #bbbb") and Replies how many comments answer it; both
	// are empty without reply information.
	Reply, Replies string
	// Head is a comment row's heading as styled parts: its Author, Time and the reply notes.
	Head []RowSpan
}

// RowSpan is a part of a row's heading with its style.
type RowSpan struct {
	Text  string
	Style SpanStyle
}

// SpanStyle is how the view draws a RowSpan; the values are Tern's span styles.
type SpanStyle string

const (
	SpanStrong SpanStyle = "strong"
	SpanMuted  SpanStyle = "muted"
	SpanAccent SpanStyle = "accent"
)

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
