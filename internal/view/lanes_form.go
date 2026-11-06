package view

import (
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

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
		return toneText(key, ui.ToneMuted, text)
	}
	gap := func(key string) tern.Element { return ui.Spacer{Common: ui.Common{Key: key}, Size: ui.GapMD} }
	children := []tern.Element{
		muted("source", "In use: "+m.LaneSource()+"."),
		toneText("names-label", fieldTone(form, model.LanesNames), "Lane list"),
		ui.Input{Common: ui.Common{Key: "names", Tone: fieldTone(form, model.LanesNames), Aria: "Lane list"}, Field: ui.Field{Text: form.Text, Cursor: caretAt(form.Caret, model.LanesNames, form.Text), Mode: "insert", Placeholder: ui.T("Lane names, comma separated"), Readonly: form.Saving}},
		muted("note", "Names in order, comma separated. Empty restores the default list. Saving never changes a label in kata."),
	}
	if unused := m.UnusedLabels(); len(unused) > 0 {
		children = append(children, muted("unused", "Not Lanes yet: "+strings.Join(unused, ", ")))
	}
	children = append(children,
		gap("gap-done"),
		ui.Rule{Common: ui.Common{Key: "done-rule"}, Label: ui.T("Done")},
		muted("done-note", "Done shows Issues closed today or in the N days before, at most M cards. Leave a field blank for the default."))
	for _, name := range []model.Field{model.LanesDays, model.LanesCards} {
		text := form.Value(name)
		children = append(children,
			toneText(string(name)+"-label", fieldTone(form, name), lanesDoneLabels[name]),
			ui.Input{Common: ui.Common{Key: string(name), Tone: fieldTone(form, name), Aria: lanesDoneLabels[name]}, Field: ui.Field{Text: text, Cursor: caretAt(form.Caret, name, text), Mode: "insert", Placeholder: ui.T(lanesDonePlaceholders[name]), Readonly: form.Saving}})
	}
	children = append(children,
		gap("gap-share"),
		choiceControl(m, model.CardLanes, model.LanesShare, "Save to", form.Field == model.LanesShare, "This machine writes lanes.toml, for you alone. The Project writes its tern.lanes metadata for everyone on this daemon, and wins over lanes.toml.", ""))
	if form.Saving {
		children = append(children, toneText("pending", ui.TonePending, "Saving Lanes…"))
	}
	if form.Err != nil {
		children = append(children, toneText("error", ui.ToneError, form.Err.Error()))
	}
	buttons := []tern.Element{cardButton("save", model.ModeLanesForm, model.LanesSubmit, "Save Lanes", true)}
	if m.SharesLanes() {
		buttons = append(buttons, cardButton("stop", model.ModeLanesForm, model.LanesStopSharing, "Stop sharing", false))
	}
	buttons = append(buttons, cardButton("cancel", model.ModeLanesForm, model.LanesCancel, "Cancel", false))
	children = append(children, buttonRow(buttons...))
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardLanes)}, Head: ui.T("Lanes"), Icon: ui.IconKanban, Modal: true, Size: ui.SizeLG}, children))
}

func fieldTone(form model.LanesFormState, name model.Field) ui.Tone {
	if form.Field == name {
		return ui.ToneAccent
	}
	return ui.ToneNeutral
}
