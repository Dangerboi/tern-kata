package model

import (
	"strings"
)

type Filters struct {
	ProjectUID, Status, Owner, Label, Relationship, Search string
}

type SearchState struct {
	Open, Results           bool
	InitialText, InitialUID string
}

// inputMode is a mode whose keys type into Search. The popup cards own theirs: see cardEntry.
func inputMode(mode Mode) bool {
	return mode == ModeSearch || mode == ModeSearchResults
}

func updateFilterAction(m Model, action Action) (Model, []Cmd) {
	switch action {
	case ClearFilters:
		m.Filters = Filters{}
		m.SelectedUID = ""
		return snapshotChanged(m)
	case OpenFilterForm:
		m.FilterForm = FilterFormState{Open: true, Field: FilterFields[0].Name, Values: m.Filters}
	case CycleStatus:
		switch m.Filters.Status {
		case "open":
			m.Filters.Status = "closed"
		case "closed":
			m.Filters.Status = ""
		default:
			m.Filters.Status = "open"
		}
		m.SelectedUID = ""
		return snapshotChanged(m)
	case SearchIssues:
		m.Search = SearchState{Open: true, InitialText: m.Filters.Search, InitialUID: m.SelectedUID}
	case MoveDown, MoveUp, PageUp, PageDown:
		m.Search.Results = true
		return updateNavigation(m, action)
	case SearchEnter:
		if m.Search.Results {
			m.Search = SearchState{}
		} else {
			m.Search.Results = true
		}
	case SearchBack:
		if m.Search.Results {
			m.Search.Results = false
		} else {
			m.Filters.Search = m.Search.InitialText
			m.SelectedUID = m.Search.InitialUID
			m.Search = SearchState{}
			return snapshotChanged(m)
		}
	case SearchQuery:
		m.Search.Results = false
	case DeleteInput, ClearInput:
		m.Filters.Search, _ = editText(m.Filters.Search, Binding{Action: action}, true, KeyMsg{})
		m.SelectedUID = ""
		return snapshotChanged(m)
	}
	return m, nil
}

func updateFilterText(m Model, msg KeyMsg) (Model, []Cmd) {
	if m.CurrentMode() != ModeSearch && m.CurrentMode() != ModeFilterForm {
		return m, nil
	}
	if m.FilterForm.Open {
		if field := filterTextField(&m.FilterForm); field != nil {
			*field, _ = editText(*field, Binding{}, false, msg)
		}
		return m, nil
	}
	text, edited := editText(m.Filters.Search, Binding{}, false, msg)
	if !edited {
		return m, nil
	}
	m.Filters.Search = text
	m.SelectedUID = ""
	return snapshotChanged(m)
}

// FilterSummary is the top bar's filter line: the active filters, empty when none is on.
func (m Model) FilterSummary() string {
	var parts []string
	if m.Filters.ProjectUID != "" {
		parts = append(parts, "project="+m.ProjectFilterName(m.Filters.ProjectUID))
	}
	if m.Filters.Status != "" {
		parts = append(parts, "status="+m.Filters.Status)
	}
	if m.Filters.Owner != "" {
		parts = append(parts, "owner="+m.Filters.Owner)
	}
	if m.Filters.Label != "" {
		parts = append(parts, "label="+m.Filters.Label)
	}
	if m.Filters.Relationship != "" {
		parts = append(parts, "relationship="+m.Filters.Relationship)
	}
	if m.Filters.Search != "" {
		parts = append(parts, "search="+m.Filters.Search)
	}
	return strings.Join(parts, " · ")
}

func (m Model) ProjectFilterName(uid string) string {
	if entry, ok := CatalogEntry(m.Snapshot, uid); ok {
		return entry.Project.Name
	}
	if m.Collection.ProjectUID == uid {
		return m.Collection.Title
	}
	return uid
}
