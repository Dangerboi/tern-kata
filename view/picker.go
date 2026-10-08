package view

import (
	"github.com/Dangerboi/tern-kata/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const (
	PickerID       = "layer.collections"
	PickerFilterID = PickerID + ".filter"
	PickerListID   = PickerID + ".list"
)

// PickerCursorID is the row under the picker cursor, or "" when nothing is listed.
func PickerCursorID(m model.Model) string {
	entries := m.PickerEntries()
	if !m.Picker.Open || len(entries) == 0 {
		return ""
	}
	return PickerListID + "." + entries[min(m.Picker.Cursor, len(entries)-1)].Key()
}

func pickerLayer(m model.Model) tern.Element {
	if !m.Picker.Open {
		return nil
	}
	entries := m.PickerEntries()
	items := make([]tern.Element, 0, len(entries))
	for _, collection := range entries {
		item := ui.Item{Common: ui.Common{Key: collection.Key()}, Label: ui.T(collection.Title)}
		if collection == m.Collection {
			item.Value = ui.T("current")
		}
		items = append(items, item)
	}
	empty := "No Collections match."
	if len(m.Collections) == 0 {
		empty = "Loading Collections…"
	}
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: "collections"}, Head: ui.T("Collections"), Modal: true, Size: ui.SizeMD, Children: []tern.Element{
		ui.Input{Common: ui.Common{Key: "filter", Tone: ui.ToneAccent, Aria: "Filter Collections"}, Field: ui.Field{Text: m.Picker.Filter, Mode: "insert", Placeholder: "Filter Collections"}},
		ui.List{CommonWithoutMax: ui.CommonWithoutMax{Key: "list"}, Selected: PickerCursorID(m), Empty: ui.T(empty), Max: ui.MaxLines(12), Children: items},
	}})
}
