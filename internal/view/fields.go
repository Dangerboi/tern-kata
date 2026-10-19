package view

import (
	"fmt"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const FieldInputID = "layer.fields.value"

func fieldLayer(m model.Model) tern.Nodes {
	if !m.FieldForm.Open {
		return nil
	}
	title := map[model.Action]string{model.AddLabelAction: "Add label", model.RemoveLabelAction: "Remove label", model.AssignOwnerAction: "Assign owner", model.ClearOwnerAction: "Clear owner", model.PriorityAction: "Priority (0-4, or - to clear)", model.SetParentAction: "Set parent (Issue id)"}[m.FieldForm.Action]
	children := []tern.Element{ui.Input{Common: ui.Common{Key: "value", Tone: ui.ToneAccent}, Field: ui.Field{Text: m.FieldForm.Value, Mode: "insert", Readonly: m.FieldForm.Saving}}}
	suggestions := m.FieldSuggestions()
	for i, label := range suggestions {
		tone := ui.ToneNeutral
		if i == m.FieldForm.Suggestion%max(len(suggestions), 1) {
			tone = ui.ToneAccent
		}
		children = append(children, ui.Text{Common: ui.Common{Key: fmt.Sprint("label", i), Tone: tone}, Text: label})
	}
	if m.FieldForm.Err != nil {
		children = append(children, ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: m.FieldForm.Err.Error()})
	}
	if m.FieldForm.Saving {
		children = append(children, ui.Text{Common: ui.Common{Key: "saving", Tone: ui.TonePending}, Text: "Saving…"})
	} else {
		children = append(children, buttonRow(cardButton("save", model.ModeFieldForm, model.FieldSubmit, "Save", true), cardButton("cancel", model.ModeFieldForm, model.FieldCancel, "Cancel", false)))
	}
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: "fields"}, Modal: true, Head: ui.T(title), Children: children})
}
