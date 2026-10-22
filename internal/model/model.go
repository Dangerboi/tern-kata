package model

import (
	"fmt"
	"time"
)

// Issue is an Issue as the Pane shows it. The gateway translates it from kata's wire shape.
type Issue struct {
	AssignmentExpiresOn *time.Time
	Author              string
	Body                string
	ClosedAt            *time.Time
	ClosedReason        *string
	CreatedAt           time.Time
	// CronOccurrence is set on an Issue a Cron job made.
	CronOccurrence  *CronOccurrence
	DeadlineOnDate  *string
	ID              int64
	Labels          []string
	Metadata        map[string]any
	Owner           *string
	Priority        *int64
	ProjectID       int64
	ProjectName     string
	ProjectUID      *string
	QualifiedID     string
	RecurrenceID    *int64
	Revision        int64
	ScheduledOnDate *string
	ShortID         string
	Status          string
	Title           string
	UID             string
	UpdatedAt       time.Time
}

// IsClosed says the Issue is closed; any other status counts as open.
func IsClosed(issue Issue) bool { return issue.Status == "closed" }

// Project is a Project as the Pane holds it. The gateway translates it from kata's wire shape.
type Project struct {
	ID       int64
	Name     string
	Revision int64
	UID      string
}

type Model struct {
	LiveState
	IssueDetail
	Create CreateState
	// pending is the Issue Create just made, selected as soon as a snapshot lists it.
	pending string
	// etag answers etagQuery: the snapshot on screen.
	etag           string
	etagQuery      SnapshotQuery
	CloseForm      CloseFormState
	DatesForm      DatesFormState
	RecurrenceForm RecurrenceFormState
	JobForm        JobFormState
	Actor          string
	// Host is this machine's host name, which tells an Attachment held here from one held elsewhere.
	Host      string
	FieldForm FieldFormState
	// Notice reports a write that has nothing to show in the Issues, and goes at the next key.
	Notice    string
	Workspace string
	Project   Project
	Daemon    Daemon
	Snapshot  Snapshot
	// Relationships is every Issue's Relationships, built from Snapshot's links each time a snapshot is applied.
	Relationships    RelationshipIndex
	Issues           []Issue
	SelectedUID      string
	Loading          bool
	Err              error
	Cols             int
	Rows, RowPx      int
	Quitting         bool
	Collection       Collection
	Collections      []Collection
	CollectionCursor int
	SidebarProjects  bool
	Bound            bool
	Picker           PickerState
	ProjectForm      ProjectFormState
	LanesForm        LanesFormState
	Focus            Column
	Groups           []IssueGroup
	Calendar         PaneCalendar
	HelpOpen         bool
	Flat             bool
	ExpandedUIDs     map[string]bool
	Drafts           map[string]Draft
	EditorUID        string
	Filters          Filters
	Search           SearchState
	IDSearch         IDSearch
	FilterForm       FilterFormState
	Page             Page
	Board            BoardState
	Jobs             JobsState
	Runs             RunsState
	Workflows        WorkflowsState
	// Moving is the Issue whose Move is in flight; further Moves wait for its result.
	Moving string
	// nonce is the random per-run prefix of every idempotency key and keys counts the keys minted.
	nonce string
	keys  int
}

func Update(m Model, msg Msg) (Model, []Cmd) {
	if _, ok := msg.(KeyMsg); ok {
		m.Notice = ""
	}
	next, cmds := updateWithDetail(m, msg)
	return next.cancelPending(m, msg), cmds
}

