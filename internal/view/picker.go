package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const (
	PickerID       = "layer.collections"
	PickerFilterID = PickerID + ".filter"
	PickerListID   = PickerID + ".list"
)

func pickerLayer(m model.Model) tern.Element {
	if !m.Picker.Open {
		return nil
	}
	entries := m.PickerEntries()
	items := make([]tern.Element, 0, len(entries))
	for _, entry := range entries {
		item := ui.Item{Common: ui.Common{Key: entry.Key()}, Label: ui.T(entry.Title)}
		if !entry.NewProject && entry.Collection == m.Collection {
			item.Value = ui.T("current")
		}
		items = append(items, item)
	}
	selected := ""
	if len(entries) > 0 {
		selected = PickerItemID(entries[min(m.Picker.Cursor, len(entries)-1)].Key())
	}
	// esc goes Back on the Projects level, so the button says so, as the footer does.
	closeLabel := "Close (esc)"
	if m.Picker.Projects && m.Picker.Filter == "" {
		closeLabel = "Back (esc)"
	}
	empty := "No Menu entries match."
	if len(m.Collections) == 0 {
		empty = "Loading Menu…"
	}
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: "collections"}, Head: ui.T("Menu"), Modal: true, Size: ui.SizeMD, Children: []tern.Element{
		ui.Input{Common: ui.Common{Key: "filter", Tone: ui.ToneAccent, Aria: "Filter Menu"}, Field: ui.Field{Text: m.Picker.Filter, Mode: "insert", Placeholder: "Filter Menu"}},
		ui.List{CommonWithoutMax: ui.CommonWithoutMax{Key: "list"}, Selected: selected, Empty: ui.T(empty), Max: ui.MaxLines(12), Children: items},
		keyButton("close", closeLabel, "escape", false),
	}})
}
