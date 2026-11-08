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
		cardButtons(t, m, "layer.create.foot", []button{{"save", title}, {"cancel", "Cancel"}}, "save")
	}
	m := press(selectedFirst(), "n", "t")
	m = press(m, "escape")
	cardButtons(t, m, "layer.create.foot", []button{{"discard", "Discard"}, {"keep", "Keep editing"}}, "keep")
	noButtons(t, m, "layer.create", "foot.save", "foot.cancel")
	m = model.Model{Cols: 160, Project: model.Project{ID: 7}}
	m = press(m, "n", "t", "ctrl+o")
	noButtons(t, m, "layer.create", "foot", "foot.save", "foot.cancel", "foot.discard")
}

func TestFieldFormEndsInButtons(t *testing.T) {
	m := press(selectedFirst(), "+")
	cardButtons(t, m, "layer.fields.foot", []button{{"save", "Save"}, {"cancel", "Cancel"}}, "save")
	m = press(m, "b", "u", "g", "enter")
	noButtons(t, m, "layer.fields", "foot", "foot.save", "foot.cancel")
}

func TestNewProjectEndsInButtons(t *testing.T) {
	m := newProjectModel(false, "escape", "escape", "end", "enter")
	cardButtons(t, m, "layer.project.foot", []button{{"create", "Create Project"}, {"cancel", "Cancel"}}, "create")
	m = press(m, "enter")
	noButtons(t, m, "layer.project", "foot", "foot.create", "foot.cancel")
}

func TestDraftEditorEndsInButtons(t *testing.T) {
	issue := model.Issue{UID: "a", ProjectID: 2, Body: "body", Revision: 1}
	m, _ := model.Update(selectedFirst(), model.DetailLoaded{UID: "a", Selected: &model.IssueDetailData{Issue: &issue}})
	m = press(m, "e")
	cardButtons(t, m, "layer.editor.foot", []button{{"save", "Save"}, {"editor", "$EDITOR"}, {"discard", "Discard"}}, "save")
	m, _ = model.Update(m, model.KeyMsg{Chord: "paste", Text: "more"})
	asking := press(m, "escape")
	cardButtons(t, asking, "layer.editor.foot", []button{{"discard", "Discard"}, {"keep", "Keep editing"}}, "keep")
	noButtons(t, asking, "layer.editor", "foot.save", "foot.editor")
	saving := press(m, "ctrl+o")
	noButtons(t, saving, "layer.editor", "foot", "foot.save", "foot.editor", "foot.discard")
}

func TestCloseAndFiltersEndInButtons(t *testing.T) {
	m := press(selectedFirst(), "x")
	cardButtons(t, m, "layer.close.foot", []button{{"save", "Close Issue"}, {"quick", "Quick done"}, {"cancel", "Cancel"}}, "save")
	pending := press(m, "ctrl+d")
	noButtons(t, pending, "layer.close", "foot", "foot.save", "foot.quick", "foot.cancel")

	m = press(selectedFirst(), "f")
	cardButtons(t, m, "layer.filters.foot", []button{{"apply", "Apply filters"}, {"cancel", "Cancel"}}, "apply")
}

func TestCardButtonNamesEveryKeyOfItsAction(t *testing.T) {
	m := press(selectedFirst(), "n", "t", "escape")
	node := pickerNodes(t, m)["layer.create.foot.keep"]
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
	cardButtons(t, m, "layer.editor.foot", []button{{"overwrite", "Overwrite"}, {"discard", "Discard Draft"}}, "overwrite")
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
	cardButtons(t, m, "layer.editor.foot", []button{{"save", "Save"}, {"kind", "Kind"}}, "save")
	for _, want := range []string{"Confirm dana's comment", "Refute dana's comment", "Supersede dana's comment", "Reply to dana's comment", "Confirm dana's comment"} {
		m = press(m, "ctrl+r")
		if got := editor(m); got != want {
			t.Fatalf("title after cycling the kind: %q, want %q", got, want)
		}
	}
	m, _ = model.Update(m, model.DraftFailed{UID: "a", Err: model.DuplicateReply{Message: "a reply of this kind already exists"}})
	cardButtons(t, m, "layer.editor.foot", []button{{"force", "Send anyway"}}, "save")
}

