package model

import "strings"

// The Filter form's fields.
const (
	FilterProject      Field = "project"
	FilterStatus       Field = "status"
	FilterOwner        Field = "owner"
	FilterLabel        Field = "label"
	FilterRelationship Field = "relationship"
)

// FilterFields is the Filter form's field list, in tab order.
var FilterFields = []FieldSpec{{FilterProject, false}, {FilterStatus, false}, {FilterOwner, true}, {FilterLabel, true}, {FilterRelationship, false}}

// FilterChoice is one value a Filter form choice field cycles through and how the form shows it.
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
	Open   bool
	Field  Field
	Values Filters
}

// value is the filter value the field called name holds, nil for a name that is not a Filter field.
func (f *Filters) value(name Field) *string {
	switch name {
	case FilterProject:
		return &f.ProjectUID
	case FilterStatus:
		return &f.Status
	case FilterOwner:
		return &f.Owner
	case FilterLabel:
		return &f.Label
	case FilterRelationship:
		return &f.Relationship
	}
	return nil
}

func filterTextField(form *FilterFormState) *string {
	if !takesText(FilterFields, form.Field) {
		return nil
	}
	return form.Values.value(form.Field)
}

// filterChoices is the values the choice field called name cycles through, nil for a text field.
func (m Model) filterChoices(name Field) []string {
	var choices []FilterChoice
	switch name {
	case FilterProject:
		values := []string{""}
		for _, project := range m.Snapshot.Catalog {
			if m.Collection.ProjectUID == "" || m.Collection.ProjectUID == project.Project.UID {
				values = append(values, project.Project.UID)
			}
		}
		return values
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

// filterFormKey is one key for the Filter form on top: a bound chord acts, any other types.
func filterFormKey(m Model, msg KeyMsg) (Model, []Cmd) {
	if binding, ok := LookupBindingForMode(msg.Chord, ModeFilterForm); ok {
		return updateFilterForm(m, binding.Action)
	}
	return updateFilterText(m, msg)
}

func updateFilterForm(m Model, action Action) (Model, []Cmd) {
	switch action {
	case InputSpace:
		if filterTextField(&m.FilterForm) != nil {
			return updateFilterText(m, KeyMsg{Text: " "})
		}
		return updateFilterForm(m, NextFilterChoice)
	case ApplyFilters:
		m.Filters = m.FilterForm.Values
		m.Filters.Owner = strings.TrimSpace(m.Filters.Owner)
		m.Filters.Label = strings.TrimSpace(m.Filters.Label)
		m.FilterForm = FilterFormState{}
		m.SelectedUID = ""
		return snapshotChanged(m)
	case CancelFilters:
		m.FilterForm = FilterFormState{}
	case NextFilterField:
		cycleField(&m, 1)
	case PreviousFilterField:
		cycleField(&m, -1)
	case ResetFilterForm:
		m.FilterForm.Values = Filters{}
	case DeleteInput, ClearInput:
		if field := filterTextField(&m.FilterForm); field != nil {
			*field, _ = editText(*field, Binding{Action: action}, true, KeyMsg{})
		} else if choice := m.FilterForm.Values.value(m.FilterForm.Field); choice != nil {
			*choice = ""
		}
	case NextFilterChoice, PreviousFilterChoice:
		choices := m.filterChoices(m.FilterForm.Field)
		if choices == nil {
			return m, nil
		}
		field := m.FilterForm.Values.value(m.FilterForm.Field)
		index := 0
		for i, value := range choices {
			if value == *field {
				index = i
				break
			}
		}
		step := 1
		if action == PreviousFilterChoice {
			step = len(choices) - 1
		}
		*field = choices[(index+step)%len(choices)]
	}
	return m, nil
}
