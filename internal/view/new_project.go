package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

func projectLayer(m model.Model) tern.Element {
	form := m.ProjectForm
	if !form.Open {
		return nil
	}
	note := "Creates the Project and binds this directory to it."
	if !form.Bind {
		note = "Creates the Project. This directory stays bound to " + m.Project.Name + "."
	}
	children := []tern.Element{
		ui.Input{Common: ui.Common{Key: inputLeaf, Tone: ui.ToneAccent, Aria: "Project name"}, Field: ui.Field{Text: form.Name, Cursor: caretAt(form.Caret, "", form.Name), Mode: "insert", Placeholder: "Project name", Readonly: form.Saving}},
		toneText("note", ui.ToneMuted, note),
	}
	if form.Err != nil {
		children = append(children, toneText("error", ui.ToneError, form.Err.Error()))
	}
	if form.Saving {
		children = append(children, toneText("saving", ui.TonePending, "Creating Project…"))
	} else {
		children = append(children, buttonRow(cardButton("create", model.ModeProjectForm, model.ProjectSubmit, "Create Project", true), cardButton("cancel", model.ModeProjectForm, model.ProjectCancel, "Cancel", false)))
	}
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardProject)}, Head: ui.T(model.NewProjectTitle), Modal: true, Size: ui.SizeMD, Children: children})
}
