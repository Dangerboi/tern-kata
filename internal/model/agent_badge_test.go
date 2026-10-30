package model_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func entry(uid, block string, minute int) model.AgentEntry {
	return model.AgentEntry{IssueUID: uid, Block: block, StartedAt: time.Date(2026, 10, 30, 9, minute, 0, 0, time.UTC)}
}

func relayed(m model.Model, states ...model.AgentState) (model.Model, []model.Cmd) {
	return model.Update(m, model.AgentStatesMsg{States: states})
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

func TestANewBlockInTheRelayReReadsAgentsJSONOnce(t *testing.T) {
	m := model.Model{AgentMapping: []model.AgentEntry{entry("a", "7", 1)}}
	m, cmds := relayed(m, model.AgentState{Block: "7", State: "idle"}, model.AgentState{Block: "9", State: "idle"})
	if want := []model.Cmd{model.RefreshAgents{}}; !reflect.DeepEqual(cmds, want) {
		t.Fatalf("first relay: %#v, want a re-read for the unmapped block 9", cmds)
	}
	// The same list again carries no new block.
	if _, cmds := relayed(m, model.AgentState{Block: "7", State: "working"}, model.AgentState{Block: "9", State: "idle"}); cmds != nil {
		t.Errorf("a relay with no new block re-read: %#v", cmds)
	}
	// A new block the mapping already holds needs no re-read.
	if _, cmds := relayed(m, model.AgentState{Block: "7", State: "idle"}, model.AgentState{Block: "9", State: "idle"}, model.AgentState{Block: "7", State: "idle"}); cmds != nil {
		t.Errorf("%#v", cmds)
	}
	held := model.Model{AgentMapping: []model.AgentEntry{entry("a", "7", 1)}}
	if _, cmds := relayed(held, model.AgentState{Block: "7", State: "idle"}); cmds != nil {
		t.Errorf("a first relay naming only mapped blocks re-read: %#v", cmds)
	}
}

func TestAnEntryIsPrunedOnlyAfterItsBlockWasListedAndThenWasNot(t *testing.T) {
	m := model.Model{AgentMapping: []model.AgentEntry{entry("a", "7", 1), entry("a", "8", 2)}}
	// Before any relay nothing is pruned, and a first relay without the blocks prunes nothing: they were never listed.
	m, cmds := relayed(m)
	for _, c := range cmds {
		if r, ok := c.(model.RefreshAgents); ok && len(r.Gone) > 0 {
			t.Fatalf("pruned on the first relay: %#v", cmds)
		}
	}
	m, cmds = relayed(m, model.AgentState{Block: "7", State: "idle"})
	for _, c := range cmds {
		if r, ok := c.(model.RefreshAgents); ok && len(r.Gone) > 0 {
			t.Fatalf("pruned a block no relay listed: %#v", cmds)
		}
	}
	// 7 was listed; the next relay lacks it: it goes. 8 was never listed: it stays.
	_, cmds = relayed(m)
	if want := []model.Cmd{model.RefreshAgents{Gone: []string{"7"}}}; !reflect.DeepEqual(cmds, want) {
		t.Fatalf("cmds %#v, want %#v", cmds, want)
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
