package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

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
// weekdayChoices are the Weekdays field's words: it takes several values at once, so it stays a row of toggles.
func weekdayChoices(f model.RecurrenceFormState) (labels []string, chosen func(int) bool, cursor int) {
	for _, day := range model.MondayFirst {
		labels = append(labels, day.String()[:3])
	}
	cursor = -1
	if f.Field == model.RecurrenceWeekdays {
		cursor = f.WeekdayAt
	}
	return labels, func(i int) bool { return f.Weekdays[model.MondayFirst[i]] }, cursor
}

func recurrenceLayer(m model.Model) tern.Element {
	f := m.RecurrenceForm
	if !f.Open {
		return nil
	}
	children := make([]tern.Element, 0, len(model.RecurrenceFields)+4)
	if f.Refused {
		return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardRecurrence)}, Modal: true, Size: ui.SizeLG, Head: ui.T("Recurrence"), Icon: ui.IconRepeat}, ui.Nodes(
			toneText("error", ui.ToneError, f.Err.Error()),
			buttonRow(cardButton("cancel", model.ModeRecurrenceForm, model.RecurrenceCancel, "Close", true)))))
	}
	if f.Raw != "" {
		children = append(children, toneText("raw", ui.ToneWarning, "Current rule, set elsewhere: "+f.Raw+"\nSaving the form replaces it."))
	}
	if note := f.ZoneNote(); note != "" {
		children = append(children, toneText("zone-note", ui.ToneWarning, note))
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
		if field.Name == model.RecurrenceWeekdays {
			labels, chosen, cursor := weekdayChoices(f)
			children = append(children, choiceField(field.Name, tone, title, prompt, labels, chosen, cursor))
			continue
		}
		if !field.Text {
			children = append(children, choiceControl(m, model.CardRecurrence, field.Name, title, field.Name == f.Field, "", ""))
			continue
		}
		children = append(children, ui.Input{
			Common: ui.Common{Key: string(field.Name), Tone: tone, Aria: title},
			Field:  ui.Field{Text: recurrenceText(f, field.Name), Cursor: caretAt(f.Caret, field.Name, recurrenceText(f, field.Name)), Mode: "insert", Readonly: f.Saving || f.StopConfirm, Prompt: []ui.Span{{T: prompt}}},
		})
		if field.Name == f.Field && (field.Name == model.RecurrenceStart || (field.Name == model.RecurrenceEnd && f.Ends == model.EndUntil)) {
			children = append(children, calendarHint(string(field.Name)))
		}
	}
	if f.Err != nil {
		children = append(children, toneText("error", ui.ToneError, f.Err.Error()))
	}
	title, save, pending := "Make recurring", "Make recurring", "Making Issue recur…"
	if f.Change {
		title, save, pending = "Change recurrence", "Save", "Saving the schedule…"
	}
	switch {
	case f.Saving && f.Stopping:
		children = append(children, toneText("pending", ui.TonePending, "Stopping the recurrence…"))
	case f.Saving:
		children = append(children, toneText("pending", ui.TonePending, pending))
	case f.StopConfirm:
		children = append(children, toneText("stop", ui.ToneWarning, "Stop this Issue recurring? Existing occurrences stay. y stops; n or esc keeps editing."),
			buttonRow(cardButton("stop-yes", model.ModeRecurrenceStop, model.RecurrenceStopConfirm, "Stop recurring", false), cardButton("stop-keep", model.ModeRecurrenceStop, model.RecurrenceStopKeep, "Keep editing", true)))
	case f.Change:
		children = append(children, buttonRow(cardButton("save", model.ModeRecurrenceForm, model.RecurrenceSave, save, true), cardButton("stop", model.ModeRecurrenceForm, model.RecurrenceStop, "Stop recurring", false), cardButton("cancel", model.ModeRecurrenceForm, model.RecurrenceCancel, "Cancel", false)))
	default:
		children = append(children, buttonRow(cardButton("save", model.ModeRecurrenceForm, model.RecurrenceSave, save, true), cardButton("cancel", model.ModeRecurrenceForm, model.RecurrenceCancel, "Cancel", false)))
	}
	return withDatePicker(m, model.CardRecurrence, ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardRecurrence)}, Modal: true, Size: ui.SizeLG, Head: ui.T(title), Icon: ui.IconRepeat}, children)))
}
