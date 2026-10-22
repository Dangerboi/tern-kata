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
	model.LanesDays:  "Days back",
	model.LanesCards: "Most cards",
}

// lanesDonePlaceholders are the defaults a blank Done field stands for.
var lanesDonePlaceholders = map[model.Field]string{
	model.LanesDays:  "14",
	model.LanesCards: "20",
}

func lanesLayer(m model.Model) tern.Element {
	form := m.LanesForm
	if !form.Open {
		return nil
	}
	muted := func(key, text string) tern.Element {
		return ui.Text{Common: ui.Common{Key: key, Tone: ui.ToneMuted}, Text: text}
	}
	gap := func(key string) tern.Element { return ui.Spacer{Common: ui.Common{Key: key}, Size: ui.GapMD} }
	children := []tern.Element{
		ui.Text{Common: ui.Common{Key: "names-label", Tone: fieldTone(form, model.LanesNames)}, Text: "Lane list"},
		ui.Input{Common: ui.Common{Key: "names", Tone: fieldTone(form, model.LanesNames), Aria: "Lane list"}, Field: ui.Field{Text: form.Text, Cursor: caretAt(form.Caret, model.LanesNames, form.Text), Mode: "insert", Placeholder: "Lane names, comma separated", Readonly: form.Saving}},
		muted("note", "Names in order, comma separated. Empty restores the default list. Saving never changes a label in kata."),
	}
	if unused := m.UnusedLabels(); len(unused) > 0 {
		children = append(children, muted("unused", "Not Lanes yet: "+strings.Join(unused, ", ")))
	}
	children = append(children,
		gap("gap-done"),
		ui.Rule{Common: ui.Common{Key: "done-rule"}, Label: ui.T("Done")},
		muted("done-note", "Done shows Issues closed in the last N days, at most M cards. Leave a field blank for the default."))
	for _, name := range []model.Field{model.LanesDays, model.LanesCards} {
		text := form.Value(name)
		children = append(children,
			ui.Text{Common: ui.Common{Key: string(name) + "-label", Tone: fieldTone(form, name)}, Text: lanesDoneLabels[name]},
			ui.Input{Common: ui.Common{Key: string(name), Tone: fieldTone(form, name), Aria: lanesDoneLabels[name]}, Field: ui.Field{Text: text, Cursor: caretAt(form.Caret, name, text), Mode: "insert", Placeholder: lanesDonePlaceholders[name], Readonly: form.Saving}})
	}
	if form.Saving {
		children = append(children, ui.Text{Common: ui.Common{Key: "pending", Tone: ui.TonePending}, Text: "Saving Lanes…"})
	}
	if form.Err != nil {
		children = append(children, ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: form.Err.Error()})
	}
	children = append(children, gap("gap-actions"), buttonRow(cardButton("save", model.ModeLanesForm, model.LanesSubmit, "Save Lanes", true), cardButton("cancel", model.ModeLanesForm, model.LanesCancel, "Cancel", false)))
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: "lanes"}, Head: ui.T("Lanes"), Modal: true, Size: ui.SizeLG, Children: children})
}

func fieldTone(form model.LanesFormState, name model.Field) ui.Tone {
	if form.Field == name {
		return ui.ToneAccent
	}
	return ui.ToneNeutral
}
