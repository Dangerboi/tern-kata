package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
	"strings"
)

// CreateFieldID is the id of the Create card's field called name.
func CreateFieldID(name model.Field) string { return "layer.create." + string(name) }

var createLabels = map[model.Field]string{
	model.CreateTitle:    "Title",
	model.CreateBody:     "Body (optional)",
	model.CreateLabels:   "Labels (optional, comma-separated)",
	model.CreateOwner:    "Owner (optional)",
	model.CreatePriority: "Priority (optional, 0-4)",
}

func createLayer(m model.Model) tern.Element {
	f := m.Create
	title := "Create Issue"
	if f.ParentUID != "" {
		title = "Create child Issue"
	}
	values := map[model.Field]string{model.CreateTitle: f.Title, model.CreateLabels: f.Labels, model.CreateOwner: f.Owner, model.CreatePriority: f.Priority}
	children := []tern.Element{}
	for _, field := range model.CreateFields {
		key := string(field.Name)
		label := createLabels[field.Name]
		tone := ui.ToneNeutral
		if field.Name == f.Field {
			tone = ui.ToneAccent
		}
		children = append(children, ui.Text{Common: ui.Common{Key: key + "-label", Tone: tone}, Text: label})
		readonly := f.Saving || f.DiscardConfirm
		if field.Name == model.CreateBody {
			children = append(children, TextEditor(key, f.Editor, readonly))
		} else {
			children = append(children, ui.Input{Common: ui.Common{Key: key, Tone: tone, Aria: label}, Field: ui.Field{Text: values[field.Name], Mode: "insert", Readonly: readonly}})
		}
	}
	if f.Field == model.CreateLabels {
		children = append(children, ui.Text{Common: ui.Common{Key: "completion", Tone: ui.ToneMuted}, Text: strings.Join(m.CreateSuggestions(), ", ")})
	}
	if f.Err != nil {
		children = append(children, ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: f.Err.Error()})
	}
	switch {
	case f.Saving:
		children = append(children, ui.Text{Common: ui.Common{Key: "saving", Tone: ui.TonePending}, Text: "Creating Issue…"})
	case f.DiscardConfirm:
		children = append(children, ui.Text{Common: ui.Common{Key: "discard", Tone: ui.ToneWarning}, Text: "Discard this Draft? y discards; n or esc keeps editing."}, discardButtons())
	default:
		children = append(children, buttonRow(keyButton("save", title+" (ctrl+o)", "ctrl+o", true), keyButton("cancel", "Cancel (esc)", "escape", false)))
	}
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: "create"}, Modal: true, Size: ui.SizeLG, Head: ui.T(title), Children: children})
}
