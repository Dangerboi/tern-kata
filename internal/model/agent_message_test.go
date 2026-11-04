package model_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// inboxPane is exitedPane with an inbox on every agent: dana/omp-<block>.
func inboxPane() model.Model {
	m := exitedPane("open")
	for i := range m.AgentMapping {
		m.AgentMapping[i].Inbox = "dana/omp-" + m.AgentMapping[i].Block
	}
	return m
}

func inboxClears(cmds []model.Cmd) []model.Cmd {
	return slices.DeleteFunc(slices.Clone(cmds), func(c model.Cmd) bool { _, ok := c.(model.ClearAgentInbox); return !ok })
}

func clearInbox(uid string, project int64, address string) model.ClearAgentInbox {
	return model.ClearAgentInbox{Workspace: "/w", ProjectID: project, UID: uid, Actor: "dana", Address: address}
}

func TestAnAgentThatExitsHasWhatIsLeftAtItsAddressClearedOnce(t *testing.T) {
	m, cmds := relay(t, inboxPane(), st("11", "working", ""), st("21", "working", ""))
	if got := inboxClears(cmds); len(got) != 0 {
		t.Fatalf("working agents cleared %v", got)
	}
	m, cmds = relay(t, m, st("11", "exited", ""), st("21", "exited", ""))
	if got, want := inboxClears(cmds), []model.Cmd{clearInbox("u1", 3, "dana/omp-11")}; !reflect.DeepEqual(got, want) {
		t.Fatalf("clears %v, want this Pane's exited agent only", got)
	}
	// The exit request is raised as before.
	if got, want := attentionWrites(cmds), []model.Cmd{raise("u1", 3, "agent exited without closing kata-u1")}; !reflect.DeepEqual(got, want) {
		t.Fatalf("attention writes %v, want %v", got, want)
	}
	m, cmds = relay(t, m, st("11", "exited", ""), st("21", "exited", ""))
	if got := inboxClears(cmds); len(got) != 0 {
		t.Fatalf("a second relay of the same exit cleared again: %v", got)
	}
	// Its block closing later clears nothing more.
	if _, cmds = pruned(m, "11"); len(inboxClears(cmds)) != 0 {
		t.Fatalf("the block of a cleared agent going cleared again: %v", cmds)
	}
}

func TestAnAgentWhoseBlockIsGoneHasItsAddressCleared(t *testing.T) {
	m, _ := relay(t, inboxPane(), st("11", "working", ""), st("13", "working", ""))
	m.AgentMapping[3].Inbox = "" // an agent that is not omp has no address to clear
	m, cmds := pruned(m, "11", "13")
	if got, want := inboxClears(cmds), []model.Cmd{clearInbox("u1", 3, "dana/omp-11")}; !reflect.DeepEqual(got, want) {
		t.Fatalf("clears %v, want %v", got, want)
	}
	if _, cmds = model.Update(m, model.StreamEvent{}); len(inboxClears(cmds)) != 0 {
		t.Fatalf("the next pass cleared again: %v", cmds)
	}
}
