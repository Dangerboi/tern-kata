package model_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func entry(uid, block string, minute int) model.AgentEntry {
	return model.AgentEntry{IssueUID: uid, Block: block, StartedAt: time.Date(2026, 10, 30, 9, minute, 0, 0, time.UTC)}
}

// relayed feeds one relayed list and returns the commands but the save of the relay for `tern-kata agents`, which every relay
// ends with and these tests are not about.
func relayed(m model.Model, states ...model.AgentState) (model.Model, []model.Cmd) {
	m, cmds := model.Update(m, model.AgentStatesMsg{States: states})
	if n := len(cmds) - 1; n >= 0 {
		if _, saved := cmds[n].(model.SaveAgentStates); saved {
			cmds = cmds[:n]
		}
	}
	if len(cmds) == 0 {
		cmds = nil
	}
	return m, cmds
}

func TestTheBadgeNamesTheMostPressingAgentWithTheCount(t *testing.T) {
	for _, tt := range []struct {
		name    string
		mapping []model.AgentEntry
		states  []model.AgentState
		text    string
		tone    model.AgentTone
		block   string
	}{
		{"working", []model.AgentEntry{entry("a", "1", 1)}, []model.AgentState{{Block: "1", State: "working"}}, "agent working", model.AgentToneAccent, "1"},
		{"waiting", []model.AgentEntry{entry("a", "1", 1)}, []model.AgentState{{Block: "1", State: "waiting_input"}}, "agent waiting for you", model.AgentToneWarning, "1"},
		{"idle", []model.AgentEntry{entry("a", "1", 1)}, []model.AgentState{{Block: "1", State: "idle"}}, "agent idle", model.AgentToneMuted, "1"},
		{"exited", []model.AgentEntry{entry("a", "1", 1)}, []model.AgentState{{Block: "1", State: "exited"}}, "agent exited", model.AgentToneMuted, "1"},
		{"no relay", []model.AgentEntry{entry("a", "1", 1)}, nil, "agent", model.AgentToneMuted, "1"},
		{"block not listed", []model.AgentEntry{entry("a", "1", 1)}, []model.AgentState{{Block: "2", State: "working"}}, "agent", model.AgentToneMuted, "1"},
		{"unknown word", []model.AgentEntry{entry("a", "1", 1)}, []model.AgentState{{Block: "1", State: "sleeping"}}, "agent", model.AgentToneMuted, "1"},
		{"waiting beats working", []model.AgentEntry{entry("a", "1", 5), entry("a", "2", 1)}, []model.AgentState{{Block: "1", State: "working"}, {Block: "2", State: "waiting_input"}}, "agent waiting for you · 2", model.AgentToneWarning, "2"},
		{"working beats idle", []model.AgentEntry{entry("a", "1", 5), entry("a", "2", 1)}, []model.AgentState{{Block: "1", State: "idle"}, {Block: "2", State: "working"}}, "agent working · 2", model.AgentToneAccent, "2"},
		{"idle beats exited", []model.AgentEntry{entry("a", "1", 5), entry("a", "2", 1)}, []model.AgentState{{Block: "1", State: "exited"}, {Block: "2", State: "idle"}}, "agent idle · 2", model.AgentToneMuted, "2"},
		{"a state beats none", []model.AgentEntry{entry("a", "1", 5), entry("a", "2", 1)}, []model.AgentState{{Block: "2", State: "exited"}}, "agent exited · 2", model.AgentToneMuted, "2"},
		{"tie goes to the newest", []model.AgentEntry{entry("a", "1", 1), entry("a", "2", 9), entry("a", "3", 4)}, []model.AgentState{{Block: "1", State: "idle"}, {Block: "2", State: "idle"}, {Block: "3", State: "idle"}}, "agent idle · 3", model.AgentToneMuted, "2"},
		{"tie of no state goes to the newest", []model.AgentEntry{entry("a", "1", 9), entry("a", "2", 1)}, nil, "agent · 2", model.AgentToneMuted, "1"},
		{"other Issues do not count", []model.AgentEntry{entry("a", "1", 1), entry("b", "2", 2)}, []model.AgentState{{Block: "2", State: "working"}}, "agent", model.AgentToneMuted, "1"},
	} {
		m := model.Model{AgentMapping: tt.mapping}
		if tt.states != nil {
			m.Agents = model.AgentStatesRelay{Received: true, States: tt.states}
		}
		got, ok := m.AgentBadgeFor("a")
		want := model.AgentBadge{Text: tt.text, Tone: tt.tone, Block: tt.block}
		if !ok || got != want {
			t.Errorf("%s: %+v (%v), want %+v", tt.name, got, ok, want)
		}
	}
	if _, ok := (model.Model{AgentMapping: []model.AgentEntry{entry("b", "1", 1)}}).AgentBadgeFor("a"); ok {
		t.Error("a badge for an Issue with no agent")
	}
}

