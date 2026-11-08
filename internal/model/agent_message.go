package model

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
	"unicode"
)

// A person messages a running agent from the Attention Page: `m` on a request an agent raised opens the Message card, whose text
// is written as a kata Attention request to the agent's own Agent address on its Issue. tern-kata's omp extension in the agent
// delivers it; the agent clears it once handled, and the Pane clears what is left when the agent exits.

const (
	// ModeAgentMessage is the Message card while the message is typed or sent.
	ModeAgentMessage Mode = "agent-message"
	// ModeAgentMessageReplace is the Message card asking before it replaces a message the agent has not cleared.
	ModeAgentMessageReplace Mode = "agent-message-replace"

	// MessageAgent is `m` on the Attention Page: the Message card for the agent that raised the selected request.
	MessageAgent Action = "message-agent"
	// AskAgentAboutComment is `m` on the Comments tab: the Message card asking the Issue's agent to answer the selected comment.
	AskAgentAboutComment Action = "ask-agent-about-comment"
	AgentMessageSend     Action = "agent-message-send"
	AgentMessageCancel   Action = "agent-message-cancel"
	AgentMessageReplace  Action = "agent-message-replace"
	AgentMessageKeep     Action = "agent-message-keep"
)

// Why `m` writes to no agent: the selected request's agent cannot receive a message, or no agent raised the request.
const (
	NoAgentRaisedRefusal       = "No agent this host started raised this request."
	AgentExitedRefusal         = "The agent has exited; no message would reach it."
	AgentNotOmpRefusal         = "The agent is not omp; messages reach only omp agents, through tern-kata's extension."
	AgentWithoutAddressRefusal = "The agent was started before messages could reach it; start a new one."
	NoAgentOnIssueRefusal      = "No agent this host started is running on this Issue."
	AgentOtherActorRefusal     = "The agent takes messages only from the actor that started it and the actors in its message_from."
	AgentUnverifiedRefusal     = "A message from another actor or to an agent on another host needs kata's identity mode: set [auth] require_token_identity = true on the daemon."
)

// AgentMessageState is the Message card: the agent it writes to and the text typed. Held is the value kata holds at the agent's
// address that the agent has not cleared, with its message in Unread, while the card asks before replacing it.
type AgentMessageState struct {
	Open, Sending      bool
	IssueUID, IssueRef string
	ProjectID          int64
	// Agent is the agent's name, its agent.toml name; Address is its Agent address.
	Agent, Address string
	Text           string
	Caret          Caret
	Held, Unread   string
	// UnreadFrom is the actor that wrote the held message ("" when it does not read as one).
	UnreadFrom string
	Err        error
	// Re is the UID of the comment the message asks the agent to answer, "" for a plain message.
	Re string

	// Picking is the card as the picker of quick replies, until one is chosen.
	Picking bool

	// Broadcast is the card writing to the agents of IssueUID and its open children (see BroadcastAgents). Targets are the agents it
	// wrote to or skipped, kept once Send is pressed; Outcome is the card showing them after every write answered.
	Broadcast bool
	Targets   []BroadcastTarget
	Outcome   bool
	// Sweep is the broadcast the wrap-up sweep sends: every running agent of this host's Panes gets the wrap-up text, and the
	// outcome keeps showing which of them have finished (see sweepAgents).
	Sweep bool
	// SweepSince is when the sweep was sent: an agent's status comment counts only when made since.
	SweepSince time.Time
}

// SendAgentMessage writes Message to an agent's Address on an Issue as `kata notify` does, as Actor, with Teammate on the slot when
// it is not "" (an agent sending names itself, which the receiving extension words as from a peer). Replacing is the slot value the
// person agreed to replace; "" writes only while the address holds nothing, for kata keeps one request per Issue and address.
type SendAgentMessage struct {
	Workspace string
	ProjectID int64
	UID       string
	Actor     string
	Teammate  string
	Address   string
	Message   string
	Replacing string
	// Re is the UID of the comment the message asks the agent to answer ("" for none), as kata stores `kata notify --re`: kata settles
	// the request only when its answer targets exactly that UID.
	Re string
}

