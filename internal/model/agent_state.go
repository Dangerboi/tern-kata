package model

import "slices"

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
// Tern's word for it (idle, working, waiting_input, exited) and LastMessage the agent's last assistant text. Command is the agent
// block's command line ("" when Tern gives none): a window on another machine numbers panes differently from the session host, so
// the command, which names the agent's prompt file, is how the Pane finds the agent's entry. Once matched, Block is the entry's block.
type AgentState struct {
	Block       string
	State       AgentStateWord
	LastMessage string
	Command     string
}

// AgentStatesMsg carries one window half's latest list of the agent blocks it sees in the session, kata Panes included. Several
// windows attach to the session and each relays its own list, so one relay is never the whole truth.
type AgentStatesMsg struct {
	States []AgentState
}

func (AgentStatesMsg) msg() {}

// AgentChange is a state a relay changed: Block went from From to To. From is "" for a block no relay had listed before.
type AgentChange struct {
	Block    string
	From, To AgentStateWord
}

// AgentStatesRelay merges the relayed lists by block. Received says a relay has arrived (an empty list is "no agents", no relay
// at all is "no state yet"). States holds the last known state of each block any relay listed, and a relay leaves the blocks it
// does not list as they were. Listed are the blocks the latest relay listed, and Changes the real transitions that relay caused:
// a second window reporting a state the first one already reported changes nothing. States stays bounded: a block that is
// neither mapped nor in the latest relay is dropped.
type AgentStatesRelay struct {
	Received bool
	States   []AgentState
	Listed   []string
	Changes  []AgentChange

	// inFlight says a RefreshAgents is out; again says a relay arrived meanwhile that wants one, with the suspects it named.
	inFlight bool
	again    bool
	suspects []string
	// asked holds the prompt files whose unknown agent already made a re-read, so each asks once.
	asked []string
}

// apply merges msg into the states. mapped says whether a block is in the Agent mapping.
func (r AgentStatesRelay) apply(msg AgentStatesMsg, mapped func(block string) bool) AgentStatesRelay {
	next := slices.Clone(r.States)
	r.Received = true
	r.Listed = make([]string, 0, len(msg.States))
	r.Changes = nil
	for _, s := range msg.States {
		r.Listed = append(r.Listed, s.Block)
		i := slices.IndexFunc(next, func(o AgentState) bool { return o.Block == s.Block })
		switch {
		case i < 0:
			next = append(next, s)
			r.Changes = append(r.Changes, AgentChange{Block: s.Block, To: s.State})
		default:
			if next[i].State != s.State {
				r.Changes = append(r.Changes, AgentChange{Block: s.Block, From: next[i].State, To: s.State})
			}
			next[i] = s
		}
	}
	r.States = next
	return r.trimmed(mapped)
}

// trimmed drops the states of blocks that are neither mapped nor listed by the latest relay.
func (r AgentStatesRelay) trimmed(mapped func(block string) bool) AgentStatesRelay {
	r.States = slices.DeleteFunc(slices.Clone(r.States), func(s AgentState) bool {
		return !mapped(s.Block) && !slices.Contains(r.Listed, s.Block)
	})
	return r
}

// entered says the latest relay moved block into state.
func (r AgentStatesRelay) entered(block string, state AgentStateWord) bool {
	return slices.ContainsFunc(r.Changes, func(c AgentChange) bool { return c.Block == block && c.To == state })
}

// enteredFinished says the latest relay moved block to idle or exited.
func (r AgentStatesRelay) enteredFinished(block string) bool {
	return r.entered(block, AgentIdle) || r.entered(block, AgentExited)
}

// answered says the latest relay moved block from waiting_input to working: its user answered it.
func (r AgentStatesRelay) answered(block string) bool {
	return slices.ContainsFunc(r.Changes, func(c AgentChange) bool {
		return c.Block == block && c.From == AgentWaitingInput && c.To == AgentWorking
	})
}
