package model_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func attentionPane() model.Model {
	return model.Model{
		Actor: "dana", Workspace: "/w", PaneBlock: "10",
		AgentMapping: []model.AgentEntry{
			{IssueUID: "u1", ProjectID: 3, Block: "11", PaneBlock: "10"},
			{IssueUID: "u1", ProjectID: 3, Block: "12", PaneBlock: "10"},
			{IssueUID: "u1", ProjectID: 3, Block: "21", PaneBlock: "20"},
			{IssueUID: "u2", ProjectID: 4, Block: "13", PaneBlock: "10"},
		},
	}
}

// relay feeds one relayed list and returns the commands but the re-reads of agents.json and Issue statuses, which these tests are
// not about.
func relay(t *testing.T, m model.Model, states ...model.AgentState) (model.Model, []model.Cmd) {
	t.Helper()
	m, cmds := model.Update(m, model.AgentStatesMsg{States: states})
	return m, slices.DeleteFunc(cmds, func(c model.Cmd) bool {
		switch c.(type) {
		case model.RefreshAgents, model.ReadAgentIssue:
			return true
		}
		return false
	})
}

func st(block string, state model.AgentStateWord, last string) model.AgentState {
	return model.AgentState{Block: block, State: state, LastMessage: last}
}

func raise(uid string, project int64, message string) model.RaiseAgentAttention {
	return model.RaiseAgentAttention{Workspace: "/w", ProjectID: project, UID: uid, Actor: "dana", Address: "dana", Message: message}
}

func clearOf(uid string, project int64, messages ...string) model.ClearAgentAttention {
	return model.ClearAgentAttention{Workspace: "/w", ProjectID: project, UID: uid, Actor: "dana", Address: "dana", Messages: messages}
}

func TestAgentEnteringWaitingRaisesTheRequestOnce(t *testing.T) {
	m, cmds := relay(t, attentionPane(), st("11", "working", ""))
	if len(cmds) != 0 {
		t.Fatalf("a working agent raised %v", cmds)
	}
	m, cmds = relay(t, m, st("11", "waiting_input", "which table?"))
	if want := []model.Cmd{raise("u1", 3, "which table?")}; !reflect.DeepEqual(cmds, want) {
		t.Fatalf("cmds %v, want %v", cmds, want)
	}
	if _, cmds = relay(t, m, st("11", "waiting_input", "which table?")); len(cmds) != 0 {
		t.Fatalf("an agent that kept waiting wrote again: %v", cmds)
	}
}

func TestAnotherPanesAgentRaisesNothingHere(t *testing.T) {
	m, cmds := relay(t, attentionPane(), st("21", "waiting_input", "mine? no"))
	if len(cmds) != 0 || len(m.AgentAttention) != 0 {
		t.Fatalf("cmds %v, raised %v", cmds, m.AgentAttention)
	}
}

func TestEveryNewlyWaitingAgentWritesItsOwnMessage(t *testing.T) {
	m, _ := relay(t, attentionPane(), st("11", "waiting_input", "first"))
	_, cmds := relay(t, m, st("11", "waiting_input", "first"), st("12", "waiting_input", "second"))
	if want := []model.Cmd{raise("u1", 3, "second")}; !reflect.DeepEqual(cmds, want) {
		t.Fatalf("cmds %v, want %v", cmds, want)
	}
}

func TestRequestStaysWhileAnyMappedAgentOnTheIssueWaits(t *testing.T) {
	m, _ := relay(t, attentionPane(), st("11", "waiting_input", "a"), st("12", "waiting_input", "b"), st("21", "waiting_input", "c"))
	m, cmds := relay(t, m, st("11", "working", ""), st("12", "waiting_input", "b"), st("21", "waiting_input", "c"))
	if len(cmds) != 0 {
		t.Fatalf("cleared while two agents wait: %v", cmds)
	}
	m, cmds = relay(t, m, st("11", "working", ""), st("12", "exited", ""), st("21", "waiting_input", "c"))
	if len(cmds) != 0 {
		t.Fatalf("cleared while another Pane's agent waits: %v", cmds)
	}
	_, cmds = relay(t, m, st("11", "working", ""), st("12", "exited", ""), st("21", "working", ""))
	if want := []model.Cmd{clearOf("u1", 3, "b", "a", "c")}; !reflect.DeepEqual(cmds, want) {
		t.Fatalf("cmds %v, want the clear once no mapped agent on the Issue waits", cmds)
	}
}

