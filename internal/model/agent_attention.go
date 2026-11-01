package model

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// AgentWaitingMessage is the request's text when the waiting agent said nothing.
const AgentWaitingMessage = "agent waiting for you"

// agentAttentionMaxBytes is the longest message kata notify takes.
const agentAttentionMaxBytes = 1024

// RaisedAttention is an Attention request this Pane wrote for a waiting agent and has not cleared.
type RaisedAttention struct {
	UID       string
	ProjectID int64
	// Message is the text this Pane last wrote.
	Message string
	// Seen are the messages of the agents on the Issue, this Pane's or another Pane's, seen waiting while this Pane held the
	// record, in the order seen. Another Pane's request may have replaced Message in the slot, and this Pane's clear takes any.
	Seen []string
}

// messages are the texts a request of this Pane's may read as now: what it wrote, then what other agents on the Issue raised.
func (r RaisedAttention) messages() []string {
	out := []string{r.Message}
	for _, s := range r.Seen {
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

// RaiseAgentAttention writes the Attention request for Address on an Issue, replacing any earlier one, as Actor.
type RaiseAgentAttention struct {
	Workspace string
	ProjectID int64
	UID       string
	Actor     string
	// Teammate is the waiting agent's name, written as the request's teammate; "" writes none.
	Teammate string
	Address  string
	Message  string
}

func (RaiseAgentAttention) cmd() {}

// ClearAgentAttention clears the request for Address on an Issue when it still reads as Actor wrote it with one of Messages; a
// request someone else wrote since, or one of Actor's with another message, is left alone.
type ClearAgentAttention struct {
	Workspace string
	ProjectID int64
	UID       string
	Actor     string
	Address   string
	Messages  []string
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
			return s.State == AgentWaitingInput
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

// raiseAgentAttention applies the relay just received (relayed) or the mapping just reloaded: an agent this Pane started that
// entered waiting in that relay raises the request with its message, as does one that waits while its Issue holds no request
// from this Pane (the Pane learned of it late); a request this Pane raised is cleared once no mapped agent on its Issue waits.
// An Issue gets at most one raise per pass, the last, so the record is what lands in kata. The record keeps the message of
// every agent on its Issue seen waiting, which the clear accepts. An Inbox address kata refuses writes nothing.
func raiseAgentAttention(m Model, relayed bool) (Model, []Cmd) {
	address, err := m.InboxAddress()
	if err != nil || m.PaneBlock == "" {
		return m, nil
	}
	var cmds []Cmd
	for _, e := range m.AgentMapping {
		if e.PaneBlock != m.PaneBlock || !agentWaiting(m.Agents.States, e.Block) {
			continue
		}
		if !(relayed && m.Agents.entered(e.Block, AgentWaitingInput)) && m.raised(e.IssueUID) {
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
		message := agentAttentionMessage(last)
		raised := RaisedAttention{UID: e.IssueUID, ProjectID: project, Message: message}
		if i := slices.IndexFunc(m.AgentAttention, func(r RaisedAttention) bool { return r.UID == e.IssueUID }); i >= 0 {
			raised.Seen = m.AgentAttention[i].Seen
			m.AgentAttention = slices.Clone(m.AgentAttention)
			m.AgentAttention[i] = raised
		} else {
			m.AgentAttention = append(slices.Clone(m.AgentAttention), raised)
		}
		raise := RaiseAgentAttention{Workspace: m.Workspace, ProjectID: project, UID: e.IssueUID, Actor: m.Actor, Teammate: e.Teammate, Address: address, Message: message}
		if i := slices.IndexFunc(cmds, func(c Cmd) bool { r, ok := c.(RaiseAgentAttention); return ok && r.UID == e.IssueUID }); i >= 0 {
			cmds[i] = raise
		} else {
			cmds = append(cmds, raise)
		}
	}
	kept := m.AgentAttention[:0:0]
	for _, r := range m.AgentAttention {
		if m.agentWaitingOn(r.UID) {
			r.Seen = m.seenWaiting(r)
			kept = append(kept, r)
			continue
		}
		cmds = append(cmds, m.clearOf(r, address))
	}
	m.AgentAttention = kept
	return m, cmds
}

// seenWaiting is the record's Seen with the message of every mapped agent on its Issue that waits now.
func (m Model) seenWaiting(r RaisedAttention) []string {
	seen := r.Seen
	for _, e := range m.AgentMapping {
		if e.IssueUID != r.UID || !agentWaiting(m.Agents.States, e.Block) {
			continue
		}
		for _, s := range m.Agents.States {
			if text := agentAttentionMessage(s.LastMessage); s.Block == e.Block && !slices.Contains(seen, text) {
				seen = append(slices.Clone(seen), text)
			}
		}
	}
	return seen
}

func (m Model) clearOf(r RaisedAttention, address string) ClearAgentAttention {
	return ClearAgentAttention{Workspace: m.Workspace, ProjectID: r.ProjectID, UID: r.UID, Actor: m.Actor, Address: address, Messages: r.messages()}
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
			cmds = append(cmds, m.clearOf(r, address))
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