func TestAgentCardEndsInButtonsAndSaysWhenThereAreNoAgents(t *testing.T) {
	m := model.Model{Cols: 160, Project: model.Project{ID: 7}, Issues: []model.Issue{{UID: "a", QualifiedID: "work#a", Status: "open"}}, SelectedUID: "a"}
	m = press(m, "w")
	cardButtons(t, m, agentNew, []button{{"split", "Split"}, {"tab", "Tab"}, {"new-background", "Background"}, {"team", "Team"}, {"pause", "Pause the team"}}, "")
	cardButtons(t, m, "layer.agent.foot.new", []button{{"cancel", "Cancel"}}, "")
	cardButtons(t, m, agentSelected, []button{{"go", "Go to"}, {"background", "Background"}, {"message", "Message"}, {"quick-reply", "Quick reply"}, {"interrupt", "Interrupt"}, {"stop", "Stop"}}, "")
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
	node := pickerNodes(t, m)["layer.create.foot.cancel"]
	if keys := node.P["keys"]; !reflect.DeepEqual(keys, []any{"ctrl+q"}) {
		t.Errorf("rebound cancel keycaps = %v", keys)
	}
	if click := node.P["actions"].(map[string]any)["click"]; click != view.ButtonClick("ctrl+q") {
		t.Errorf("rebound cancel click = %v, want the key click of ctrl+q", click)
	}
}

func TestDestructiveConfirmsAreDangerButtons(t *testing.T) {
	m := press(selectedFirst(), "n", "t", "escape")
	if got := pickerNodes(t, m)["layer.create.foot.discard"].P["variant"]; got != "danger" {
		t.Errorf("discard variant = %v", got)
	}
}

func TestMenuButtonShowsTheKeyThatOpensTheMenu(t *testing.T) {
	node := pickerNodes(t, selectedFirst())["main.topbar.right.menu"]
	if node.K != "button" || !reflect.DeepEqual(node.P["keys"], []any{"esc"}) || node.P["size"] != nil {
		t.Errorf("Menu button = %s keys %v size %v, want a default-size button with the esc keycap", node.K, node.P["keys"], node.P["size"])
	}
}

func TestMenuButtonShowsNoKeycapWhereEscDoesNotOpenTheMenu(t *testing.T) {
	m := press(selectedFirst(), "enter")
	if mode := m.CurrentMode(); mode != model.ModeDetail {
		t.Fatalf("fixture: mode %q, want detail", mode)
	}
	node := pickerNodes(t, m)["main.topbar.right.menu"]
	if keys, _ := node.P["keys"].([]any); len(keys) != 0 {
		t.Errorf("Menu button keys %v with the detail focused, want none", keys)
	}
}

func TestAgentCardDisablesWhatCannotRunWithAReason(t *testing.T) {
	m := model.Model{Cols: 160, Project: model.Project{ID: 7}, Issues: []model.Issue{{UID: "a", QualifiedID: "work#a", Status: "open"}}, SelectedUID: "a"}
	nodes := pickerNodes(t, press(m, "w"))
	for _, id := range []string{"go", "background", "interrupt", "stop"} {
		node := nodes[agentSelected+"."+id]
		if node.P["disabled"] != "No agent on this Issue yet." || node.P["variant"] == "primary" {
			t.Errorf("%s: disabled %v, variant %v", id, node.P["disabled"], node.P["variant"])
		}
	}
	if nodes[agentNew+".split"].P["disabled"] != nil || nodes["layer.agent.foot.new.cancel"].P["disabled"] != nil {
		t.Error("buttons that can run are disabled")
	}
}

// The Agent card's button ids: a new agent's row, and the selected agent's.
const (
	agentNew      = "layer.agent.foot.new.buttons"
	agentSelected = "layer.agent.foot.agent.buttons"
)

func TestAgentCardOffersForgetOnlyWhileItListsAnAgentOnAnotherHost(t *testing.T) {
	m := model.Model{Cols: 160, Project: model.Project{ID: 7}, Issues: []model.Issue{{UID: "a", QualifiedID: "work#a", Status: "open"}}, SelectedUID: "a", HostID: "a0a0a0a0", Actor: "dana"}
	if _, drawn := pickerNodes(t, press(m, "w"))[agentSelected+".forget"]; drawn {
		t.Error("Forget is drawn with no agent on another host")
	}
	record := model.AgentRecord{Address: "dana/omp-8", Host: "desk", HostID: "b1b1b1b1", Block: "8", Name: "omp", Actor: "dana", MessageFrom: []string{}}
	m.Issues[0].Metadata = map[string]any{model.AgentRecordKey(record.HostID, record.Block): record}
	if node := pickerNodes(t, press(m, "w"))[agentSelected+".forget"]; node.K != "button" || node.P["disabled"] != nil {
		t.Errorf("Forget with the desk agent selected: %s, disabled %v; want an enabled button", node.K, node.P["disabled"])
	}
}

