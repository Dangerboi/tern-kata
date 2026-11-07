package model

import (
	"errors"
	"fmt"
	"strings"
)

// Agents find and message each other through `tern-kata agents` and `tern-kata message`: the first lists the agents this host's
// Panes started that have not exited, the second writes a message to one of them (or to each on an Issue) the way the Message
// card does, as the sender's actor with the sender's teammate on it, so the receiving extension words it as from a peer.

// AgentStatesSaved answers SaveAgentStates.
type AgentStatesSaved struct{ Err error }

func (AgentStatesSaved) msg() {}

// SaveAgentStates records the latest merged relay in tern-kata's state folder, where `tern-kata agents` reads it: a Pane is the
// only process that hears the window half's relay, and the command line runs without one.
type SaveAgentStates struct{ States []AgentState }

func (SaveAgentStates) cmd() {}

// CheckAgentMessage is the text trimmed, or the refusal a message to an agent gets: nothing typed, or more than kata takes.
func CheckAgentMessage(text string) (string, error) {
	text = strings.TrimSpace(text)
	switch {
	case text == "":
		return "", errors.New("Type a message.")
	case len(text) > agentAttentionMaxBytes:
		return "", fmt.Errorf("The message is %d bytes; kata takes at most %d.", len(text), agentAttentionMaxBytes)
	}
	return text, nil
}

// RunningAgents lists the agents of entries that have not exited, in the mapping's order, each with the state the relay gave its
// block ("" when no relay listed it).
func RunningAgents(entries []AgentEntry, states []AgentState) []IssueAgent {
	var running []IssueAgent
	for _, entry := range entries {
		agent := agentWithState(entry, states)
		if agent.State != AgentExited {
			running = append(running, agent)
		}
	}
	return running
}

// OnIssue says ref names the agent's Issue: its qualified ref (<project>#<short id>, also written <project>/<short id>), its uid,
// or its short id alone.
func (e AgentEntry) OnIssue(ref string) bool {
	if ref == "" {
		return false
	}
	if project, short, ok := strings.Cut(ref, "/"); ok {
		ref = project + "#" + short
	}
	_, short, _ := strings.Cut(e.IssueRef, "#")
	return ref == e.IssueRef || ref == e.IssueUID || ref == short
}

// AgentsOnIssue keeps the agents on the Issue ref names.
func AgentsOnIssue(agents []IssueAgent, ref string) []IssueAgent {
	var found []IssueAgent
	for _, a := range agents {
		if a.OnIssue(ref) {
			found = append(found, a)
		}
	}
	return found
}

// MessageTargets are the running agents a message to `to` goes to: the one holding that Agent address, else every agent on the
// Issue `to` names that can receive a message, except the sender (own is its address, "" for none). An agent that cannot receive one
// (not omp, or started before it had an address) is left out; when nothing is left the error says why.
func MessageTargets(running []IssueAgent, to, own string) ([]IssueAgent, error) {
	for _, a := range running {
		if a.Address != "" && a.Address == to {
			return []IssueAgent{a}, nil
		}
	}
	onIssue := AgentsOnIssue(running, to)
	var targets []IssueAgent
	for _, a := range onIssue {
		if a.Address != "" && a.Address != own {
			targets = append(targets, a)
		}
	}
	switch {
	case len(targets) > 0:
		return targets, nil
	case len(onIssue) > 0:
		return nil, fmt.Errorf("no agent on %s can receive a message from you (none other than yourself runs omp with tern-kata's extension)", to)
	}
	return nil, fmt.Errorf("no running agent at or on %q; `tern-kata agents` lists them", to)
}
