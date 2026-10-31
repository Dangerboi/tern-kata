package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// calendarLeaf is the key of the date picker's calendar under its overlay.
const calendarLeaf = "calendar"

func calendarLayerKey(card model.Card) string { return layerKey(card) + "-calendar" }

// CalendarID is the node of card's date picker calendar.
func CalendarID(card model.Card) string {
	return "layer." + calendarLayerKey(card) + "." + calendarLeaf
}

// withDatePicker adds the open date picker to the card's layer: a calendar under the date field it picks for, with the day the
// keys are on ringed, the field's day selected and the card's other date marked.
func withDatePicker(m model.Model, card model.Card, layer tern.Nodes) tern.Element {
	picker, ok := m.DatePicker()
	if !ok || picker.Card != card {
		return layer
	}
	cal := ui.Calendar{
		Common:    ui.Common{Key: calendarLeaf, Aria: "Calendar"},
		Month:     picker.Cursor[:7],
		Today:     picker.Today,
		WeekStart: ui.WeekMon,
		Cursor:    picker.Cursor,
	}
	if picker.Value != "" {
		cal.Value = ui.Day(picker.Value)
	}
	if picker.Other != "" {
		cal.Marks = []ui.DayMark{{Date: picker.Other}}
	}
	return append(layer, ui.Overlay{Common: ui.Common{Key: calendarLayerKey(card)}, Anchor: ui.AtNode(FieldID(card, picker.Field), "below"), Size: ui.SizeSM, Children: ui.Nodes(cal)})
}

// calendarHint is the muted line under the focused date field that names the key opening its calendar.
func calendarHint(key string) tern.Element {
	return toneText(key+"-calendar-hint", ui.ToneMuted, "ctrl+d opens a calendar")
}
