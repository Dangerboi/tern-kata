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
	note := "This directory stays on " + m.Project.Name + "."
	if form.Bind {
		note = "This directory will use it."
	}
	children := labelledInput(inputLeaf, "Name", "", true, ui.Input{Common: ui.Common{Key: inputLeaf, Tone: ui.ToneAccent, Aria: "Name"}, Field: ui.Field{Text: form.Name, Cursor: caretAt(form.Caret, "", form.Name), Mode: "insert", Placeholder: ui.T("Project name"), Readonly: form.Saving}})
	children = append(children, toneText("note", ui.ToneMuted, note))
	busy := ""
	if form.Err != nil {
		children = append(children, toneText("error", ui.ToneError, form.Err.Error()))
	}
	if form.Saving {
		busy = model.Progress("Creating", "Project")
		children = append(children, toneText("saving", ui.TonePending, busy))
	}
	create := unavailable(cardButton("create", model.ModeProjectForm, model.ProjectSubmit, "Create", true), busy)
	cancel := unavailable(exitButton("cancel", model.ModeProjectForm, model.ProjectCancel, "Cancel"), busy)
	children = append(children, buttonRow(create, cancel))
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardProject)}, Head: ui.T(model.NewProjectTitle), Icon: ui.IconFolderPlus, Modal: true, Size: ui.SizeMD}, children))
}
