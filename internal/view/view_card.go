package view

import (
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
		tone := ui.ToneNeutral
		title := viewFieldTitle(field.Name)
		prompt := title + ": "
		if field.Name == form.Field {
			tone = ui.ToneAccent
			prompt = "> " + prompt
		}
		if !field.Text {
			children = append(children, choiceControl(m, model.CardView, field.Name, title, field.Name == form.Field, "", ""))
			continue
		}
		value := form.Name
		placeholder := "Any"
		if field.Name == model.ViewName {
			placeholder = "required"
		} else {
			value = form.Values.Value(field.Name)
		}
		children = append(children, ui.Input{
			Common: ui.Common{Key: string(field.Name), Tone: tone, Aria: title},
			Field:  ui.Field{Text: value, Cursor: caretAt(form.Caret, field.Name, value), Mode: "insert", Prompt: []ui.Span{{T: prompt}}, Placeholder: ui.T(placeholder), Readonly: form.Saving},
		})
	}
	if form.Saving {
		children = append(children, toneText("pending", ui.TonePending, "Saving view…"))
	}
	if form.Err != nil {
		children = append(children, toneText("error", ui.ToneError, form.Err.Error()))
	}
	children = append(children, buttonRow(cardButton("save", model.ModeViewForm, model.SaveViewForm, "Save view", true), cardButton("cancel", model.ModeViewForm, model.CancelViewForm, "Cancel", false)))
	head := "New view"
	if form.Self != "" {
		head = "Edit view " + form.Self
	}
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardView)}, Head: ui.T(head), Modal: true, Size: ui.SizeMD, Children: children})
}
