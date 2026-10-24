package view

import (
	"fmt"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

func fieldLayer(m model.Model) tern.Nodes {
	if !m.FieldForm.Open {
		return nil
	}
	title := m.FieldFormTitle()
	children := []tern.Element{ui.Input{Common: ui.Common{Key: inputLeaf, Tone: ui.ToneAccent}, Field: ui.Field{Text: m.FieldForm.Value, Cursor: caretAt(m.FieldForm.Caret, "", m.FieldForm.Value), Mode: "insert", Readonly: m.FieldForm.Saving}}}
	suggestions := m.FieldSuggestions()
	for i, label := range suggestions {
		tone := ui.ToneNeutral
		if i == m.FieldForm.Suggestion%max(len(suggestions), 1) {
			tone = ui.ToneAccent
		}
		children = append(children, ui.Text{Common: ui.Common{Key: fmt.Sprint("label", i), Tone: tone}, Text: label})
	}
	if found := m.FieldForm.Found; found != nil {
		status, tone := "This Issue is deleted. Enter restores it.", ui.ToneMuted
		if !found.Deleted {
			status, tone = "This Issue is not deleted.", ui.ToneError
		}
		children = append(children,
			ui.Text{Common: ui.Common{Key: "found", Tone: ui.ToneAccent}, Text: found.QualifiedID + " " + found.Title},
			ui.Text{Common: ui.Common{Key: "found-state", Tone: tone}, Text: status})
	}
	if m.FieldForm.Err != nil {
		children = append(children, ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: m.FieldForm.Err.Error()})
	}
	if m.FieldForm.Saving {
		children = append(children, ui.Text{Common: ui.Common{Key: "saving", Tone: ui.TonePending}, Text: "Saving…"})
	} else {
		children = append(children, buttonRow(cardButton("save", model.ModeFieldForm, model.FieldSubmit, saveLabel(m), true), cardButton("cancel", model.ModeFieldForm, model.FieldCancel, "Cancel", false)))
	}
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardField)}, Modal: true, Head: ui.T(title), Children: children})
}

// saveLabel is the word on the form's save button: a restore finds the Issue first, then restores it.
func saveLabel(m model.Model) string {
	switch {
	case m.FieldForm.Action != model.RestoreDeletedAction:
		return "Save"
	case m.FieldForm.Found == nil:
		return "Find"
	}
	return "Restore"
}
