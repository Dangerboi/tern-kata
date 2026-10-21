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

// LanesFieldID is the id of the Lanes card's field called name.
func LanesFieldID(name model.Field) string { return LanesID + "." + string(name) }

var lanesDoneLabels = map[model.Field]string{
	model.LanesDays:  "Done days (blank for the default)",
	model.LanesCards: "Done cards (blank for the default)",
}

func lanesLayer(m model.Model) tern.Element {
	form := m.LanesForm
	if !form.Open {
		return nil
	}
	children := []tern.Element{
		ui.Input{Common: ui.Common{Key: "names", Tone: fieldTone(form, model.LanesNames), Aria: "Lanes"}, Field: ui.Field{Text: form.Text, Cursor: caretAt(form.Caret, model.LanesNames, form.Text), Mode: "insert", Placeholder: "Lanes, comma separated"}},
		ui.Text{Common: ui.Common{Key: "note", Tone: ui.ToneMuted}, Text: "Names in order, comma separated; an empty field restores the default. Saving never changes a label in kata."},
	}
	for _, name := range []model.Field{model.LanesDays, model.LanesCards} {
		text := form.Value(name)
		children = append(children,
			ui.Text{Common: ui.Common{Key: string(name) + "-label", Tone: fieldTone(form, name)}, Text: lanesDoneLabels[name]},
			ui.Input{Common: ui.Common{Key: string(name), Tone: fieldTone(form, name), Aria: lanesDoneLabels[name]}, Field: ui.Field{Text: text, Cursor: caretAt(form.Caret, name, text), Mode: "insert"}})
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

func fieldTone(form model.LanesFormState, name model.Field) ui.Tone {
	if form.Field == name {
		return ui.ToneAccent
	}
	return ui.ToneNeutral
}
