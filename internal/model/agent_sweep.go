package model

import (
	"slices"
	"strings"
	"time"
)

// The wrap-up sweep is a palette command of the window half: it sends the "wrap up" quick reply to every running agent this
// host's Panes started, on whatever Issue each runs, through the broadcast's write (each agent at its own address, an unread
// message never replaced), and keeps the broadcast's outcome card open to show which agents have finished since.

// NoAgentsToSweepRefusal is why a sweep opens no card.
const NoAgentsToSweepRefusal = "No agent is running."

// Where an agent of a sweep stands, as the outcome card words it.
const (
	SweepFinishedExited = "finished: exited"
	SweepFinishedIdle   = "finished: idle after the wrap-up"
	SweepIdleNoComment  = "not yet: idle, no status comment"
	SweepWaiting        = "not yet: waiting for you"
	SweepNotYet         = "not yet"
)

// SweepStanding is where the sweep stands with one agent: Finished, and Text as the card words it.
type SweepStanding struct {
	Finished bool
	Text     string
}

// sweepAgents is the wrap-up sweep: the wrap-up text to every agent this host's Panes started that has not exited. An agent that
// cannot take it, or holds an unread message, is listed with why; the write is the broadcast's.
func sweepAgents(m Model) (Model, []Cmd) {
	if m.Stale {
		m.Notice = "Daemon disconnected; reconnect before sweeping."
		return m, nil
	}
	if !m.agentsKnown() {
		m.sweepPending = true
		return m, nil
	}
	m.sweepPending = false
	return sweepKnownAgents(m)
}

// agentsKnown says the Pane has both the first PruneAgents answer and the first relay: until then the mapping or the states a
// sweep reads are empty, not the truth.
func (m Model) agentsKnown() bool {
	return m.agentsPruned && m.Agents.Received
}

// runPendingSweep runs a sweep asked for before the mapping and the first relay were in, once both are.
func runPendingSweep(m Model) (Model, []Cmd) {
	if !m.sweepPending || !m.agentsKnown() {
		return m, nil
	}
	return sweepAgents(m)
}

