package model

import (
	"slices"
	"strings"
)

type ActivityTab uint8

const (
	TabComments ActivityTab = iota
	TabEvents
	TabLinks
	TabAttachments
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

// ShowCardMsg is a click on a related Issue's id on the Board: its card is selected, or its detail opens when the Board does not draw it.
type ShowCardMsg struct{ UID string }

func (ShowCardMsg) msg() {}

type FocusColumnMsg struct{ Column Column }

func (FocusColumnMsg) msg() {}

type ActivityTabMsg struct{ Tab ActivityTab }

func (ActivityTabMsg) msg() {}

type DetailItemMsg struct {
	Tab ActivityTab
	Key string
}

func (DetailItemMsg) msg() {}

// tabSpec is one Activity tab: its label, the kind of row it lists, and what it says when empty: a title, a sentence with one %s
// per action (hint), and the actions whose keys the view draws as keycaps in the sentence.
type tabSpec struct {
	label   string
	kind    RowKind
	empty   string
	hint    string
	actions []Action
}

// tabTable lists the Activity tabs in order: the label, the kind of row it lists and what it says when empty. The label, a
// row kind's tab and the empty text all come from it; a test fails for a tab left out.
var tabTable = [TabCount]tabSpec{
	TabComments:    {"Comments", RowComment, "No comments", "Press %s to add one.", []Action{AddComment}},
	TabEvents:      {"Events", RowEvent, "No events", "Events appear as the Issue changes.", nil},
	TabLinks:       {"Links", RowLink, "No links", "Press %s or %s to add one.", []Action{AddBlockerAction, AddRelatedAction}},
	TabAttachments: {"Attachments", RowAttachment, "No attachments", "Press %s to attach a file.", []Action{AttachFileAction}},
}

func (s ActivityTab) String() string { return tabTable[s].label }

// EmptyDetail is what a tab with no rows says: a Title, and a Hint sentence with one %s for each of Actions, in order, where the
// view draws that action's key as a keycap.
type EmptyDetail struct {
	Title, Hint string
	Actions     []Action
}

// EmptyDetail is what the active tab says when it has no rows: the Attachments tab of an Issue on a daemon without Docbank
// says why nothing can be attached.
func (m Model) EmptyDetail() EmptyDetail {
	spec := tabTable[m.tab]
	if m.tab == TabAttachments && m.authority != nil && m.authority.AttachmentsOff {
		return EmptyDetail{Title: spec.empty, Hint: DocbankOffMessage}
	}
	return EmptyDetail{Title: spec.empty, Hint: spec.hint, Actions: spec.actions}
}

// DetailRow is one row of an Activity tab (Type is a history event's kata type). A Comment or Event row has its Author and Time (Model.DetailTime); a Link row has Relation (the code),
// RelationLabel (its words) and Ref instead; a history row also carries its sentence in parts.
type DetailRow struct {
	Key, Type, Author, Time, Body, ReferenceUID string
	// Message and Evidence are a close event's message and its evidence line; they sit under the event's sentence.
	Message  string
	Evidence []RowSpan
	// Creation is a comment row's creation line, empty when kata gave no verification state.
	Creation string
	// Ref is the referenced Issue's id as shown (#<short_id> in the detail Issue's Project), and Relation how it relates to the
	// detail's Issue. RefTitle is that Issue's title when it is loaded.
	Relation, RelationLabel, Ref, RefTitle string
	// Qualified and Status are the referenced Issue's qualified id and status as its Link carries them (empty for an event row).
	Qualified, Status string
	File              Attachment
	History           *HistorySentence
	// Reply says what a comment answers ("confirming dana in #bbbb") and Replies how many comments answer it; both
	// are empty without reply information.
	Reply, Replies string
	Head           []RowSpan
	Tags           []RowSpan
}

type RowSpan struct {
	Text  string
	Style SpanStyle
}

// SpanStyle is how the view draws a RowSpan; the values are Tern's span styles.
type SpanStyle string

const (
	SpanStrong SpanStyle = "strong"
	SpanTime   SpanStyle = "muted mono"
	SpanMuted  SpanStyle = "muted"
	SpanMono   SpanStyle = "mono"
	SpanAccent SpanStyle = "accent"
)

// HistorySentence is "<Verb> <Object>" (the row's Author did it) with the Verb's
// tone. Object stops before the row's Ref when the sentence names the referenced Issue, and After is what follows the Ref
// ("as blocker and related").
type HistorySentence struct {
	Verb, Object, After string
	Tone                VerbTone
}

func (m Model) DetailRowCount(tab ActivityTab) int {
	if m.authority == nil {
		return 0
	}
	switch tab {
	case TabLinks:
		return len(m.authority.Links) - m.hierarchyLinkCount()
	case TabComments:
		return len(m.authority.Comments)
	case TabEvents:
		return len(m.authority.History) - m.hiddenEventCount()
	case TabAttachments:
		if m.authority.Issue != nil {
			return len(Attachments(m.authority.Issue.Metadata))
		}
	}
	return 0
}

// hierarchyLinkCount is the Links that are the Issue's parent or children: the Links tab leaves them to the Parent property and the
// Children table.
func (m Model) hierarchyLinkCount() int {
	n := 0
	for _, link := range m.authority.Links {
		if link.Type == "parent" || link.Type == "child" {
			n++
		}
	}
	return n
}

// hiddenEventCount is the History events the Events tab does not list (see EventHidden).
func (m Model) hiddenEventCount() int {
	n := 0
	for _, event := range m.authority.History {
		if EventHidden(event) {
			n++
		}
	}
	return n
}

// Width tiers, in Pane columns. MinCols fits a phone held upright (about 47 columns) and sideways beside an agent (43);
// below TwoLineRowCols an Issue row puts its title on a line of its own and pages move half as far. FoldedLegendCols is the narrow
// layout's edge: below it the key legend folds behind a keys chip, the Page bar shows icons for the other Pages, the detail's tabs
// drop their counts, the heading drops the counts and the Daemon chip, and the Agent card puts Cancel last. SideBySideCols matches
// kata's own TUI.
const (
	MinCols          = 40
	TwoLineRowCols   = 60
	FoldedLegendCols = 80
	SideBySideCols   = 140
)

// NarrowerThan reports whether the Pane is known to be under cols wide; a width Tern has not reported yet counts as wide.
func (m Model) NarrowerThan(cols int) bool { return m.Cols > 0 && m.Cols < cols }

// SidebarShown reports whether the Collections sidebar is a column: an unbound
// directory shows it while a fixed Collection is current and drops it once a Project
// is entered. Everywhere else the picker replaces it.
func (m Model) SidebarShown() bool {
	return !m.Bound && !m.Collection.IsProject() && m.Err == nil
}

func (m Model) VisibleColumns() []Column {
	if m.Cols < MinCols {
		return nil
	}
	if m.Cols < SideBySideCols {
		return []Column{m.Focus}
	}
	columns := []Column{ColumnList}
	if !m.hidden || m.OnJobs() || m.OnRuns() {
		columns = append(columns, ColumnDetail)
	}
	if m.SidebarShown() {
		columns = append([]Column{ColumnCollections}, columns...)
	}
	return columns
}

func (m Model) CanFocusDetail() bool {
	return m.SelectedUID != "" && slices.Contains(m.VisibleColumns(), ColumnList) && slices.Contains(m.VisibleColumns(), ColumnDetail)
}

func drillIn(m Model) (Model, []Cmd) {
	switch {
	case m.BoardShown():
		m = openLane(m)
	case m.Focus == ColumnCollections:
		m.Focus = ColumnList
	case m.Focus == ColumnList && m.SelectedUID != "":
		m = m.focusDetail()
	case m.Focus == ColumnDetail:
		return m.openCursorRow()
	}
	return m, nil
}

func drillOut(m Model) (Model, []Cmd) {
	switch m.Focus {
	case ColumnDetail:
		if back, cmds, ok := m.jumpBack(); ok {
			return back, cmds
		}
		m.Focus = ColumnList
	case ColumnList:
		if back, cmds, ok := m.backAcross(); ok {
			return back, cmds
		}
		if !m.SidebarShown() {
			return openPicker(m), reloadViews()
		}
		m.Focus = ColumnCollections
		return m, reloadViews()
	}
	return m, nil
}

// EvidenceText is the evidence line as plain text.
func (r DetailRow) EvidenceText() string {
	var b strings.Builder
	for _, part := range r.Evidence {
		b.WriteString(part.Text)
	}
	return b.String()
}
