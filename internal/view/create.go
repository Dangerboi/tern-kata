package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
	"strings"
)

var createLabels = map[model.Field]string{
	model.CreateTitle:     "Title",
	model.CreateBody:      "Body (optional)",
	model.CreateLabels:    "Labels (optional, comma-separated)",
	model.CreateOwner:     "Owner (optional)",
	model.CreatePriority:  "Priority (optional, 0-4)",
	model.CreateScheduled: "Scheduled (optional, " + model.DateInputHint + ")",
	model.CreateDeadline:  "Deadline (optional, " + model.DateInputHint + ")",
}

func createLayer(m model.Model) tern.Element {
	f := m.Create
	title := "Create Issue"
	if f.ParentUID != "" {
		title = "Create child Issue"
	}
	values := map[model.Field]string{model.CreateTitle: f.Title, model.CreateLabels: f.Labels, model.CreateOwner: f.Owner, model.CreatePriority: f.Priority, model.CreateScheduled: f.Dates.Scheduled, model.CreateDeadline: f.Dates.Deadline}
	children := []tern.Element{}
	for _, field := range model.CreateFields {
		key := string(field.Name)
		label := createLabels[field.Name]
		tone := ui.ToneNeutral
		if field.Name == f.Field {
			tone = ui.ToneAccent
		}
		children = append(children, toneText(key+"-label", tone, label))
		readonly := f.Saving || f.DiscardConfirm
		if field.Name == model.CreateBody {
			children = append(children, TextEditor(key, f.Editor, readonly))
		} else {
			children = append(children, ui.Input{Common: ui.Common{Key: key, Tone: tone, Aria: label}, Field: ui.Field{Text: values[field.Name], Cursor: caretAt(f.Caret, field.Name, values[field.Name]), Mode: "insert", Readonly: readonly}})
		}
		if field.Name == model.CreateScheduled || field.Name == model.CreateDeadline {
			children = append(children, toneText(key+"-preview", ui.ToneMuted, m.CreatePreview(field.Name)))
		}
	}
	if f.Field == model.CreateLabels {
		children = append(children, toneText("completion", ui.ToneMuted, strings.Join(m.CreateSuggestions(), ", ")))
	}
	if f.Err != nil {
		children = append(children, toneText("error", ui.ToneError, f.Err.Error()))
	}
	switch {
	case f.Saving:
		children = append(children, toneText("saving", ui.TonePending, "Creating Issue…"))
	case f.DiscardConfirm:
		children = append(children, toneText("discard", ui.ToneWarning, "Discard this Draft? y discards; n or esc keeps editing."), discardButtons())
	default:
		children = append(children, buttonRow(cardButton("save", model.ModeCreate, model.SaveDraftAction, title, true), cardButton("cancel", model.ModeCreate, model.CancelDraft, "Cancel", false)))
	}
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardCreate)}, Modal: true, Size: ui.SizeLG, Head: ui.T(title), Children: children})
}
