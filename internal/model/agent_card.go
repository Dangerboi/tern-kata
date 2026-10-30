package model

// The Agent card opens when `w` is pressed on an Issue: it lists the Issue's agents, goes to one, sends one to the background, or
// starts a new agent in a split, a tab or the background.

// ModeAgentCard is the mode of the Agent card.
const ModeAgentCard Mode = "agent-card"

// The Agent card's own actions; its cursor keys are MoveDown, MoveUp, MoveFirst and MoveLast. The three that start an agent
// are AgentCardSplit, AgentCardTab and AgentCardNewBackground; AgentCardBackground sends the selected agent to the background.
const (
	AgentCardGo            Action = "agent-card-go"
	AgentCardSplit         Action = "agent-card-split"
	AgentCardTab           Action = "agent-card-tab"
	AgentCardNewBackground Action = "agent-card-new-background"
	AgentCardBackground    Action = "agent-card-background"
	AgentCardCancel        Action = "agent-card-cancel"
)

// AgentCardState is the Agent card.
type AgentCardState struct {
	Open     bool
	IssueUID string
	IssueRef string
	Cursor   int
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
	if a.Background && a.Open {
		return "open"
	}
	placement := a.Placement
	if a.Background {
		placement = AgentBackground
	}
	name, _ := placement.words()
	return name
}

// openAgentCard opens the card for the Issue.
func openAgentCard(m Model, issue Issue) Model {
	m.AgentCard = AgentCardState{Open: true, IssueUID: issue.UID, IssueRef: issue.QualifiedID}
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
	case AgentCardSplit:
		return startAgent(m, AgentSplit)
	case AgentCardTab:
		return startAgent(m, AgentTab)
	case AgentCardNewBackground:
		return startAgent(m, AgentBackground)
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
	return placeAgent(m, e.Block), []Cmd{SendAgentBack{Block: e.Block, IssueRef: e.IssueRef}}
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
