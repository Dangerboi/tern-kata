package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// recurrenceTitles name each field in one or two words: the label sits above its control and the format help is a hint under it.
var recurrenceTitles = map[model.Field]string{
	model.RecurrenceFrequency: "Frequency",
	model.RecurrenceEvery:     "Every",
	model.RecurrenceWeekdays:  "Weekdays",
	model.RecurrenceDay:       "Day of month",
	model.RecurrenceMonth:     "Month",
	model.RecurrenceEnds:      "Ends",
	model.RecurrenceStart:     "Start date",
	model.RecurrenceZone:      "Time zone",
}

// recurrenceHints are the format help lines shown under a text field while it has focus.
var recurrenceHints = map[model.Field]string{
	model.RecurrenceDay: "1-31, last, 2nd tue, last fri",
}

// recurrenceDateHint is the hint of the two date fields: the card reads a plain date only, so it names no other form.
const recurrenceDateHint = "Date like 2026-11-20."

var everyUnits = map[model.Frequency]string{model.Daily: "days", model.Weekly: "weeks", model.Monthly: "months", model.Yearly: "years"}

func recurrenceTitle(f model.RecurrenceFormState, name model.Field) string {
	switch {
	case name == model.RecurrenceEnd && f.Ends == model.EndCount:
		return "Occurrences"
	case name == model.RecurrenceEnd:
		return "End date"
	}
	return recurrenceTitles[name]
}

// isDateField says the field takes a date: the start, and the end when the series ends on a date.
func isDateField(f model.RecurrenceFormState, name model.Field) bool {
	return name == model.RecurrenceStart || (name == model.RecurrenceEnd && f.Ends == model.EndUntil)
}

// recurrenceHint is the one muted line under the focused field, "" when the field has none.
func recurrenceHint(f model.RecurrenceFormState, name model.Field) string {
	if isDateField(f, name) {
		return recurrenceDateHint
	}
	return recurrenceHints[name]
}

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
	overlay := ui.Overlay{Common: ui.Common{Key: layerKey(model.CardRecurrence)}, Modal: true, Size: ui.SizeLG, Icon: ui.IconRepeat}
	if ref := m.RecurrenceRef(); ref != "" {
		overlay.Meta = refMeta(ref)
	}
	if f.Refused {
		overlay.Head = ui.T("Recurrence")
		return ui.Nodes(popup(overlay, []tern.Element{
			toneText("error", ui.ToneError, f.Err.Error()),
			buttonRow(exitButton("cancel", model.ModeRecurrenceForm, model.RecurrenceCancel, "Close"))}))
	}
	overlay.Head = ui.T("Make recurring")
	children := make([]tern.Element, 0, len(model.RecurrenceFields)*3+6)
	if f.Change {
		overlay.Head = ui.T("Change recurrence")
		if f.Raw == "" {
			children = append(children, toneText("summary", ui.ToneMuted, f.Opened.Text()))
		}
	}
	if f.Raw != "" {
		children = append(children, toneText("raw", ui.ToneWarning, "Set outside Tern. Saving replaces it."),
			ui.Text{Common: ui.Common{Key: "raw-rule", Tone: ui.ToneMuted}, Spans: []ui.Span{monoSpan(f.Raw, "muted")}})
	}
	if note := f.ZoneNote(); note != "" {
		children = append(children, toneText("zone-note", ui.ToneWarning, note))
	}
	for _, field := range model.RecurrenceFields {
		if f.Shows(field.Name) {
			children = append(children, recurrenceField(m, f, field)...)
		}
	}
	children = append(children, selectRoom(m, model.CardRecurrence, f.Field)...)
	if f.Err != nil {
		children = append(children, toneText("error", ui.ToneError, model.Cannot("save", "the recurrence", f.Err)))
	}
	save := cardButton("save", model.ModeRecurrenceForm, model.RecurrenceSave, "Save", true)
	cancel := exitButton("cancel", model.ModeRecurrenceForm, model.RecurrenceCancel, "Cancel")
	stop := cardButton("stop", model.ModeRecurrenceForm, model.RecurrenceStop, "Stop recurring", false)
	switch {
	case f.Saving && f.Stopping:
		children = append(children, toneText("pending", ui.TonePending, model.Progress("Stopping", "")))
	case f.Saving:
		children = append(children, toneText("pending", ui.TonePending, model.Progress("Saving", "")))
	}
	switch {
	case f.Saving:
		children = append(children, formFoot(unavailable(save, "Saving"), cancel))
	case f.StopConfirm:
		children = append(children, toneText("question", ui.ToneWarning, "Stop this Issue recurring? Existing occurrences stay."),
			confirmFoot(cardButton("stop-yes", model.ModeRecurrenceStop, model.RecurrenceStopConfirm, "Stop recurring", false), exitButton("stop-keep", model.ModeRecurrenceStop, model.RecurrenceStopKeep, "Keep editing"), true))
	case f.Change:
		children = append(children, formFoot(save, cancel, stop))
	default:
		children = append(children, formFoot(save, cancel))
	}
	return withCalendar(m, model.CardRecurrence, ui.Nodes(popup(overlay, children)))
}

// recurrenceField is one field of the card: its label above, then its control, then the format hint while the field has focus.
// A choice drawn by Tern carries its own label.
func recurrenceField(m model.Model, f model.RecurrenceFormState, field model.FieldSpec) []tern.Element {
	name, focused := field.Name, field.Name == f.Field
	title := recurrenceTitle(f, name)
	tone := ui.ToneNeutral
	if focused {
		tone = ui.ToneAccent
	}
	switch {
	case name == model.RecurrenceWeekdays:
		labels, chosen, cursor := weekdayChoices(f)
		return []tern.Element{fieldLabel(string(name), title, focused), choiceField(name, tone, title, "", labels, chosen, cursor)}
	case !field.Text:
		return []tern.Element{choiceControl(m, model.CardRecurrence, name, title, focused, "", "")}
	}
	text := recurrenceText(f, name)
	input := ui.Input{
		Common: ui.Common{Key: string(name), Tone: ui.ToneNeutral, Aria: title},
		Field:  ui.Field{Text: text, Cursor: caretAt(f.Caret, name, text), Mode: "insert", Readonly: f.Saving || f.StopConfirm},
	}
	if focused {
		input.Tone = ui.ToneAccent
	}
	if isDateField(f, name) {
		input.Placeholder = ui.T("YYYY-MM-DD")
	}
	if name == model.RecurrenceEvery {
		input.Unit = everyUnits[f.Frequency]
	}
	return labelledInput(string(name), title, recurrenceHint(f, name), focused, input)
}
