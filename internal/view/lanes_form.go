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

var lanesDonePlaceholders = map[model.Field]string{
	model.LanesDays:  "14",
	model.LanesCards: "20",
}

// lanesField is one text field of the Lanes card, as flat children of the card so its input keeps the id FieldID gives it: its
// label (accent while it has focus, error when it is the field the error is about), the input, and under it the error or,
// while the field has focus, its one-line hint.
func lanesField(form model.LanesFormState, name model.Field, label, placeholder, hint string) []tern.Element {
	tone := fieldTone(form, name)
	failed := form.Err != nil && form.ErrField == name
	if failed {
		tone = ui.ToneError
	}
	text := form.Value(name)
	field := []tern.Element{
		toneText(string(name)+"-label", tone, label),
		ui.Input{Common: ui.Common{Key: string(name), Tone: tone, Aria: label}, Field: ui.Field{Text: text, Cursor: caretAt(form.Caret, name, text), Mode: "insert", Placeholder: ui.T(placeholder), Readonly: form.Saving}},
	}
	switch {
	case failed:
		field = append(field, toneText(string(name)+"-error", ui.ToneError, form.Err.Error()))
	case form.Field == name:
		field = append(field, toneText(string(name)+"-hint", ui.ToneMuted, hint))
	}
	return field
}

func lanesLayer(m model.Model) tern.Element {
	form := m.LanesForm
	if !form.Open {
		return nil
	}
	children := lanesField(form, model.LanesNames, "Lanes", "Labels in order", "Labels in order, separated by commas. Blank restores the defaults.")
	if unused := m.UnusedLabels(); len(unused) > 0 {
		children = append(children, toneText("unused", ui.ToneMuted, "Other labels: "+strings.Join(unused, ", ")))
	}
	children = append(children, ui.Rule{Common: ui.Common{Key: "done-rule"}, Label: ui.T("Done")})
	children = append(children, lanesField(form, model.LanesDays, lanesDoneLabels[model.LanesDays], lanesDonePlaceholders[model.LanesDays], "Whole days, 1 to 3650. Blank is 14.")...)
	children = append(children, lanesField(form, model.LanesCards, lanesDoneLabels[model.LanesCards], lanesDonePlaceholders[model.LanesCards], "1 to 500. Blank is 20.")...)
	children = append(children, choiceControl(m, model.CardLanes, model.LanesShare, "Save to", form.Field == model.LanesShare, lanesShareHint(form), ""))
	if form.Saving {
		children = append(children, toneText("pending", ui.TonePending, model.Progress("Saving", "")))
	}
	if form.Err != nil && form.ErrField == "" {
		children = append(children, toneText("error", ui.ToneError, form.Err.Error()))
	}
	var others []ui.Button
	if m.SharesLanes() {
		others = append(others, cardButton("stop", model.ModeLanesForm, model.LanesStopSharing, "Stop sharing", false))
	}
	children = append(children, formFoot(cardButton("save", model.ModeLanesForm, model.LanesSubmit, "Save", true), exitButton("cancel", model.ModeLanesForm, model.LanesCancel, "Cancel"), others...))
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardLanes)}, Head: ui.T("Lanes"), Meta: ui.T(m.LanesMeta()), Icon: ui.IconKanban, Modal: true, Size: ui.SizeLG}, children))
}

// lanesShareHint is the one line under Save to that belongs to the chosen place.
func lanesShareHint(form model.LanesFormState) string {
	if form.Share {
		return "Everyone on this daemon. Overrides lanes.toml."
	}
	return "Only for you, in lanes.toml."
}

func fieldTone(form model.LanesFormState, name model.Field) ui.Tone {
	if form.Field == name {
		return ui.ToneAccent
	}
	return ui.ToneNeutral
}
