package model

// AgentStateWord is Tern's word for an agent block's state.
type AgentStateWord string

// The state words Tern gives an agent block.
const (
	AgentIdle         AgentStateWord = "idle"
	AgentWorking      AgentStateWord = "working"
	AgentWaitingInput AgentStateWord = "waiting_input"
	AgentExited       AgentStateWord = "exited"
)

// AgentState is one agent block as the window half relays it: Block is the block id as decimal text, State is
// Tern's word for it (idle, working, waiting_input, exited) and LastMessage the agent's last assistant text.
type AgentState struct {
	Block       string
	State       AgentStateWord
	LastMessage string
}

// AgentStatesMsg carries the window half's latest list of every agent block in the session, kata Panes included.
type AgentStatesMsg struct {
	States []AgentState
}

func (AgentStatesMsg) msg() {}

// AgentStatesRelay is the latest relayed list, and whether one has arrived: an empty list is "no agents", no
// relay at all is "no state yet". Previous is the list the relay before it carried.
type AgentStatesRelay struct {
	Received bool
	States   []AgentState
	Previous []AgentState
}

func (r AgentStatesRelay) apply(msg AgentStatesMsg) AgentStatesRelay {
	return AgentStatesRelay{Received: true, States: msg.States, Previous: r.States}
}
