package model

import (
	"slices"
	"strings"
	"time"
)

var recurrenceEndLabels = map[EndKind]string{EndNever: "Never", EndUntil: "On a date", EndCount: "After a number of occurrences"}

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

// filterControl is the status and the relationship as a segmented row: both are short closed sets, and the Relationship is the
// last field of the card, where a select's menu would be clipped by the card body after its first option.
func filterControl(name Field) Control {
	if name == FilterStatus || name == FilterRelationship {
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
	if name == ViewBase || name == FilterProject {
		control = segmentedOrSelect(len(labels))
	}
	return newChoices(control, labels, slices.Index(choices, m.ViewForm.choiceValue(name))), true
}

func jobControl(name Field, n int) Control {
	switch name {
	case JobFieldKind, JobFieldTrigger, JobFieldPolicy, JobFieldOverlap, JobFieldCatchup:
		return ControlSelect
	}
	return segmentedOrSelect(n)
}

func jobChoiceSet(m Model, name Field) (ChoiceSet, bool) {
	if name == JobFieldEnabled {
		return yesNoSwitch(m.JobForm.Enabled), true
	}
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
	return newChoices(jobControl(name, len(choices)), labels, chosen), true
}

func recurrenceChoiceSet(m Model, name Field) (ChoiceSet, bool) {
	f := m.RecurrenceForm
	var labels []string
	var chosen int
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
	return newChoices(ControlTabs, []string{"This machine", "The Project"}, boolIndex(m.LanesForm.Share)), true
}

func workerChoiceSet(m Model, name Field) (ChoiceSet, bool) {
	if name != WorkerFieldTrust {
		return ChoiceSet{}, false
	}
	return yesNoSwitch(m.WorkerCard.Trusted), true
}
