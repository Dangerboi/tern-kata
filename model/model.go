package model

import g "go.kenn.io/kata/pkg/client/generated"

type Issue = g.UIIssue

// IsClosed says the Issue is closed; any other status counts as open.
func IsClosed(issue Issue) bool { return issue.Status == "closed" }

type Project = g.ProjectOut

type Model struct {
	LiveState
	IssueDetail
	Create            CreateState
	PendingCreatedUID string
	CloseForm         CloseFormState
	Actor             string
	FieldForm         FieldFormState
	Workspace         string
	Project           Project
	Daemon            Daemon
	Snapshot          g.UISnapshotResponseBody
	// ChildCounts are the daemon's direct-child counts by Issue UID, whatever the
	// filters; kata leaves out an Issue with no children.
	ChildCounts         map[string]g.ChildCounts
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
	SidebarProjects     bool
	Bound               bool
	Picker              PickerState
	ProjectForm         ProjectFormState
	Focus               Column
	Groups              []IssueGroup
	LocalDate, TimeZone string
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
	return updateWithDetail(m, msg)
}

func update(m Model, msg Msg) (Model, []Cmd) {
	if msg, ok := msg.(FocusFieldMsg); ok {
		return focusField(m, msg.Field), nil
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
	}
	if _, ok := msg.(KeyMsg); ok && m.CloseForm.Open {
		return updateClose(m, msg)
	}
	switch msg := msg.(type) {
	case KeyMsg:
		if m.CurrentMode() == ModePicker {
			return updatePicker(m, msg)
		}
	case PickerSelected:
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
		return refreshLive(m)
	case Start:
		m.Workspace = msg.Workspace
		m = m.openRequested(msg.IssueRef)
		m.Loading = true
		m.Err = nil
		return m, []Cmd{LoadProject{Workspace: msg.Workspace}}
	case ProjectLoaded:
		m, _ = updateProject(m, msg)
		m.Cursor = msg.Snapshot.Cursor
		m, cmds := startStream(m, 0)
		if m.OpenIssueRef() != "" {
			m, openCmds := openIssue(m, m.OpenIssueRef())
			return m, append(cmds, openCmds...)
		}
		return m, cmds
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
	case CollectionLoaded:
		return collectionLoaded(m, msg)
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
		}
	case IssueOpened:
		m = m.openByReference(msg.Ref, msg.Selected)
	case IssueOpenFailed:
		if msg.Ref == m.OpenIssueRef() {
			m = m.openFailed(msg.Ref)
			m.Err = msg.Err
		}
	case KeyMsg:
		if m.FieldForm.Open {
			return updateFieldKey(m, msg)
		}
		if inputMode(m.CurrentMode()) {
			if binding, ok := LookupBindingForMode(msg.Chord, m.CurrentMode()); ok {
				return updateFilterAction(m, binding.Action)
			}
			return updateFilterText(m, msg)
		}
		if binding, ok := LookupBindingForMode(msg.Chord, m.CurrentMode()); ok {
			if binding.Action == SearchIssues || binding.Action == CycleStatus || binding.Action == OpenFilterForm || binding.Action == ClearFilters {
				return updateFilterAction(m, binding.Action)
			}
			if binding.Action == OpenClose {
				return openClose(m), nil
			}
			if binding.Action == OpenPicker {
				return openPicker(m), nil
			}
			if binding.Action == ReopenAction {
				issue := closeIssue(m)
				if issue != nil && !m.Stale && !m.CloseForm.Pending {
					m.CloseForm = CloseFormState{UID: issue.UID, Pending: true}
					return m, []Cmd{ReopenIssueCmd{Workspace: m.Workspace, UID: issue.UID, ProjectID: issue.ProjectID, Actor: m.Actor}}
				}
				return m, nil
			}
			if fieldAction(binding.Action) {
				return openField(m, binding.Action)
			}
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
		return selectRow(m, msg.UID), nil
	case ResizeMsg:
		m.Cols = msg.Cols
	default:
		return updateLive(m, msg)
	}
	return m, nil
}
