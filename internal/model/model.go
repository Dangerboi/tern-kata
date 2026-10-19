package model

import (
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
	DeadlineOnDate      *string
	ID                  int64
	Labels              []string
	Metadata            map[string]any
	Owner               *string
	Priority            *int64
	ProjectID           int64
	ProjectName         string
	ProjectUID          *string
	QualifiedID         string
	RecurrenceID        *int64
	Revision            int64
	ScheduledOnDate     *string
	ShortID             string
	Status              string
	Title               string
	UID                 string
	UpdatedAt           time.Time
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
	etag                string
	etagQuery           SnapshotQuery
	CloseForm           CloseFormState
	RecurrenceForm      RecurrenceFormState
	Actor               string
	FieldForm           FieldFormState
	Workspace           string
	Project             Project
	Daemon              Daemon
	Snapshot            Snapshot
	Issues              []Issue
	SelectedUID         string
	Loading             bool
	Err                 error
	Cols                int
	Quitting            bool
	Collection          Collection
	Collections         []Collection
	CollectionCursor    int
	SidebarProjects     bool
	Bound               bool
	Picker              PickerState
	ProjectForm         ProjectFormState
	Focus               Column
	Groups              []IssueGroup
	Calendar            PaneCalendar
	HelpOpen            bool
	Flat                bool
	ExpandedUIDs        map[string]bool
	Drafts              map[string]Draft
	EditorUID           string
	Filters             Filters
	Search              SearchState
	IDSearch            IDSearch
	FilterForm          FilterFormState
}

func Update(m Model, msg Msg) (Model, []Cmd) {
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
	if next, cmds, handled := updateEditor(m, msg); handled {
		return next, cmds
	}
	if next, cmds, handled := updateProjectForm(m, msg); handled {
		return next, cmds
	}
	switch msg.(type) {
	case CloseFieldMsg, CloseResultMsg:
		return updateClose(m, msg)
	case RecurrenceResultMsg:
		return updateRecurrence(m, msg)
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
	case FieldWritten:
		if m.FieldForm.UID != msg.UID {
			return m, nil
		}
		m.FieldForm.Saving = false
		if msg.Err != nil {
			m.FieldForm.Open = true
			m.FieldForm.Err = msg.Err
			return m, nil
		}
		m.FieldForm = FieldFormState{}
		m = m.issueWritten(msg.UID)
		return snapshotChanged(m)
	case Start:
		m.Workspace = msg.Workspace
		m = m.openRequested(msg.IssueRef)
		m.Loading = true
		m.Err = nil
		return m, []Cmd{LoadProject{Workspace: msg.Workspace}}
	case ProjectLoaded:
		return updateProject(m, msg)
	case DetailLoaded:
		m = m.detailLoaded(msg)
	case DetailFailed:
		m = m.detailFailed(msg)
	case JumpIssueMsg:
		return m.jumpTo(msg.UID), nil
	case FocusColumnMsg:
		if msg.Column <= ColumnDetail {
			m.Focus = msg.Column
			if m.Focus == ColumnDetail {
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
		m.Cols = msg.Cols
	default:
		return updateLive(m, msg)
	}
	return m, nil
}
