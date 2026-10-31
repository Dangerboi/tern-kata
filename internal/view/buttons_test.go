package view_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
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
		if node.K != "button" || text(node) != b.text {
			t.Errorf("%s.%s is %s %q, want a button of %q", card, b.id, node.K, text(node), b.text)
		}
		if keys, _ := node.P["keys"].([]any); len(keys) == 0 {
			t.Errorf("%s.%s shows no keycaps", card, b.id)
		}
		if (node.P["variant"] == "primary") != (b.id == primary) {
			t.Errorf("%s.%s variant = %v, primary is %s", card, b.id, node.P["variant"], primary)
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
	m := newProjectModel(false, "escape", "escape", "end", "enter")
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
	node := pickerNodes(t, m)["layer.create.actions.keep"]
	if keys := node.P["keys"]; !reflect.DeepEqual(keys, []any{"n", "esc"}) || text(node) != "Keep editing" {
		t.Errorf("keep button = %q with keys %v", text(node), keys)
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

func TestAgentCardEndsInButtonsAndSaysWhenThereAreNoAgents(t *testing.T) {
	m := model.Model{Cols: 160, Project: model.Project{ID: 7}, Issues: []model.Issue{{UID: "a", QualifiedID: "work#a", Status: "open"}}, SelectedUID: "a"}
	m = press(m, "w")
	cardButtons(t, m, "layer.agent.actions", []button{{"go", "Go to agent"}, {"split", "New in a split"}, {"tab", "New in a tab"},
		{"new-background", "New in the background"}, {"background", "Send to background"}, {"cancel", "Cancel"}}, "")
	if got := text(pickerNodes(t, m)["layer.agent.none"]); got != "No agents on this Issue yet." {
		t.Errorf("empty card says %q", got)
	}
}

func TestCardButtonKeycapsAndClickFollowARebind(t *testing.T) {
	saved := slices.Clone(model.Keymap)
	t.Cleanup(func() { model.Keymap = saved })
	for i, b := range model.Keymap {
		if b.Mode == model.ModeCreate && b.Action == model.CancelDraft && !b.Headless() {
			model.Keymap[i].Chord, model.Keymap[i].Key = "ctrl+q", "ctrl+q"
		}
	}
	m := press(selectedFirst(), "n", "t")
	node := pickerNodes(t, m)["layer.create.actions.cancel"]
	if keys := node.P["keys"]; !reflect.DeepEqual(keys, []any{"ctrl+q"}) {
		t.Errorf("rebound cancel keycaps = %v", keys)
	}
	if click := node.P["actions"].(map[string]any)["click"]; click != view.KeyClick("ctrl+q") {
		t.Errorf("rebound cancel click = %v, want the key click of ctrl+q", click)
	}
}

func TestDestructiveConfirmsAreDangerButtons(t *testing.T) {
	m := press(selectedFirst(), "n", "t", "escape")
	if got := pickerNodes(t, m)["layer.create.actions.discard"].P["variant"]; got != "danger" {
		t.Errorf("discard variant = %v", got)
	}
}

func TestMenuButtonShowsTheKeyThatOpensTheMenu(t *testing.T) {
	node := pickerNodes(t, selectedFirst())["main.topbar.right.menu"]
	if node.K != "button" || !reflect.DeepEqual(node.P["keys"], []any{"esc"}) || node.P["size"] != nil {
		t.Errorf("Menu button = %s keys %v size %v, want a default-size button with the esc keycap", node.K, node.P["keys"], node.P["size"])
	}
}

func TestAgentCardDisablesWhatCannotRunWithAReason(t *testing.T) {
	m := model.Model{Cols: 160, Project: model.Project{ID: 7}, Issues: []model.Issue{{UID: "a", QualifiedID: "work#a", Status: "open"}}, SelectedUID: "a"}
	nodes := pickerNodes(t, press(m, "w"))
	for _, id := range []string{"go", "background"} {
		node := nodes["layer.agent.actions."+id]
		if node.P["disabled"] != "No agent on this Issue yet" || node.P["variant"] == "primary" {
			t.Errorf("%s: disabled %v, variant %v", id, node.P["disabled"], node.P["variant"])
		}
	}
	if nodes["layer.agent.actions.split"].P["disabled"] != nil || nodes["layer.agent.actions.cancel"].P["disabled"] != nil {
		t.Error("buttons that can run are disabled")
	}
}
