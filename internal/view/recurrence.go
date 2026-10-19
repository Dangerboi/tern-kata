package view

import (
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// RecurrenceFieldID is the id of the Recurrence card's field called name.
func RecurrenceFieldID(name model.Field) string { return "layer.recurrence." + string(name) }

var recurrenceTitles = map[model.Field]string{
	model.RecurrenceFrequency: "Frequency",
	model.RecurrenceWeekdays:  "Weekdays",
	model.RecurrenceDay:       "Day (1-31, last, 2nd tue, last fri)",
	model.RecurrenceMonth:     "Month",
	model.RecurrenceEnds:      "Ends",
	model.RecurrenceStart:     "Start date (YYYY-MM-DD)",
	model.RecurrenceZone:      "Time zone",
}

// everyUnits is what the Every field counts at each frequency.
var everyUnits = map[model.Frequency]string{model.Daily: "days", model.Weekly: "weeks", model.Monthly: "months", model.Yearly: "years"}

var endLabels = map[model.EndKind]string{model.EndNever: "Never", model.EndUntil: "On a date", model.EndCount: "After a number of occurrences"}

// recurrenceTitle is the label of the field called name as the card stands: Every names what it counts, and the end field
// asks for a date or a count by the end chosen.
func recurrenceTitle(f model.RecurrenceFormState, name model.Field) string {
	switch {
	case name == model.RecurrenceEvery:
		return "Every (" + everyUnits[f.Frequency] + ")"
	case name == model.RecurrenceEnd && f.Ends == model.EndCount:
		return "Occurrences"
	case name == model.RecurrenceEnd:
		return "End date (YYYY-MM-DD)"
	}
	return recurrenceTitles[name]
}

// recurrenceText is what the card shows in the text field called name.
func recurrenceText(f model.RecurrenceFormState, name model.Field) string {
	switch name {
	case model.RecurrenceEvery:
		return f.Every
	case model.RecurrenceDay:
		return f.Day
	case model.RecurrenceEnd:
		return f.EndText()
	case model.RecurrenceStart:
		return f.Start
	case model.RecurrenceZone:
		return f.Zone
	}
	return ""
}

// recurrenceChoices is the choice field called name: its choices' labels in the order the model picks them by index, which
// are chosen, and the choice under the keys' cursor, -1 for none.
func recurrenceChoices(f model.RecurrenceFormState, name model.Field) (labels []string, chosen func(int) bool, cursor int) {
	switch name {
	case model.RecurrenceFrequency:
		for _, frequency := range model.Frequencies {
			labels = append(labels, strings.ToUpper(string(frequency[:1]))+strings.ToLower(string(frequency[1:])))
		}
		return labels, func(i int) bool { return model.Frequencies[i] == f.Frequency }, -1
	case model.RecurrenceWeekdays:
		for _, day := range model.MondayFirst {
			labels = append(labels, day.String()[:3])
		}
		cursor = -1
		if f.Field == name {
			cursor = f.WeekdayAt
		}
		return labels, func(i int) bool { return f.Weekdays[model.MondayFirst[i]] }, cursor
	case model.RecurrenceMonth:
		for month := time.January; month <= time.December; month++ {
			labels = append(labels, month.String()[:3])
		}
		return labels, func(i int) bool { return i+1 == f.Month }, -1
	}
	for _, kind := range model.RecurrenceEndKinds {
		labels = append(labels, endLabels[kind])
	}
	return labels, func(i int) bool { return model.RecurrenceEndKinds[i] == f.Ends }, -1
}

func recurrenceLayer(m model.Model) tern.Element {
	f := m.RecurrenceForm
	if !f.Open {
		return nil
	}
	children := make([]tern.Element, 0, len(model.RecurrenceFields)+4)
	if f.Refused {
		return ui.Nodes(ui.Overlay{Common: ui.Common{Key: "recurrence"}, Modal: true, Size: ui.SizeLG, Head: ui.T("Recurrence"), Children: ui.Nodes(
			ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: f.Err.Error()},
			buttonRow(cardButton("cancel", model.ModeRecurrenceForm, model.RecurrenceCancel, "Close", true)))})
	}
	if f.Raw != "" {
		children = append(children, ui.Text{Common: ui.Common{Key: "raw", Tone: ui.ToneWarning}, Text: "Current rule, set elsewhere: " + f.Raw + "\nSaving the form replaces it."})
	}
	for _, field := range model.RecurrenceFields {
		if !f.Shows(field.Name) {
			continue
		}
		tone := ui.ToneNeutral
		title := recurrenceTitle(f, field.Name)
		prompt := title + ": "
		if field.Name == f.Field {
			tone = ui.ToneAccent
			prompt = "> " + prompt
		}
		if !field.Text {
			labels, chosen, cursor := recurrenceChoices(f, field.Name)
			children = append(children, choiceField(field.Name, tone, title, prompt, labels, chosen, cursor))
			continue
		}
		children = append(children, ui.Input{
			Common: ui.Common{Key: string(field.Name), Tone: tone, Aria: title},
			Field:  ui.Field{Text: recurrenceText(f, field.Name), Mode: "insert", Readonly: f.Saving || f.StopConfirm, Prompt: []ui.Span{{T: prompt}}},
		})
	}
	if f.Err != nil {
		children = append(children, ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: f.Err.Error()})
	}
	title, save, pending := "Make recurring", "Make recurring", "Making Issue recur…"
	if f.Change {
		title, save, pending = "Change recurrence", "Save", "Saving the schedule…"
	}
	switch {
	case f.Saving && f.Stopping:
		children = append(children, ui.Text{Common: ui.Common{Key: "pending", Tone: ui.TonePending}, Text: "Stopping the recurrence…"})
	case f.Saving:
		children = append(children, ui.Text{Common: ui.Common{Key: "pending", Tone: ui.TonePending}, Text: pending})
	case f.StopConfirm:
		children = append(children, ui.Text{Common: ui.Common{Key: "stop", Tone: ui.ToneWarning}, Text: "Stop this Issue recurring? Existing occurrences stay. y stops; n or esc keeps editing."},
			buttonRow(cardButton("stop-yes", model.ModeRecurrenceStop, model.RecurrenceStopConfirm, "Stop recurring", false), cardButton("stop-keep", model.ModeRecurrenceStop, model.RecurrenceStopKeep, "Keep editing", true)))
	case f.Change:
		children = append(children, buttonRow(cardButton("save", model.ModeRecurrenceForm, model.RecurrenceSave, save, true), cardButton("stop", model.ModeRecurrenceForm, model.RecurrenceStop, "Stop recurring", false), cardButton("cancel", model.ModeRecurrenceForm, model.RecurrenceCancel, "Cancel", false)))
	default:
		children = append(children, buttonRow(cardButton("save", model.ModeRecurrenceForm, model.RecurrenceSave, save, true), cardButton("cancel", model.ModeRecurrenceForm, model.RecurrenceCancel, "Cancel", false)))
	}
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: "recurrence"}, Modal: true, Size: ui.SizeLG, Head: ui.T(title), Children: children})
}
