package model

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
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
	State AgentStateWord
}

// pressing ranks a state word: lower is more pressing. A word nobody knows ranks with "no state".
func pressing(state AgentStateWord) int {
	switch state {
	case AgentWaitingInput:
		return 0
	case AgentWorking:
		return 1
	case AgentIdle:
		return 2
	case AgentExited:
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
func AgentStateWords(state AgentStateWord) (string, AgentTone) {
	switch state {
	case AgentWaitingInput:
		return "agent waiting for you", AgentToneWarning
	case AgentWorking:
		return "agent working", AgentToneAccent
	case AgentIdle:
		return "agent idle", AgentToneMuted
	case AgentExited:
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
	return badgeOf(m.IssueAgents(issueUID))
}

func badgeOf(agents []IssueAgent) (AgentBadge, bool) {
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

// AgentBadgeIndex answers AgentBadgeFor for many Issues: each Issue's agents, with their relayed states, gathered in one pass,
// so a row costs a lookup rather than a scan of the Agent mapping and the relayed states.
type AgentBadgeIndex struct {
	agents map[string][]IssueAgent
}

// AgentBadges indexes the agents of every Issue that has one, for a frame that asks the badge of every row.
func (m Model) AgentBadges() AgentBadgeIndex {
	if len(m.AgentMapping) == 0 {
		return AgentBadgeIndex{}
	}
	states := make(map[string]AgentStateWord, len(m.Agents.States))
	for _, s := range m.Agents.States {
		if _, seen := states[s.Block]; !seen {
			states[s.Block] = s.State
		}
	}
	agents := make(map[string][]IssueAgent)
	for _, entry := range m.AgentMapping {
		agents[entry.IssueUID] = append(agents[entry.IssueUID], IssueAgent{AgentEntry: entry, State: states[entry.Block]})
	}
	return AgentBadgeIndex{agents: agents}
}

// For is the Agent badge of the Issue, as AgentBadgeFor.
func (x AgentBadgeIndex) For(issueUID string) (AgentBadge, bool) {
	return badgeOf(x.agents[issueUID])
}

// AgentJumpMsg is a click on an Agent badge: Block is the agent it names, as decimal text.
type AgentJumpMsg struct{ Block string }

func (AgentJumpMsg) msg() {}

// jumpToAgent is the one way to go to an agent (a badge click or the Agent card): a background agent that is not open is opened beside this Pane,
// any other agent is focused. Opening is only ever the user's act, so nothing but a jump runs it.
func jumpToAgent(m Model, block string) (Model, []Cmd) {
	for _, e := range m.AgentMapping {
		if e.Block == block && e.Background && !e.Open {
			return placeAgent(m, block), []Cmd{OpenAgent{Block: block, PaneBlock: m.PaneBlock}}
		}
	}
	return m, []Cmd{FocusAgent{Block: block}}
}

// FocusAgent asks Tern to show an agent's block.
type FocusAgent struct{ Block string }

func (FocusAgent) cmd() {}

// AgentFocused answers FocusAgent; Err carries Tern's own words.
type AgentFocused struct{ Err error }

func (AgentFocused) msg() {}

func agentFocused(m Model, msg AgentFocused) Model {
	if msg.Err != nil {
		m.Notice = "Cannot focus the agent: " + msg.Err.Error()
	}
	return m
}

// RefreshAgents asks for agents.json to be read again. Suspects are mapped blocks some relay listed before and the latest relay
// does not list: that is no proof they are gone, for another window may simply not see them, so the gateway drops an entry (and
// its prompt file) only when `tern ls --json` does not list its block either. PaneBlock is this Pane's block: an entry whose starting Pane is no longer
// a live block is adopted by it ("" adopts nothing).
type RefreshAgents struct {
	Suspects  []string
	PaneBlock string
}

func (RefreshAgents) cmd() {}

// AgentsRefreshed answers RefreshAgents with the mapping as it is now; on Err the mapping stays.
type AgentsRefreshed struct {
	Agents []AgentEntry
	Err    error
}

func (AgentsRefreshed) msg() {}

func (m Model) agentMapped(block string) bool {
	return slices.ContainsFunc(m.AgentMapping, func(e AgentEntry) bool { return e.Block == block })
}

// agentsRefreshed takes in the mapping and, when a relay wanted a refresh while this one was out, asks for the next.
func agentsRefreshed(m Model, msg AgentsRefreshed) (Model, []Cmd) {
	if msg.Err == nil {
		m.AgentMapping = msg.Agents
		m.Agents = m.Agents.trimmed(m.agentMapped)
	}
	m.Agents.inFlight = false
	m, raised := raiseAgentAttention(m, false)
	if msg.Err == nil && !m.Agents.again {
		var followed []Cmd
		m, followed = followBackground(m)
		raised = append(raised, followed...)
	}
	if !m.Agents.again {
		return m, raised
	}
	suspects := slices.DeleteFunc(m.Agents.suspects, func(b string) bool { return !m.agentMapped(b) })
	m.Agents.again, m.Agents.suspects, m.Agents.inFlight = false, nil, true
	return m, append(raised, RefreshAgents{Suspects: suspects, PaneBlock: m.PaneBlock})
}

// promptTail finds the prompt file of a tern-kata agent in a command line: the part from the agent-prompts folder on, "" when
// the command names none. A tab or background agent's is `prompt-<starter>-<random>.md`, a split agent's `prompt-<random>.md`.
var promptTail = regexp.MustCompile(`agent-prompts[/\\]prompt-(?:\d+-)?\w+\.md`)

// resolveAgent gives a relayed agent the block of the entry it belongs to: the entry whose prompt file its command names, else the
// entry whose block is its pane id. A window on another machine numbers panes differently from the session host, so only the
// command is common to all windows. An agent no entry claims keeps its own id.
func (m Model) resolveAgent(s AgentState) AgentState {
	for _, e := range m.AgentMapping {
		if e.PromptFile != "" && strings.Contains(s.Command, e.PromptFile) {
			s.Block = e.Block
			return s
		}
	}
	return s
}

// relayAgents takes in a relayed list by merging it into the last known states, each agent filed under the block of its entry
// (see resolveAgent), so every window's relay agrees on one block per agent. A block the relay does not list keeps its state.
// agents.json is read again, once per prompt file, when a relayed agent's command names a tern-kata prompt file no entry has
// (another kata Pane on this host may just have started it), or when a mapped block some relay listed before is missing from it.
// That block is only a suspect: the gateway drops its entry when Tern no longer lists the block. One refresh is out at a time;
// a relay that wants one meanwhile gets it when the answer is in.
func relayAgents(m Model, msg AgentStatesMsg) (Model, []Cmd) {
	before := m.Agents.States
	states := make([]AgentState, len(msg.States))
	for i, s := range msg.States {
		states[i] = m.resolveAgent(s)
	}
	m.Agents = m.Agents.apply(AgentStatesMsg{States: states}, m.agentMapped)
	var suspects []string
	for _, s := range before {
		if !slices.Contains(m.Agents.Listed, s.Block) && m.agentMapped(s.Block) {
			suspects = append(suspects, s.Block)
		}
	}
	unmapped := false
	for _, s := range states {
		if m.agentMapped(s.Block) {
			continue
		}
		if tail := promptTail.FindString(s.Command); tail != "" && !slices.Contains(m.Agents.asked, tail) {
			m.Agents.asked = append(slices.Clone(m.Agents.asked), tail)
			unmapped = true
		}
	}
	// A split or tab agent of this Pane that was answered or finished may have been sent to the background by another Pane, which
	// no relay shows; agents.json says so.
	moved := false
	for _, e := range m.AgentMapping {
		if e.PaneBlock != m.PaneBlock || e.Background || m.PaneBlock == "" {
			continue
		}
		if m.Agents.answered(e.Block) && !slices.Contains(m.Agents.recheck, e.Block) {
			m.Agents.recheck = append(slices.Clone(m.Agents.recheck), e.Block)
		}
		moved = moved || m.Agents.answered(e.Block) || m.Agents.enteredFinished(e.Block)
	}
	if len(suspects) == 0 && !unmapped && !moved {
		return m, nil
	}
	if m.Agents.inFlight {
		m.Agents.again = true
		for _, b := range suspects {
			if !slices.Contains(m.Agents.suspects, b) {
				m.Agents.suspects = append(slices.Clone(m.Agents.suspects), b)
			}
		}
		return m, nil
	}
	m.Agents.inFlight = true
	return m, []Cmd{RefreshAgents{Suspects: suspects, PaneBlock: m.PaneBlock}}
}
