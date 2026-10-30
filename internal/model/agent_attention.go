package model

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// AgentWaitingMessage is the request's text when the waiting agent said nothing.
const AgentWaitingMessage = "agent waiting for you"

// agentWaitingInput is Tern's state word for an agent that waits for its user.
const agentWaitingInput = "waiting_input"

// agentAttentionMaxBytes is the longest message kata notify takes.
const agentAttentionMaxBytes = 1024

// RaisedAttention is an Attention request this Pane wrote for a waiting agent and has not cleared.
type RaisedAttention struct {
	UID       string
	ProjectID int64
}

// RaiseAgentAttention writes the Attention request for Address on an Issue, replacing any earlier one, as Actor.
type RaiseAgentAttention struct {
	Workspace string
	ProjectID int64
	UID       string
	Actor     string
	Address   string
	Message   string
}

func (RaiseAgentAttention) cmd() {}

// ClearAgentAttention clears the request for Address on an Issue whatever it holds.
type ClearAgentAttention struct {
	Workspace string
	ProjectID int64
	UID       string
	Actor     string
	Address   string
}

func (ClearAgentAttention) cmd() {}

// AgentAttentionWritten answers RaiseAgentAttention and ClearAgentAttention for the Issue with UID.
type AgentAttentionWritten struct {
	UID   string
	Clear bool
	Err   error
}

func (AgentAttentionWritten) msg() {}

// agentAttentionMessage is the agent's last message cut to the bytes kata takes, on a rune boundary, or the fixed text when blank.
func agentAttentionMessage(last string) string {
	if strings.TrimSpace(last) == "" {
		return AgentWaitingMessage
	}
	if len(last) <= agentAttentionMaxBytes {
		return last
	}
	cut := agentAttentionMaxBytes
	for cut > 0 && !utf8.RuneStart(last[cut]) {
		cut--
	}
	return last[:cut]
}

// agentWaiting reports whether the relay shows the agent block waiting for input.
func agentWaiting(states []AgentState, block string) bool {
	for _, s := range states {
		if s.Block == block {
			return s.State == agentWaitingInput
		}
	}
	return false
}

// agentWaitingOn reports whether any mapped agent on the Issue, whichever Pane started it, waits now.
func (m Model) agentWaitingOn(uid string) bool {
	for _, e := range m.AgentMapping {
		if e.IssueUID == uid && agentWaiting(m.Agents.States, e.Block) {
			return true
		}
	}
	return false
}

func (m Model) projectOf(e AgentEntry) int64 {
	if e.ProjectID != 0 {
		return e.ProjectID
	}
	if issue := m.issueByUID(e.IssueUID); issue != nil {
		return issue.ProjectID
	}
	return 0
}

func (m Model) raised(uid string) bool {
	for _, r := range m.AgentAttention {
		if r.UID == uid {
			return true
		}
	}
	return false
}

// raiseAgentAttention applies the relay just received: an agent this Pane started that entered waiting raises the request with
// its message, and a request this Pane raised is cleared once no mapped agent on its Issue waits. An Inbox address kata refuses
// writes nothing.
func raiseAgentAttention(m Model) (Model, []Cmd) {
	address, err := m.InboxAddress()
	if err != nil || m.PaneBlock == "" {
		return m, nil
	}
	var cmds []Cmd
	for _, e := range m.AgentMapping {
		if e.PaneBlock != m.PaneBlock || !agentWaiting(m.Agents.States, e.Block) || agentWaiting(m.Agents.Previous, e.Block) {
			continue
		}
		project := m.projectOf(e)
		if project == 0 {
			continue
		}
		var last string
		for _, s := range m.Agents.States {
			if s.Block == e.Block {
				last = s.LastMessage
			}
		}
		if !m.raised(e.IssueUID) {
			m.AgentAttention = append(m.AgentAttention, RaisedAttention{UID: e.IssueUID, ProjectID: project})
		}
		cmds = append(cmds, RaiseAgentAttention{Workspace: m.Workspace, ProjectID: project, UID: e.IssueUID, Actor: m.Actor, Address: address, Message: agentAttentionMessage(last)})
	}
	kept := m.AgentAttention[:0:0]
	for _, r := range m.AgentAttention {
		if m.agentWaitingOn(r.UID) {
			kept = append(kept, r)
			continue
		}
		cmds = append(cmds, ClearAgentAttention{Workspace: m.Workspace, ProjectID: r.ProjectID, UID: r.UID, Actor: m.Actor, Address: address})
	}
	m.AgentAttention = kept
	return m, cmds
}

// QuitAgentAttention is what a quitting Pane still owes: the clears of the requests it raised, except where another Pane's
// mapped agent on the Issue still waits.
func (m Model) QuitAgentAttention() []Cmd {
	address, err := m.InboxAddress()
	if err != nil {
		return nil
	}
	var cmds []Cmd
	for _, r := range m.AgentAttention {
		if !m.agentWaitingOnOthers(r.UID) {
			cmds = append(cmds, ClearAgentAttention{Workspace: m.Workspace, ProjectID: r.ProjectID, UID: r.UID, Actor: m.Actor, Address: address})
		}
	}
	return cmds
}

// agentWaitingOnOthers is agentWaitingOn for the agents other Panes started: this Pane's own go with it.
func (m Model) agentWaitingOnOthers(uid string) bool {
	for _, e := range m.AgentMapping {
		if e.IssueUID == uid && e.PaneBlock != m.PaneBlock && agentWaiting(m.Agents.States, e.Block) {
			return true
		}
	}
	return false
}

func agentAttentionWritten(m Model, msg AgentAttentionWritten) Model {
	if msg.Err != nil {
		verb := "raise"
		if msg.Clear {
			verb = "clear"
		}
		m.Notice = fmt.Sprintf("Cannot %s the Attention request for an agent: %s", verb, msg.Err)
	}
	return m
}
