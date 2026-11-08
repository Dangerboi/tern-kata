package model

import "strings"

const (
	FilterProject      Field = "project"
	FilterStatus       Field = "status"
	FilterOwner        Field = "owner"
	FilterLabel        Field = "label"
	FilterRelationship Field = "relationship"
	// FilterSearch is the search text, a field of the View card only; the Filter card edits it in the search input.
	FilterSearch Field = "search"
)

// filterTable declares the Filter form's fields, in tab order, and its busy condition. Every key of the form dismisses its
// error. The Board ignores the status filter, so its card hides that field.
var filterTable = fieldTable[FilterFormState]{
	form:       func(m *Model) *FilterFormState { return &m.FilterForm },
	hidden:     func(m Model, name Field) bool { return !m.FilterFieldShown(name) },
	dismissErr: true,
	fields:     filterFieldList(func(f *FilterFormState) *Filters { return &f.Values }, Model.FilterChoices),
}

var FilterFields = filterTable.specs()

type FilterChoice struct{ Value, Label string }

// The Filter form's fixed choice lists; the empty value first is "no filter".
var (
	FilterStatuses      = []FilterChoice{{"", "all"}, {"open", "open"}, {"closed", "closed"}}
	FilterRelationships = []FilterChoice{{"", "Any"}, {"parent", "Has parent"}, {"child", "Has child"}, {"blocks", "Blocks"}, {"blocked_by", "Blocked by"}, {"related", "Related"}}
)

// FilterChoiceLabel is how the form shows value, the first choice's label for a value not in choices.
func FilterChoiceLabel(choices []FilterChoice, value string) string {
	for _, choice := range choices {
		if choice.Value == value {
			return choice.Label
		}
	}
	return choices[0].Label
}

type FilterFormState struct {
	formCard
	Values Filters
	// Sent is the save the card is waiting on, so only its own result closes or fails the card.
	Sent SaveFilters
}

func (f Filters) Value(name Field) string {
	switch name {
	case FilterProject:
		return f.ProjectUID
	case FilterStatus:
		return f.Status
	case FilterOwner:
		return f.Owner
	case FilterLabel:
		return f.Label
	case FilterRelationship:
		return f.Relationship
	case FilterSearch:
		return f.Search
	}
	return ""
}

func (m Model) FilterChoices(name Field) []string {
	if name == FilterProject {
		return m.projectChoices(m.Collection.ProjectUID)
	}
	return fixedFilterChoices(name)
}

// projectChoices is the Project field's values: empty for no Project, then the UID of every Project in the catalog, or only
// only's when it is not empty.
func (m Model) projectChoices(only string) []string {
	values := []string{""}
	for _, project := range m.Snapshot.Catalog {
		if only == "" || only == project.Project.UID {
			values = append(values, project.Project.UID)
		}
	}
	return values
}

func fixedFilterChoices(name Field) []string {
	var choices []FilterChoice
	switch name {
	case FilterStatus:
		choices = FilterStatuses
	case FilterRelationship:
		choices = FilterRelationships
	}
	values := make([]string, len(choices))
	for i, choice := range choices {
		values[i] = choice.Value
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

func filterFormKey(m Model, msg KeyMsg) (Model, []Cmd) {
	binding, bound := LookupBindingForMode(msg.Chord, ModeFilterForm)
	bound = bound && !(isCaretChord(msg.Chord) && filterTable.textAt(&m.FilterForm, m.FilterForm.Field) != nil)
	if !bound || binding.Action != SaveFilterDefault {
		m.FilterForm.Err = nil
	}
	if bound {
		return updateFilterForm(m, binding.Action)
	}
	return updateFilterText(m, msg)
}

func trimmedFilters(f Filters) Filters {
	f.Owner = strings.TrimSpace(f.Owner)
	f.Label = strings.TrimSpace(f.Label)
	return f
}

func updateFilterForm(m Model, action Action) (Model, []Cmd) {
	switch action {
	case InputSpace:
		if filterTable.textAt(&m.FilterForm, m.FilterForm.Field) != nil {
			return updateFilterText(m, KeyMsg{Text: " "})
		}
		filterTable.stepFocused(&m, false)
		return m, nil
	case SaveFilterDefault:
		return saveFilterDefault(m)
	case ApplyFilters:
		m.Filters = trimmedFilters(m.FilterForm.Values)
		m.FilterForm = FilterFormState{}
		m.SelectedUID = ""
		return snapshotChanged(m)
	case CancelFilters:
		m.FilterForm = FilterFormState{}
	case ResetFilterForm:
		status := m.FilterForm.Values.Status
		m.FilterForm.Values = Filters{}
		if !m.FilterFieldShown(FilterStatus) {
			m.FilterForm.Values.Status = status
		}
	case DeleteInput, ClearInput:
		filterTable.resetFocused(&m)
	}
	return m, nil
}
