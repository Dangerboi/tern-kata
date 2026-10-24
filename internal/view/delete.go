package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

func deleteLayer(m model.Model) tern.Element {
	form := m.DeleteForm
	if !form.Open {
		return nil
	}
	children := []tern.Element{
		ui.Text{Common: ui.Common{Key: "issue", Tone: ui.ToneAccent}, Text: form.QualifiedID + " " + form.Title},
		ui.Text{Common: ui.Common{Key: "note", Tone: ui.ToneMuted}, Text: "A deleted Issue can be restored: press u right after, or use Restore a deleted Issue in the Menu."},
	}
	for _, spec := range model.DeleteFields {
		name := spec.Name
		tone := ui.ToneNeutral
		if form.Field == name {
			tone = ui.ToneAccent
		}
		label, placeholder, text := "Reason (optional)", "none", form.Reason
		if name == model.DeleteID {
			label, placeholder, text = "Type "+form.QualifiedID+" to confirm", form.QualifiedID, form.ID
		}
		children = append(children,
			ui.Text{Common: ui.Common{Key: string(name) + "-label", Tone: tone}, Text: label},
			ui.Input{Common: ui.Common{Key: string(name), Tone: tone, Aria: label}, Field: ui.Field{Text: text, Cursor: caretAt(form.Caret, name, text), Mode: "insert", Placeholder: placeholder, Readonly: form.Saving}})
	}
	if form.Err != nil {
		children = append(children, ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: form.Err.Error()})
	}
	if form.Saving {
		children = append(children, ui.Text{Common: ui.Common{Key: "pending", Tone: ui.TonePending}, Text: "Deleting Issue…"})
	} else {
		children = append(children, buttonRow(cardButton("delete", model.ModeDeleteForm, model.DeleteSubmit, "Delete", true), cardButton("cancel", model.ModeDeleteForm, model.DeleteCancel, "Cancel", false)))
	}
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardDelete)}, Head: ui.T("Delete #" + form.ShortID), Modal: true, Size: ui.SizeLG, Children: children})
}
