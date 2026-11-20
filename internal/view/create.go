package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
	"strings"
)

// createBodyLines caps the Body editor so the foot and any error stay in view; the Draft editor card keeps its taller one.
const createBodyLines = 6

var createLabels = map[model.Field]string{
	model.CreateTitle:     "Title",
	model.CreateBody:      "Body",
	model.CreateLabels:    "Labels",
	model.CreateOwner:     "Owner",
	model.CreatePriority:  "Priority",
	model.CreateScheduled: "Scheduled",
	model.CreateDeadline:  "Deadline",
}

// createHints are the one muted line under a field, always shown.
var createHints = map[model.Field]string{
	model.CreateLabels:    "Separate with commas.",
	model.CreateScheduled: "Enter " + model.DateInputHint + ".",
	model.CreateDeadline:  "Enter " + model.DateInputHint + ".",
}

func createLayer(m model.Model) tern.Element {
	f := m.Create
	title := "Create Issue"
	if f.ParentUID != "" {
		title = "Create child Issue"
	}
	values := map[model.Field]string{model.CreateTitle: f.Title, model.CreateLabels: f.Labels, model.CreateOwner: f.Owner, model.CreateScheduled: f.Dates.Scheduled, model.CreateDeadline: f.Dates.Deadline}
	children := []tern.Element{}
	readonly := f.Saving || f.DiscardConfirm
	for _, field := range model.CreateFields {
		key := string(field.Name)
		label := createLabels[field.Name]
		focused := field.Name == f.Field
		tone := ui.ToneNeutral
		if focused {
			tone = ui.ToneAccent
		}
		switch field.Name {
		case model.CreatePriority:
			children = append(children, choiceControl(m, model.CardCreate, field.Name, label, focused, "", ""))
			continue
		case model.CreateBody:
			editor := ui.Editor{Common: ui.Common{Key: key, Tone: ui.ToneAccent}, Field: ui.Field{Text: f.Editor.Text, Cursor: &f.Editor.Cursor, Anchor: &f.Editor.Anchor, Readonly: readonly}, MaxLines: createBodyLines}
			children = append(children, labelledInput(key, label, createHints[field.Name], focused, editor)...)
			continue
		}
		input := ui.Input{Common: ui.Common{Key: key, Tone: tone, Aria: label}, Field: ui.Field{Text: values[field.Name], Cursor: caretAt(f.Caret, field.Name, values[field.Name]), Mode: "insert", Readonly: readonly}}
		children = append(children, labelledInput(key, label, createHints[field.Name], focused, input)...)
		if (field.Name == model.CreateScheduled || field.Name == model.CreateDeadline) && strings.TrimSpace(values[field.Name]) != "" {
			children = append(children, toneText(key+"-preview", ui.ToneMuted, m.CreatePreview(field.Name)))
		}
	}
	if f.Field == model.CreateLabels && !f.DiscardConfirm {
		if suggestions := m.CreateSuggestions(); len(suggestions) > 0 {
			children = append(children, toneText("completion", ui.ToneMuted, strings.Join(suggestions, ", ")))
		}
	}
	if f.Err != nil {
		children = append(children, toneText("error", ui.ToneError, f.Err.Error()))
	}
	switch {
	case f.Saving:
		save := cardButton("save", model.ModeCreate, model.SaveDraftAction, title, true)
		save.Disabled = ui.Because(model.Progress("Creating", "Issue"))
		cancel := exitButton("cancel", model.ModeCreate, model.CancelDraft, "Cancel")
		cancel.Disabled = ui.Because(model.Progress("Creating", "Issue"))
		children = append(children, toneText("pending", ui.TonePending, model.Progress("Creating", "Issue")), formFoot(save, cancel))
	case f.DiscardConfirm:
		children = append(children, toneText("question", ui.ToneWarning, "Discard what you wrote?"), discardButtons())
	default:
		children = append(children, formFoot(cardButton("save", model.ModeCreate, model.SaveDraftAction, title, true), exitButton("cancel", model.ModeCreate, model.CancelDraft, "Cancel")))
	}
	overlay := ui.Overlay{Common: ui.Common{Key: layerKey(model.CardCreate)}, Modal: true, Size: ui.SizeLG, Head: ui.T(title), Icon: ui.IconPlus}
	if ref := m.CreateParentRef(); ref != "" {
		overlay.Meta = refMeta(ref)
	}
	return withCalendar(m, model.CardCreate, ui.Nodes(popup(overlay, children)))
}