func TestClearsOnlyTheIssueWhoseAgentsStoppedWaiting(t *testing.T) {
	m, _ := relay(t, attentionPane(), st("11", "waiting_input", "a"), st("13", "waiting_input", "b"))
	m, cmds := relay(t, m, st("11", "waiting_input", "a"), st("13", "working", ""))
	if want := []model.Cmd{clearOf("u2", 4, "b")}; !reflect.DeepEqual(cmds, want) {
		t.Fatalf("cmds %v, want %v", cmds, want)
	}
	if want := []model.RaisedAttention{{UID: "u1", ProjectID: 3, Message: "a", Seen: []string{"a"}}}; !reflect.DeepEqual(m.AgentAttention, want) {
		t.Fatalf("raised %v, want %v", m.AgentAttention, want)
	}
}

func TestQuitClearsWhatThePaneRaisedUnlessAnotherPanesAgentWaits(t *testing.T) {
	m, _ := relay(t, attentionPane(), st("11", "waiting_input", "a"), st("13", "waiting_input", "b"))
	if want := []model.Cmd{clearOf("u1", 3, "a"), clearOf("u2", 4, "b")}; !reflect.DeepEqual(m.QuitAgentAttention(), want) {
		t.Fatalf("quit cmds %v, want %v", m.QuitAgentAttention(), want)
	}
	m, _ = relay(t, m, st("11", "waiting_input", "a"), st("13", "waiting_input", "b"), st("21", "waiting_input", "c"))
	if want := []model.Cmd{clearOf("u2", 4, "b")}; !reflect.DeepEqual(m.QuitAgentAttention(), want) {
		t.Fatalf("quit cmds %v, want only the Issue no other Pane's agent waits on", m.QuitAgentAttention())
	}
	if cmds := attentionPane().QuitAgentAttention(); len(cmds) != 0 {
		t.Fatalf("a Pane that raised nothing clears %v", cmds)
	}
}

func TestBlankMessageBecomesTheFixedText(t *testing.T) {
	for _, last := range []string{"", " \n\t"} {
		_, cmds := relay(t, attentionPane(), st("11", "waiting_input", last))
		if want := []model.Cmd{raise("u1", 3, "agent waiting for you")}; !reflect.DeepEqual(cmds, want) {
			t.Fatalf("%q: cmds %v, want %v", last, cmds, want)
		}
	}
}

func TestLongMessageIsCutAt1024BytesOnARuneBoundary(t *testing.T) {
	long := strings.Repeat("a", 1023) + "é" // the 2-byte rune straddles byte 1024
	_, cmds := relay(t, attentionPane(), st("11", "waiting_input", long))
	got := cmds[0].(model.RaiseAgentAttention).Message
	if got != strings.Repeat("a", 1023) {
		t.Fatalf("message of %d bytes, want the 1023 a's without half a rune", len(got))
	}
	exact := strings.Repeat("é", 512)
	_, cmds = relay(t, attentionPane(), st("11", "waiting_input", exact+"x"))
	if got := cmds[0].(model.RaiseAgentAttention).Message; got != exact {
		t.Fatalf("message of %d bytes, want 1024", len(got))
	}
}

func TestRefusedInboxAddressWritesNothing(t *testing.T) {
	m := attentionPane()
	m.InboxUser = strings.Repeat("x", 129)
	m, cmds := relay(t, m, st("11", "waiting_input", "a"))
	if len(cmds) != 0 || len(m.AgentAttention) != 0 || len(m.QuitAgentAttention()) != 0 {
		t.Fatalf("cmds %v, raised %v", cmds, m.AgentAttention)
	}
}