func sweepKnownAgents(m Model) (Model, []Cmd) {
	targets := m.runningTargets(func(e AgentEntry) (string, bool) { return e.IssueRef, true })
	for _, issue := range m.Issues {
		targets = append(targets, m.remoteTargets(issue.UID, issue.QualifiedID)...)
	}
	if len(targets) == 0 {
		m.Notice = NoAgentsToSweepRefusal
		return m, nil
	}
	m.AgentMessage = AgentMessageState{Open: true, Broadcast: true, Sweep: true, SweepSince: m.Now}
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

// trackSweep notes the agents of an open sweep the latest relay moved to working after the wrap-up reached them, and reads the
// Issue's comments of each one idle after it, to see whether it commented since the sweep was sent.
func trackSweep(m Model) (Model, []Cmd) {
	f := &m.AgentMessage
	if !f.Open || !f.Sweep {
		return m, nil
	}
	targets := append([]BroadcastTarget(nil), f.Targets...)
	for i, t := range targets {
		if t.Skipped == "" && t.Answered && !t.Worked && !t.Remote() && m.Agents.entered(t.Block, AgentWorking) {
			targets[i].Worked = true
		}
	}
	f.Targets = targets
	return readSweepComments(m)
}

// readSweepComments asks, for each agent of an open sweep that is idle after the wrap-up and not yet seen to have commented, for
// the comments of its Issue. A stream event of a new comment asks again; one read per agent is in flight at a time.
func readSweepComments(m Model) (Model, []Cmd) {
	f := &m.AgentMessage
	if !f.Open || !f.Sweep {
		return m, nil
	}
	targets := append([]BroadcastTarget(nil), f.Targets...)
	var cmds []Cmd
	for i, t := range targets {
		if t.Skipped != "" || !t.Answered || t.Commented || t.Reading {
			continue
		}
		// An agent on another host has no relayed state: its comment since the sweep is the only sign, so it is read until it comes.
		if t.Remote() && m.remoteRecordGone(t) || !t.Remote() && (!t.Worked || m.agentState(t.Block) != AgentIdle || !m.agentMapped(t.Block)) {
			continue
		}
		targets[i].Reading = true
		cmds = append(cmds, ReadSweepComments{Workspace: m.Workspace, ProjectID: t.ProjectID, UID: t.IssueUID, Address: t.Address, Teammate: addressName(t.Address), Since: f.SweepSince})
	}
	f.Targets = targets
	return m, cmds
}

// addressName is the part of an Agent address after its last "/": the teammate its comments carry.
func addressName(address string) string {
	return address[strings.LastIndex(address, "/")+1:]
}

// ReadSweepComments reads the comments of an Issue through the Daemon, to see whether one by Teammate was made at or after Since.
type ReadSweepComments struct {
	Workspace string
	ProjectID int64
	UID       string
	Address   string
	Teammate  string
	Since     time.Time
}

func (ReadSweepComments) cmd() {}

// SweepCommentsRead answers ReadSweepComments for the agent at Address on the Issue UID: Commented says the Issue holds a comment
// by the agent's teammate since the sweep was sent.
type SweepCommentsRead struct {
	UID, Address string
	Commented    bool
	Err          error
}

func (SweepCommentsRead) msg() {}

// sweepCommentsRead applies the answer to one read: a comment makes the agent's idle count, a failed read leaves it for the next.
func sweepCommentsRead(m Model, msg SweepCommentsRead) Model {
	f := &m.AgentMessage
	targets := slices.Clone(f.Targets)
	for i, t := range targets {
		if t.IssueUID == msg.UID && t.Address == msg.Address {
			targets[i].Reading = false
			targets[i].Commented = targets[i].Commented || (msg.Err == nil && msg.Commented)
		}
	}
	f.Targets = targets
	return m
}

// SweepStatus is where the sweep stands with a target it wrote to: finished once the agent exited (or its block is gone), or is
// idle after it worked on the wrap-up and has commented on its Issue since; not yet otherwise. Last is the agent's last message,
// first line only (capped as the Agent card's is), "" when none.
func (m Model) SweepStatus(t BroadcastTarget) (standing SweepStanding, last string) {
	if t.Remote() {
		return m.remoteSweepStatus(t), ""
	}
	switch state := m.agentState(t.Block); {
	case state == AgentExited || !m.agentMapped(t.Block):
		standing = SweepStanding{true, SweepFinishedExited}
	case state == AgentIdle && t.Worked && t.Commented:
		standing = SweepStanding{true, SweepFinishedIdle}
	case state == AgentIdle && t.Worked:
		standing = SweepStanding{false, SweepIdleNoComment}
	case state == AgentWaitingInput:
		standing = SweepStanding{false, SweepWaiting}
	default:
		standing = SweepStanding{false, SweepNotYet}
	}
	return standing, m.AgentLastLine(t.Block)
}

// remoteSweepStatus is where the sweep stands with an agent on another host: "on <host>: finished" once it has commented on its
// Issue since the sweep or its record is gone, else "on <host>: not yet".
func (m Model) remoteSweepStatus(t BroadcastTarget) SweepStanding {
	if t.Commented || m.remoteRecordGone(t) {
		return SweepStanding{true, "on " + t.Host + ": finished"}
	}
	return SweepStanding{false, "on " + t.Host + ": not yet"}
}

// remoteRecordGone says the Issue of the remote target is loaded and no longer holds the agent's record.
func (m Model) remoteRecordGone(t BroadcastTarget) bool {
	if m.issueByUID(t.IssueUID) == nil {
		return false
	}
	return !slices.ContainsFunc(m.RemoteAgents(t.IssueUID), func(r RemoteAgent) bool { return r.HostID == t.HostID && r.Block == t.Block })
}

// SweepFinished counts the agents the sweep wrote to that have finished.
func (m Model) SweepFinished() int {
	n := 0
	for _, t := range m.AgentMessage.Targets {
		if t.Skipped == "" {
			if standing, _ := m.SweepStatus(t); standing.Finished {
				n++
			}
		}
	}
	return n
}
