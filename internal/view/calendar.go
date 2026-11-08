package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const calendarLeaf = "calendar"

func calendarLayerKey(card model.Card) string { return layerKey(card) + "-calendar" }

func CalendarID(card model.Card) string {
	return "layer." + calendarLayerKey(card) + "." + calendarLeaf
}

func withCalendar(m model.Model, card model.Card, layer tern.Nodes) tern.Element {
	shown, ok := m.ShownCalendar()
	if !ok || shown.Card != card {
		return layer
	}
	cal := ui.Calendar{
		Common:    ui.Common{Key: calendarLeaf, Aria: "Calendar"},
		Month:     shown.Cursor[:7],
		Today:     shown.Today,
		WeekStart: ui.WeekMon,
		Cursor:    shown.Cursor,
	}
	if shown.Value != "" {
		cal.Value = ui.Day(shown.Value)
	}
	if shown.Other != "" {
		cal.Marks = []ui.DayMark{{Date: shown.Other}}
	}
	return append(layer, ui.Overlay{Common: ui.Common{Key: calendarLayerKey(card)}, Anchor: ui.AtNode(FieldID(card, shown.Field), "below"), Size: ui.SizeSM, Children: ui.Nodes(cal)})
}

func calendarKey(mode model.Mode) string {
	if keys := model.CardKeys(mode, model.ShowCalendar); len(keys) > 0 {
		return keys[0].Key
	}
	return ""
}

func calendarHint(key string, mode model.Mode) tern.Element {
	return toneText(key+"-calendar-hint", ui.ToneMuted, calendarKey(mode)+" opens a calendar")
}
