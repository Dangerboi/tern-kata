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
		focused := form.Field == name
		tone := ui.ToneNeutral
		if focused {
			tone = ui.ToneAccent
		}
		label := model.DatesLabel(name)
		text := form.Value(name)
		input := ui.Input{Common: ui.Common{Key: string(name), Tone: tone, Aria: label}, Field: ui.Field{Text: text, Cursor: caretAt(form.Caret, name, text), Mode: "insert", Placeholder: ui.T("YYYY-MM-DD"), Readonly: form.Saving}}
		children = append(children, fieldLabel(string(name), label, focused), input, toneText(string(name)+"-preview", ui.ToneMuted, m.DatesPreview(name)))
		if focused {
			children = append(children, toneText(string(name)+"-hint", ui.ToneMuted, "Date like 2026-11-20, today, tomorrow or +3d. Blank clears it."))
		}
	}
	if form.Err != nil {
		children = append(children, toneText("error", ui.ToneError, form.Err.Error()))
	}
	if form.Saving {
		children = append(children, toneText("pending", ui.TonePending, model.Progress("Saving", "")))
	} else {
		children = append(children, formFoot(cardButton("save", model.ModeDatesForm, model.DatesSubmit, "Save", true), exitButton("cancel", model.ModeDatesForm, model.DatesCancel, "Cancel")))
	}
	return withCalendar(m, model.CardDates, ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardDates)}, Head: ui.T("Dates"), Meta: refMeta(form.Ref), Icon: ui.IconClock, Modal: true, Size: ui.SizeLG}, children)))
}