func TestEntryWithoutAProjectUsesTheModelsIssue(t *testing.T) {
	m := attentionPane()
	m.AgentMapping = []model.AgentEntry{{IssueUID: "u9", Block: "11", PaneBlock: "10"}}
	_, cmds := relay(t, m, st("11", "waiting_input", "a"))
	if len(cmds) != 0 {
		t.Fatalf("no Project known, yet wrote %v", cmds)
	}
	m.Issues = []model.Issue{{UID: "u9", ProjectID: 7}}
	_, cmds = relay(t, m, st("11", "waiting_input", "a"))
	if want := []model.Cmd{raise("u9", 7, "a")}; !reflect.DeepEqual(cmds, want) {
		t.Fatalf("cmds %v, want %v", cmds, want)
	}
}

func TestAgentWaitingBeforeItsEntryIsMappedRaisesOneRequestWhenTheMappingArrives(t *testing.T) {
	pane := attentionPane()
	mapping := pane.AgentMapping
	pane.AgentMapping = nil
	m, cmds := relay(t, pane, st("11", "waiting_input", "which table?"))
	if len(cmds) != 0 {
		t.Fatalf("an unmapped agent raised %v", cmds)
	}
	m, cmds = model.Update(m, model.AgentsRefreshed{Agents: mapping})
	if want := []model.Cmd{raise("u1", 3, "which table?")}; !reflect.DeepEqual(cmds, want) {
		t.Fatalf("cmds %v, want %v", cmds, want)
	}
	if _, cmds = relay(t, m, st("11", "waiting_input", "which table?")); len(cmds) != 0 {
		t.Fatalf("a second request: %v", cmds)
	}
	if _, cmds = model.Update(m, model.AgentsRefreshed{Agents: mapping}); len(cmds) != 0 {
		t.Fatalf("a reload raised again: %v", cmds)
	}
}

func TestAgentWaitingWhenItsEntryIsMappedFirstRaisesOneRequest(t *testing.T) {
	m, cmds := model.Update(attentionPane(), model.AgentsRefreshed{Agents: attentionPane().AgentMapping})
	if len(cmds) != 0 {
		t.Fatalf("no relay yet, yet %v", cmds)
	}
	m, cmds = relay(t, m, st("11", "waiting_input", "which table?"))
	if want := []model.Cmd{raise("u1", 3, "which table?")}; !reflect.DeepEqual(cmds, want) {
		t.Fatalf("cmds %v, want %v", cmds, want)
	}
	if _, cmds = relay(t, m, st("11", "waiting_input", "which table?")); len(cmds) != 0 {
		t.Fatalf("a second request: %v", cmds)
	}
}

func TestAdoptedWaitingAgentRaisesARequestWhenThePaneAdoptsIt(t *testing.T) {
	m := attentionPane()
	m.AgentMapping = []model.AgentEntry{{IssueUID: "u1", ProjectID: 3, Block: "21", PaneBlock: "20"}}
	m, _ = relay(t, m, st("21", "waiting_input", "q?"))
	adopted := []model.AgentEntry{{IssueUID: "u1", ProjectID: 3, Block: "21", PaneBlock: "10"}}
	_, cmds := model.Update(m, model.AgentsPruned{Agents: adopted})
	if want := []model.Cmd{raise("u1", 3, "q?")}; !reflect.DeepEqual(cmds, want) {
		t.Fatalf("cmds %v, want %v", cmds, want)
	}
}

func TestDroppingAWaitingAgentsEntryClearsItsRequestWithoutAnotherRelay(t *testing.T) {
	m, _ := relay(t, attentionPane(), st("13", "waiting_input", "b"))
	m, cmds := model.Update(m, model.AgentsRefreshed{Agents: attentionPane().AgentMapping[:3]})
	if want := []model.Cmd{clearOf("u2", 4, "b")}; !reflect.DeepEqual(cmds, want) {
		t.Fatalf("cmds %v, want %v", cmds, want)
	}
	if len(m.AgentAttention) != 0 {
		t.Fatalf("raised %v", m.AgentAttention)
	}
}

