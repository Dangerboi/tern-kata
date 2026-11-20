package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// daemonLayer is the card that adds or edits an entry of kata's daemon config. A local entry has no URL, token or insecure
// switch, and a token of kind none has no value, so the card draws only the fields that apply.
func daemonLayer(m model.Model) tern.Element {
	form := m.DaemonForm
	if !form.Open {
		return nil
	}
	var children []tern.Element
	for _, spec := range model.DaemonFields {
		name := spec.Name
		if !form.Shows(name) {
			continue
		}
		focused := form.Field == name
		label := form.Label(name)
		hint := form.Hint(name)
		if !spec.Text {
			if control := choiceControl(m, model.CardDaemon, name, label, focused, hint, ""); control != nil {
				children = append(children, control)
			}
			continue
		}
		tone := ui.ToneNeutral
		if focused {
			tone = ui.ToneAccent
		}
		text := daemonValue(form, name)
		input := ui.Input{Common: ui.Common{Key: string(name), Tone: tone, Aria: label}, Field: ui.Field{Text: text, Cursor: caretAt(form.Caret, name, text), Mode: "insert", Readonly: form.Saving}}
		if name == model.DaemonSecret && form.Values.Auth == model.AuthInline {
			input.Secret = true
		}
		children = append(children, labelledInput(string(name), label, hint, focused, input)...)
	}
	busy := ""
	if form.Err != nil {
		children = append(children, toneText("error", ui.ToneError, form.Err.Error()))
	}
	if form.Saving {
		busy = model.Progress("Saving", "")
		children = append(children, toneText("pending", ui.TonePending, busy))
	}
	save := unavailable(cardButton("save", model.ModeDaemonForm, model.DaemonSubmit, "Save", true), busy)
	cancel := unavailable(exitButton("cancel", model.ModeDaemonForm, model.DaemonCancel, "Cancel"), busy)
	children = append(children, buttonRow(save, cancel))
	head := "Add daemon"
	if form.Editing() {
		head = "Edit daemon"
	}
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardDaemon)}, Head: ui.T(head), Icon: ui.IconServer, Modal: true, Size: ui.SizeMD}, children))
}

func daemonValue(form model.DaemonFormState, name model.Field) string {
	v := form.Values
	switch name {
	case model.DaemonName:
		return v.Name
	case model.DaemonURL:
		return v.URL
	case model.DaemonSecret:
		return v.Secret
	}
	return ""
}

// daemonRemoveLayer is the question that asks before an entry leaves the config.
func daemonRemoveLayer(m model.Model) tern.Element {
	r := m.DaemonRemove
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
		busy = model.Progress("Removing", "")
		children = append(children, toneText("pending", ui.TonePending, busy))
	}
	remove := unavailable(destructive(cardButton("remove", model.ModeDaemonRemove, model.DaemonRemoveConfirm, "Remove", false)), busy)
	cancel := unavailable(exitButton("cancel", model.ModeDaemonRemove, model.DaemonRemoveCancel, "Cancel"), busy)
	children = append(children, buttonRow(remove, cancel))
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardDaemonRemove)}, Head: ui.T("Remove daemon"), Icon: ui.IconTrash, Modal: true, Size: ui.SizeMD}, children))
}