func TestKataPanesInTheRelayNeverMatch(t *testing.T) {
	m := model.Model{AgentMapping: []model.AgentEntry{entry("a", "7", 1)}}
	m, _ = relayed(m, model.AgentState{Block: "9", State: "working"}, model.AgentState{Block: "7", State: "idle"})
	if badge, _ := m.AgentBadgeFor("a"); badge.Text != "agent idle" {
		t.Errorf("badge %q", badge.Text)
	}
	if agents := m.IssueAgents("a"); len(agents) != 1 || agents[0].Block != "7" || agents[0].State != "idle" {
		t.Errorf("agents %+v", agents)
	}
}

func TestAnAgentOfAnUnknownPromptFileInTheRelayReReadsAgentsJSONOnce(t *testing.T) {
	m := model.Model{AgentMapping: []model.AgentEntry{entry("a", "7", 1)}}
	other := model.AgentState{Block: "9", State: "idle", Command: "omp @/state/agent-prompts/prompt-4-5.md"}
	m, cmds := relayed(m, model.AgentState{Block: "7", State: "idle"}, other)
	if want := []model.Cmd{model.RefreshAgents{Seq: 1}}; !reflect.DeepEqual(cmds, want) {
		t.Fatalf("first relay: %#v, want a re-read for the unknown prompt file", cmds)
	}
	m, _ = model.Update(m, model.AgentsRefreshed{Agents: m.AgentMapping})
	// The same prompt file again, even under another id, asks nothing.
	other.Block = "10"
	if _, cmds := relayed(m, model.AgentState{Block: "7", State: "working"}, other); cmds != nil {
		t.Errorf("a prompt file that already asked re-read: %#v", cmds)
	}
	// An unknown id with no prompt file, or a command that names none of tern-kata's, asks nothing.
	if _, cmds := relayed(m, model.AgentState{Block: "7", State: "idle"}, model.AgentState{Block: "11", State: "idle"}, model.AgentState{Block: "12", State: "idle", Command: "omp"}); cmds != nil {
		t.Errorf("unknown ids re-read: %#v", cmds)
	}
	held := model.Model{AgentMapping: []model.AgentEntry{entry("a", "7", 1)}}
	if _, cmds := relayed(held, model.AgentState{Block: "7", State: "idle"}); cmds != nil {
		t.Errorf("a first relay naming only mapped blocks re-read: %#v", cmds)
	}
}

func TestTheRefreshedMappingReplacesTheModelsAndAnErrorKeepsIt(t *testing.T) {
	m := model.Model{AgentMapping: []model.AgentEntry{entry("a", "7", 1)}}
	m, _ = model.Update(m, model.AgentsRefreshed{Agents: []model.AgentEntry{entry("b", "9", 2)}})
	if len(m.AgentMapping) != 1 || m.AgentMapping[0].Block != "9" {
		t.Fatalf("mapping %+v", m.AgentMapping)
	}
	m, _ = model.Update(m, model.AgentsRefreshed{Err: errBoom})
	if len(m.AgentMapping) != 1 {
		t.Fatalf("an error changed the mapping: %+v", m.AgentMapping)
	}
}

func TestAClickOnTheBadgeFocusesAnAgentThatIsNotInTheBackground(t *testing.T) {
	_, cmds := model.Update(model.Model{}, model.AgentJumpMsg{Block: "7"})
	if want := []model.Cmd{model.FocusAgent{Block: "7"}}; !reflect.DeepEqual(cmds, want) {
		t.Fatalf("cmds %#v", cmds)
	}
}

func TestAnAgentOfAnUnknownSplitPromptFileReReadsAgentsJSONOnceAndEveryNameShapeMatches(t *testing.T) {
	for _, name := range []string{"prompt-24734716657667-3836283849.md", "prompt-3836283849.md", "prompt-4-5.md"} {
		m := model.Model{AgentMapping: []model.AgentEntry{entry("a", "7", 1)}}
		other := model.AgentState{Block: "9", State: "idle", Command: "omp @/state/agent-prompts/" + name}
		m, cmds := relayed(m, model.AgentState{Block: "7", State: "idle"}, other)
		if want := []model.Cmd{model.RefreshAgents{Seq: 1}}; !reflect.DeepEqual(cmds, want) {
			t.Fatalf("%s: first relay %#v, want a re-read", name, cmds)
		}
		m, _ = model.Update(m, model.AgentsRefreshed{Agents: m.AgentMapping})
		if _, cmds := relayed(m, model.AgentState{Block: "7", State: "working"}, other); cmds != nil {
			t.Errorf("%s: asked twice: %#v", name, cmds)
		}
	}
}

