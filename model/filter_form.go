package model

import "strings"

// filterFieldCount is the Filter form's fields: project, status, owner, label and relationship.
const filterFieldCount = 5

type FilterFormState struct {
	Open   bool
	Field  int
	Values Filters
}

func filterTextField(form *FilterFormState) *string {
	switch form.Field {
	case 2:
		return &form.Values.Owner
	case 3:
		return &form.Values.Label
	}
	return nil
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
		} else {
			switch m.FilterForm.Field {
			case 0:
				m.FilterForm.Values.ProjectUID = ""
			case 1:
				m.FilterForm.Values.Status = ""
			case 4:
				m.FilterForm.Values.Relationship = ""
			}
		}
	case NextFilterChoice, PreviousFilterChoice:
		var field *string
		var choices []string
		switch m.FilterForm.Field {
		case 0:
			field = &m.FilterForm.Values.ProjectUID
			choices = []string{""}
			for _, project := range m.Snapshot.Catalog {
				if m.Collection.ProjectUID == "" || m.Collection.ProjectUID == project.Project.UID {
					choices = append(choices, project.Project.UID)
				}
			}
		case 1:
			field, choices = &m.FilterForm.Values.Status, []string{"", "open", "closed"}
		case 4:
			field, choices = &m.FilterForm.Values.Relationship, []string{"", "parent", "child", "blocks", "blocked_by", "related"}
		default:
			return m, nil
		}
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