func TestAgentCardHeadNamesTheCardAndItsMetaTheIssue(t *testing.T) {
	m := model.Model{Cols: 160, Project: model.Project{ID: 7}, Issues: []model.Issue{{UID: "a", QualifiedID: "work#a", Status: "open"}}, SelectedUID: "a"}
	node := pickerNodes(t, press(m, "w"))["layer.agent"]
	if node.P["head"] != "Agents" || node.P["icon"] != "users" {
		t.Errorf("head = %v, icon = %v, want Agents and users", node.P["head"], node.P["icon"])
	}
	if meta, _ := node.P["meta"].([]any); len(meta) != 1 || meta[0].(map[string]any)["t"] != "work#a" {
		t.Errorf("meta = %v, want the Issue's reference", node.P["meta"])
	}
}

func TestAgentCardAsksBeforeStoppingNamingTheAgentAndWhatIsDropped(t *testing.T) {
	m := model.Model{Cols: 160, Project: model.Project{ID: 7}, Issues: []model.Issue{{UID: "a", QualifiedID: "work#a", Status: "open"}}, SelectedUID: "a",
		AgentMapping: []model.AgentEntry{{IssueUID: "a", Block: "31", Teammate: "builder"}},
		Agents:       model.AgentStatesRelay{States: []model.AgentState{{Block: "31", State: model.AgentWorking}}}}
	m = press(press(m, "w"), "x")
	cardButtons(t, m, "layer.agent.foot", []button{{"stop-yes", "Stop agent"}, {"stop-keep", "Keep it"}}, "stop-keep")
	got := text(pickerNodes(t, m)["layer.agent.stop-ask"])
	if !strings.Contains(got, "builder") || !strings.Contains(got, "work#a") || !strings.Contains(got, "unsaved work") {
		t.Errorf("the question says %q", got)
	}
}

func teamCardModel() model.Model {
	return model.Model{Cols: 160, Project: model.Project{ID: 7}, Issues: []model.Issue{{UID: "a", QualifiedID: "work#a", Status: "open"}}, SelectedUID: "a",
		Snapshot: model.Snapshot{Links: []model.Link{{Type: "parent", FromIssueUID: "c", FromStatus: "open", ToIssueUID: "a"}}}}
}

func TestAgentCardOffersATeamAndDisablesItWithTheReason(t *testing.T) {
	nodes := pickerNodes(t, press(teamCardModel(), "w"))
	if node := nodes[agentNew+".team"]; node.K != "button" || text(node) != "Team" || node.P["disabled"] != nil {
		t.Errorf("team button: %s %q disabled %v", node.K, text(node), node.P["disabled"])
	}
	bare := teamCardModel()
	bare.Snapshot = model.Snapshot{}
	if got := pickerNodes(t, press(bare, "w"))[agentNew+".team"].P["disabled"]; got != model.TeamNoChildReason {
		t.Errorf("a team with no child: disabled %v, want %q", got, model.TeamNoChildReason)
	}
}

func TestTeamDialogShowsWhereTheCoordinatorGoesAndHowManyChildrenRun(t *testing.T) {
	m := press(teamCardModel(), "w", "c")
	cardButtons(t, m, "layer.agent.foot", []button{{"team-split", "Split"}, {"team-tab", "Tab"}, {"team-background", "Background"}, {"team-less", "Fewer"}, {"team-more", "More"},
		{"team-start", "Start the team"}, {"team-cancel", "Cancel"}}, "team-start")
	nodes := pickerNodes(t, m)
	if got := text(nodes["layer.agent.team-where"]); got != "Coordinator: split" {
		t.Errorf("where says %q", got)
	}
	if got := text(nodes["layer.agent.team-limit"]); got != "Children at once: 2 (1 to 8)" {
		t.Errorf("limit says %q", got)
	}
	m = press(m, "g", "up")
	nodes = pickerNodes(t, m)
	if text(nodes["layer.agent.team-where"]) != "Coordinator: background" || text(nodes["layer.agent.team-limit"]) != "Children at once: 3 (1 to 8)" {
		t.Errorf("after g and up: %q, %q", text(nodes["layer.agent.team-where"]), text(nodes["layer.agent.team-limit"]))
	}
}
