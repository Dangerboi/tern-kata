package model

import "strings"

// The wrap-up sweep is a palette command of the window half: it sends the "wrap up" quick reply to every running agent this
// host's Panes started, on whatever Issue each runs, through the broadcast's write (each agent at its own address, an unread
// message never replaced), and keeps the broadcast's outcome card open to show which agents have finished since.

// NoAgentsToSweepRefusal is why a sweep opens no card.
const NoAgentsToSweepRefusal = "No agent is running."

// Where an agent of a sweep stands, as the outcome card words it.
const (
	SweepFinishedExited = "finished: exited"
	SweepFinishedIdle   = "finished: idle after the wrap-up"
	SweepWaiting        = "not yet: waiting for you"
	SweepNotYet         = "not yet"
)

// sweepAgents is the wrap-up sweep: the wrap-up text to every agent this host's Panes started that has not exited. An agent that
// cannot take it, or holds an unread message, is listed with why; the write is the broadcast's.
func sweepAgents(m Model) (Model, []Cmd) {
	if m.Stale {
		m.Notice = "Daemon disconnected; reconnect before sweeping."
		return m, nil
	}
	var targets []BroadcastTarget
	for _, e := range m.AgentMapping {
		if !agentExited(m.Agents.States, e.Block) {
			targets = append(targets, m.broadcastTarget(e, e.IssueRef))
		}
	}
	if len(targets) == 0 {
		m.Notice = NoAgentsToSweepRefusal
		return m, nil
	}
	m.AgentMessage = AgentMessageState{Open: true, Broadcast: true, Sweep: true}
	return writeBroadcast(m, targets, QuickReplyWrapUp)
}

// agentState is the state the relay gave the block, "" when no relay listed it.
func (m Model) agentState(block string) AgentStateWord {
	for _, s := range m.Agents.States {
		if s.Block == block {
			return s.State
		}
	}
	return ""
}

// trackSweep notes the agents of an open sweep the latest relay moved to working after the wrap-up reached them.
func trackSweep(m Model) Model {
	f := &m.AgentMessage
	if !f.Open || !f.Sweep {
		return m
	}
	targets := append([]BroadcastTarget(nil), f.Targets...)
	for i, t := range targets {
		if t.Skipped == "" && t.Answered && !t.Worked && m.Agents.entered(t.Block, AgentWorking) {
			targets[i].Worked = true
		}
	}
	f.Targets = targets
	return m
}

// SweepStatus is where the sweep stands with a target it wrote to: finished once the agent exited (or its block is gone), or is
// idle after it worked on the wrap-up; not yet otherwise. Last is the agent's last message, first line only, "" when none.
func (m Model) SweepStatus(t BroadcastTarget) (status, last string) {
	state := m.agentState(t.Block)
	switch {
	case state == AgentExited || !m.agentMapped(t.Block):
		status = SweepFinishedExited
	case state == AgentIdle && t.Worked:
		status = SweepFinishedIdle
	case state == AgentWaitingInput:
		status = SweepWaiting
	default:
		status = SweepNotYet
	}
	for _, s := range m.Agents.States {
		if s.Block == t.Block {
			last, _, _ = strings.Cut(strings.TrimSpace(s.LastMessage), "\n")
		}
	}
	return status, last
}

// SweepFinished counts the agents the sweep wrote to that have finished.
func (m Model) SweepFinished() int {
	n := 0
	for _, t := range m.AgentMessage.Targets {
		if t.Skipped == "" {
			if status, _ := m.SweepStatus(t); strings.HasPrefix(status, "finished") {
				n++
			}
		}
	}
	return n
}
