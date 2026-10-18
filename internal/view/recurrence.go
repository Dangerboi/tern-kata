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
	model.RecurrenceEvery:     "Every",
	model.RecurrenceWeekdays:  "Weekdays",
	model.RecurrenceDay:       "Day (1-31, last, 2nd tue, last fri)",
	model.RecurrenceMonth:     "Month",
	model.RecurrenceEnds:      "Ends",
	model.RecurrenceEnd:       "End date (YYYY-MM-DD) or count",
	model.RecurrenceStart:     "Start date (YYYY-MM-DD)",
	model.RecurrenceZone:      "Time zone",
}

var endLabels = map[model.EndKind]string{model.EndNever: "Never", model.EndUntil: "On a date", model.EndCount: "After a number of occurrences"}

// recurrenceText is what the card shows in the field called name.
func recurrenceText(f model.RecurrenceFormState, name model.Field) string {
	switch name {
	case model.RecurrenceFrequency:
		return strings.ToUpper(string(f.Frequency[:1])) + strings.ToLower(string(f.Frequency[1:]))
	case model.RecurrenceEvery:
		return f.Every
	case model.RecurrenceWeekdays:
		var b strings.Builder
		for i := range 7 {
			day := time.Weekday((i + 1) % 7)
			mark := "[ ] "
			if f.Weekdays[day] {
				mark = "[x] "
			}
			if name == f.Field && i == f.WeekdayAt {
				b.WriteString("▸")
			}
			b.WriteString(mark + day.String()[:3] + "  ")
		}
		return strings.TrimSpace(b.String())
	case model.RecurrenceDay:
		return f.Day
	case model.RecurrenceMonth:
		return time.Month(f.Month).String()
	case model.RecurrenceEnds:
		return endLabels[f.Ends]
	case model.RecurrenceEnd:
		return f.EndText()
	case model.RecurrenceStart:
		return f.Start
	case model.RecurrenceZone:
		return f.Zone
	}
	return ""
}

func recurrenceLayer(m model.Model) tern.Element {
	f := m.RecurrenceForm
	if !f.Open {
		return nil
	}
	children := make([]tern.Element, 0, len(model.RecurrenceFields)+3)
	for _, field := range model.RecurrenceFields {
		if !f.Shows(field.Name) {
			continue
		}
		tone := ui.ToneNeutral
		title := recurrenceTitles[field.Name]
		prompt := title + ": "
		if field.Name == f.Field {
			tone = ui.ToneAccent
			prompt = "> " + prompt
		}
		placeholder := ""
		if field.Name == model.RecurrenceDay {
			placeholder = "the start date's day"
		}
		children = append(children, ui.Input{
			Common: ui.Common{Key: string(field.Name), Tone: tone, Aria: title},
			Field:  ui.Field{Text: recurrenceText(f, field.Name), Mode: "insert", Readonly: f.Saving || !field.Text, Prompt: []ui.Span{{T: prompt}}, Placeholder: placeholder},
		})
	}
	if f.Err != nil {
		children = append(children, ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: f.Err.Error()})
	}
	if f.Saving {
		children = append(children, ui.Text{Common: ui.Common{Key: "pending", Tone: ui.TonePending}, Text: "Making Issue recur…"})
	} else {
		children = append(children, buttonRow(keyButton("save", "Make recurring (ctrl+o)", "ctrl+o", true), keyButton("cancel", "Cancel (esc)", "escape", false)))
	}
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: "recurrence"}, Modal: true, Size: ui.SizeLG, Head: ui.T("Make recurring"), Children: children})
}
