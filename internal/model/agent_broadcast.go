package model

import (
	"fmt"
	"slices"
)

const (
	// ModeAgentBroadcastOutcome is the Message card showing who a broadcast reached and who it skipped.
	ModeAgentBroadcastOutcome Mode = "agent-broadcast-outcome"

	// BroadcastAgents is `S` on an Issue: the Message card for every running agent on it and on its open children.
	BroadcastAgents Action = "broadcast-agents"
)

// Why `S` opens no card, and why a broadcast skips an agent whose slot holds a message it has not cleared.
const (
	NoAgentsToBroadcastRefusal = "No agent is running on this Issue or its open children."
	AgentHoldsMessageRefusal   = "The agent holds an unread message; it was not replaced."
)

// BroadcastTarget is one agent a broadcast reaches for: its name and Agent address, and the Issue it runs on. Skipped is why the
// message was not written to it, "" while it is to be or was written; Answered says kata answered the write.
type BroadcastTarget struct {
	Agent, Address     string
	Block              string
	IssueUID, IssueRef string
	ProjectID          int64
	Skipped            string
	Answered           bool
	// Worked says a sweep saw the agent working after the wrap-up reached it, so an idle agent now has done the wrap-up.
	Worked bool
	// Commented says a sweep found a comment by the agent on its Issue since the sweep was sent; Reading says that read is in flight.
	Commented, Reading bool
	// Host and HostID name the host of an agent another host started (an Agent record); both are "" for an agent of this host.
	Host, HostID string
}

// Remote says the target runs on another host.
func (t BroadcastTarget) Remote() bool { return t.HostID != "" }

// BroadcastTargets are the agents a broadcast on the Issue uid (qualified id ref) goes through: those this Pane knows on it and
// on its open children that have not exited, then the agents other hosts published on them, each with the reason its message cannot
// be written, "" when it can. An agent that exited, or runs on a closed child, is not one of them.
func (m Model) BroadcastTargets(uid, ref string) []BroadcastTarget {
	refs := map[string]string{uid: ref}
	order := []string{uid}
	for _, child := range m.Relationships.Of(uid).OpenChildren() {
		refs[child.UID] = child.QualifiedID
		order = append(order, child.UID)
	}
	targets := m.runningTargets(func(e AgentEntry) (string, bool) {
		issueRef, ok := refs[e.IssueUID]
		return issueRef, ok
	})
	for _, issueUID := range order {
		targets = append(targets, m.remoteTargets(issueUID, refs[issueUID])...)
	}
	return targets
}

// remoteTargets are the agents other hosts published on the Issue uid (qualified id ref), each with the reason the sender rule
// gives, "" when it can take the message.
func (m Model) remoteTargets(uid, ref string) []BroadcastTarget {
	var targets []BroadcastTarget
	for _, r := range m.RemoteAgents(uid) {
		targets = append(targets, m.remoteBroadcastTarget(r, uid, ref))
	}
	return targets
}

func (m Model) remoteBroadcastTarget(r RemoteAgent, uid, ref string) BroadcastTarget {
	t := BroadcastTarget{Agent: r.Name, Address: r.Address, Block: r.Block, IssueUID: uid, IssueRef: ref, Skipped: m.remoteMessageRefusal(r),
		Host: r.HostLabel(), HostID: r.HostID}
	if issue := m.issueByUID(uid); issue != nil {
		t.ProjectID = issue.ProjectID
	}
	return t
}

// runningTargets are the agents of the mapping that have not exited and for which issueRef names an Issue, each with that ref.
func (m Model) runningTargets(issueRef func(AgentEntry) (string, bool)) []BroadcastTarget {
	var targets []BroadcastTarget
	for _, e := range m.AgentMapping {
		ref, ok := issueRef(e)
		if !ok || agentExited(m.Agents.States, e.Block) {
			continue
		}
		targets = append(targets, m.broadcastTarget(e, ref))
	}
	return targets
}

func (m Model) broadcastTarget(e AgentEntry, issueRef string) BroadcastTarget {
	return BroadcastTarget{Agent: e.Teammate, Address: e.Address, Block: e.Block, IssueUID: e.IssueUID, IssueRef: issueRef,
		ProjectID: m.projectOf(e), Skipped: m.agentMessageRefusal(e)}
}

// openAgentBroadcast is `S`: the Message card for the agents of the selected Issue and its open children, or the reason there is
// none in a Notice.
func openAgentBroadcast(m Model) (Model, []Cmd) {
	issue := closeIssue(m)
	if issue == nil {
		return m, nil
	}
	if len(m.BroadcastTargets(issue.UID, issue.QualifiedID)) == 0 {
		m.Notice = NoAgentsToBroadcastRefusal
		return m, nil
	}
	m.AgentMessage = AgentMessageState{Open: true, Broadcast: true, IssueUID: issue.UID, IssueRef: issue.QualifiedID, ProjectID: issue.ProjectID}
	return m, nil
}

// sendAgentBroadcast writes the typed message to every agent that can take it, each at its own address on its own Issue, never
// replacing what an agent holds. Agents found since the card opened count; the outcome shows once every write has answered.
func sendAgentBroadcast(m Model) (Model, []Cmd) {
	text, ok := m.messageText()
	if !ok {
		return m, nil
	}
	return writeBroadcast(m, m.BroadcastTargets(m.AgentMessage.IssueUID, m.AgentMessage.IssueRef), text)
}

// writeBroadcast writes text to each of targets that can take it; the card shows the outcome once every write has answered, at
// once when none is to be written.
func writeBroadcast(m Model, targets []BroadcastTarget, text string) (Model, []Cmd) {
	f := &m.AgentMessage
	f.Targets = targets
	f.Err = nil
	var cmds []Cmd
	for _, t := range f.Targets {
		if t.Skipped == "" {
			cmds = append(cmds, SendAgentMessage{Workspace: m.Workspace, ProjectID: t.ProjectID, UID: t.IssueUID, Actor: m.Actor, Address: t.Address, Message: text})
		}
	}
	f.Sending = len(cmds) > 0
	f.Outcome = !f.Sending
	return m, cmds
}

// broadcastSent applies kata's answer to one write of a broadcast: a failure or a held message skips that agent, and the outcome
// shows once no write is left unanswered.
func broadcastSent(m Model, msg AgentMessageSent) Model {
	f := &m.AgentMessage
	targets := slices.Clone(f.Targets)
	i := slices.IndexFunc(targets, func(t BroadcastTarget) bool {
		return t.Skipped == "" && !t.Answered && t.IssueUID == msg.UID && t.Address == msg.Address
	})
	if i < 0 {
		return m
	}
	targets[i].Answered = true
	switch {
	case msg.Err != nil:
		targets[i].Skipped = fmt.Sprintf("The message could not be written: %s", msg.Err)
	case msg.Held != "":
		targets[i].Skipped = AgentHoldsMessageRefusal
	default:
		targets[i].Worked = !targets[i].Remote() && m.agentState(targets[i].Block) == AgentWorking
	}
	f.Targets = targets
	if !slices.ContainsFunc(targets, func(t BroadcastTarget) bool { return t.Skipped == "" && !t.Answered }) {
		f.Sending, f.Outcome = false, true
	}
	return m
}

// BroadcastReached counts the agents the broadcast wrote to.
func (f AgentMessageState) BroadcastReached() int {
	return len(slices.DeleteFunc(slices.Clone(f.Targets), func(t BroadcastTarget) bool { return t.Skipped != "" }))
}
