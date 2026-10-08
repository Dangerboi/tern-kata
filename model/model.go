package model

import g "go.kenn.io/kata/pkg/client/generated"

type Issue = g.UIIssue
type Project = g.ProjectOut

type Model struct {
	LiveState
	Create              CreateState
	PendingCreatedUID   string
	CloseForm           CloseFormState
	Actor               string
	FieldForm           FieldFormState
	Workspace           string
	OpenIssueRef        string
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
	Bound               bool
	Picker              PickerState
	Focus               Column
	Groups              []IssueGroup
	LocalDate, TimeZone string
	HelpOpen            bool
	DetailHidden        bool
	Detail              *g.UISelectedAuthority
	DetailUID           string
	DetailLoading       bool
	DetailErr           error
	DetailSection       DetailSection
	DetailIndex         int
	DetailBack          []string
	Flat                bool
	ExpandedUIDs        map[string]bool
	Drafts              map[string]Draft
	EditorUID           string
	Filters             Filters
	Search              SearchState
	FilterForm          FilterFormState
}

func Update(m Model, msg Msg) (Model, []Cmd) {
	if action, ok := msg.(ActionMsg); ok {
		if action.Action == "key" {
			msg = KeyMsg{Chord: action.Value}
		} else if action.Action == ToggleExpansion && m.CurrentMode() == ModeList {
			for _, row := range m.VisibleRows() {
				if row.UID == action.Value && row.HasChildren {
					m, _ = update(m, SelectMsg{UID: row.UID})
					for binding := range BindingsForMode(m.CurrentMode(), false) {
						if binding.Action == action.Action {
							msg = KeyMsg{Chord: binding.Chord}
							break
						}
					}
					break
				}
			}
		}
	}
	m, cmds := update(m, msg)
	m, detailCmds := EnsureDetail(m)
	return m, append(cmds, detailCmds...)
}

func update(m Model, msg Msg) (Model, []Cmd) {
	if next, cmds, handled := updateCreate(m, msg); handled {
		return next, cmds
	}
	if next, cmds, handled := updateEditor(m, msg); handled {
		return next, cmds
	}
	switch msg.(type) {
	case CloseFieldMsg, CloseSubmitMsg, CloseResultMsg:
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
	case FilterFormActionMsg:
		if m.FilterForm.Open && (msg.Action == ApplyFilters || msg.Action == CancelFilters) {
			return updateFilterForm(m, msg.Action)
		}
	case FilterFieldMsg:
		if m.FilterForm.Open && msg.Field >= 0 && msg.Field < 5 {
			m.FilterForm.Field = msg.Field
		}
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
		m.DetailUID = ""
		return refreshLive(m)
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
			if m.Focus == ColumnDetail {
				m.DetailHidden = false
			}
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
				if row.Key == msg.Key {
					m.DetailIndex = i
					break
				}
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
		for _, issue := range m.VisibleRows() {
			if issue.UID == msg.UID {
				m.PendingCreatedUID = ""
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