func (SendAgentMessage) cmd() {}

// AgentMessageSent answers SendAgentMessage for the Issue with UID. Held is the slot value found at the address in place of
// nothing or of the value replaced, with its message in Unread ("" when it does not read as a request); nothing was written then.
type AgentMessageSent struct {
	UID string
	// Address is the address the write was for, which tells a broadcast's writes on one Issue apart.
	Address string
	Held    string
	Unread  string
	// UnreadFrom is the actor that wrote the held message ("" when it does not read as a request).
	UnreadFrom string
	Err        error
}

func (AgentMessageSent) msg() {}

// ClearAgentAddress clears whatever is left at an exited agent's Address on its Issue, guarded on the value read, so a write that
// lands in between is not lost unseen, and removes the agent's Agent record (found by Block). An agent with no Address has only
// its record removed.
type ClearAgentAddress struct {
	Workspace string
	ProjectID int64
	UID       string
	Actor     string
	Block     string
	Address   string
}

func (ClearAgentAddress) cmd() {}

// AgentAddressCleared answers ClearAgentAddress for the Issue with UID.
type AgentAddressCleared struct {
	UID   string
	Block string
	Err   error
}

func (AgentAddressCleared) msg() {}

// messageTarget is the agent a message on request goes to, and why `m` is off ("" when it is on). The agent is one this host's
// Panes started on the request's Issue whose name the request carries, in a request this Pane's actor wrote, as an agent's
// Attention request is: the newest that waits, else the newest. For a request no such agent raised, it is the newest agent another
// host published on that Issue whose name the request carries. A request that reads as the one this Pane writes for an agent that
// exited, whose entry went with its block, is the exited agent's.
func (m Model) messageTarget(request AttentionRequest) (AgentEntry, string) {
	var target AgentEntry
	found := false
	ours := request.Slot.Kind == "" && !request.Slot.Broadcast && request.Slot.From == m.Actor
	if ours {
		target, found = m.newestAgentOn(request.IssueUID, func(e AgentEntry) bool { return e.Teammate == request.Slot.Teammate })
	}
	if !found && request.Slot.Kind == "" && !request.Slot.Broadcast && request.Slot.Teammate != "" {
		remote, ok := m.newestRemoteAgentOn(request.IssueUID, func(r RemoteAgent) bool { return r.Name == request.Slot.Teammate })
		if ok {
			return AgentEntry{Teammate: remote.Name, Address: remote.Address}, m.remoteMessageRefusal(remote)
		}
	}
	switch {
	case !found && ours && request.Slot.Message == agentExitedMessage(request.QualifiedID()):
		return AgentEntry{}, AgentExitedRefusal
	case !found:
		return AgentEntry{}, NoAgentRaisedRefusal
	}
	return target, m.agentMessageRefusal(target)
}

// newestAgentOn is the agent this host's Panes started on an Issue, among those match accepts, that this Pane's actor may message
// or whose address is not yet known: the newest that waits, else the newest.
func (m Model) newestAgentOn(issueUID string, match func(AgentEntry) bool) (AgentEntry, bool) {
	var target AgentEntry
	found, waiting := false, false
	for _, e := range m.AgentMapping {
		if e.IssueUID != issueUID || !match(e) {
			continue
		}
		if e.Address != "" && !MayMessage(m.Actor, AgentAddressActor(e.Address), e.MessageFromActors()) {
			continue
		}
		waits := agentWaiting(m.Agents.States, e.Block)
		if !found || waits && !waiting || waits == waiting && e.StartedAt.After(target.StartedAt) {
			target, found, waiting = e, true, waits
		}
	}
	return target, found
}