func TestTwoAgentsWaitingOnOneIssueInOneRelayRaiseOneRequest(t *testing.T) {
	m, cmds := relay(t, attentionPane(), st("11", "waiting_input", "a"), st("12", "waiting_input", "b"))
	if want := []model.Cmd{raise("u1", 3, "b")}; !reflect.DeepEqual(cmds, want) {
		t.Fatalf("cmds %v, want one raise carrying the recorded message", cmds)
	}
	if want := []model.RaisedAttention{{UID: "u1", ProjectID: 3, Message: "b", Seen: []string{"a", "b"}}}; !reflect.DeepEqual(m.AgentAttention, want) {
		t.Fatalf("raised %v, want %v", m.AgentAttention, want)
	}
}

func TestTheClearTakesTheMessageOfAnotherPanesAgentThatOverwroteTheRequest(t *testing.T) {
	m, _ := relay(t, attentionPane(), st("11", "waiting_input", "msgA"), st("21", "waiting_input", "msgB"))
	m, cmds := relay(t, m, st("21", "working", ""))
	if len(cmds) != 0 {
		t.Fatalf("an agent still waits, yet %v", cmds)
	}
	// The other Pane's agent was answered before this Pane re-read anything; the slot may hold either message.
	_, cmds = relay(t, m, st("11", "working", ""))
	if want := []model.Cmd{clearOf("u1", 3, "msgA", "msgB")}; !reflect.DeepEqual(cmds, want) {
		t.Fatalf("cmds %v, want the clear to accept both messages", cmds)
	}
}

func TestAdoptingAnAgentWritesNothingOverAnotherRequest(t *testing.T) {
	m, _ := relay(t, attentionPane(), st("11", "waiting_input", "msgA"), st("21", "waiting_input", "msgB"))
	adopted := append(slices.Clone(attentionPane().AgentMapping[:2]), model.AgentEntry{IssueUID: "u1", ProjectID: 3, Block: "21", PaneBlock: "10"})
	m, cmds := model.Update(m, model.AgentsPruned{Agents: adopted})
	if len(cmds) != 0 {
		t.Fatalf("cmds %v, want no write", cmds)
	}
	_, cmds = relay(t, m, st("11", "working", ""), st("21", "working", ""))
	if want := []model.Cmd{clearOf("u1", 3, "msgA", "msgB")}; !reflect.DeepEqual(cmds, want) {
		t.Fatalf("cmds %v, want %v", cmds, want)
	}
}

func TestRaisedRequestNamesTheWaitingAgentAndItsClearIsUnchanged(t *testing.T) {
	m := attentionPane()
	m.AgentMapping[0].Teammate = "builder"
	m.AgentMapping[1].Teammate = "reviewer"
	m, cmds := relay(t, m, st("11", "waiting_input", "which table?"))
	want := raise("u1", 3, "which table?")
	want.Teammate = "builder"
	if !reflect.DeepEqual(cmds, []model.Cmd{want}) {
		t.Fatalf("cmds %v, want %v", cmds, want)
	}
	// The other agent on the Issue waits next: its request names it, so the two read as two senders.
	m, cmds = relay(t, m, st("11", "waiting_input", "which table?"), st("12", "waiting_input", "which column?"))
	want = raise("u1", 3, "which column?")
	want.Teammate = "reviewer"
	if !reflect.DeepEqual(cmds, []model.Cmd{want}) {
		t.Fatalf("cmds %v, want %v", cmds, want)
	}
	_, cmds = relay(t, m, st("11", "working", ""), st("12", "working", ""))
	if want := []model.Cmd{clearOf("u1", 3, "which column?", "which table?")}; !reflect.DeepEqual(cmds, want) {
		t.Fatalf("cmds %v, want %v", cmds, want)
	}
}
