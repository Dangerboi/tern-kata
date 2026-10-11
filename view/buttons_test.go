package view_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/model"
	"github.com/Dangerboi/tern-kata/view"
	g "go.kenn.io/kata/pkg/client/generated"
)

type button struct{ id, text, click string }

// cardButtons are the buttons the card ends in, by node id, all clicking a key.
func cardButtons(t *testing.T, m model.Model, card string, want []button, primary string) {
	t.Helper()
	nodes := pickerNodes(t, m)
	for _, b := range want {
		node, ok := nodes[card+"."+b.id]
		if !ok {
			t.Errorf("%s has no %s button", card, b.id)
			continue
		}
		if text(node) != b.text {
			t.Errorf("%s.%s text = %q, want %q", card, b.id, text(node), b.text)
		}
		actions, _ := node.P["actions"].(map[string]any)
		if actions["click"] != view.KeyClick(b.click) {
			t.Errorf("%s.%s click = %v, want %s", card, b.id, actions["click"], view.KeyClick(b.click))
		}
		if (node.P["tone"] == "accent") != (b.id == primary) {
			t.Errorf("%s.%s tone = %v, primary is %s", card, b.id, node.P["tone"], primary)
		}
	}
}

func noButtons(t *testing.T, m model.Model, card string, ids ...string) {
	t.Helper()
	nodes := pickerNodes(t, m)
	if _, ok := nodes[card]; !ok {
		t.Fatalf("%s is not drawn", card)
	}
	for _, id := range ids {
		if _, ok := nodes[card+"."+id]; ok {
			t.Errorf("%s.%s drawn while the card is pending", card, id)
		}
	}
}

func press(m model.Model, chords ...string) model.Model {
	for _, chord := range chords {
		text := ""
		if len([]rune(chord)) == 1 {
			text = chord
		}
		m, _ = model.Update(m, model.KeyMsg{Chord: chord, Text: text})
	}
	return m
}

func TestCreateCardEndsInButtons(t *testing.T) {
	for chord, title := range map[string]string{"n": "Create Issue", "shift+n": "Create child Issue"} {
		m := press(selectedFirst(), chord)
		cardButtons(t, m, "layer.create.actions", []button{{"save", title + " (ctrl+o)", "ctrl+o"}, {"cancel", "Cancel (esc)", "escape"}}, "save")
	}
	m := press(selectedFirst(), "n", "t")
	m = press(m, "escape")
	cardButtons(t, m, "layer.create.actions", []button{{"discard", "Discard (y)", "y"}, {"keep", "Keep editing (n / esc)", "n"}}, "keep")
	noButtons(t, m, "layer.create", "actions.save", "actions.cancel")
	m = model.Model{Cols: 160, Project: model.Project{ID: 7}}
	m = press(m, "n", "t", "ctrl+o")
	noButtons(t, m, "layer.create", "actions", "actions.save", "actions.cancel", "actions.discard")
}

func TestFieldFormEndsInButtons(t *testing.T) {
	m := press(selectedFirst(), "+")
	cardButtons(t, m, "layer.fields.actions", []button{{"save", "Save (enter)", "enter"}, {"cancel", "Cancel (esc)", "escape"}}, "save")
	m = press(m, "b", "u", "g", "enter")
	noButtons(t, m, "layer.fields", "actions", "actions.save", "actions.cancel")
}

func TestNewProjectEndsInButtons(t *testing.T) {
	m := newProjectModel(false, "escape", "end", "enter")
	cardButtons(t, m, "layer.project.actions", []button{{"create", "Create Project (enter)", "enter"}, {"cancel", "Cancel (esc)", "escape"}}, "create")
	m = press(m, "enter")
	noButtons(t, m, "layer.project", "actions", "actions.create", "actions.cancel")
}

func TestDraftEditorEndsInButtons(t *testing.T) {
	issue := model.Issue{UID: "a", ProjectID: 2, Body: "body", Revision: 1}
	m, _ := model.Update(selectedFirst(), model.DetailLoaded{UID: "a", Selected: &g.UISelectedAuthority{Issue: &issue}})
	m = press(m, "e")
	cardButtons(t, m, "layer.editor.actions", []button{{"save", "Save (ctrl+o)", "ctrl+o"}, {"editor", "$EDITOR (ctrl+e)", "ctrl+e"}, {"discard", "Discard (esc)", "escape"}}, "save")
	m, _ = model.Update(m, model.KeyMsg{Chord: "paste", Text: "more"})
	asking := press(m, "escape")
	cardButtons(t, asking, "layer.editor.actions", []button{{"discard", "Discard (y)", "y"}, {"keep", "Keep editing (n / esc)", "n"}}, "keep")
	noButtons(t, asking, "layer.editor", "actions.save", "actions.editor")
	saving := press(m, "ctrl+o")
	noButtons(t, saving, "layer.editor", "actions", "actions.save", "actions.editor", "actions.discard")
}

func TestCloseAndFiltersButtonsClickKeys(t *testing.T) {
	m := press(selectedFirst(), "x")
	nodes := pickerNodes(t, m)
	for id, want := range map[string]string{"layer.close.actions.save": view.KeyClick("ctrl+o"), "layer.close.actions.quick": view.KeyClick("ctrl+d"), "layer.close.actions.cancel": view.KeyClick("escape")} {
		actions, _ := nodes[id].P["actions"].(map[string]any)
		if actions["click"] != want {
			t.Errorf("%s click = %v, want %s", id, actions["click"], want)
		}
	}
	if nodes["layer.close.actions.save"].P["tone"] != "accent" {
		t.Error("Close Issue button is not the primary")
	}
	pending := press(m, "ctrl+d")
	noButtons(t, pending, "layer.close", "actions", "actions.save", "actions.quick", "actions.cancel")

	m = press(selectedFirst(), "f")
	cardButtons(t, m, "layer.filters.actions", []button{{"apply", "Apply filters", "ctrl+o"}, {"cancel", "Cancel", "escape"}}, "apply")
}

func TestDraftConflictEndsInButtons(t *testing.T) {
	issue := model.Issue{UID: "a", ProjectID: 2, Body: "body", Revision: 1}
	m, _ := model.Update(selectedFirst(), model.DetailLoaded{UID: "a", Selected: &g.UISelectedAuthority{Issue: &issue}})
	m = press(m, "e", "ctrl+o")
	latest := issue
	latest.Revision, latest.Body = 2, "Remote body"
	m, _ = model.Update(m, model.DraftConflict{UID: "a", Issue: latest})
	cardButtons(t, m, "layer.editor.actions", []button{{"overwrite", "Overwrite (o)", "o"}, {"discard", "Discard Draft (d)", "d"}}, "overwrite")
}
