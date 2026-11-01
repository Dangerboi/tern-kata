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
	// Exited says the request is the one for an agent that exited leaving its Issue open. It lives with the Issue, not with the
	// agent's entry in the mapping: pruning the entry leaves it, and only the Issue closing, another agent on the Issue waiting
	// (its request replaces this one) or the Pane quitting clears it.
	Exited bool
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
	return attentionPass(m, relayed, false)
}

// attentionOnIssues is the pass for the Issues changing: the status of an Issue the Collection does not list, last read for an
// agent that exited, is read again, so closing the Issue clears the request.
func attentionOnIssues(m Model) (Model, []Cmd) {
	return attentionPass(m, false, true)
}

func attentionPass(m Model, relayed, reread bool) (Model, []Cmd) {
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
		m, cmds = putRaise(m, cmds, e.IssueUID, project, e.Teammate, address, agentAttentionMessage(last), false)
	}
	m, cmds = raiseExitedAttention(m, cmds, address, reread)
	m, cmds = readExitedIssues(m, cmds, reread)
	kept := m.AgentAttention[:0:0]
	for _, r := range m.AgentAttention {
		waiting := m.agentWaitingOn(r.UID)
		if waiting || (r.Exited && m.issueStatus(r.UID) != "closed") {
			if waiting {
				r.Exited = false
			}
			r.Seen = m.seenWaiting(r)
			kept = append(kept, r)
			continue
		}
		cmds = append(cmds, m.clearOf(r, address))
	}
	m.AgentAttention = kept
	return m, cmds
}

// putRaise records the request this Pane writes for an Issue, replacing its record, and queues the write; an Issue gets at most one
// write per pass, the last.
func putRaise(m Model, cmds []Cmd, uid string, project int64, teammate, address, message string, exited bool) (Model, []Cmd) {
	raised := RaisedAttention{UID: uid, ProjectID: project, Message: message, Exited: exited}
	if i := slices.IndexFunc(m.AgentAttention, func(r RaisedAttention) bool { return r.UID == uid }); i >= 0 {
		raised.Seen = m.AgentAttention[i].Seen
		m.AgentAttention = slices.Clone(m.AgentAttention)
		m.AgentAttention[i] = raised
	} else {
		m.AgentAttention = append(slices.Clone(m.AgentAttention), raised)
	}
	raise := RaiseAgentAttention{Workspace: m.Workspace, ProjectID: project, UID: uid, Actor: m.Actor, Teammate: teammate, Address: address, Message: message}
	if i := slices.IndexFunc(cmds, func(c Cmd) bool { r, ok := c.(RaiseAgentAttention); return ok && r.UID == uid }); i >= 0 {
		cmds[i] = raise
	} else {
		cmds = append(cmds, raise)
	}
	return m, cmds
}

// agentExitedMessage is the request's text for an agent that exited while its Issue is open.
func agentExitedMessage(ref string) string { return "agent exited without closing " + ref }

// issueStatus is the status of an Issue by UID as the Collection lists it, else as the last read through the Daemon gave it; ""
// when neither is known.
func (m Model) issueStatus(uid string) string {
	if issue := m.issueByUID(uid); issue != nil {
		return issue.Status
	}
	return m.AgentClose.known[uid]
}

// agentExited reports whether the relay shows the agent block exited; an idle agent is between turns.
func agentExited(states []AgentState, block string) bool {
	return slices.ContainsFunc(states, func(s AgentState) bool { return s.Block == block && s.State == AgentExited })
}

// raiseExitedAttention raises the request for an agent this Pane started that exited while its Issue is open: one the relay shows
// exited, or one whose block is gone (AgentGone), which counts as exited though no relay said so. It raises nothing when an agent
// on the Issue waits (its own request stands) or the record already holds that text, so a second window's relay of the same state
// writes nothing. An Issue the Collection does not list is read through the Daemon first, as for ending a finished Background
// agent, and again when the Issues change (reread); the answer runs the pass again.
func raiseExitedAttention(m Model, cmds []Cmd, address string, reread bool) (Model, []Cmd) {
	exited := slices.Clone(m.AgentGone)
	for _, e := range m.AgentMapping {
		if e.PaneBlock == m.PaneBlock && agentExited(m.Agents.States, e.Block) {
			exited = append(exited, e)
		}
	}
	for _, e := range exited {
		if m.agentWaitingOn(e.IssueUID) {
			m.AgentGone = m.withoutGone(e)
			continue
		}
		project := m.projectOf(e)
		if project == 0 {
			m.AgentGone = m.withoutGone(e)
			continue
		}
		status := m.issueStatus(e.IssueUID)
		if status == "" && slices.Contains(m.AgentClose.unreadable, e.IssueUID) {
			m.AgentGone = m.withoutGone(e)
			continue
		}
		if status == "" || (reread && m.issueByUID(e.IssueUID) == nil) {
			m, cmds = readIssueStatus(m, cmds, project, e.IssueUID)
			continue
		}
		m.AgentGone = m.withoutGone(e)
		message := agentExitedMessage(e.IssueRef)
		if status == "closed" || slices.ContainsFunc(m.AgentAttention, func(r RaisedAttention) bool { return r.UID == e.IssueUID && r.Message == message }) {
			continue
		}
		m, cmds = putRaise(m, cmds, e.IssueUID, project, e.Teammate, address, message, true)
	}
	return m, cmds
}

func (m Model) withoutGone(e AgentEntry) []AgentEntry {
	if !slices.ContainsFunc(m.AgentGone, func(g AgentEntry) bool { return g.Block == e.Block }) {
		return m.AgentGone
	}
	return slices.DeleteFunc(slices.Clone(m.AgentGone), func(g AgentEntry) bool { return g.Block == e.Block })
}

// readIssueStatus asks the Daemon for the status of an Issue the Collection does not list, unless a read of it is in flight.
func readIssueStatus(m Model, cmds []Cmd, project int64, uid string) (Model, []Cmd) {
	if m.issueByUID(uid) == nil && !slices.Contains(m.AgentClose.reading, uid) && !slices.Contains(m.AgentClose.unreadable, uid) {
		m.AgentClose.reading = append(slices.Clone(m.AgentClose.reading), uid)
		cmds = append(cmds, ReadAgentIssue{Workspace: m.Workspace, ProjectID: project, UID: uid})
	}
	return m, cmds
}

// readExitedIssues reads again the status of an Issue holding an exit request the Collection does not list (while its status is
// not known, and when the Issues change), so closing the Issue clears the request though no agent entry is left to ask about it.
func readExitedIssues(m Model, cmds []Cmd, reread bool) (Model, []Cmd) {
	for _, r := range m.AgentAttention {
		if r.Exited && m.issueByUID(r.UID) == nil && (reread || m.issueStatus(r.UID) == "") {
			m, cmds = readIssueStatus(m, cmds, r.ProjectID, r.UID)
		}
	}
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
