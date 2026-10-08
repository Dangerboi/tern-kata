package model

import g "go.kenn.io/kata/pkg/client/generated"

type Issue = g.UIIssue
type Project = g.ProjectOut

type Model struct {
	LiveState
	Workspace           string
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
	Focus               Column
	Groups              []IssueGroup
	LocalDate, TimeZone string
	HelpOpen            bool
	Flat bool
	ExpandedUIDs map[string]bool
	Filters Filters
	Search SearchState
	FilterForm FilterFormState
}

func Update(m Model, msg Msg) (Model, []Cmd) {
	switch msg := msg.(type) {
	case Start:
		m.Workspace = msg.Workspace
		m.Loading = true
		m.Err = nil
		return m, []Cmd{LoadProject{Workspace: msg.Workspace}}
	case ProjectLoaded:
		m, _ = updateProject(m, msg)
		m.Cursor = msg.Snapshot.Cursor
		return startStream(m, 0)
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
	case KeyMsg:
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
			if binding.Action == DrillOut {
				m.Focus = ColumnCollections
				return m, nil
			}
			if m.Focus == ColumnCollections {
				return updateCollectionNavigation(m, binding.Action)
			}
			return updateNavigation(m, binding.Action)
		}
	case SelectMsg:
		for _, issue := range m.VisibleRows() {
			if issue.UID == msg.UID {
				m.SelectedUID = msg.UID
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
