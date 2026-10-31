package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

func datesLayer(m model.Model) tern.Element {
	form := m.DatesForm
	if !form.Open {
		return nil
	}
	children := make([]tern.Element, 0, 10)
	for _, spec := range model.DatesFields {
		name := spec.Name
		tone := ui.ToneNeutral
		if form.Field == name {
			tone = ui.ToneAccent
		}
		label := model.DatesLabel(name)
		text := form.Value(name)
		children = append(children,
			toneText(string(name)+"-label", tone, label),
			ui.Input{Common: ui.Common{Key: string(name), Tone: tone, Aria: label}, Field: ui.Field{Text: text, Cursor: caretAt(form.Caret, name, text), Mode: "insert", Placeholder: ui.T("none"), Readonly: form.Saving}},
			toneText(string(name)+"-preview", ui.ToneMuted, m.DatesPreview(name)))
	}
	children = append(children, toneText("note", ui.ToneMuted, "Type "+model.DateInputHint+", or press ctrl+d for a calendar. Leave a field blank to clear it."))
	if form.Err != nil {
		children = append(children, toneText("error", ui.ToneError, form.Err.Error()))
	}
	if form.Saving {
		children = append(children, toneText("pending", ui.TonePending, "Saving dates…"))
	} else {
		children = append(children, buttonRow(cardButton("save", model.ModeDatesForm, model.DatesSubmit, "Save dates", true), cardButton("cancel", model.ModeDatesForm, model.DatesCancel, "Cancel", false)))
	}
	return withDatePicker(m, model.CardDates, ui.Nodes(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardDates)}, Head: ui.T("Dates"), Modal: true, Size: ui.SizeLG, Children: children}))
}
