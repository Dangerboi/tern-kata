package model

// The Agent card opens when `w`, `W` or `S` is pressed on an Issue that already has agents: it lists them, goes to one, or starts
// another where the opening key asked.

// ModeAgentCard is the mode of the Agent card.
const ModeAgentCard Mode = "agent-card"

// The Agent card's own actions; its cursor keys are MoveDown, MoveUp, MoveFirst and MoveLast.
const (
	AgentCardGo         Action = "agent-card-go"
	AgentCardStart      Action = "agent-card-start"
	AgentCardBackground Action = "agent-card-background"
	AgentCardCancel     Action = "agent-card-cancel"
)

// AgentCardState is the Agent card. Placement is what the opening key asked, and what `n` starts.
type AgentCardState struct {
	Open      bool
	IssueUID  string
	IssueRef  string
	Placement AgentPlacement
	Cursor    int
}

// AgentCardRows are the agents the card lists: the Issue's mapped agents, in the mapping's order.
func (m Model) AgentCardRows() []IssueAgent {
	if !m.AgentCard.Open {
		return nil
	}
	return m.IssueAgents(m.AgentCard.IssueUID)
}

// AgentCardCursor is the selected row, kept inside the list as agents come and go.
func (m Model) AgentCardCursor() int {
	return max(0, min(m.AgentCard.Cursor, len(m.AgentCardRows())-1))
}

// AgentWhere says where an agent's block is: "open" for a background agent that is out of the hidden session, "background" for one
// in it, "tab" for one started in a tab, "split" otherwise.
func AgentWhere(a AgentEntry) string {
	if a.Background {
		if a.Open {
			return "open"
		}
		return string(AgentBackground)
	}
	if a.Placement == AgentTab {
		return string(AgentTab)
	}
	return string(AgentSplit)
}

// openAgentCard opens the card for the Issue with the placement the opening key asked.
func openAgentCard(m Model, issue Issue, placement AgentPlacement) Model {
	m.AgentCard = AgentCardState{Open: true, IssueUID: issue.UID, IssueRef: issue.QualifiedID, Placement: placement}
	return m
}

func updateAgentCardKey(m Model, msg KeyMsg) (Model, []Cmd) {
	binding, bound := LookupBindingForMode(msg.Chord, ModeAgentCard)
	if !bound {
		return m, nil
	}
	rows := m.AgentCardRows()
	cursor := m.AgentCardCursor()
	switch binding.Action {
	case AgentCardCancel:
		m.AgentCard = AgentCardState{}
	case AgentCardGo:
		if len(rows) == 0 {
			return m, nil
		}
		block := rows[cursor].Block
		m.AgentCard = AgentCardState{}
		return jumpToAgent(m, block)
	case AgentCardBackground:
		if len(rows) == 0 {
			return m, nil
		}
		return sendToBackground(m, rows[cursor].AgentEntry)
	case AgentCardStart:
		placement := m.AgentCard.Placement
		m.AgentCard = AgentCardState{}
		return startAgent(placement)(m)
	case MoveDown:
		m.AgentCard.Cursor = min(cursor+1, max(0, len(rows)-1))
	case MoveUp:
		m.AgentCard.Cursor = max(0, cursor-1)
	case MoveFirst:
		m.AgentCard.Cursor = 0
	case MoveLast:
		m.AgentCard.Cursor = max(0, len(rows)-1)
	}
	return m, nil
}

// sendToBackground is `b`: an agent in view goes to the hidden session, through the step going back uses. One already in the
// background and not open stays there; the card closes with a Notice saying so.
func sendToBackground(m Model, e AgentEntry) (Model, []Cmd) {
	m.AgentCard = AgentCardState{}
	if e.Background && !e.Open {
		m.Notice = "That agent is already in the background."
		return m, nil
	}
	return m, []Cmd{SendAgentBack{Block: e.Block, IssueRef: e.IssueRef}}
}

// AgentCardSelected is a click on an agent of the card's list; Block is its block id.
type AgentCardSelected struct{ Block string }

func (AgentCardSelected) msg() {}

func agentCardSelected(m Model, msg AgentCardSelected) Model {
	for i, a := range m.AgentCardRows() {
		if a.Block == msg.Block {
			m.AgentCard.Cursor = i
		}
	}
	return m
}
