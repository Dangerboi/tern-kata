package model_test

import (
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestRelayedAgentStatesReplaceTheLatestList(t *testing.T) {
	var m model.Model
	if m.Agents.Received {
		t.Fatal("a Model that never saw a relay says it has")
	}
	first := []model.AgentState{{Block: "7", State: "working", LastMessage: "a"}}
	m, _ = model.Update(m, model.AgentStatesMsg{States: first})
	if !m.Agents.Received || !reflect.DeepEqual(m.Agents.States, first) {
		t.Fatalf("after the first relay: %+v", m.Agents)
	}
	m, _ = model.Update(m, model.AgentStatesMsg{States: []model.AgentState{}})
	if !m.Agents.Received || len(m.Agents.States) != 0 {
		t.Fatalf("an empty relay is no agents, not no relay: %+v", m.Agents)
	}
}

func TestEveryRelayEndsWithTheSaveOfTheMergedStates(t *testing.T) {
	states := []model.AgentState{{Block: "7", State: "working", LastMessage: "a"}}
	_, cmds := model.Update(model.Model{}, model.AgentStatesMsg{States: states})
	if want := (model.SaveAgentStates{States: states}); len(cmds) == 0 || !reflect.DeepEqual(cmds[len(cmds)-1], want) {
		t.Fatalf("cmds %#v, want a last %#v", cmds, want)
	}
}
