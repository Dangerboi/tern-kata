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
	IssueUID, IssueRef string
	ProjectID          int64
	Skipped            string
	Answered           bool
}

// BroadcastTargets are the agents a broadcast on the Issue uid (qualified id ref) goes through: those this Pane knows on it and
// on its open children that have not exited, each with the reason its message cannot be written, "" when it can. An agent that
// exited, or runs on a closed child, is not one of them.
func (m Model) BroadcastTargets(uid, ref string) []BroadcastTarget {
	refs := map[string]string{uid: ref}
	for _, child := range m.Relationships.Of(uid).OpenChildren() {
		refs[child.UID] = child.QualifiedID
	}
	var targets []BroadcastTarget
	for _, e := range m.AgentMapping {
		issueRef, ok := refs[e.IssueUID]
		if !ok || agentExited(m.Agents.States, e.Block) {
			continue
		}
		targets = append(targets, BroadcastTarget{Agent: e.Teammate, Address: e.Address, IssueUID: e.IssueUID, IssueRef: issueRef,
			ProjectID: m.projectOf(e), Skipped: m.agentMessageRefusal(e)})
	}
	return targets
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
	f := &m.AgentMessage
	f.Targets = m.BroadcastTargets(f.IssueUID, f.IssueRef)
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
