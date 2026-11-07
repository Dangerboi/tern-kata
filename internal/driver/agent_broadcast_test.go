package driver_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// broadcastAgent is an omp agent dana's Pane started on the Issue uid, with the block id and address the number gives.
func broadcastAgent(uid, number string) model.AgentEntry {
	return model.AgentEntry{IssueUID: uid, IssueRef: "work#" + uid, ProjectID: 7, Block: number, Teammate: "omp", Address: "dana/omp-" + number,
		StartedAt: now.Add(-time.Hour)}
}

// broadcastPane is the Issues Page on work#p, which has an open child work#c1 and a closed child work#c2, with the agents mapped.
func broadcastPane(t *testing.T, states []model.AgentState, agents ...model.AgentEntry) driver.State {
	t.Helper()
	parent, open, closed := boardCard("p"), boardCard("c1"), boardCard("c2")
	parent.QualifiedID, open.QualifiedID, closed.QualifiedID = "work#p", "work#c1", "work#c2"
	closed.Status = "closed"
	s := projectPane(t, 160, parent, open, closed)
	s.Model.Actor = "dana"
	s.Model.SelectedUID = "p"
	s.Model.Relationships = model.NewRelationshipIndex([]model.Link{
		{ID: 1, Type: "parent", FromIssueUID: "c1", FromQualifiedID: "work#c1", FromStatus: "open", ToIssueUID: "p", ToQualifiedID: "work#p", ToStatus: "open"},
		{ID: 2, Type: "parent", FromIssueUID: "c2", FromQualifiedID: "work#c2", FromStatus: "closed", ToIssueUID: "p", ToQualifiedID: "work#p", ToStatus: "open"},
	})
	s = step(t, s, model.AgentsPruned{Agents: agents}).State
	if states == nil {
		states = []model.AgentState{}
	}
	s = step(t, s, model.AgentStatesMsg{States: states}).State
	return s
}

func sentMessages(cmds []model.Cmd) map[string]model.SendAgentMessage {
	found := map[string]model.SendAgentMessage{}
	for _, c := range cmds {
		if send, ok := c.(model.SendAgentMessage); ok {
			found[send.Address] = send
		}
	}
	return found
}

func TestABroadcastWritesOnceToEachRunningAgentOfTheIssueAndItsOpenChildren(t *testing.T) {
	notOmp := broadcastAgent("p", "34")
	notOmp.Address, notOmp.NoAddress = "", model.AgentNotOmp
	s := broadcastPane(t, []model.AgentState{{Block: "35", State: model.AgentExited}},
		broadcastAgent("p", "31"), broadcastAgent("c1", "32"), broadcastAgent("c1", "33"), notOmp, broadcastAgent("p", "35"), broadcastAgent("c2", "36"))

	open := step(t, s, tern.Key{Name: "s", Shift: true})
	if m := open.State.Model; m.TopCard() != model.CardAgentMessage || !m.AgentMessage.Broadcast || len(open.Cmds) != 0 {
		t.Fatalf("card %v broadcast %v cmds %v, want the Message card as a broadcast", m.TopCard(), m.AgentMessage.Broadcast, open.Cmds)
	}
	wantShown(t, "the broadcast card", open.State, "Message the agents", "work#p", "open children")

	sending := step(t, step(t, open.State, tern.Key{Text: "schema changed"}).State, key("enter"))
	got := sentMessages(sending.Cmds)
	want := map[string]model.SendAgentMessage{
		"dana/omp-31": {Workspace: "/repo", ProjectID: 7, UID: "p", Actor: "dana", Address: "dana/omp-31", Message: "schema changed"},
		"dana/omp-32": {Workspace: "/repo", ProjectID: 7, UID: "c1", Actor: "dana", Address: "dana/omp-32", Message: "schema changed"},
		"dana/omp-33": {Workspace: "/repo", ProjectID: 7, UID: "c1", Actor: "dana", Address: "dana/omp-33", Message: "schema changed"},
	}
	if len(sending.Cmds) != len(want) || len(got) != len(want) {
		t.Fatalf("cmds %#v, want one write each to %v", sending.Cmds, want)
	}
	for address, w := range want {
		if got[address] != w {
			t.Errorf("write to %s: %+v, want %+v", address, got[address], w)
		}
	}

	s = step(t, sending.State, model.AgentMessageSent{UID: "p", Address: "dana/omp-31"}).State
	s = step(t, s, model.AgentMessageSent{UID: "c1", Address: "dana/omp-32"}).State
	if s.Model.AgentMessage.Outcome {
		t.Fatalf("the outcome showed before every write answered")
	}
	s = step(t, s, model.AgentMessageSent{UID: "c1", Address: "dana/omp-33", Held: `{"from":"dana","message":"re-check"}`, Unread: "re-check"}).State
	if s.Model.CurrentMode() != model.ModeAgentBroadcastOutcome {
		t.Fatalf("mode %s, want the outcome", s.Model.CurrentMode())
	}
	wantShown(t, "the outcome", s, "2 agents got the message.", "Skipped omp on work#c1: "+model.AgentHoldsMessageRefusal, "Skipped omp on work#p: "+model.AgentNotOmpRefusal)
	wantNotShown(t, "the outcome", s, "work#c2", "exited")

	if closed := step(t, s, key("escape")).State; closed.Model.TopCard() != model.NoCard {
		t.Errorf("esc left the card %v", closed.Model.TopCard())
	}
}

func TestABroadcastFailedWriteIsListedSkipped(t *testing.T) {
	s := broadcastPane(t, nil, broadcastAgent("p", "31"))
	s = step(t, step(t, step(t, s, tern.Key{Name: "s", Shift: true}).State, tern.Key{Text: "hello"}).State, key("enter")).State
	s = step(t, s, model.AgentMessageSent{UID: "p", Address: "dana/omp-31", Err: errors.New("daemon down")}).State
	wantShown(t, "the outcome", s, "No agent got the message.", "daemon down")
}

func TestABroadcastWithNoRunningAgentOpensNoCard(t *testing.T) {
	s := broadcastPane(t, []model.AgentState{{Block: "31", State: model.AgentExited}}, broadcastAgent("p", "31"), broadcastAgent("c2", "36"))
	out := step(t, s, tern.Key{Name: "s", Shift: true})
	if m := out.State.Model; m.TopCard() != model.NoCard || m.Notice != model.NoAgentsToBroadcastRefusal {
		t.Fatalf("card %v notice %q, want no card and the refusal", m.TopCard(), m.Notice)
	}
}
