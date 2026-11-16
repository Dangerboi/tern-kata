package model_test

import (
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

const remotePrompt = "/home/u/.local/state/tern-kata/agent-prompts/prompt-24734716657667-3836283849.md"

// remoteEntry is a background agent of the Pane "9" whose block the session host knows as "5".
func remoteEntry() model.AgentEntry {
	e := backgroundEntry("5", "9", true)
	e.IssueUID, e.ProjectID, e.PromptFile = "a", 3, remotePrompt
	return e
}

// remoteState is the same agent as a window on another machine lists it: another id, the command naming the prompt file.
func remoteState(state model.AgentStateWord, last string) model.AgentState {
	return model.AgentState{Block: "1052771002302467", State: state, LastMessage: last, Command: "omp @" + remotePrompt}
}

func TestARemoteRelayGivesTheEntrysBlockItsState(t *testing.T) {
	m := model.Model{Workspace: "/w", PaneBlock: "9", AgentMapping: []model.AgentEntry{remoteEntry()}}
	m, cmds := relayed(m, remoteState("working", ""))
	if len(cmds) != 0 {
		t.Fatalf("cmds %#v", cmds)
	}
	if stateOf(m, "5") != "working" {
		t.Fatalf("states %+v", m.Agents.States)
	}
	if badge, _ := m.AgentBadgeFor("a"); badge.Text != "agent working" || badge.Block != "5" {
		t.Errorf("badge %+v", badge)
	}
}

func TestARemoteRelayDrivesAttentionGoingBackAndAutoCloseAsALocalOneDoes(t *testing.T) {
	m := model.Model{Workspace: "/w", Actor: "dana", PaneBlock: "9", AgentMapping: []model.AgentEntry{remoteEntry()}}
	m, _ = relayed(m, remoteState("waiting_input", "which table?"))
	if len(m.AgentAttention) == 0 {
		t.Errorf("no attention raised for a waiting remote agent")
	}
	_, cmds := relayed(m, remoteState("working", ""))
	if want := []model.SendAgentBack{{Block: "5", IssueRef: "demo#ab12"}}; !reflect.DeepEqual(sentBack(cmds), want) {
		t.Errorf("going back: %#v", cmds)
	}

	finished := finishedEntry("5", "a")
	finished.PromptFile = remotePrompt
	closing := closePane([]model.Issue{closed("a")}, finished)
	_, cmds = relayed(closing, remoteState("idle", ""))
	if want := []model.CloseAgent{{Block: "5"}}; !reflect.DeepEqual(closes(cmds), want) {
		t.Errorf("auto-close: %#v", cmds)
	}
}

func TestTwoWindowsWithDifferentIdsNeverDisagreeIntoSuspects(t *testing.T) {
	m := model.Model{PaneBlock: "9", AgentMapping: []model.AgentEntry{remoteEntry()}}
	local := model.AgentState{Block: "5", State: "idle", Command: "omp @" + remotePrompt}
	for i := range 3 {
		var cmds []model.Cmd
		m, cmds = relayed(m, local)
		if cmds != nil {
			t.Fatalf("local relay %d: %#v", i, cmds)
		}
		if m, cmds = relayed(m, remoteState("idle", "")); cmds != nil {
			t.Fatalf("remote relay %d: %#v", i, cmds)
		}
	}
	if len(m.Agents.States) != 1 || m.Agents.States[0].Block != "5" {
		t.Errorf("states %+v", m.Agents.States)
	}
	// A window that lists the agent by id alone (an older half) agrees too.
	if m, cmds := relayed(m, model.AgentState{Block: "5", State: "idle"}); cmds != nil || len(m.Agents.States) != 1 {
		t.Errorf("by id: %#v %+v", cmds, m.Agents.States)
	}
}

func TestAForkWhoseLineStillNamesTheOriginalsSecretFileIsNotTheOriginal(t *testing.T) {
	m := model.Model{Workspace: "/w", PaneBlock: "9", AgentMapping: []model.AgentEntry{remoteEntry()}}
	fork := model.AgentState{Block: "31", State: "waiting_input", Command: "tern-kata agent --token-file=" + remotePrompt + ".token -- omp"}
	original := model.AgentState{Block: "5", State: "idle", Command: "omp @" + remotePrompt}
	m, _ = relayed(m, fork, original)
	if got := stateOf(m, "5"); got != "idle" {
		t.Errorf("the original's state is %q, the fork's overwrote it: %+v", got, m.Agents.States)
	}
	if got := stateOf(m, "31"); got != "waiting_input" {
		t.Errorf("the fork keeps its own block: %q", got)
	}
}

func TestAReopenedAgentWhoseLineNamesItsPromptFileByFlagIsStillThatAgent(t *testing.T) {
	m := model.Model{Workspace: "/w", PaneBlock: "9", AgentMapping: []model.AgentEntry{remoteEntry()}}
	reopened := model.AgentState{Block: "1052771002302467", State: "working", Command: "tern-kata agent --prompt-file=" + remotePrompt + " --ref=demo#ab12 -- omp"}
	m, cmds := relayed(m, reopened)
	if len(cmds) != 0 {
		t.Fatalf("cmds %#v", cmds)
	}
	if got := stateOf(m, "5"); got != "working" {
		t.Errorf("the reopened agent is not its entry's: %+v", m.Agents.States)
	}
}
