package model

// AgentState is one agent block as the window half relays it: Block is the block id as decimal text, State is
// Tern's word for it (idle, working, waiting_input, exited) and LastMessage the agent's last assistant text.
type AgentState struct {
	Block       string
	State       string
	LastMessage string
}

// AgentStatesMsg carries the window half's latest list of every agent block in the session, kata Panes included.
type AgentStatesMsg struct {
	States []AgentState
}

func (AgentStatesMsg) msg() {}

// AgentStatesRelay is the latest relayed list, and whether one has arrived: an empty list is "no agents", no
// relay at all is "no state yet".
type AgentStatesRelay struct {
	Received bool
	States   []AgentState
}

func (r AgentStatesRelay) apply(msg AgentStatesMsg) AgentStatesRelay {
	return AgentStatesRelay{Received: true, States: msg.States}
}
