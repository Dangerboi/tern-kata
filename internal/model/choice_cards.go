package model

import (
	"slices"
	"strings"
	"time"
)

// recurrenceEndLabels are the words of the Ends field's choices.
var recurrenceEndLabels = map[EndKind]string{EndNever: "Never", EndUntil: "On a date", EndCount: "After a number of occurrences"}

// filterChoiceText is how the Filter card shows value of the choice field called name.
func (m Model) filterChoiceText(name Field, value string) string {
	switch name {
	case FilterProject:
		if value != "" {
			return m.ProjectFilterName(value)
		}
		return "Any Project in this Collection"
	case FilterStatus:
		return FilterChoiceLabel(FilterStatuses, value)
	}
	return FilterChoiceLabel(FilterRelationships, value)
}

// filterControl is the control of a Filter choice field: Status is tabs, Project and Relationship are selects.
func filterControl(name Field) Control {
	if name == FilterStatus {
		return ControlTabs
	}
	return ControlSelect
}

func filterChoiceSet(m Model, name Field) (ChoiceSet, bool) {
	choices := m.FilterChoices(name)
	if len(choices) == 0 {
		return ChoiceSet{}, false
	}
	labels := make([]string, len(choices))
	for i, choice := range choices {
		labels[i] = m.filterChoiceText(name, choice)
	}
	return newChoices(filterControl(name), labels, slices.Index(choices, m.FilterForm.Values.Value(name))), true
}

// viewChoiceText is how the View card shows value of the choice field called name.
func (m Model) viewChoiceText(name Field, value string) string {
	if name != ViewBase {
		if name == FilterProject && value == "" {
			return "Any Project"
		}
		return m.filterChoiceText(name, value)
	}
	for _, base := range m.ViewBases() {
		if base.Key() == value {
			if base.IsProject() {
				return "Project " + base.Title
			}
			return base.Title
		}
	}
	return value
}

func viewChoiceSet(m Model, name Field) (ChoiceSet, bool) {
	choices := m.ViewChoices(name)
	if len(choices) == 0 {
		return ChoiceSet{}, false
	}
	labels := make([]string, len(choices))
	for i, choice := range choices {
		labels[i] = m.viewChoiceText(name, choice)
	}
	control := filterControl(name)
	if name == ViewBase {
		control = segmentedOrSelect(len(labels))
	}
	return newChoices(control, labels, slices.Index(choices, m.ViewForm.choiceValue(name))), true
}

// jobControl is the control of a Job choice field: the named ones are selects, Enabled is a switch and any other follows the
// number of its choices.
func jobControl(name Field, n int) Control {
	switch name {
	case JobFieldKind, JobFieldTrigger, JobFieldPolicy, JobFieldOverlap, JobFieldCatchup:
		return ControlSelect
	case JobFieldEnabled:
		return ControlSwitch
	}
	return segmentedOrSelect(n)
}

func jobChoiceSet(m Model, name Field) (ChoiceSet, bool) {
	choices := m.JobForm.JobChoices(name)
	if len(choices) == 0 && !slices.ContainsFunc(JobFields, func(f FieldSpec) bool { return f.Name == name && !f.Text }) {
		return ChoiceSet{}, false
	}
	labels := make([]string, len(choices))
	chosen := -1
	for i, choice := range choices {
		labels[i] = choice.Label
		if choice.Chosen {
			chosen = i
		}
	}
	set := newChoices(jobControl(name, len(choices)), labels, chosen)
	if name == JobFieldEnabled {
		set.OnIndex = 0 // the choices are Yes, then No
	}
	return set, true
}

func recurrenceChoiceSet(m Model, name Field) (ChoiceSet, bool) {
	f := m.RecurrenceForm
	var labels []string
	chosen := -1
	switch name {
	case RecurrenceFrequency:
		for _, frequency := range Frequencies {
			labels = append(labels, strings.ToUpper(string(frequency[:1]))+strings.ToLower(string(frequency[1:])))
		}
		chosen = slices.Index(Frequencies, f.Frequency)
	case RecurrenceMonth:
		for month := time.January; month <= time.December; month++ {
			labels = append(labels, month.String()[:3])
		}
		chosen = f.Month - 1
	case RecurrenceEnds:
		for _, kind := range RecurrenceEndKinds {
			labels = append(labels, recurrenceEndLabels[kind])
		}
		chosen = slices.Index(RecurrenceEndKinds, f.Ends)
	default:
		return ChoiceSet{}, false
	}
	return newChoices(ControlSelect, labels, chosen), true
}

func lanesChoiceSet(m Model, name Field) (ChoiceSet, bool) {
	if name != LanesShare {
		return ChoiceSet{}, false
	}
	chosen := 0
	if m.LanesForm.Share {
		chosen = 1
	}
	return newChoices(ControlTabs, []string{"This machine", "The Project"}, chosen), true
}

func workerChoiceSet(m Model, name Field) (ChoiceSet, bool) {
	if name != WorkerFieldTrust {
		return ChoiceSet{}, false
	}
	chosen := 0
	if m.WorkerCard.Trusted {
		chosen = 1
	}
	set := newChoices(ControlSwitch, []string{"No", "Yes"}, chosen)
	set.OnIndex = 1
	return set, true
}
