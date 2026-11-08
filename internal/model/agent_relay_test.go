package model_test

import (
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func suspectsOf(cmds []model.Cmd) (suspects []string, asked bool) {
	for _, c := range cmds {
		if r, ok := c.(model.RefreshAgents); ok {
			return r.Suspects, true
		}
	}
	return nil, false
}

func stateOf(m model.Model, block string) model.AgentStateWord {
	for _, s := range m.Agents.States {
		if s.Block == block {
			return s.State
		}
	}
	return ""
}

func TestARelayThatOmitsAMappedBlockLeavesItsStateAlone(t *testing.T) {
	m := model.Model{AgentMapping: []model.AgentEntry{entry("a", "7", 1), entry("a", "8", 2)}}
	m, _ = relayed(m, model.AgentState{Block: "7", State: "working"}, model.AgentState{Block: "8", State: "idle"})
	// The other window sees only block 8, and the two windows alternate.
	for range 3 {
		var cmds []model.Cmd
		m, cmds = relayed(m, model.AgentState{Block: "8", State: "idle"})
		if stateOf(m, "7") != "working" || stateOf(m, "8") != "idle" {
			t.Fatalf("states %+v", m.Agents.States)
		}
		if len(m.Agents.Changes) != 0 {
			t.Fatalf("an omitted block or a repeated state counted as a change: %+v", m.Agents.Changes)
		}
		if suspects, _ := suspectsOf(cmds); len(suspects) != 0 && !reflect.DeepEqual(suspects, []string{"7"}) {
			t.Fatalf("suspects %v", suspects)
		}
		m, _ = model.Update(m, model.AgentsRefreshed{Agents: m.AgentMapping})
		m, _ = relayed(m, model.AgentState{Block: "7", State: "working"}, model.AgentState{Block: "8", State: "idle"})
		if len(m.Agents.Changes) != 0 {
			t.Fatalf("a window reporting a known state caused %+v", m.Agents.Changes)
		}
	}
}

func TestABlockMissingFromARelayIsASuspectNotGoneAndNothingIsSuspectedBeforeItWasListed(t *testing.T) {
	m := model.Model{PaneBlock: "9", AgentMapping: []model.AgentEntry{entry("a", "7", 1), entry("a", "8", 2)}}
	m, cmds := relayed(m)
	if _, asked := suspectsOf(cmds); asked {
		t.Fatalf("a first relay without the blocks refreshed: %#v", cmds)
	}
	m, _ = relayed(m, model.AgentState{Block: "7", State: "idle"})
	_, cmds = relayed(m)
	if want := []model.Cmd{model.RefreshAgents{Suspects: []string{"7"}, PaneBlock: "9", Seq: 1}}; !reflect.DeepEqual(cmds, want) {
		t.Fatalf("cmds %#v, want %#v", cmds, want)
	}
}

func TestOnlyOneRefreshIsOutAndTheNextRelayThatNeedsOneGetsItWhenTheAnswerIsIn(t *testing.T) {
	m := model.Model{PaneBlock: "9", AgentMapping: []model.AgentEntry{entry("a", "7", 1), entry("a", "8", 2)}}
	m, _ = relayed(m, model.AgentState{Block: "7", State: "idle"}, model.AgentState{Block: "8", State: "idle"})
	m, cmds := relayed(m, model.AgentState{Block: "8", State: "idle"})
	if want := []model.Cmd{model.RefreshAgents{Suspects: []string{"7"}, PaneBlock: "9", Seq: 1}}; !reflect.DeepEqual(cmds, want) {
		t.Fatalf("cmds %#v, want %#v", cmds, want)
	}
	m, cmds = relayed(m, model.AgentState{Block: "7", State: "idle"})
	if _, asked := suspectsOf(cmds); asked {
		t.Fatalf("a second refresh went out while one was in flight: %#v", cmds)
	}
	m, cmds = model.Update(m, model.AgentsRefreshed{Agents: m.AgentMapping})
	if want := []model.Cmd{model.RefreshAgents{Suspects: []string{"8"}, PaneBlock: "9", Seq: 2}}; !reflect.DeepEqual(cmds, want) {
		t.Fatalf("after the answer: %#v, want %#v", cmds, want)
	}
	m, cmds = model.Update(m, model.AgentsRefreshed{Agents: m.AgentMapping})
	if len(cmds) != 0 {
		t.Fatalf("an answer with nothing waiting asked again: %#v", cmds)
	}
	_ = m
}

func TestThePrunedBlocksStateAndEveryUnmappedUnlistedStateGoWithThem(t *testing.T) {
	m := model.Model{AgentMapping: []model.AgentEntry{entry("a", "7", 1), entry("a", "8", 2)}}
	m, _ = relayed(m, model.AgentState{Block: "7", State: "idle"}, model.AgentState{Block: "8", State: "idle"}, model.AgentState{Block: "30", State: "working"})
	if len(m.Agents.States) != 3 {
		t.Fatalf("states %+v", m.Agents.States)
	}
	// Block 30 is not mapped and the next relay does not list it: its state is dropped. 7 is mapped and kept.
	m, _ = relayed(m, model.AgentState{Block: "8", State: "idle"})
	if len(m.Agents.States) != 2 || stateOf(m, "7") != "idle" {
		t.Fatalf("states %+v", m.Agents.States)
	}
	// The gateway pruned block 7: its entry is not mapped any more, and the latest relay does not list it.
	m, _ = model.Update(m, model.AgentsRefreshed{Agents: []model.AgentEntry{entry("a", "8", 2)}})
	if len(m.Agents.States) != 1 || stateOf(m, "7") != "" {
		t.Fatalf("states %+v", m.Agents.States)
	}
}

func TestASecondWindowReportingTheSameStateRaisesNoSecondAttentionNorGoingBack(t *testing.T) {
	m := attentionPane()
	m, cmds := relay(t, m, st("11", "waiting_input", "q?"))
	if want := []model.Cmd{raise("u1", 3, "q?")}; !reflect.DeepEqual(cmds, want) {
		t.Fatalf("cmds %v", cmds)
	}
	// The other window lists the same agent in the same state, then omits it, then lists it again.
	for _, states := range [][]model.AgentState{{st("11", "waiting_input", "q?")}, {st("12", "idle", "")}, {st("11", "waiting_input", "q?")}} {
		m, cmds = relay(t, m, states...)
		for _, c := range cmds {
			switch c.(type) {
			case model.RaiseAgentAttention, model.ClearAgentAttention, model.SendAgentBack:
				t.Fatalf("a repeated state wrote %#v", c)
			}
		}
		m, _ = model.Update(m, model.AgentsRefreshed{Agents: m.AgentMapping})
	}
}

func TestAWaitingAgentGoesBackOnceAcrossTwoWindowsRelays(t *testing.T) {
	e := backgroundEntry("5", "9", true)
	m := model.Model{PaneBlock: "9", AgentMapping: []model.AgentEntry{e, backgroundEntry("6", "9", true)}}
	m, _ = relayed(m, model.AgentState{Block: "5", State: "waiting_input"}, model.AgentState{Block: "6", State: "idle"})
	// Window A sees block 5 working; window B, which lists block 5 too, reports the same.
	m, cmds := relayed(m, model.AgentState{Block: "5", State: "working"}, model.AgentState{Block: "6", State: "idle"})
	if want := []model.SendAgentBack{{Block: "5", IssueRef: "demo#ab12"}}; !reflect.DeepEqual(sentBack(cmds), want) {
		t.Fatalf("first window: %#v", cmds)
	}
	_, cmds = relayed(m, model.AgentState{Block: "5", State: "working"})
	if back := sentBack(cmds); len(back) != 0 {
		t.Fatalf("second window sent it back again: %#v", back)
	}
}

func TestAFinishedAgentIsReadOnceWhenItFinishesNotOnEveryWindowsRelay(t *testing.T) {
	m := closePane(nil, finishedEntry("5", "a"), finishedEntry("6", "b"))
	m, _ = relayed(m, model.AgentState{Block: "5", State: "working"}, model.AgentState{Block: "6", State: "working"})
	m, cmds := relayed(m, model.AgentState{Block: "5", State: "idle"}, model.AgentState{Block: "6", State: "working"})
	if got := reads(cmds); len(got) != 1 || got[0].UID != "a" {
		t.Fatalf("reads %#v", got)
	}
	m, _ = model.Update(m, model.AgentIssueRead{UID: "a", Status: "open"})
	// The other window reports the same state, and one that omits block 5.
	for _, states := range [][]model.AgentState{{{Block: "5", State: "idle"}, {Block: "6", State: "working"}}, {{Block: "6", State: "working"}}} {
		_, cmds = relayed(m, states...)
		if got := reads(cmds); len(got) != 0 {
			t.Fatalf("read again on a repeated state: %#v", got)
		}
	}
}

func TestAnAdoptedAgentGetsAttentionGoingBackAndAutoCloseFromThisPane(t *testing.T) {
	m := attentionPane()
	m.Issues = []model.Issue{closed("u1")}
	agent := backgroundEntry("21", "20", true)
	agent.IssueUID, agent.ProjectID = "u1", 3
	m.AgentMapping = []model.AgentEntry{agent}
	m, _ = relay(t, m, st("21", "working", ""))
	m, cmds := relay(t, m, st("21", "waiting_input", "a"))
	if len(cmds) != 0 || len(m.AgentAttention) != 0 {
		t.Fatalf("another Pane's agent raised %v %v", cmds, m.AgentAttention)
	}
	// Pane 20 is gone: a refresh hands its agent to this Pane (10), then the user answers it.
	agent.PaneBlock = "10"
	m, _ = model.Update(m, model.AgentsRefreshed{Agents: []model.AgentEntry{agent}})
	m, _ = relay(t, m, st("21", "working", ""))
	m, cmds = relay(t, m, st("21", "waiting_input", "again"))
	if want := []model.Cmd{raise("u1", 3, "again")}; !reflect.DeepEqual(cmds, want) {
		t.Fatalf("an adopted agent's attention: %v", cmds)
	}
	m, cmds = relay(t, m, st("21", "working", ""))
	if back := sentBack(cmds); len(back) != 1 || back[0].Block != "21" {
		t.Fatalf("going back %#v", cmds)
	}
	m.AgentMapping[0].Open = false
	_, cmds = relay(t, m, st("21", "idle", ""))
	if want := []model.CloseAgent{{Block: "21"}}; !reflect.DeepEqual(closes(cmds), want) {
		t.Fatalf("auto-close %#v", cmds)
	}
}

func TestThePruneAtStartCarriesThePanesBlock(t *testing.T) {
	_, cmds := model.Update(model.Model{PaneBlock: "9"}, model.Start{Workspace: "/repo"})
	if want := []model.Cmd{model.LoadProject{Workspace: "/repo"}, model.LoadViews{}, model.PruneAgents{Workspace: "/repo", PaneBlock: "9", Seq: 1}, model.LoadAuthCapabilities{Workspace: "/repo", Account: true}}; !reflect.DeepEqual(cmds, want) {
		t.Fatalf("cmds %#v", cmds)
	}
}