func update(m Model, msg Msg) (Model, []Cmd) {
	switch msg := msg.(type) {
	case FocusFieldMsg:
		return focusField(m, msg.Field), nil
	case ChooseMsg:
		return chooseField(m, msg.Field, msg.Index), nil
	case KeyMsg:
		if next, cmds, handled := updateTopCardKey(m, msg); handled {
			return next, cmds
		}
	case EditMsg:
		if next, handled := editBody(m, msg.Target, func(e TextEditor) TextEditor {
			return e.ApplyEdit(msg.From, msg.To, msg.Text, msg.Cursor, msg.Len)
		}); handled {
			return next, nil
		}
	case UndoMsg:
		if next, handled := editBody(m, msg.Target, TextEditor.Undo); handled {
			return next, nil
		}
	}
	if next, cmds, handled := updateCreate(m, msg); handled {
		return next, cmds
	}
	if next, cmds, handled := updateWorkflows(m, msg); handled {
		return next, cmds
	}
	if next, cmds, handled := updateEditor(m, msg); handled {
		return next, cmds
	}
	if next, cmds, handled := updateProjectForm(m, msg); handled {
		return next, cmds
	}
	switch msg := msg.(type) {
	case CloseFieldMsg, CloseResultMsg:
		return updateClose(m, msg)
	case RecurrenceResultMsg:
		return updateRecurrence(m, msg)
	case DatesResultMsg:
		return updateDates(m, msg)
	}
	if msg, ok := msg.(PickerSelected); ok {
		return pickCollectionKey(m, msg.Key)
	}
	switch msg := msg.(type) {
	case FieldValueMsg:
		if m.FieldForm.Open && !m.FieldForm.Saving {
			m.FieldForm.Value = msg.Value
			m.FieldForm.Err = nil
			m.FieldForm.Suggestion = 0
		}
	case FieldLabelsLoaded:
		if m.FieldForm.Open && m.FieldForm.UID == msg.UID {
			m.FieldForm.Labels = msg.Labels
			m.FieldForm.Err = msg.Err
		}
	case MoveResultMsg:
		return updateMove(m, msg)
	case FieldWritten:
		return fieldWritten(m, msg.UID, "", msg.Err)
	case FileAttached:
		report := ""
		if issue := m.issueByUID(msg.Ref); issue != nil {
			report = fmt.Sprintf("Attached %s to %s", msg.Name, issue.ShortID)
		}
		return fieldWritten(m, msg.Ref, report, msg.Err)
	case FileDetached:
		if msg.Cleared {
			return fieldWritten(m, msg.Ref, msg.Report(), nil)
		}
		return fieldWritten(m, msg.Ref, "", msg.Err)
	case Start:
		m.Workspace = msg.Workspace
		m.nonce = msg.Nonce
		m = m.openRequested(msg.IssueRef)
		m.Loading = true
		m.Err = nil
		return m, []Cmd{LoadProject{Workspace: msg.Workspace}}
	case ProjectLoaded:
		return updateProject(m, msg)
	case LanesLoaded:
		m = lanesLoaded(m, msg)
	case LanesSaved:
		m = lanesSaved(m, msg)
	case JobsLoaded:
		m = jobsLoaded(m, msg)
	case JobWritten:
		m = jobWritten(m, msg)
	case WorkerPickersLoaded:
		m = workerPickersLoaded(m, msg)
	case JobWorkflowsLoaded:
		m = jobWorkflowsLoaded(m, msg)
	case OpenJobMsg:
		return openJob(m, msg.UID)
	case JobSelectMsg:
		m.Jobs.Selected, m.Focus = msg.UID, ColumnList
	case RunsLoaded:
		m = runsLoaded(m, msg)
	case RunShown:
		m = runShown(m, msg)
	case RunSelectMsg:
		m.Runs.Selected, m.Focus = msg.UID, ColumnList
	case DetailLoaded:
		m = m.detailLoaded(msg)
	case DetailFailed:
		m = m.detailFailed(msg)
	case JumpIssueMsg:
		return m.jumpTo(msg.UID, nil), nil
	case ShowCardMsg:
		return showCard(m, msg.UID), nil
	case FocusColumnMsg:
		if msg.Column <= ColumnDetail {
			m.Focus = msg.Column
			if m.Focus == ColumnDetail && !m.OnJobs() && !m.OnRuns() {
				m = m.focusDetail()
			}
		}
	case ActivityTabMsg:
		if msg.Tab < TabCount {
			m = m.chooseTab(msg.Tab)
		}
	case DetailItemMsg:
		if msg.Tab < TabCount {
			m = m.chooseRow(msg.Tab, msg.Key)
		}
	case CollectionSelected:
		return updateCollection(m, msg)
	case CollectionPointerSelected:
		for _, entry := range m.SidebarEntries() {
			if msg.Key == entry.Key() {
				if entry.Projects || entry.Back {
					m.Focus = ColumnCollections
				}
				return openEntry(m, entry, false)
			}
		}
	case Failed:
		m.Loading = false
		m.Err = msg.Err
	case ActionMsg:
		switch msg.Action {
		case SelectIssueAction:
			return openIssue(m, msg.Value)
		case ToggleExpansion:
			return toggleRow(m, msg.Value)
		case OpenRow:
			return openRow(m, msg.Value)
		case OpenPicker:
			return openPicker(m), nil
		}
	case IssueOpened:
		m = m.openByReference(msg.Ref, msg.Selected)
	case IssueOpenFailed:
		if msg.Ref == m.OpenIssueRef() {
			m = m.openFailed(msg.Ref)
			m.Err = msg.Err
		}
	case KeyMsg:
		mode := m.CurrentMode()
		if binding, ok := LookupBindingForMode(msg.Chord, mode); ok && binding.entry != nil {
			return binding.entry.handle(m)
		}
		if inputMode(mode) {
			return updateFilterText(m, msg)
		}
	case SelectMsg:
		return selectRow(m, msg.UID), nil
	case ResizeMsg:
		m.Cols, m.Rows, m.RowPx = msg.Cols, msg.Rows, msg.RowPx
	case PaneRowsMsg:
		m.Rows = msg.Rows
	default:
		return updateLive(m, msg)
	}
	return m, nil
}