func TestTheBadgeIndexAnswersAsTheBadgePerIssue(t *testing.T) {
	m := model.Model{
		AgentMapping: []model.AgentEntry{entry("a", "1", 5), entry("b", "2", 1), entry("a", "3", 1), entry("a", "4", 9), entry("c", "5", 2), entry("b", "6", 3)},
		Agents: model.AgentStatesRelay{Received: true, States: []model.AgentState{{Block: "1", State: "working"}, {Block: "3", State: "waiting_input"},
			{Block: "4", State: "working"}, {Block: "2", State: "idle"}, {Block: "6", State: "idle"}}},
	}
	index := m.AgentBadges()
	for _, tt := range []struct {
		uid  string
		want model.AgentBadge
		ok   bool
	}{
		{"a", model.AgentBadge{Text: "agent waiting for you · 3", Tone: model.AgentToneWarning, Block: "3"}, true},
		{"b", model.AgentBadge{Text: "agent idle · 2", Tone: model.AgentToneMuted, Block: "6"}, true},
		{"c", model.AgentBadge{Text: "agent", Tone: model.AgentToneMuted, Block: "5"}, true},
		{"none", model.AgentBadge{}, false},
	} {
		got, ok := index.For(tt.uid)
		if ok != tt.ok || got != tt.want {
			t.Errorf("%s: badge %+v (%v), want %+v (%v)", tt.uid, got, ok, tt.want, tt.ok)
		}
	}
	if _, ok := (model.Model{}).AgentBadges().For("a"); ok {
		t.Error("a badge without a mapping")
	}
}

func unreadModel(address string, state string, slot any) model.Model {
	e := entry("a", "1", 1)
	e.Address = address
	issue := model.Issue{UID: "a", Metadata: map[string]any{}}
	if slot != nil {
		issue.Metadata[model.NotificationKey(address)] = slot
	}
	m := model.Model{AgentMapping: []model.AgentEntry{e}, Issues: []model.Issue{issue}}
	m.Agents = model.AgentStatesRelay{Received: true, States: []model.AgentState{{Block: "1", State: model.AgentStateWord(state)}}}
	return m
}

func TestAnUnclearedMessageAtTheAgentAddressShowsOnTheBadgeAndTheRowAndClearingRemovesIt(t *testing.T) {
	slot := map[string]any{"from": "me", "message": "  look at this\nand that"}
	m := unreadModel("me/omp-1", "working", slot)
	want := model.AgentBadge{Text: "agent working · message unread", Tone: model.AgentToneWarning, Block: "1"}
	if got, _ := m.AgentBadgeFor("a"); got != want {
		t.Errorf("badge %+v, want %+v", got, want)
	}
	if got, _ := m.AgentBadges().For("a"); got != want {
		t.Errorf("indexed badge %+v, want %+v", got, want)
	}
	if agents := m.IssueAgents("a"); len(agents) != 1 || agents[0].Unread != "look at this" {
		t.Errorf("agents %+v", agents)
	}

	cleared := unreadModel("me/omp-1", "working", nil)
	plain := model.AgentBadge{Text: "agent working", Tone: model.AgentToneAccent, Block: "1"}
	if got, _ := cleared.AgentBadgeFor("a"); got != plain {
		t.Errorf("cleared badge %+v, want %+v", got, plain)
	}
	if got, _ := cleared.AgentBadges().For("a"); got != plain {
		t.Errorf("cleared indexed badge %+v, want %+v", got, plain)
	}
	if agents := cleared.IssueAgents("a"); agents[0].Unread != "" {
		t.Errorf("cleared agents %+v", agents)
	}
}

func TestAMessageForAnotherAddressOrAnExitedAgentOrNotARequestShowsNothing(t *testing.T) {
	for name, m := range map[string]model.Model{
		"exited":        unreadModel("me/omp-1", "exited", map[string]any{"from": "me", "message": "hi"}),
		"no sender":     unreadModel("me/omp-1", "working", map[string]any{"message": "hi"}),
		"reply notice":  unreadModel("me/omp-1", "working", map[string]any{"from": "me", "message": "hi", "kind": "reply"}),
		"no address":    unreadModel("", "working", map[string]any{"from": "me", "message": "hi"}),
		"not an object": unreadModel("me/omp-1", "working", "hi"),
	} {
		for _, got := range []model.AgentBadge{first(m.AgentBadgeFor("a")), first(m.AgentBadges().For("a"))} {
			if strings.Contains(got.Text, "unread") {
				t.Errorf("%s: badge %q", name, got.Text)
			}
		}
	}
}

func first(b model.AgentBadge, _ bool) model.AgentBadge { return b }