// agentMessageRefusal is why a message reaches no omp agent e, "" when it can: the agent exited, it is not omp, or it was started
// before it had an Agent address.
func (m Model) agentMessageRefusal(e AgentEntry) string {
	switch {
	case agentExited(m.Agents.States, e.Block):
		return AgentExitedRefusal
	case e.NoAddress == AgentNotOmp:
		return AgentNotOmpRefusal
	case e.Address == "":
		return AgentWithoutAddressRefusal
	}
	return MessageSenderRefusal(m.Actor, AgentAddressActor(e.Address), e.MessageFromActors(), false, m.VerifiedSenders)
}

// MayMessage is whether sender's actor is one the agent acts on: its own actor (owner, the actor in its Agent address) or one of
// its message_from.
func MayMessage(sender, owner string, messageFrom []string) bool {
	return sender == owner || slices.Contains(messageFrom, sender)
}

// MessageSenderRefusal is the sender rule, one for the Pane and `tern-kata message`: why a message from sender reaches the agent
// whose own actor is owner and whose message_from is messageFrom, "" when it does. The sender must be the owner or listed; and
// when it is not the owner, or the agent runs on another host (remote), the daemon must verify actors (verified), for without it
// the actor on a slot is whatever its writer claims.
func MessageSenderRefusal(sender, owner string, messageFrom []string, remote, verified bool) string {
	switch {
	case !MayMessage(sender, owner, messageFrom):
		return AgentOtherActorRefusal
	case (sender != owner || remote) && !verified:
		return AgentUnverifiedRefusal
	}
	return ""
}

// AgentAddressActor is the actor an Agent address names, whose messages its agent acts on: everything before its last "/", "" when it has none.
func AgentAddressActor(address string) string {
	i := strings.LastIndex(address, "/")
	if i < 0 {
		return ""
	}
	return address[:i]
}

// openAgentCardMessage is `m` (or `r`, when picking) on the Agent card: the Message card for the selected row's agent, wherever
// its block is, or the reason there is none in a Notice. The Agent card closes as it does when going to an agent.
func openAgentCardMessage(m Model, picking bool) (Model, []Cmd) {
	if reason := m.AgentCardUnavailable(AgentCardMessage); reason != "" {
		m.Notice = reason
		return m, nil
	}
	card := m.AgentCard
	if r, ok := m.AgentCardSelectedRemote(); ok {
		var projectID int64
		if issue := m.issueByUID(card.IssueUID); issue != nil {
			projectID = issue.ProjectID
		}
		m.AgentCard = AgentCardState{}
		m.AgentMessage = AgentMessageState{Open: true, Picking: picking, IssueUID: card.IssueUID, IssueRef: card.IssueRef, ProjectID: projectID, Agent: r.Name, Address: r.Address}
		return m, nil
	}
	e := m.AgentCardRows()[m.AgentCardCursor()].AgentEntry
	m.AgentCard = AgentCardState{}
	m.AgentMessage = AgentMessageState{Open: true, Picking: picking, IssueUID: card.IssueUID, IssueRef: card.IssueRef, ProjectID: m.projectOf(e), Agent: e.Teammate, Address: e.Address}
	return m, nil
}

// openAgentMessage is `m`: the Message card for the agent behind the selected request, or the reason there is none in a Notice.
func openAgentMessage(m Model) (Model, []Cmd) {
	return openAgentMessageFor(m, false)
}

// openAgentMessageFor opens the Message card for the agent behind the selected request, as the picker of quick replies when
// picking.
func openAgentMessageFor(m Model, picking bool) (Model, []Cmd) {
	request, ok := m.SelectedAttention()
	if !ok || m.attentionClearing(request.IssueUID) {
		return m, nil
	}
	target, reason := m.messageTarget(request)
	if reason != "" {
		m.Notice = reason
		return m, nil
	}
	m.AgentMessage = AgentMessageState{Open: true, Picking: picking, IssueUID: request.IssueUID, IssueRef: request.QualifiedID(), ProjectID: request.ProjectID,
		Agent: target.Teammate, Address: target.Address}
	return m, nil
}

