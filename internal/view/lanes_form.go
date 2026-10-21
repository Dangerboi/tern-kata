package view

import (
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const (
	LanesID      = "layer.lanes"
	LanesInputID = LanesID + ".names"
)

func lanesLayer(m model.Model) tern.Element {
	form := m.LanesForm
	if !form.Open {
		return nil
	}
	children := []tern.Element{
		ui.Input{Common: ui.Common{Key: "names", Tone: ui.ToneAccent, Aria: "Lanes"}, Field: ui.Field{Text: form.Text, Cursor: caretAt(form.Caret, "", form.Text), Mode: "insert", Placeholder: "Lanes, comma separated"}},
		ui.Text{Common: ui.Common{Key: "note", Tone: ui.ToneMuted}, Text: "Names in order, comma separated; an empty field restores the default. Saving never changes a label in kata."},
	}
	if unused := m.UnusedLabels(); len(unused) > 0 {
		children = append(children, ui.Text{Common: ui.Common{Key: "unused", Tone: ui.ToneMuted}, Text: "Labels not Lanes yet: " + strings.Join(unused, ", ")})
	}
	if form.Err != nil {
		children = append(children, ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: form.Err.Error()})
	}
	children = append(children, buttonRow(cardButton("save", model.ModeLanesForm, model.LanesSubmit, "Save Lanes", true), cardButton("cancel", model.ModeLanesForm, model.LanesCancel, "Cancel", false)))
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: "lanes"}, Head: ui.T("Lanes"), Modal: true, Size: ui.SizeMD, Children: children})
}
