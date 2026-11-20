package view

import (
	"strconv"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// The secret itself is never drawn: the Pane never holds it.
func tokenLayer(m model.Model) tern.Element {
	form := m.TokenForm
	if !form.Open {
		return nil
	}
	children := make([]tern.Element, 0, 3*len(model.TokenFields)+2)
	if form.Done {
		children = append(children,
			ui.Text{Common: ui.Common{Key: "file"}, Spans: []ui.Span{{T: "The secret is in "}, monoSpan(form.DoneFile, ""), {T: ". Only you can read it; kata cannot show it again."}}},
			buttonRow(exitButton("close", model.ModeTokenForm, model.TokenSubmit, "Close")))
		return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardToken)}, Head: ui.T("Token " + strconv.FormatInt(form.DoneID, 10) + " created"), Icon: ui.IconKey, Modal: true, Size: ui.SizeLG}, children))
	}
	for _, spec := range model.TokenFields {
		name := spec.Name
		focused := form.Field == name
		tone := ui.ToneNeutral
		if focused {
			tone = ui.ToneAccent
		}
		label := model.TokenLabel(name)
		hint := model.TokenHint(name)
		if !spec.Text {
			if control := choiceControl(m, model.CardToken, name, label, focused, hint, ""); control != nil {
				children = append(children, control)
			}
			continue
		}
		text := form.Values.Value(name)
		children = append(children, labelledInput(string(name), label, hint, focused, ui.Input{Common: ui.Common{Key: string(name), Tone: tone, Aria: label}, Field: ui.Field{Text: text, Cursor: caretAt(form.Caret, name, text), Mode: "insert", Readonly: form.Saving}})...)
	}
	busy := ""
	if form.Err != nil {
		children = append(children, toneText("error", ui.ToneError, form.Err.Error()))
	}
	if form.Saving {
		busy = model.Progress("Creating", "token")
		children = append(children, toneText("pending", ui.TonePending, busy))
	}
	create := unavailable(cardButton("save", model.ModeTokenForm, model.TokenSubmit, "Create token", true), busy)
	cancel := unavailable(exitButton("cancel", model.ModeTokenForm, model.TokenCancel, "Cancel"), busy)
	children = append(children, buttonRow(create, cancel))
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardToken)}, Head: ui.T("New token"), Icon: ui.IconKey, Modal: true, Size: ui.SizeLG}, children))
}

func revokeLayer(m model.Model) tern.Element {
	r := m.RevokeCard
	if !r.Open {
		return nil
	}
	children := []tern.Element{
		toneText("question", ui.ToneWarning, r.Question()),
		toneText("consequence", ui.ToneMuted, r.Consequence()),
	}
	busy := ""
	if r.Err != nil {
		children = append(children, toneText("error", ui.ToneError, r.Err.Error()))
	}
	if r.Saving {
		busy = model.Progress("Revoking", "token")
		children = append(children, toneText("pending", ui.TonePending, busy))
	}
	revoke := unavailable(destructive(cardButton("revoke", model.ModeRevokeToken, model.RevokeConfirm, "Revoke", false)), busy)
	cancel := unavailable(exitButton("cancel", model.ModeRevokeToken, model.RevokeCancel, "Cancel"), busy)
	children = append(children, buttonRow(revoke, cancel))
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardRevoke)}, Head: ui.T("Revoke token"), Icon: ui.IconKeyRound, Modal: true, Size: ui.SizeMD}, children))
}