// askAgentAboutComment is `m` on the Comments tab: the Message card for the Issue's agent, asking it to answer the comment under the
// cursor, or the reason there is none in a Notice. The agent is the one the Attention Page's `m` picks for a plain request: the
// newest that waits, else the newest.
func askAgentAboutComment(m Model) (Model, []Cmd) {
	row, ok := m.withRows().cursorRow()
	issue := m.shownIssue()
	if m.tab != TabComments || !ok || row.Kind != RowComment || issue == nil {
		return m, nil
	}
	target, found := m.newestAgentOn(issue.UID, func(e AgentEntry) bool { return m.agentMessageRefusal(e) == "" })
	if !found {
		target, found = m.newestAgentOn(issue.UID, func(AgentEntry) bool { return true })
	}
	reason := NoAgentOnIssueRefusal
	if found {
		reason = m.agentMessageRefusal(target)
	}
	if reason != "" {
		m.Notice = reason
		return m, nil
	}
	m.AgentMessage = AgentMessageState{Open: true, IssueUID: issue.UID, IssueRef: issue.QualifiedID, ProjectID: issue.ProjectID,
		Agent: target.Teammate, Address: target.Address, Re: row.Key}
	return m, nil
}

// AgentMessageMode is the keymap mode of the Message card: asking before a replace while kata holds an unread message, or picking
// a quick reply.
func (m Model) AgentMessageMode() Mode {
	switch {
	case m.AgentMessage.Outcome:
		return ModeAgentBroadcastOutcome
	case m.AgentMessage.Held != "":
		return ModeAgentMessageReplace
	case m.AgentMessage.Picking:
		return ModeAgentQuickReply
	}
	return ModeAgentMessage
}

func agentMessageKey(m Model, msg KeyMsg) (Model, []Cmd) {
	f := &m.AgentMessage
	if f.Sending {
		return m, nil
	}
	mode := m.AgentMessageMode()
	binding, bound := LookupBindingForMode(msg.Chord, mode)
	switch {
	case bound && binding.Action == AgentMessageSend && f.Broadcast:
		return sendAgentBroadcast(m)
	case bound && binding.Action == AgentMessageSend:
		return sendAgentMessage(m, "")
	case bound && binding.Action == AgentMessageReplace:
		return sendAgentMessage(m, f.Held)
	case bound && binding.Action == AgentMessageKeep:
		f.Held, f.Unread, f.UnreadFrom = "", "", ""
	case bound && binding.Action == AgentMessageCancel:
		m.AgentMessage = AgentMessageState{}
	case mode == ModeAgentQuickReply:
		if q, ok := quickReplyOf(binding.Action); ok && bound {
			f.Picking, f.Text = false, q.Text
			return sendAgentMessage(m, "")
		}
	case mode == ModeAgentMessage:
		var edited bool
		if f.Text, edited = editText(f.Text, &f.Caret, "", bound, msg); edited {
			f.Err = nil
		}
	}
	return m, nil
}

// messageText is the typed message, trimmed, or false with the reason on the card when it cannot be sent.
func (m *Model) messageText() (string, bool) {
	f := &m.AgentMessage
	text, err := CheckAgentMessage(f.Text)
	switch {
	case err != nil:
		f.Err = err
	case m.Stale:
		f.Err = errors.New("Daemon disconnected; reconnect before sending.")
	default:
		return text, true
	}
	return "", false
}

// sendAgentMessage writes the typed message, trimmed, over replacing (the held value the person agreed to replace, "" for none).
func sendAgentMessage(m Model, replacing string) (Model, []Cmd) {
	text, ok := m.messageText()
	if !ok {
		return m, nil
	}
	f := &m.AgentMessage
	f.Sending, f.Err = true, nil
	return m, []Cmd{SendAgentMessage{Workspace: m.Workspace, ProjectID: f.ProjectID, UID: f.IssueUID, Actor: m.Actor, Address: f.Address, Message: text, Replacing: replacing, Re: f.Re}}
}

