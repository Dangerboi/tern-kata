package model

import (
	"fmt"
	"slices"
)

// The state words Tern gives an agent block.
const (
	stateWorking = "working"
	stateIdle    = "idle"
	stateExited  = "exited"
)

// AgentTone is the colour word of an Agent badge, as the view's span styles name it.
type AgentTone string

const (
	AgentToneAccent  AgentTone = "accent"
	AgentToneWarning AgentTone = "warning"
	AgentToneMuted   AgentTone = "muted"
)

// IssueAgent is an agent the Agent mapping holds for an Issue, with the state the last relay gave its block. State is "" when
// no relay has listed the block (no relay yet, no plugin, or the block is not an agent block yet).
type IssueAgent struct {
	AgentEntry
	State string
}

// pressing ranks a state word: lower is more pressing. A word nobody knows ranks with "no state".
func pressing(state string) int {
	switch state {
	case agentWaitingInput:
		return 0
	case stateWorking:
		return 1
	case stateIdle:
		return 2
	case stateExited:
		return 3
	}
	return 4
}

// IssueAgents lists the agents mapped to the Issue, in the mapping's order, each with its relayed state.
func (m Model) IssueAgents(issueUID string) []IssueAgent {
	var agents []IssueAgent
	for _, entry := range m.AgentMapping {
		if entry.IssueUID != issueUID {
			continue
		}
		agent := IssueAgent{AgentEntry: entry}
		for _, s := range m.Agents.States {
			if s.Block == entry.Block {
				agent.State = s.State
				break
			}
		}
		agents = append(agents, agent)
	}
	return agents
}

// AgentStateWords is how the badge and the Agent card name an agent's state, and the tone of the words.
func AgentStateWords(state string) (string, AgentTone) {
	switch state {
	case agentWaitingInput:
		return "agent waiting for you", AgentToneWarning
	case stateWorking:
		return "agent working", AgentToneAccent
	case stateIdle:
		return "agent idle", AgentToneMuted
	case stateExited:
		return "agent exited", AgentToneMuted
	}
	return "agent", AgentToneMuted
}

// AgentBadge is what the Agent badge says about an Issue's agents and the one agent a click on it focuses.
type AgentBadge struct {
	Text  string
	Tone  AgentTone
	Block string
}

// ChooseAgent is the agent a badge names: the most pressing state (waiting, working, idle, exited, none), the newest start on a tie.
func ChooseAgent(agents []IssueAgent) (IssueAgent, bool) {
	if len(agents) == 0 {
		return IssueAgent{}, false
	}
	best := agents[0]
	for _, a := range agents[1:] {
		rank, bestRank := pressing(a.State), pressing(best.State)
		if rank < bestRank || rank == bestRank && !a.StartedAt.Before(best.StartedAt) {
			best = a
		}
	}
	return best, true
}

// AgentBadgeFor is the Agent badge of an Issue: the chosen agent's state, plus " · N" for N > 1 mapped agents. ok is false
// when no agent is mapped to the Issue; a closed Issue keeps its badge.
func (m Model) AgentBadgeFor(issueUID string) (AgentBadge, bool) {
	agents := m.IssueAgents(issueUID)
	best, ok := ChooseAgent(agents)
	if !ok {
		return AgentBadge{}, false
	}
	badge := AgentBadge{Block: best.Block}
	badge.Text, badge.Tone = AgentStateWords(best.State)
	if len(agents) > 1 {
		badge.Text += fmt.Sprintf(" · %d", len(agents))
	}
	return badge, true
}

// AgentJumpMsg is a click on an Agent badge: Block is the agent it names, as decimal text.
type AgentJumpMsg struct{ Block string }

func (AgentJumpMsg) msg() {}

// FocusAgent asks Tern to show an agent's block.
type FocusAgent struct{ Block string }

func (FocusAgent) cmd() {}

// jumpToAgent is the one way to an agent, from a badge click or the Agent card.
func jumpToAgent(m Model, block string) (Model, []Cmd) {
	return m, []Cmd{FocusAgent{Block: block}}
}

// AgentFocused answers FocusAgent; Err carries Tern's own words.
type AgentFocused struct{ Err error }

func (AgentFocused) msg() {}

func agentFocused(m Model, msg AgentFocused) Model {
	if msg.Err != nil {
		m.Notice = "Cannot focus the agent: " + msg.Err.Error()
	}
	return m
}

// RefreshAgents asks for agents.json to be read again, first without the entries of the Gone blocks and their prompt files.
type RefreshAgents struct{ Gone []string }

func (RefreshAgents) cmd() {}

// AgentsRefreshed answers RefreshAgents with the mapping as it is now; on Err the mapping stays.
type AgentsRefreshed struct {
	Agents []AgentEntry
	Err    error
}

func (AgentsRefreshed) msg() {}

func agentsRefreshed(m Model, msg AgentsRefreshed) Model {
	if msg.Err == nil {
		m.AgentMapping = msg.Agents
	}
	return m
}

// relayAgents takes in a relayed list. A block the relay before did not list may be an agent another kata Pane on this host just
// started, so agents.json is read again, once for the relay, and only when the mapping does not have the block. A block the relay
// before listed and this one does not is gone: its entry and prompt file go. Nothing is pruned for a block no relay has listed,
// and so nothing before the second relay.
func relayAgents(m Model, msg AgentStatesMsg) (Model, []Cmd) {
	m.Agents = m.Agents.apply(msg)
	now := func(block string) func(AgentState) bool { return func(s AgentState) bool { return s.Block == block } }
	var gone []string
	for _, s := range m.Agents.Previous {
		if !slices.ContainsFunc(m.Agents.States, now(s.Block)) {
			gone = append(gone, s.Block)
		}
	}
	unmapped := false
	for _, s := range m.Agents.States {
		if slices.ContainsFunc(m.Agents.Previous, now(s.Block)) {
			continue
		}
		if !slices.ContainsFunc(m.AgentMapping, func(e AgentEntry) bool { return e.Block == s.Block }) {
			unmapped = true
		}
	}
	if len(gone) == 0 && !unmapped {
		return m, nil
	}
	return m, []Cmd{RefreshAgents{Gone: gone}}
}
