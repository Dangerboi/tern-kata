package view

import (
	"strconv"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

var PickerListID = LayerID(model.CardPicker) + ".list"

// pickerHeads are the card's head at each Menu level.
var pickerHeads = map[model.MenuLevel]string{model.MenuTop: "Menu", model.MenuViews: model.ViewsTitle, model.MenuProjects: model.ProjectsTitle}

func pickerLayer(m model.Model) tern.Element {
	if !m.Picker.Open {
		return nil
	}
	entries := m.PickerEntries()
	items := make([]tern.Element, 0, len(entries)+3)
	section := ""
	for _, entry := range entries {
		// The top level groups its rows under a rule per Section; the first group has no label.
		if m.Picker.Level == model.MenuTop && m.Picker.Filter == "" && entry.Section != section {
			section = entry.Section
			items = append(items, ui.Rule{Common: ui.Common{Key: "section." + section}, Label: ui.T(section)})
		}
		item := ui.Item{Common: ui.Common{Key: entry.Key()}, Label: ui.T(m.EntryTitle(entry))}
		if entry.Hint != "" {
			item.Hint = []string{entry.Hint}
		}
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
		if entry.Kind == model.EntryNotice {
			if notice, ok := m.NoticeOf(model.VersionNoticeKind(entry.Index)); ok {
				item.Value = ui.T(notice.Action)
			}
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
	empty := "Loading…"
	if len(m.Collections) > 0 {
		empty = "Nothing matches " + strconv.Quote(m.Picker.Filter) + "."
	}
	notes := []tern.Element{}
	if q, ok := m.Question(); ok {
		notes = append(notes, question(q))
	}
	if text, failed := m.ViewsNote(); text != "" {
		tone := ui.ToneWarning
		if failed {
			tone = ui.ToneError
		}
		notes = append(notes, toneText("views-note", tone, text))
	}
	children := []tern.Element{
		ui.Input{Common: ui.Common{Key: inputLeaf, Tone: ui.ToneAccent, Aria: "Filter"}, Field: ui.Field{Text: m.Picker.Filter, Cursor: caretAt(m.Picker.Caret, "", m.Picker.Filter), Mode: "insert", Placeholder: ui.T("Filter")}, Clear: true},
		ui.List{CommonWithoutMax: ui.CommonWithoutMax{Key: "list"}, Selected: selected, Empty: ui.T(empty), Max: ui.MaxLines(12), Children: items},
	}
	children = append(children, notes...)
	children = append(children, buttonRow(exitButton("close", model.ModePicker, model.ClosePicker, closeLabel)))
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardPicker)}, Head: ui.T(pickerHeads[m.Picker.Level]), Modal: true, Size: ui.SizeMD}, children))
}
