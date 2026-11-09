package model

import (
	"strings"
)

type Filters struct {
	ProjectUID, Status, Owner, Label, Relationship, Search string
}

type SearchState struct {
	Open, Results bool
	// Meaning says the query runs on kata's ranked search (by meaning) at enter, not as a filter of the words (by words).
	// Refused says ctrl+t was refused because the Collection does not show one Project.
	Meaning, Refused        bool
	InitialText, InitialUID string
	Caret                   Caret
}

func (m Model) SearchInputFocused() bool {
	return (m.Search.Open && !m.Search.Results || m.PageSearch.Open) && m.Err == nil && !m.FirstLoad() && m.Focus == ColumnList
}

// inputMode is a mode whose keys type into Search. The popup cards own theirs: see cardEntry.
func inputMode(mode Mode) bool {
	return mode == ModeSearch || mode == ModeSearchResults || mode == ModePageSearch
}

func clearFilters(m Model) (Model, []Cmd) {
	m.Filters, m.ActiveView = Filters{}, ActiveView{}
	m.SelectedUID = ""
	if m.Ranked.Have {
		m.Ranked = RankedSearch{}
		return relist(m)
	}
	m.Ranked = RankedSearch{}
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
	// A search that opens on a ranked answer continues by meaning; any other starts by words.
	meaning := m.rankedShown()
	if !meaning {
		m.Ranked = RankedSearch{}
	}
	m.Search = SearchState{Open: true, Meaning: meaning, InitialText: m.Filters.Search, InitialUID: m.SelectedUID}
	return m
}

func searchMove(m Model, action Action) Model {
	m.Search.Results = true
	return moveSelection(m, action)
}

func searchEnter(m Model) (Model, []Cmd) {
	if !m.Search.Results {
		m.Search.Results = true
		return runSearchByMeaning(m)
	}
	meaning := m.Search.Meaning
	m.Search = SearchState{}
	if meaning {
		return relist(m)
	}
	return m, nil
}

func searchBack(m Model) (Model, []Cmd) {
	if m.Search.Results {
		m.Search.Results = false
		return m, nil
	}
	meaning := m.Search.Meaning
	m.Filters.Search = m.Search.InitialText
	m.SelectedUID = m.Search.InitialUID
	m.Search = SearchState{}
	if meaning {
		if m.Ranked.Query != m.Filters.Search {
			m.Ranked = RankedSearch{}
		}
		m.rankedReads.Discard()
		return relist(m)
	}
	return snapshotChanged(m)
}

func searchErasing(action Action) func(Model) (Model, []Cmd) {
	return func(m Model) (Model, []Cmd) {
		text, edited := eraseText(m.Filters.Search, &m.Search.Caret, "", action)
		if !edited || text == m.Filters.Search {
			return m, nil
		}
		return searchEdit(m, text)
	}
}

func searchEdit(m Model, text string) (Model, []Cmd) {
	m.Search.Refused = false
	if m.Search.Meaning {
		return meaningTyped(m, text)
	}
	m.Filters.Search = text
	m.SelectedUID = ""
	return snapshotChanged(m)
}

func updateFilterText(m Model, msg KeyMsg) (Model, []Cmd) {
	if m.CurrentMode() == ModePageSearch {
		return updatePageSearchText(m, msg)
	}
	if m.CurrentMode() != ModeSearch && m.CurrentMode() != ModeFilterForm {
		return m, nil
	}
	if m.FilterForm.Open {
		if field := filterTable.textAt(&m.FilterForm, m.FilterForm.Field); field != nil {
			*field, _ = editText(*field, &m.FilterForm.Caret, m.FilterForm.Field, false, msg)
		}
		return m, nil
	}
	text, edited := editText(m.Filters.Search, &m.Search.Caret, "", false, msg)
	if !edited { // the caret may still have moved
		return m, nil
	}
	return searchEdit(m, text)
}

type FilterTerm struct {
	Field Field
	Value string
}

func ActiveFilters(f Filters, projectName func(uid string) string) []FilterTerm {
	var terms []FilterTerm
	add := func(field Field, value string) {
		if value != "" {
			terms = append(terms, FilterTerm{Field: field, Value: value})
		}
	}
	if f.ProjectUID != "" {
		add(FilterProject, projectName(f.ProjectUID))
	}
	add(FilterStatus, f.Status)
	add(FilterOwner, f.Owner)
	add(FilterLabel, f.Label)
	add(FilterRelationship, f.Relationship)
	add(FilterSearch, f.Search)
	return terms
}

// DescribeFilters is the filters that are set as name=value words joined by " · ", empty when none is set: how the CLI words them.
func DescribeFilters(f Filters, projectName func(uid string) string) string {
	terms := ActiveFilters(f, projectName)
	parts := make([]string, len(terms))
	for i, term := range terms {
		parts[i] = string(term.Field) + "=" + term.Value
	}
	return strings.Join(parts, " · ")
}

// FilterLine is what the filter line under the top bar shows: the active filters, and a note naming where they came from,
// "view <name>" while they are the saved view's this Pane opened or "default" while they equal the Project's default. The
// Board leaves the status filter out. With no terms and no note there is no line.
func (m Model) FilterLine() (terms []FilterTerm, note string) {
	shown := m.Filters
	if m.OnBoard() {
		shown.Status = ""
	}
	terms = ActiveFilters(shown, m.ProjectFilterName)
	if name := m.CurrentSavedView(); name != "" {
		return terms, "view " + name
	}
	if len(terms) > 0 && m.Filters == m.DefaultFilters {
		note = "default"
	}
	return terms, note
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