// agentMessageSent applies kata's answer: a failure stays on the card, an unread message held at the address asks before replacing
// it, and a written message closes the card with a Notice.
func agentMessageSent(m Model, msg AgentMessageSent) (Model, []Cmd) {
	f := &m.AgentMessage
	if f.Open && f.Sending && f.Broadcast {
		m = broadcastSent(m, msg)
		return readSweepComments(m)
	}
	if !f.Open || !f.Sending || f.IssueUID != msg.UID {
		return m, nil
	}
	f.Sending = false
	switch {
	case msg.Err != nil:
		f.Err = msg.Err
	case msg.Held != "":
		f.Held, f.Unread, f.UnreadFrom = msg.Held, msg.Unread, msg.UnreadFrom
	default:
		m.Notice = fmt.Sprintf("Sent the message to %s on %s.", f.Agent, f.IssueRef)
		m.AgentMessage = AgentMessageState{}
	}
	return m, nil
}

// goneAgentClear is how far the clear for one exited agent has got.
type goneAgentClear int

const (
	clearAsked goneAgentClear = iota + 1
	clearDone
)

// clonedGoneAgentClears is a copy of the gone-agent notes, never nil, for a change that must not touch the Model it came from.
func (m Model) clonedGoneAgentClears() map[string]goneAgentClear {
	if clears := maps.Clone(m.goneAgentClears); clears != nil {
		return clears
	}
	return map[string]goneAgentClear{}
}

// sweepGoneAgents clears what is left at the address of each agent this Pane started that has exited or whose block went, and
// removes its Agent record (an agent with no address has one too), once per agent: nobody reads that address again. A clear that
// failed is asked again at the next sweep. The sweep keeps a note only of the agents still mapped or gone.
func sweepGoneAgents(m Model) (Model, []Cmd) {
	var cmds []Cmd
	clears := m.clonedGoneAgentClears()
	clearOnce := func(e AgentEntry) {
		project := m.projectOf(e)
		if _, noted := clears[e.Block]; project == 0 || noted {
			return
		}
		clears[e.Block] = clearAsked
		cmds = append(cmds, ClearAgentAddress{Workspace: m.Workspace, ProjectID: project, UID: e.IssueUID, Actor: m.Actor, Block: e.Block, Address: e.Address})
	}
	for _, e := range m.AgentGone {
		clearOnce(e)
	}
	for _, e := range m.AgentMapping {
		if e.PaneBlock == m.PaneBlock && agentExited(m.Agents.States, e.Block) {
			clearOnce(e)
		}
	}
	for block, state := range clears {
		if state == clearDone && m.addressForgotten(block) {
			delete(clears, block)
		}
	}
	m.goneAgentClears = clears
	return m, cmds
}

// addressForgotten says the agent with block is neither mapped nor gone, so its cleared address needs no note.
func (m Model) addressForgotten(block string) bool {
	return !m.agentMapped(block) && !slices.ContainsFunc(m.AgentGone, func(g AgentEntry) bool { return g.Block == block })
}

func agentAddressCleared(m Model, msg AgentAddressCleared) Model {
	clears := m.clonedGoneAgentClears()
	switch {
	case msg.Err != nil:
		delete(clears, msg.Block)
		m.Notice = fmt.Sprintf("Cannot clear the message left for an agent that exited: %s", msg.Err)
	case msg.Block != "":
		clears[msg.Block] = clearDone
	}
	m.goneAgentClears = clears
	return m
}

// PlainText is s with each control character (an escape, a carriage return) replaced by a space: text another actor wrote must not
// steer the terminal that shows it.
func PlainText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}
