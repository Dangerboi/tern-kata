package driver_test

import (
	"errors"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

func TestTheSweepWritesTheWrapUpOnceToEveryRunningAgentOnAnyIssue(t *testing.T) {
	notOmp := broadcastAgent("p", "34")
	notOmp.Address, notOmp.NoAddress = "", model.AgentNotOmp
	other := broadcastAgent("c1", "37")
	other.Address = "erin/omp-37"
	s := broadcastPane(t, []model.AgentState{{Block: "35", State: model.AgentExited}},
		broadcastAgent("p", "31"), broadcastAgent("c2", "36"), broadcastAgent("c1", "33"), notOmp, other, broadcastAgent("p", "35"))

	sending := step(t, s, view.DecodeAction("", string(model.WrapUpSweepAction), ""))
	got := sentMessages(sending.Cmds)
	want := map[string]model.SendAgentMessage{
		"dana/omp-31": {Workspace: "/repo", ProjectID: 7, UID: "p", Actor: "dana", Address: "dana/omp-31", Message: model.QuickReplyWrapUp},
		"dana/omp-36": {Workspace: "/repo", ProjectID: 7, UID: "c2", Actor: "dana", Address: "dana/omp-36", Message: model.QuickReplyWrapUp},
		"dana/omp-33": {Workspace: "/repo", ProjectID: 7, UID: "c1", Actor: "dana", Address: "dana/omp-33", Message: model.QuickReplyWrapUp},
	}
	if len(sending.Cmds) != len(want) || len(got) != len(want) {
		t.Fatalf("cmds %#v, want one write each to %v", sending.Cmds, want)
	}
	for address, w := range want {
		if got[address] != w {
			t.Errorf("write to %s: %+v, want %+v", address, got[address], w)
		}
	}
}

func TestTheSweepWaitsForTheAgentMappingAndTheFirstRelay(t *testing.T) {
	s := broadcastPane(t, nil)
	s.Model.AgentMapping = nil
	s.Model.Agents = model.AgentStatesRelay{}
	sweep := view.DecodeAction("", string(model.WrapUpSweepAction), "")
	out := step(t, s, sweep)
	if len(out.Cmds) != 0 || out.State.Model.Notice == model.NoAgentsToSweepRefusal {
		t.Fatalf("cmds %v notice %q, want the sweep held without a refusal", out.Cmds, out.State.Model.Notice)
	}

	out = step(t, out.State, model.AgentsPruned{Agents: []model.AgentEntry{broadcastAgent("p", "31"), broadcastAgent("c1", "33"), broadcastAgent("p", "35")}})
	if len(out.Cmds) != 0 {
		t.Fatalf("cmds %v after the mapping alone, want none", out.Cmds)
	}

	out = step(t, out.State, model.AgentStatesMsg{States: []model.AgentState{{Block: "35", State: model.AgentExited}}})
	sent := 0
	for _, c := range out.Cmds {
		if _, ok := c.(model.SendAgentMessage); ok {
			sent++
		}
	}
	got := sentMessages(out.Cmds)
	if sent != 2 || len(got) != 2 || got["dana/omp-31"].Message == "" || got["dana/omp-33"].Message == "" {
		t.Fatalf("cmds %#v, want one write each to 31 and 33 and none to 35", out.Cmds)
	}

	out = step(t, out.State, model.AgentStatesMsg{States: []model.AgentState{{Block: "35", State: model.AgentExited}}})
	if len(sentMessages(out.Cmds)) != 0 {
		t.Fatalf("cmds %#v, a later relay must not sweep again", out.Cmds)
	}
}

func TestTheSweepSkipsAHeldAgentAndListsTheOnesItCannotReach(t *testing.T) {
	notOmp := broadcastAgent("p", "34")
	notOmp.Address, notOmp.NoAddress = "", model.AgentNotOmp
	other := broadcastAgent("c1", "37")
	other.Address = "erin/omp-37"
	s := broadcastPane(t, nil, broadcastAgent("p", "31"), broadcastAgent("c1", "33"), notOmp, other)
	s = step(t, s, view.DecodeAction("", string(model.WrapUpSweepAction), "")).State
	s = step(t, s, model.AgentMessageSent{UID: "p", Address: "dana/omp-31"}).State
	s = step(t, s, model.AgentMessageSent{UID: "c1", Address: "dana/omp-33", Held: `{"from":"dana","message":"re-check"}`, Unread: "re-check"}).State

	wantShown(t, "the sweep", s, "Wrap-up sweep", "omp on work#p: not yet",
		"Skipped omp on work#c1: "+model.AgentHoldsMessageRefusal,
		"Skipped omp on work#p: "+model.AgentNotOmpRefusal,
		"Skipped omp on work#c1: "+model.AgentOtherActorRefusal)
	if held := s.Model.AgentMessage.Targets[1]; held.Address != "dana/omp-33" || held.Skipped == "" {
		t.Errorf("the held agent %+v is not skipped", held)
	}
}

func TestTheSweepFollowsAgentsToFinished(t *testing.T) {
	s := broadcastPane(t, []model.AgentState{{Block: "31", State: model.AgentIdle}, {Block: "33", State: model.AgentWorking}},
		broadcastAgent("p", "31"), broadcastAgent("c1", "33"), broadcastAgent("c1", "34"))
	s = step(t, s, view.DecodeAction("", string(model.WrapUpSweepAction), "")).State
	for _, a := range []struct{ uid, address string }{{"p", "dana/omp-31"}, {"c1", "dana/omp-33"}, {"c1", "dana/omp-34"}} {
		s = step(t, s, model.AgentMessageSent{UID: a.uid, Address: a.address}).State
	}
	wantShown(t, "just after sending", s, "0 of 3 agents have finished the wrap-up.")
	wantNotShown(t, "just after sending", s, "finished: ")

	// 31 was idle and has not worked yet; 33 was working, so its idle is the wrap-up done; 34 goes on working.
	s = step(t, s, model.AgentStatesMsg{States: []model.AgentState{{Block: "31", State: model.AgentIdle}, {Block: "33", State: model.AgentIdle, LastMessage: "Committed abc\nrest"}, {Block: "34", State: model.AgentWorking}}}).State
	wantShown(t, "after 33 idled", s, "1 of 3 agents have finished the wrap-up.", "omp on work#c1: finished: idle after the wrap-up (Committed abc)", "not yet")

	s = step(t, s, model.AgentStatesMsg{States: []model.AgentState{{Block: "31", State: model.AgentWorking}}}).State
	s = step(t, s, model.AgentStatesMsg{States: []model.AgentState{{Block: "31", State: model.AgentExited}, {Block: "34", State: model.AgentWaitingInput}}}).State
	wantShown(t, "after 31 exited", s, "2 of 3 agents have finished the wrap-up.", "finished: exited", "not yet: waiting for you")
}

func TestTheSweepWithNoRunningAgentOpensNoCard(t *testing.T) {
	s := broadcastPane(t, []model.AgentState{{Block: "31", State: model.AgentExited}}, broadcastAgent("p", "31"))
	out := step(t, s, view.DecodeAction("", string(model.WrapUpSweepAction), ""))
	if m := out.State.Model; m.TopCard() != model.NoCard || m.Notice != model.NoAgentsToSweepRefusal || len(out.Cmds) != 0 {
		t.Fatalf("card %v notice %q cmds %v, want no card and the refusal", m.TopCard(), m.Notice, out.Cmds)
	}
}

func TestTheSweepListsAFailedWriteAsSkipped(t *testing.T) {
	s := broadcastPane(t, nil, broadcastAgent("p", "31"))
	s = step(t, s, view.DecodeAction("", string(model.WrapUpSweepAction), "")).State
	s = step(t, s, model.AgentMessageSent{UID: "p", Address: "dana/omp-31", Err: errors.New("daemon down")}).State
	wantShown(t, "the sweep", s, "0 of 0 agents", "daemon down")
}
