package model

import (
	"strings"
	"unicode/utf8"
)

type Filters struct {
	ProjectUID, Status, Owner, Label, Relationship, Search string
}

type SearchState struct {
	Open, Results           bool
	InitialText, InitialUID string
}

func inputMode(mode Mode) bool {
	return mode == ModeSearch || mode == ModeSearchResults || mode == ModeFilterForm || mode == ModePicker
}

func updateFilterAction(m Model, action Action) (Model, []Cmd) {
	if m.FilterForm.Open {
		return updateFilterForm(m, action)
	}
	switch action {
	case ClearFilters:
		m.Filters = Filters{}
		m.SelectedUID = ""
		return snapshotChanged(m)
	case OpenFilterForm:
		m.FilterForm = FilterFormState{Open: true, Values: m.Filters}
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
	case DeleteInput:
		m.Filters.Search = deleteLastRune(m.Filters.Search)
		m.SelectedUID = ""
		return snapshotChanged(m)
	case ClearInput:
		m.Filters.Search = ""
		m.SelectedUID = ""
		return snapshotChanged(m)
	}
	return m, nil
}

func deleteLastRune(text string) string {
	_, size := utf8.DecodeLastRuneInString(text)
	return text[:len(text)-size]
}

func updateFilterText(m Model, msg KeyMsg) (Model, []Cmd) {
	if m.CurrentMode() != ModeSearch && m.CurrentMode() != ModeFilterForm {
		return m, nil
	}
	text := msg.Text
	if text == "" && utf8.RuneCountInString(msg.Chord) == 1 {
		text = msg.Chord
	}
	if msg.Chord == "space" {
		text = " "
	}
	if text == "" {
		return m, nil
	}
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\n", " "), "\r", " ")
	if m.FilterForm.Open {
		if field := filterTextField(&m.FilterForm); field != nil {
			*field += text
		}
		return m, nil
	}
	m.Filters.Search += text
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
