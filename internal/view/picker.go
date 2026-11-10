package view

import (
	"strconv"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

var PickerListID = LayerID(model.CardPicker) + ".list"

func pickerLayer(m model.Model) tern.Element {
	if !m.Picker.Open {
		return nil
	}
	entries := m.PickerEntries()
	items := make([]tern.Element, 0, len(entries))
	for _, entry := range entries {
		item := ui.Item{Common: ui.Common{Key: entry.Key()}, Label: ui.T(m.EntryTitle(entry))}
		if entry.Kind == model.EntrySavedView && entry.Title == m.CurrentSavedView() {
			item.Value = ui.T("current")
		}
		if entry.Kind == model.EntryLettered && entry.Page == m.Page || entry.IsCollection() && entry.Collection == m.Collection {
			item.Value = ui.T("current")
		}
		if entry.Kind == model.EntryViews && m.CurrentView() != "" {
			item.Value = ui.T(m.CurrentView())
		}
		if entry.Kind == model.EntryViews && m.CurrentSavedView() != "" {
			item.Value = ui.T(m.CurrentSavedView())
		}
		items = append(items, item)
	}
	selected := ""
	if len(entries) > 0 {
		selected = PickerItemID(entries[min(m.Picker.Cursor, len(entries)-1)].Key())
	}
	// esc goes Back on the Views and Projects levels, so the button says so, as the footer does.
	closeLabel := "Close"
	if m.Picker.Level != model.MenuTop && m.Picker.Filter == "" {
		closeLabel = "Back"
	}
	empty := "No Menu entries match."
	if len(m.Collections) == 0 {
		empty = "Loading Menu…"
	}
	notes := []tern.Element{}
	if m.Views.Deleting != "" {
		notes = append(notes, toneText("delete-view", ui.ToneAccent, model.DeleteViewQuestion(m.Views.Deleting)))
	}
	if m.Views.Err != nil {
		notes = append(notes, toneText("views-error", ui.ToneError, "Saved views not read: "+m.Views.Err.Error()))
	}
	for i, problem := range m.Views.Problems {
		notes = append(notes, toneText("views-problem-"+strconv.Itoa(i), ui.ToneWarning, "Saved view left out: "+problem.Error()))
	}
	children := []tern.Element{
		ui.Input{Common: ui.Common{Key: inputLeaf, Tone: ui.ToneAccent, Aria: "Filter Menu"}, Field: ui.Field{Text: m.Picker.Filter, Cursor: caretAt(m.Picker.Caret, "", m.Picker.Filter), Mode: "insert", Placeholder: ui.T("Filter Menu")}, Clear: true},
		ui.List{CommonWithoutMax: ui.CommonWithoutMax{Key: "list"}, Selected: selected, Empty: ui.T(empty), Max: ui.MaxLines(12), Children: items},
	}
	children = append(children, notes...)
	children = append(children, buttonRow(cardButton("close", model.ModePicker, model.ClosePicker, closeLabel, false)))
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardPicker)}, Head: ui.T("Menu"), Modal: true, Size: ui.SizeMD}, children))
}
