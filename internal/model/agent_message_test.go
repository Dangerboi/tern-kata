package model_test

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// addressPane is exitedPane with an Agent address on every agent: dana/omp-<block>.
func addressPane() model.Model {
	m := exitedPane("open")
	for i := range m.AgentMapping {
		m.AgentMapping[i].Address = "dana/omp-" + m.AgentMapping[i].Block
	}
	return m
}

func addressClears(cmds []model.Cmd) []model.Cmd {
	return slices.DeleteFunc(slices.Clone(cmds), func(c model.Cmd) bool { _, ok := c.(model.ClearAgentAddress); return !ok })
}

func clearAddress(uid string, project int64, block, address string) model.ClearAgentAddress {
	return model.ClearAgentAddress{Workspace: "/w", ProjectID: project, UID: uid, Actor: "dana", Block: block, Address: address}
}

func TestAnAgentThatExitsHasWhatIsLeftAtItsAddressClearedOnce(t *testing.T) {
	m, cmds := relay(t, addressPane(), st("11", "working", ""), st("21", "working", ""))
	if got := addressClears(cmds); len(got) != 0 {
		t.Fatalf("working agents cleared %v", got)
	}
	m, cmds = relay(t, m, st("11", "exited", ""), st("21", "exited", ""))
	if got, want := addressClears(cmds), []model.Cmd{clearAddress("u1", 3, "11", "dana/omp-11")}; !reflect.DeepEqual(got, want) {
		t.Fatalf("clears %v, want this Pane's exited agent only", got)
	}
	// The exit request is raised as before.
	if got, want := attentionWrites(cmds), []model.Cmd{raise("u1", 3, "agent exited without closing kata-u1")}; !reflect.DeepEqual(got, want) {
		t.Fatalf("attention writes %v, want %v", got, want)
	}
	m, cmds = relay(t, m, st("11", "exited", ""), st("21", "exited", ""))
	if got := addressClears(cmds); len(got) != 0 {
		t.Fatalf("a second relay of the same exit cleared again: %v", got)
	}
	// Its block closing later clears nothing more.
	if _, cmds = pruned(m, "11"); len(addressClears(cmds)) != 0 {
		t.Fatalf("the block of a cleared agent going cleared again: %v", cmds)
	}
}

func TestAnAgentWhoseBlockIsGoneHasItsAddressCleared(t *testing.T) {
	m, _ := relay(t, addressPane(), st("11", "working", ""), st("13", "working", ""))
	m.AgentMapping[3].Address = "" // an agent that is not omp has no address to clear, only a record to remove
	m, cmds := pruned(m, "11", "13")
	if got, want := addressClears(cmds), []model.Cmd{clearAddress("u1", 3, "11", "dana/omp-11"), clearAddress("u2", 4, "13", "")}; !reflect.DeepEqual(got, want) {
		t.Fatalf("clears %v, want %v", got, want)
	}
	if _, cmds = model.Update(m, model.StreamEvent{}); len(addressClears(cmds)) != 0 {
		t.Fatalf("the next pass cleared again: %v", cmds)
	}
}

func TestAFailedClearIsAskedAgainAndASuccessfulOneIsNot(t *testing.T) {
	m, _ := relay(t, addressPane(), st("11", "working", ""))
	m, cmds := relay(t, m, st("11", "exited", ""))
	if len(addressClears(cmds)) != 1 {
		t.Fatalf("clears %v, want one", cmds)
	}
	if _, cmds = model.Update(m, model.StreamEvent{}); len(addressClears(cmds)) != 0 {
		t.Fatalf("asked again while the first is unanswered: %v", cmds)
	}
	m, _ = model.Update(m, model.AgentAddressCleared{UID: "u1", Block: "11", Err: errors.New("daemon down")})
	m, cmds = model.Update(m, model.StreamEvent{})
	if got, want := addressClears(cmds), []model.Cmd{clearAddress("u1", 3, "11", "dana/omp-11")}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after a failure clears %v, want %v", got, want)
	}
	m, _ = model.Update(m, model.AgentAddressCleared{UID: "u1", Block: "11"})
	if _, cmds = model.Update(m, model.StreamEvent{}); len(addressClears(cmds)) != 0 {
		t.Fatalf("asked again after a success: %v", cmds)
	}
}
