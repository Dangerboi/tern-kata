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
	Open, Saving bool
	Field        Field
	Values       Filters
	Caret        Caret
	// Err is why the last save as default did not happen; the card and its values stay.
	Err error
	// Sent is the save the card is waiting on, so only its own result closes or fails the card.
	Sent SaveFilters
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

// Value is the filter value the field called name holds, "" for a name that is not a Filter field.
func (f Filters) Value(name Field) string {
	if value := f.value(name); value != nil {
		return *value
	}
	return ""
}

func filterTextField(form *FilterFormState) *string {
	if !takesText(FilterFields, form.Field) {
		return nil
	}
	return form.Values.value(form.Field)
}

// FilterChoices is the values the choice field called name cycles through, in the order the form shows them; nil for a text
// field.
func (m Model) FilterChoices(name Field) []string {
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

// chooseFilter gives the focused choice field its value at index i in FilterChoices; an index out of range changes nothing.
func chooseFilter(m Model, i int) Model {
	if choices := m.FilterChoices(m.FilterForm.Field); i >= 0 && i < len(choices) {
		*m.FilterForm.Values.value(m.FilterForm.Field) = choices[i]
	}
	return m
}

// filterFormKey is one key for the Filter form on top: a bound chord acts, any other types.
func filterFormKey(m Model, msg KeyMsg) (Model, []Cmd) {
	binding, bound := LookupBindingForMode(msg.Chord, ModeFilterForm)
	bound = bound && !(isCaretChord(msg.Chord) && filterTextField(&m.FilterForm) != nil)
	if m.FilterForm.Saving && !(bound && binding.Action == CancelFilters) {
		return m, nil // the save has not answered: the values stay as sent
	}
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
		if filterTextField(&m.FilterForm) != nil {
			return updateFilterText(m, KeyMsg{Text: " "})
		}
		return updateFilterForm(m, NextFilterChoice)
	case SaveFilterDefault:
		return saveFilterDefault(m)
	case ApplyFilters:
		m.Filters = trimmedFilters(m.FilterForm.Values)
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
		status := m.FilterForm.Values.Status
		m.FilterForm.Values = Filters{}
		if !m.FilterFieldShown(FilterStatus) {
			m.FilterForm.Values.Status = status
		}
	case DeleteInput, ClearInput:
		if field := filterTextField(&m.FilterForm); field != nil {
			*field, _ = editText(*field, &m.FilterForm.Caret, m.FilterForm.Field, Binding{Action: action}, true, KeyMsg{})
		} else if choice := m.FilterForm.Values.value(m.FilterForm.Field); choice != nil {
			*choice = ""
		}
	case NextFilterChoice, PreviousFilterChoice:
		choices := m.FilterChoices(m.FilterForm.Field)
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
