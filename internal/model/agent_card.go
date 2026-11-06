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
	AgentCardMessage       Action = "agent-card-message"
	AgentCardInterrupt     Action = "agent-card-interrupt"
	AgentCardStop          Action = "agent-card-stop"
	AgentCardCancel        Action = "agent-card-cancel"
)

// ModeAgentCardStop is the Agent card asking whether to stop the selected agent; AgentCardStopConfirm stops it and
// AgentCardStopKeep goes back to the list.
const (
	ModeAgentCardStop    Mode   = "agent-card-stop"
	AgentCardStopConfirm Action = "agent-card-stop-confirm"
	AgentCardStopKeep    Action = "agent-card-stop-keep"
)

// Why the Agent card's Interrupt and Stop are off for the selected agent.
const (
	AgentCardExitedReason  = "That agent has exited."
	AgentCardNotOmpReason  = "Interrupt reaches only omp agents; that agent is not omp."
	agentCardInterruptSent = "Interrupt sent."
)

// AgentCardState is the Agent card.
type AgentCardState struct {
	Open     bool
	IssueUID string
	IssueRef string
	Cursor   int
	// Stopping is the block the card asks about stopping; "" while it shows the list.
	Stopping string
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

// AgentCardMode is the keymap mode of the Agent card: asking before a stop, or showing the list.
func (m Model) AgentCardMode() Mode {
	if m.AgentCard.Stopping != "" {
		return ModeAgentCardStop
	}
	return ModeAgentCard
}

// AgentCardStopTarget is the agent the card asks about stopping.
func (m Model) AgentCardStopTarget() (AgentEntry, bool) {
	for _, e := range m.AgentMapping {
		if e.Block == m.AgentCard.Stopping {
			return e, true
		}
	}
	return AgentEntry{}, false
}

// AgentLabel names an agent in a sentence: its teammate name and block, or the block alone for an agent started before names were kept.
func AgentLabel(e AgentEntry) string {
	if e.Teammate != "" {
		return e.Teammate + " (block " + e.Block + ")"
	}
	return "the agent in block " + e.Block
}

func updateAgentCardKey(m Model, msg KeyMsg) (Model, []Cmd) {
	binding, bound := LookupBindingForMode(msg.Chord, m.AgentCardMode())
	if !bound {
		return m, nil
	}
	if m.AgentCard.Stopping != "" {
		block := m.AgentCard.Stopping
		m.AgentCard.Stopping = ""
		if binding.Action == AgentCardStopConfirm {
			m.AgentCard = AgentCardState{}
			return m, []Cmd{StopAgent{Block: block}}
		}
		return m, nil
	}
	rows := m.AgentCardRows()
	cursor := m.AgentCardCursor()
	switch binding.Action {
	case AgentCardCancel:
		m.AgentCard = AgentCardState{}
	case AgentCardGo:
		if m.AgentCardUnavailable(AgentCardGo) != "" {
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
	case AgentCardMessage, AgentCardQuickReply:
		if len(rows) == 0 {
			return m, nil
		}
		return openAgentCardMessage(m, binding.Action == AgentCardQuickReply)
	case AgentCardInterrupt:
		if len(rows) == 0 {
			return m, nil
		}
		return interruptAgent(m, rows[cursor].AgentEntry)
	case AgentCardStop:
		if len(rows) == 0 {
			return m, nil
		}
		if reason := m.AgentCardUnavailable(AgentCardStop); reason != "" {
			m.AgentCard = AgentCardState{}
			m.Notice = reason
			return m, nil
		}
		m.AgentCard.Stopping = rows[cursor].Block
		return m, nil
	case AgentCardSplit:
		return startAgent(m, AgentCardSplit, AgentSplit)
	case AgentCardTab:
		return startAgent(m, AgentCardTab, AgentTab)
	case AgentCardNewBackground:
		return startAgent(m, AgentCardNewBackground, AgentBackground)
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
// background and not open stays there; the card closes with the card's reason as a Notice.
func sendToBackground(m Model, e AgentEntry) (Model, []Cmd) {
	reason := m.AgentCardUnavailable(AgentCardBackground)
	m.AgentCard = AgentCardState{}
	if reason != "" {
		m.Notice = reason
		return m, nil
	}
	return placeAgent(m, e.Block), []Cmd{SendAgentBack{Block: e.Block, IssueRef: e.IssueRef}}
}

// interruptAgent is `i`: Escape ends the agent's current turn, and the card closes with the card's reason as a Notice when it
// cannot.
func interruptAgent(m Model, e AgentEntry) (Model, []Cmd) {
	reason := m.AgentCardUnavailable(AgentCardInterrupt)
	m.AgentCard = AgentCardState{}
	if reason != "" {
		m.Notice = reason
		return m, nil
	}
	m.Notice = agentCardInterruptSent
	return m, []Cmd{InterruptAgent{Block: e.Block}}
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

// AgentCardUnavailable is why action cannot run on the Agent card now, or "" when it can. The card's handlers refuse with this
// reason as a Notice, and the card's buttons are drawn disabled with it; the one difference is that with no agent listed the card
// stays open and says nothing.
func (m Model) AgentCardUnavailable(action Action) string {
	rows := m.AgentCardRows()
	switch action {
	case AgentCardGo:
		if len(rows) == 0 {
			return noAgentReason
		}
	case AgentCardBackground:
		if len(rows) == 0 {
			return noAgentReason
		}
		if e := rows[m.AgentCardCursor()]; e.Background && !e.Open {
			return "That agent is already in the background."
		}
	case AgentCardMessage, AgentCardQuickReply:
		if len(rows) == 0 {
			return noAgentReason
		}
		return m.agentMessageRefusal(rows[m.AgentCardCursor()].AgentEntry)
	case AgentCardInterrupt, AgentCardStop:
		if len(rows) == 0 {
			return noAgentReason
		}
		e := rows[m.AgentCardCursor()]
		switch {
		case e.State == AgentExited:
			return AgentCardExitedReason
		case action == AgentCardStop:
		case e.NoAddress == AgentNotOmp:
			return AgentCardNotOmpReason
		}
	case AgentCardSplit, AgentCardTab, AgentCardNewBackground:
		issue := m.issueByUID(m.AgentCard.IssueUID)
		if issue == nil {
			return "That Issue is not loaded."
		}
		return IssueRefusal(*issue, action)
	}
	return ""
}

const noAgentReason = "No agent on this Issue yet."
