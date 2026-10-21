package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// DatesID is the id of the Dates card's layer.
const DatesID = "layer.dates"

// DatesFieldID is the id of the Dates card's field called name.
func DatesFieldID(name model.Field) string { return DatesID + "." + string(name) }

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
			ui.Text{Common: ui.Common{Key: string(name) + "-label", Tone: tone}, Text: label},
			ui.Input{Common: ui.Common{Key: string(name), Tone: tone, Aria: label}, Field: ui.Field{Text: text, Cursor: caretAt(form.Caret, name, text), Mode: "insert", Placeholder: "none", Readonly: form.Saving}},
			ui.Text{Common: ui.Common{Key: string(name) + "-preview", Tone: ui.ToneMuted}, Text: m.DatesPreview(name)})
	}
	children = append(children, ui.Text{Common: ui.Common{Key: "note", Tone: ui.ToneMuted}, Text: "Type " + model.DateInputHint + ". Leave a field blank to clear it."})
	if form.Err != nil {
		children = append(children, ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: form.Err.Error()})
	}
	if form.Saving {
		children = append(children, ui.Text{Common: ui.Common{Key: "pending", Tone: ui.TonePending}, Text: "Saving dates…"})
	} else {
		children = append(children, buttonRow(cardButton("save", model.ModeDatesForm, model.DatesSubmit, "Save dates", true), cardButton("cancel", model.ModeDatesForm, model.DatesCancel, "Cancel", false)))
	}
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: "dates"}, Head: ui.T("Dates"), Modal: true, Size: ui.SizeLG, Children: children})
}
