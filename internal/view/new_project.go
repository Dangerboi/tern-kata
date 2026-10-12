package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const (
	ProjectID      = "layer.project"
	ProjectInputID = ProjectID + ".name"
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
		ui.Input{Common: ui.Common{Key: "name", Tone: ui.ToneAccent, Aria: "Project name"}, Field: ui.Field{Text: form.Name, Mode: "insert", Placeholder: "Project name", Readonly: form.Saving}},
		ui.Text{Common: ui.Common{Key: "note", Tone: ui.ToneMuted}, Text: note},
	}
	if form.Err != nil {
		children = append(children, ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: form.Err.Error()})
	}
	if form.Saving {
		children = append(children, ui.Text{Common: ui.Common{Key: "saving", Tone: ui.TonePending}, Text: "Creating Project…"})
	} else {
		children = append(children, buttonRow(keyButton("create", "Create Project (enter)", "enter", true), keyButton("cancel", "Cancel (esc)", "escape", false)))
	}
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: "project"}, Head: ui.T(model.NewProjectTitle), Modal: true, Size: ui.SizeMD, Children: children})
}
