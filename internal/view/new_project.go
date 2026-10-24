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
		ui.Text{Common: ui.Common{Key: "note", Tone: ui.ToneMuted}, Text: note},
	}
	if form.Err != nil {
		children = append(children, ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: form.Err.Error()})
	}
	if form.Saving {
		children = append(children, ui.Text{Common: ui.Common{Key: "saving", Tone: ui.TonePending}, Text: "Creating Project…"})
	} else {
		children = append(children, buttonRow(cardButton("create", model.ModeProjectForm, model.ProjectSubmit, "Create Project", true), cardButton("cancel", model.ModeProjectForm, model.ProjectCancel, "Cancel", false)))
	}
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardProject)}, Head: ui.T(model.NewProjectTitle), Modal: true, Size: ui.SizeMD, Children: children})
}
