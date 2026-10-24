package view_test

import (
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

type button struct{ id, text string }

// cardButtons are the buttons the card ends in, by node id; text is the words the label starts with. The chord and the keys
// in the label come from the card bindings, which the driver's card test checks against the keys.
func cardButtons(t *testing.T, m model.Model, card string, want []button, primary string) {
	t.Helper()
	nodes := pickerNodes(t, m)
	for _, b := range want {
		node, ok := nodes[card+"."+b.id]
		if !ok {
			t.Errorf("%s has no %s button", card, b.id)
			continue
		}
		if !strings.HasPrefix(text(node), b.text+" (") {
			t.Errorf("%s.%s text = %q, want %q and the keys", card, b.id, text(node), b.text)
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
		cardButtons(t, m, "layer.create.actions", []button{{"save", title}, {"cancel", "Cancel"}}, "save")
	}
	m := press(selectedFirst(), "n", "t")
	m = press(m, "escape")
	cardButtons(t, m, "layer.create.actions", []button{{"discard", "Discard"}, {"keep", "Keep editing"}}, "keep")
	noButtons(t, m, "layer.create", "actions.save", "actions.cancel")
	m = model.Model{Cols: 160, Project: model.Project{ID: 7}}
	m = press(m, "n", "t", "ctrl+o")
	noButtons(t, m, "layer.create", "actions", "actions.save", "actions.cancel", "actions.discard")
}

func TestFieldFormEndsInButtons(t *testing.T) {
	m := press(selectedFirst(), "+")
	cardButtons(t, m, "layer.fields.actions", []button{{"save", "Save"}, {"cancel", "Cancel"}}, "save")
	m = press(m, "b", "u", "g", "enter")
	noButtons(t, m, "layer.fields", "actions", "actions.save", "actions.cancel")
}

func TestNewProjectEndsInButtons(t *testing.T) {
	m := newProjectModel(false, "escape", "end", "enter")
	cardButtons(t, m, "layer.project.actions", []button{{"create", "Create Project"}, {"cancel", "Cancel"}}, "create")
	m = press(m, "enter")
	noButtons(t, m, "layer.project", "actions", "actions.create", "actions.cancel")
}

func TestDraftEditorEndsInButtons(t *testing.T) {
	issue := model.Issue{UID: "a", ProjectID: 2, Body: "body", Revision: 1}
	m, _ := model.Update(selectedFirst(), model.DetailLoaded{UID: "a", Selected: &model.IssueDetailData{Issue: &issue}})
	m = press(m, "e")
	cardButtons(t, m, "layer.editor.actions", []button{{"save", "Save"}, {"editor", "$EDITOR"}, {"discard", "Discard"}}, "save")
	m, _ = model.Update(m, model.KeyMsg{Chord: "paste", Text: "more"})
	asking := press(m, "escape")
	cardButtons(t, asking, "layer.editor.actions", []button{{"discard", "Discard"}, {"keep", "Keep editing"}}, "keep")
	noButtons(t, asking, "layer.editor", "actions.save", "actions.editor")
	saving := press(m, "ctrl+o")
	noButtons(t, saving, "layer.editor", "actions", "actions.save", "actions.editor", "actions.discard")
}

func TestCloseAndFiltersEndInButtons(t *testing.T) {
	m := press(selectedFirst(), "x")
	cardButtons(t, m, "layer.close.actions", []button{{"save", "Close Issue"}, {"quick", "Quick done"}, {"cancel", "Cancel"}}, "save")
	pending := press(m, "ctrl+d")
	noButtons(t, pending, "layer.close", "actions", "actions.save", "actions.quick", "actions.cancel")

	m = press(selectedFirst(), "f")
	cardButtons(t, m, "layer.filters.actions", []button{{"apply", "Apply filters"}, {"cancel", "Cancel"}}, "apply")
}

func TestCardButtonNamesEveryKeyOfItsAction(t *testing.T) {
	m := press(selectedFirst(), "n", "t", "escape")
	if got := text(pickerNodes(t, m)["layer.create.actions.keep"]); got != "Keep editing (n / esc)" {
		t.Errorf("keep button = %q", got)
	}
}

func TestDraftConflictEndsInButtons(t *testing.T) {
	issue := model.Issue{UID: "a", ProjectID: 2, Body: "body", Revision: 1}
	m, _ := model.Update(selectedFirst(), model.DetailLoaded{UID: "a", Selected: &model.IssueDetailData{Issue: &issue}})
	m = press(m, "e", "ctrl+o")
	latest := issue
	latest.Revision, latest.Body = 2, "Remote body"
	m, _ = model.Update(m, model.DraftConflict{UID: "a", Issue: latest})
	cardButtons(t, m, "layer.editor.actions", []button{{"overwrite", "Overwrite"}, {"discard", "Discard Draft"}}, "overwrite")
}

func TestReplyDraftNamesItsKindAndOffersTheKindKeyAndSendAnyway(t *testing.T) {
	issue := model.Issue{UID: "a", ProjectID: 2, Body: "body", Revision: 1}
	comments := []model.Comment{{UID: "c1", Author: "dana", Body: "claim"}}
	m, _ := model.Update(selectedFirst(), model.DetailLoaded{UID: "a", Selected: &model.IssueDetailData{Issue: &issue, Comments: comments}})
	m = press(m, "enter", "shift+r")
	editor := func(m model.Model) string {
		title, _ := pickerNodes(t, m)["layer.editor"].P["head"].(string)
		return title
	}
	if got := editor(m); !strings.Contains(got, "Reply to dana's comment") {
		t.Fatalf("reply title: %q", got)
	}
	cardButtons(t, m, "layer.editor.actions", []button{{"save", "Save"}, {"kind", "Kind"}}, "save")
	for _, want := range []string{"Confirm dana's comment", "Refute dana's comment", "Supersede dana's comment", "Reply to dana's comment", "Confirm dana's comment"} {
		m = press(m, "ctrl+r")
		if got := editor(m); got != want {
			t.Fatalf("title after cycling the kind: %q, want %q", got, want)
		}
	}
	m, _ = model.Update(m, model.DraftFailed{UID: "a", Err: model.DuplicateReply{Message: "a reply of this kind already exists"}})
	cardButtons(t, m, "layer.editor.actions", []button{{"force", "Send anyway"}}, "save")
}
