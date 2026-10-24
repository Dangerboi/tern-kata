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
	Caret                   Caret
}

// SearchInputFocused reports whether the Search input, not a popup card, holds the text caret.
func (m Model) SearchInputFocused() bool {
	return m.Search.Open && !m.Search.Results && m.Err == nil && !m.FirstLoad() && m.Focus == ColumnList
}

// inputMode is a mode whose keys type into Search. The popup cards own theirs: see cardEntry.
func inputMode(mode Mode) bool {
	return mode == ModeSearch || mode == ModeSearchResults
}

func clearFilters(m Model) (Model, []Cmd) {
	m.Filters = Filters{}
	m.SelectedUID = ""
	return snapshotChanged(m)
}

// FilterFieldShown says the Filter card draws the field: the Board ignores the status filter, so its card has no Status row (the
// card still carries the value, for Apply and Save as default).
func (m Model) FilterFieldShown(name Field) bool { return !(name == FilterStatus && m.OnBoard()) }

func openFilterForm(m Model) Model {
	m.FilterForm = FilterFormState{Open: true, Field: FilterFields[0].Name, Values: m.Filters}
	return m
}

func cycleStatus(m Model) (Model, []Cmd) {
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
}

func openSearch(m Model) Model {
	m.Search = SearchState{Open: true, InitialText: m.Filters.Search, InitialUID: m.SelectedUID}
	return m
}

// searchMove is a row or page key in the search modes: it moves through the results, which it shows.
func searchMove(m Model, action Action) Model {
	m.Search.Results = true
	return moveSelection(m, action)
}

// searchEnter shows the results from the query, and commits the search from the results.
func searchEnter(m Model) Model {
	if m.Search.Results {
		m.Search = SearchState{}
	} else {
		m.Search.Results = true
	}
	return m
}

// searchBack goes from the results to the query, and from the query cancels the search.
func searchBack(m Model) (Model, []Cmd) {
	if m.Search.Results {
		m.Search.Results = false
		return m, nil
	}
	m.Filters.Search = m.Search.InitialText
	m.SelectedUID = m.Search.InitialUID
	m.Search = SearchState{}
	return snapshotChanged(m)
}

// searchErasing is the handler of the key that deletes before the caret or clears the query.
func searchErasing(action Action) func(Model) (Model, []Cmd) {
	return func(m Model) (Model, []Cmd) {
		text, edited := eraseText(m.Filters.Search, &m.Search.Caret, "", action)
		if !edited || text == m.Filters.Search {
			return m, nil
		}
		return searchEdit(m, text)
	}
}

// searchEdit sets the query to text, as backspace and ctrl+u do.
func searchEdit(m Model, text string) (Model, []Cmd) {
	m.Filters.Search = text
	m.SelectedUID = ""
	return snapshotChanged(m)
}

func updateFilterText(m Model, msg KeyMsg) (Model, []Cmd) {
	if m.CurrentMode() != ModeSearch && m.CurrentMode() != ModeFilterForm {
		return m, nil
	}
	if m.FilterForm.Open {
		if field := filterTextField(&m.FilterForm); field != nil {
			*field, _ = editText(*field, &m.FilterForm.Caret, m.FilterForm.Field, false, msg)
		}
		return m, nil
	}
	text, edited := editText(m.Filters.Search, &m.Search.Caret, "", false, msg)
	if !edited { // the caret may still have moved
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
	if m.Filters.Status != "" && !m.OnBoard() {
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
	summary := strings.Join(parts, " · ")
	if summary != "" && m.Filters == m.DefaultFilters {
		summary += " (default)"
	}
	return summary
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
