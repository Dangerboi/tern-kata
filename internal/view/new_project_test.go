package view_test

import (
	"errors"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

const newProjectIDPrefix = "layer.project"

// newProjectModel is a loaded Pane in /home/dev/my-repo, bound to Project
// smoke or not, with keys sent to it.
func newProjectModel(bound bool, keys ...string) model.Model {
	m := pickerLoaded(bound)
	m.Workspace = "/home/dev/my-repo"
	for _, key := range keys {
		m, _ = model.Update(m, model.KeyMsg{Chord: key, Text: key})
	}
	return m
}

func TestRenderSidebarEndsWithNewProjectWhenUnbound(t *testing.T) {
	m := newProjectModel(false)
	list := pickerNodes(t, m)[view.CollectionsID]
	ids := pickerChildIDs(list)
	if len(ids) == 0 || ids[len(ids)-1] != view.CollectionsID+".new-project" {
		t.Fatalf("sidebar rows end with %v", ids)
	}
	if list.P["selected"] == view.CollectionsID+".new-project" {
		t.Fatalf("New Project is selected before the cursor reaches it")
	}

	// A bound directory has no sidebar, so no row for it either.
	if _, ok := pickerNodes(t, newProjectModel(true))[view.CollectionsID+".new-project"]; ok {
		t.Fatal("a bound directory rendered the sidebar's New Project row")
	}

	m = newProjectModel(false, "escape", "end")
	nodes := pickerNodes(t, m)
	if nodes[view.CollectionsID].P["selected"] != view.CollectionsID+".new-project" || nodes[view.CollectionsID+".new-project"].P["label"] != "New Project" {
		t.Fatalf("selected row = %v, label = %v", nodes[view.CollectionsID].P["selected"], nodes[view.CollectionsID+".new-project"].P["label"])
	}
}

func TestRenderPickerRowForNewProject(t *testing.T) {
	m := newProjectModel(true, "escape", "end")
	nodes := pickerNodes(t, m)
	row, ok := nodes["layer.collections.list.new-project"]
	if !ok {
		t.Fatalf("picker rows = %v", pickerChildIDs(nodes["layer.collections.list"]))
	}
	if row.P["label"] != "New Project" || row.P["value"] != nil {
		t.Fatalf("row = %v: it is not a Collection, so never marked current", row.P)
	}
	if nodes["layer.collections.list"].P["selected"] != "layer.collections.list.new-project" {
		t.Fatalf("selected = %v", nodes["layer.collections.list"].P["selected"])
	}
}

func TestRenderNewProjectFormFollowsTheModel(t *testing.T) {
	closed := newProjectModel(false, "escape", "end")
	if _, ok := pickerNodes(t, closed)[newProjectIDPrefix]; ok {
		t.Fatal("a closed form rendered")
	}

	m := newProjectModel(false, "escape", "end", "enter")
	nodes := pickerNodes(t, m)
	if _, ok := nodes[newProjectIDPrefix]; !ok {
		t.Fatal("form not rendered")
	}
	if view.ProjectInputID != newProjectIDPrefix+".name" {
		t.Fatalf("name input id %q", view.ProjectInputID)
	}
	if nodes[view.ProjectInputID].P["text"] != "my-repo" {
		t.Fatalf("name input = %v", nodes[view.ProjectInputID].P)
	}

	// A rejection shows kata's message and keeps the input.
	m, _ = model.Update(m, model.KeyMsg{Chord: "enter"})
	m, _ = model.Update(m, model.ProjectSaved{Err: errors.New("project with this name was archived")})
	nodes = pickerNodes(t, m)
	if nodes[newProjectIDPrefix+".error"].P["text"] != "project with this name was archived" || nodes[view.ProjectInputID].P["text"] != "my-repo" {
		t.Fatalf("rejected form: error=%v input=%v", nodes[newProjectIDPrefix+".error"].P, nodes[view.ProjectInputID].P)
	}

	// While the daemon works the input is read-only.
	m, _ = model.Update(m, model.KeyMsg{Chord: "enter"})
	nodes = pickerNodes(t, m)
	if nodes[view.ProjectInputID].P["readonly"] != true {
		t.Fatalf("saving form: input=%v", nodes[view.ProjectInputID].P)
	}
	if _, ok := nodes[newProjectIDPrefix+".saving"]; !ok {
		t.Fatal("no saving marker")
	}
}

func TestRenderNewProjectFormCoversThePicker(t *testing.T) {
	m := newProjectModel(true, "escape", "end", "enter")
	nodes := pickerNodes(t, m)
	if _, ok := nodes[newProjectIDPrefix]; !ok {
		t.Fatal("form not rendered over the picker")
	}
	if _, ok := nodes["layer.collections"]; ok {
		t.Fatal("picker still rendered beside the form")
	}
	m, _ = model.Update(m, model.KeyMsg{Chord: "escape"})
	nodes = pickerNodes(t, m)
	if _, ok := nodes["layer.collections"]; !ok {
		t.Fatal("esc did not bring the picker back")
	}
	if _, ok := nodes[newProjectIDPrefix]; ok {
		t.Fatal("form still rendered after esc")
	}
}
