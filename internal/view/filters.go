package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const SearchID = "main.columns.list.search"

// labelHint is the one muted line under the Label field while it has focus.
const labelHint = "Separate with commas; any one matches."

func filterLayer(m model.Model) tern.Element {
	if !m.FilterForm.Open {
		return nil
	}
	children := make([]tern.Element, 0, len(model.FilterFields)+2)
	for _, field := range model.FilterFields {
		if !m.FilterFieldShown(field.Name) {
			continue
		}
		title := model.FilterFieldLabel(field.Name)
		focused := field.Name == m.FilterForm.Field
		if !field.Text {
			children = append(children, choiceControl(m, model.CardFilter, field.Name, title, focused, "", ""))
			continue
		}
		tone := ui.ToneNeutral
		if focused {
			tone = ui.ToneAccent
		}
		value := m.FilterForm.Values.Value(field.Name)
		input := ui.Input{
			Common: ui.Common{Key: string(field.Name), Tone: tone, Aria: title},
			Field:  ui.Field{Text: value, Cursor: caretAt(m.FilterForm.Caret, field.Name, value), Mode: "insert", Placeholder: ui.T("Any")},
		}
		hint := ""
		if field.Name == model.FilterLabel {
			hint = labelHint
		}
		children = append(children, labelledInput(string(field.Name), title, hint, focused, input)...)
	}
	children = append(children, selectRoom(m, model.CardFilter, m.FilterForm.Field)...)
	if m.FilterForm.Saving {
		children = append(children, toneText("pending", ui.TonePending, model.Progress("Saving", "")))
	}
	if m.FilterForm.Err != nil {
		children = append(children, toneText("error", ui.ToneError, m.FilterForm.Err.Error()))
	}
	children = append(children, formFoot(
		cardButton("apply", model.ModeFilterForm, model.ApplyFilters, "Apply", true),
		exitButton("cancel", model.ModeFilterForm, model.CancelFilters, "Cancel"),
		cardButton("default", model.ModeFilterForm, model.SaveFilterDefault, "Save as default", false),
	))
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardFilter)}, Head: ui.T("Filter"), Meta: ui.T(m.Collection.Title), Icon: ui.IconFunnel, Modal: true, Size: ui.SizeLG}, children))
}

// filterTitles are the filter fields' labels, which the View card shares with the Filter card.
var filterTitles = map[model.Field]string{
	model.FilterProject:      model.FilterFieldLabel(model.FilterProject),
	model.FilterStatus:       model.FilterFieldLabel(model.FilterStatus),
	model.FilterOwner:        model.FilterFieldLabel(model.FilterOwner),
	model.FilterLabel:        model.FilterFieldLabel(model.FilterLabel),
	model.FilterRelationship: model.FilterFieldLabel(model.FilterRelationship),
	model.FilterSearch:       model.FilterFieldLabel(model.FilterSearch),
}
