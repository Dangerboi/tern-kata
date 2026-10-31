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
	m.Agents.placing = slices.DeleteFunc(slices.Clone(m.Agents.placing), func(b string) bool { return b == msg.Block })
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
// session when a relay moves it from waiting_input to working, which means its user answered it. It counts as open when the
// entry says so or when the relay shows it outside the hidden session, so an agent another Pane opened goes back on the first
// answer. A second window reporting the same state is no move. An agent that is idle or has
// exited stays where it is, and so does every other Pane's agent (that Pane sends its own back).
func sendAgentsBack(m Model) (Model, []Cmd) {
	if m.PaneBlock == "" {
		return m, nil
	}
	var cmds []Cmd
	for _, e := range m.AgentMapping {
		if e.Background && (e.Open || m.Agents.seenShown(e.Block)) && e.PaneBlock == m.PaneBlock &&
			m.Agents.answered(e.Block) {
			cmds = append(cmds, SendAgentBack{Block: e.Block, IssueRef: e.IssueRef})
			m = placeAgent(m, e.Block)
		}
	}
	return m, cmds
}

// placeAgent marks block as being moved by this Pane, so no relay changes its open flag until the move is answered.
func placeAgent(m Model, block string) Model {
	m.Agents.placing = append(slices.Clone(m.Agents.placing), block)
	return m
}

// SetAgentOpen writes the open flag of an entry as the window half's relay shows it: the agent is out of the hidden session
// (Open) or back in it, moved there by Tern's own focus or by hand rather than by this Pane.
// Background also marks the entry a background agent: the relay shows it in the hidden session.
type SetAgentOpen struct {
	Block      string
	Open       bool
	Background bool
}

func (SetAgentOpen) cmd() {}

// AgentOpenSet answers SetAgentOpen; on Err the entry stays as it was.
type AgentOpenSet struct {
	Block      string
	Open       bool
	Background bool
	Err        error
}

func (AgentOpenSet) msg() {}

func agentOpenSet(m Model, msg AgentOpenSet) Model {
	m.Agents.flagging = slices.DeleteFunc(slices.Clone(m.Agents.flagging), func(b string) bool { return b == msg.Block })
	if msg.Err != nil {
		m.Notice = "Cannot record where the agent is: " + msg.Err.Error()
		return m
	}
	m.AgentMapping = slices.Clone(m.AgentMapping)
	for i := range m.AgentMapping {
		if m.AgentMapping[i].Block == msg.Block {
			m.AgentMapping[i].Open = msg.Open
			m.AgentMapping[i].Background = m.AgentMapping[i].Background || msg.Background
		}
	}
	return m
}

// followAgents makes the open flag of this Pane's agents follow what the latest relay shows: out of the hidden session
// marks a background entry open, in it marks any entry not open and background (another Pane sent it there). The window half opens an agent when Tern focuses it in the hidden session, and the
// user may move one by hand; this Pane learns of it here. An agent the relay does not place (unknown) changes nothing, and neither
// does one whose flag is being written or which this Pane is moving itself, so there is at most one write per change.
func followAgents(m Model, listed []AgentState) (Model, []Cmd) {
	if m.PaneBlock == "" {
		return m, nil
	}
	var cmds []Cmd
	for _, s := range listed {
		if s.Hidden == SeenUnknown || slices.Contains(m.Agents.flagging, s.Block) || slices.Contains(m.Agents.placing, s.Block) {
			continue
		}
		for _, e := range m.AgentMapping {
			shown := s.Hidden == SeenShown
			if e.Block != s.Block || e.PaneBlock != m.PaneBlock || (e.Background && e.Open == shown) || (!e.Background && shown) {
				continue
			}
			m.Agents.flagging = append(slices.Clone(m.Agents.flagging), e.Block)
			cmds = append(cmds, SetAgentOpen{Block: e.Block, Open: shown, Background: !shown && !e.Background})
		}
	}
	return m, cmds
}
