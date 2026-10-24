package view

import (
	"strconv"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// tokenLayer is the Token card: the fields of a new API token, or once it is made the token's id and the file that holds its
// secret. The secret itself is never drawn: the Pane never holds it.
func tokenLayer(m model.Model) tern.Element {
	form := m.TokenForm
	if !form.Open {
		return nil
	}
	children := make([]tern.Element, 0, 3*len(model.TokenFields)+4)
	if form.Done {
		children = append(children,
			ui.Text{Common: ui.Common{Key: "done", Tone: ui.ToneSuccess}, Text: "Created token " + strconv.FormatInt(form.DoneID, 10) + "."},
			ui.Text{Common: ui.Common{Key: "file"}, Text: "Its secret is in " + form.DoneFile + ", readable by you alone. It is not shown anywhere and cannot be read from kata again."},
			buttonRow(cardButton("close", model.ModeTokenForm, model.TokenSubmit, "Close", true)))
		return ui.Nodes(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardToken)}, Head: ui.T("Token created"), Modal: true, Size: ui.SizeLG, Children: children})
	}
	for _, spec := range model.TokenFields {
		name := spec.Name
		tone := ui.ToneNeutral
		if form.Field == name {
			tone = ui.ToneAccent
		}
		label := model.TokenLabel(name)
		text := form.Values.Value(name)
		children = append(children,
			ui.Text{Common: ui.Common{Key: string(name) + "-label", Tone: tone}, Text: label},
			ui.Input{Common: ui.Common{Key: string(name), Tone: tone, Aria: label}, Field: ui.Field{Text: text, Cursor: caretAt(form.Caret, name, text), Mode: "insert", Readonly: form.Saving}})
	}
	children = append(children, ui.Text{Common: ui.Common{Key: "note", Tone: ui.ToneMuted}, Text: "The secret is written to a new file only you can read, and is never shown. The file must not exist yet."})
	if form.Err != nil {
		children = append(children, ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: form.Err.Error()})
	}
	if form.Saving {
		children = append(children, ui.Text{Common: ui.Common{Key: "pending", Tone: ui.TonePending}, Text: "Creating token…"})
	} else {
		children = append(children, buttonRow(cardButton("save", model.ModeTokenForm, model.TokenSubmit, "Create token", true), cardButton("cancel", model.ModeTokenForm, model.TokenCancel, "Cancel", false)))
	}
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardToken)}, Head: ui.T("New token"), Modal: true, Size: ui.SizeLG, Children: children})
}

// revokeLayer is the confirmation before a token is revoked.
func revokeLayer(m model.Model) tern.Element {
	r := m.RevokeCard
	if !r.Open {
		return nil
	}
	children := []tern.Element{ui.Text{Common: ui.Common{Key: "question", Tone: ui.ToneWarning}, Text: r.Question()}}
	if r.Err != nil {
		children = append(children, ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: r.Err.Error()})
	}
	if r.Saving {
		children = append(children, ui.Text{Common: ui.Common{Key: "pending", Tone: ui.TonePending}, Text: "Revoking token…"})
	} else {
		children = append(children, buttonRow(cardButton("revoke", model.ModeRevokeToken, model.RevokeConfirm, "Revoke", false), cardButton("keep", model.ModeRevokeToken, model.RevokeCancel, "Keep", true)))
	}
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardRevoke)}, Head: ui.T("Revoke token"), Modal: true, Size: ui.SizeLG, Children: children})
}
