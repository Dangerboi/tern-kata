package view

import (
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

var viewTitles = map[model.Field]string{
	model.ViewName:     "Name",
	model.ViewBase:     "Base",
	model.FilterSearch: "Search",
}

func viewFieldTitle(name model.Field) string {
	if t, ok := viewTitles[name]; ok {
		return t
	}
	return filterTitles[name]
}

func viewLayer(m model.Model) tern.Element {
	form := m.ViewForm
	if !form.Open {
		return nil
	}
	children := make([]tern.Element, 0, len(model.ViewFields)+3)
	for _, field := range model.ViewFields {
		focused := field.Name == form.Field
		title := viewFieldTitle(field.Name)
		if !field.Text {
			children = append(children, choiceControl(m, model.CardView, field.Name, title, focused, "", ""))
			continue
		}
		value, placeholder := form.Name, ""
		if field.Name != model.ViewName {
			value, placeholder = form.Values.Value(field.Name), "Any"
		}
		tone := ui.ToneNeutral
		if focused {
			tone = ui.ToneAccent
		}
		children = append(children, labelledInput(string(field.Name), title, "", focused, ui.Input{
			Common: ui.Common{Key: string(field.Name), Tone: tone, Aria: title},
			Field:  ui.Field{Text: value, Cursor: caretAt(form.Caret, field.Name, value), Mode: "insert", Placeholder: ui.T(placeholder), Readonly: form.Saving},
		})...)
	}
	children = append(children, selectRoom(m, model.CardView, form.Field)...)
	if form.Saving {
		children = append(children, toneText("pending", ui.TonePending, model.Progress("Saving", "")))
	}
	if form.Err != nil {
		children = append(children, toneText("error", ui.ToneError, model.Cannot("save", strings.TrimSpace("view "+form.Name), form.Err)))
	}
	children = append(children, formFoot(cardButton("save", model.ModeViewForm, model.SaveViewForm, "Save", true), exitButton("cancel", model.ModeViewForm, model.CancelViewForm, "Cancel")))
	head, meta := "New view", ui.Rich{}
	if form.Self != "" {
		head, meta = "Edit view", ui.T(form.Self)
	}
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardView)}, Head: ui.T(head), Meta: meta, Icon: ui.IconFunnel, Modal: true, Size: ui.SizeMD}, children))
}
