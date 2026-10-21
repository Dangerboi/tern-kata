package model

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
)

const (
	ModeProjectForm Mode   = "project-form"
	ProjectSubmit   Action = "project-submit"
	ProjectCancel   Action = "project-cancel"
	// NewProjectKey and NewProjectTitle name the New Project row that ends the
	// sidebar and the picker, as Collection.Key and Title name a Collection's.
	NewProjectKey   = "new-project"
	NewProjectTitle = "New Project"
)

// ProjectFormState is the New Project form. Bind also binds the current
// directory to the Project, as kata init does; without it the form only
// creates the Project, as kata projects create does.
type ProjectFormState struct {
	Open, Saving, Bind bool
	Name               string
	Caret              Caret
	Err                error
}

// CreateProject creates a Project and changes no workspace file: kata projects create.
type CreateProject struct{ Workspace, Actor, Name string }

func (CreateProject) cmd() {}

// BindProject creates a Project, or finds the active one of that name, and
// binds Workspace to it: kata init --project.
type BindProject struct{ Workspace, Actor, Name string }

func (BindProject) cmd() {}

// ProjectSaved is the daemon's answer to CreateProject or BindProject.
type ProjectSaved struct {
	Project Project
	Err     error
}

func (ProjectSaved) msg() {}

// openProjectForm opens the form over whatever is showing. Only a directory
// that is not bound is bound to the new Project.
func openProjectForm(m Model) Model {
	name := filepath.Base(m.Workspace)
	if name == "." || name == string(filepath.Separator) {
		name = ""
	}
	m.ProjectForm = ProjectFormState{Open: true, Bind: !m.Bound, Name: name}
	return m
}

func submitProject(m Model) (Model, []Cmd) {
	form := &m.ProjectForm
	name := strings.TrimSpace(form.Name)
	switch {
	case name == "":
		form.Err = errors.New("Name is required.")
		return m, nil
	case m.Stale:
		form.Err = errors.New("Daemon disconnected; reconnect before creating a Project.")
		return m, nil
	}
	form.Saving, form.Err = true, nil
	if form.Bind {
		return m, []Cmd{BindProject{Workspace: m.Workspace, Actor: m.Actor, Name: name}}
	}
	return m, []Cmd{CreateProject{Workspace: m.Workspace, Actor: m.Actor, Name: name}}
}

func updateProjectForm(m Model, msg Msg) (Model, []Cmd, bool) {
	switch msg := msg.(type) {
	case CollectionPointerSelected:
		// A click on the sidebar's New Project row is enter on it.
		if msg.Key == NewProjectKey && !m.ProjectForm.Open {
			m.Focus, m.CollectionCursor = ColumnCollections, entryIndex(m.SidebarEntries(), NewProjectKey)
			return openProjectForm(m), nil, true
		}
	case ProjectSaved:
		next, cmds := projectSaved(m, msg)
		return next, cmds, true
	}
	return m, nil, false
}

// projectSaved ends the form. A rejection keeps it open with the input and the
// daemon's message. Otherwise the new Project becomes the Collection, as if it
// had been picked: a Project the directory is bound to also becomes the bound one.
func projectSaved(m Model, msg ProjectSaved) (Model, []Cmd) {
	if !m.ProjectForm.Open || !m.ProjectForm.Saving {
		return m, nil
	}
	if msg.Err != nil {
		m.ProjectForm.Saving, m.ProjectForm.Err = false, msg.Err
		return m, nil
	}
	if m.ProjectForm.Bind {
		m.Project, m.Bound = msg.Project, true
	}
	collection := ProjectCollection(msg.Project.UID, msg.Project.Name)
	if !slices.Contains(m.Collections, collection) {
		// The snapshot on screen predates the Project; list it until the next one arrives.
		m.Collections = append(slices.Clip(m.Collections), collection)
		m.Snapshot.Catalog = append(slices.Clip(m.Snapshot.Catalog), CatalogProject{Project: msg.Project})
	}
	m.ProjectForm, m.Picker, m.Focus = ProjectFormState{}, PickerState{}, ColumnList
	return updateCollection(m, CollectionSelected{Collection: collection})
}

func updateProjectFormKey(m Model, msg KeyMsg) (Model, []Cmd) {
	form := &m.ProjectForm
	binding, bound := LookupBindingForMode(msg.Chord, ModeProjectForm)
	switch {
	case form.Saving:
		return m, nil
	case bound && binding.Action == ProjectSubmit:
		return submitProject(m)
	case bound && binding.Action == ProjectCancel:
		m.ProjectForm = ProjectFormState{}
	default:
		var edited bool
		if form.Name, edited = editText(form.Name, &form.Caret, "", binding, bound, msg); edited {
			form.Err = nil
		}
	}
	return m, nil
}
