package model

import (
	"fmt"
	"slices"
)

// OpenAgent moves a background agent's block right of this Pane's block (PaneBlock "" names the focused one) and marks its entry open.
type OpenAgent struct{ Block, PaneBlock string }

func (OpenAgent) cmd() {}

// SendAgentBack moves an agent's block back into its own tab of the hidden session, names that tab IssueRef and
// clears the entry's open flag and marks it background.
type SendAgentBack struct{ Block, IssueRef string }

func (SendAgentBack) cmd() {}

// AgentPlaced answers OpenAgent (Back false) and SendAgentBack (Back true). Moved says the block did move and its entry now reads
// Open as the gateway wrote it; Err carries Tern's words for a step that failed, also after the block moved (the tab name).
type AgentPlaced struct {
	Block string
	Back  bool
	Moved bool
	Open  bool
	Err   error
}

func (AgentPlaced) msg() {}

func agentPlaced(m Model, msg AgentPlaced) Model {
	if msg.Moved {
		m.AgentMapping = slices.Clone(m.AgentMapping)
		for i := range m.AgentMapping {
			if m.AgentMapping[i].Block == msg.Block {
				m.AgentMapping[i].Open = msg.Open
				m.AgentMapping[i].Background = m.AgentMapping[i].Background || msg.Back
			}
		}
	}
	if msg.Err != nil {
		what := "open the agent"
		if msg.Back {
			what = "move the agent back"
		}
		m.Notice = fmt.Sprintf("Cannot %s: %s", what, msg.Err)
	}
	return m
}

// sendAgentsBack is the rule for going back: an agent this Pane started that is a background agent and open goes back to the hidden
// session when the relay shows it working after waiting_input, which means its user answered it. An agent that is idle or has
// exited stays where it is, and so does every other Pane's agent (that Pane sends its own back).
func sendAgentsBack(m Model) (Model, []Cmd) {
	if m.PaneBlock == "" {
		return m, nil
	}
	stateIn := func(states []AgentState, block string) string {
		for _, s := range states {
			if s.Block == block {
				return s.State
			}
		}
		return ""
	}
	var cmds []Cmd
	for _, e := range m.AgentMapping {
		if e.Background && e.Open && e.PaneBlock == m.PaneBlock &&
			stateIn(m.Agents.States, e.Block) == stateWorking && stateIn(m.Agents.Previous, e.Block) == agentWaitingInput {
			cmds = append(cmds, SendAgentBack{Block: e.Block, IssueRef: e.IssueRef})
		}
	}
	return m, cmds
}
