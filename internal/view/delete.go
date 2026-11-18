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
		ui.Text{Common: ui.Common{Key: "issue"}, Spans: []ui.Span{{T: form.Title, S: "strong"}}},
		ui.Text{Common: ui.Common{Key: "note"}, Spans: prose("Restore it with "+model.Key("u")+" right after, or from the Menu.", "muted")},
	}
	for _, spec := range model.DeleteFields {
		name := spec.Name
		tone := ui.ToneNeutral
		if form.Field == name {
			tone = ui.ToneAccent
		}
		label, placeholder, text := "Reason", "", form.Reason
		if name == model.DeleteID {
			label, placeholder, text = "Confirm id", form.Ref, form.ID
		}
		input := ui.Input{Common: ui.Common{Key: string(name), Tone: tone, Aria: label}, Field: ui.Field{Text: text, Cursor: caretAt(form.Caret, name, text), Mode: "insert", Readonly: form.Saving}}
		if placeholder != "" {
			input.Placeholder = ui.T(placeholder)
		}
		children = append(children, labelledInput(string(name), label, "", form.Field == name, input)...)
	}
	busy := ""
	if form.Err != nil {
		children = append(children, toneText("error", ui.ToneError, form.Err.Error()))
	}
	if form.Saving {
		busy = model.Progress("Deleting", "Issue")
		children = append(children, toneText("pending", ui.TonePending, busy))
	}
	cancel := exitButton("cancel", model.ModeDeleteForm, model.DeleteCancel, "Cancel")
	del := destructive(cardButton("delete", model.ModeDeleteForm, model.DeleteSubmit, "Delete", false))
	children = append(children, buttonRow(unavailable(del, busy), unavailable(cancel, busy)))
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardDelete)}, Head: ui.T("Delete Issue"), Meta: refMeta(form.Ref), Icon: ui.IconTrash, Modal: true, Size: ui.SizeMD}, children))
}
