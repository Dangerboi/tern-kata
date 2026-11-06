package model

import (
	"errors"
	"fmt"
	"slices"
	"strings"
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
	MessageAgent        Action = "message-agent"
	AgentMessageSend    Action = "agent-message-send"
	AgentMessageCancel  Action = "agent-message-cancel"
	AgentMessageReplace Action = "agent-message-replace"
	AgentMessageKeep    Action = "agent-message-keep"
)

// Why `m` writes to no agent: the selected request's agent cannot receive a message, or no agent raised the request.
const (
	NoAgentRaisedRefusal       = "No agent this host started raised this request."
	AgentExitedRefusal         = "The agent has exited; no message would reach it."
	AgentNotOmpRefusal         = "The agent is not omp; messages reach only omp agents, through tern-kata's extension."
	AgentWithoutAddressRefusal = "The agent was started before messages could reach it; start a new one."
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
	Err            error

	// Picking is the card as the picker of quick replies, until one is chosen.
	Picking bool
}

// SendAgentMessage writes Message to an agent's Address on an Issue as `kata notify` does, as Actor. Replacing is the slot value the
// person agreed to replace; "" writes only while the address holds nothing, for kata keeps one request per Issue and address.
type SendAgentMessage struct {
	Workspace string
	ProjectID int64
	UID       string
	Actor     string
	Address   string
	Message   string
	Replacing string
}

func (SendAgentMessage) cmd() {}

// AgentMessageSent answers SendAgentMessage for the Issue with UID. Held is the slot value found at the address in place of
// nothing or of the value replaced, with its message in Unread ("" when it does not read as a request); nothing was written then.
type AgentMessageSent struct {
	UID    string
	Held   string
	Unread string
	Err    error
}

func (AgentMessageSent) msg() {}

// ClearAgentAddress clears whatever is left at an exited agent's Address on its Issue, guarded on the value read, so a write that
// lands in between is not lost unseen.
type ClearAgentAddress struct {
	Workspace string
	ProjectID int64
	UID       string
	Actor     string
	Address   string
}

func (ClearAgentAddress) cmd() {}

// AgentAddressCleared answers ClearAgentAddress for the Issue with UID.
type AgentAddressCleared struct {
	UID string
	Err error
}

func (AgentAddressCleared) msg() {}

// messageTarget is the agent a message on request goes to, and why `m` is off ("" when it is on). The agent is one this host's
// Panes started on the request's Issue whose name the request carries, in a request this Pane's actor wrote, as an agent's
// Attention request is: the newest that waits, else the newest. A request that reads as the one this Pane writes for an agent that
// exited, whose entry went with its block, is the exited agent's.
func (m Model) messageTarget(request AttentionRequest) (AgentEntry, string) {
	var target AgentEntry
	found, waiting := false, false
	ours := request.Slot.Kind == "" && !request.Slot.Broadcast && request.Slot.From == m.Actor
	if ours {
		for _, e := range m.AgentMapping {
			if e.IssueUID != request.IssueUID || e.Teammate != request.Slot.Teammate {
				continue
			}
			waits := agentWaiting(m.Agents.States, e.Block)
			if !found || waits && !waiting || waits == waiting && e.StartedAt.After(target.StartedAt) {
				target, found, waiting = e, true, waits
			}
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
	return ""
}

// openAgentCardMessage is `m` (or `r`, when picking) on the Agent card: the Message card for the selected row's agent, wherever
// its block is, or the reason there is none in a Notice. The Agent card closes as it does when going to an agent.
func openAgentCardMessage(m Model, picking bool) (Model, []Cmd) {
	if reason := m.AgentCardUnavailable(AgentCardMessage); reason != "" {
		m.Notice = reason
		return m, nil
	}
	e := m.AgentCardRows()[m.AgentCardCursor()].AgentEntry
	card := m.AgentCard
	projectID := e.ProjectID
	if issue := m.issueByUID(card.IssueUID); issue != nil && projectID == 0 {
		projectID = issue.ProjectID
	}
	m.AgentCard = AgentCardState{}
	m.AgentMessage = AgentMessageState{Open: true, Picking: picking, IssueUID: card.IssueUID, IssueRef: card.IssueRef, ProjectID: projectID, Agent: e.Teammate, Address: e.Address}
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

// AgentMessageMode is the keymap mode of the Message card: asking before a replace while kata holds an unread message, or picking
// a quick reply.
func (m Model) AgentMessageMode() Mode {
	switch {
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
	case bound && binding.Action == AgentMessageSend:
		return sendAgentMessage(m, "")
	case bound && binding.Action == AgentMessageReplace:
		return sendAgentMessage(m, f.Held)
	case bound && binding.Action == AgentMessageKeep:
		f.Held, f.Unread = "", ""
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

// sendAgentMessage writes the typed message, trimmed, over replacing (the held value the person agreed to replace, "" for none).
func sendAgentMessage(m Model, replacing string) (Model, []Cmd) {
	f := &m.AgentMessage
	text := strings.TrimSpace(f.Text)
	switch {
	case text == "":
		f.Err = errors.New("Type a message.")
		return m, nil
	case len(text) > agentAttentionMaxBytes:
		f.Err = fmt.Errorf("The message is %d bytes; kata takes at most %d.", len(text), agentAttentionMaxBytes)
		return m, nil
	case m.Stale:
		f.Err = errors.New("Daemon disconnected; reconnect before sending.")
		return m, nil
	}
	f.Sending, f.Err = true, nil
	return m, []Cmd{SendAgentMessage{Workspace: m.Workspace, ProjectID: f.ProjectID, UID: f.IssueUID, Actor: m.Actor, Address: f.Address, Message: text, Replacing: replacing}}
}

// agentMessageSent applies kata's answer: a failure stays on the card, an unread message held at the address asks before replacing
// it, and a written message closes the card with a Notice.
func agentMessageSent(m Model, msg AgentMessageSent) Model {
	f := &m.AgentMessage
	if !f.Open || !f.Sending || f.IssueUID != msg.UID {
		return m
	}
	f.Sending = false
	switch {
	case msg.Err != nil:
		f.Err = msg.Err
	case msg.Held != "":
		f.Held, f.Unread = msg.Held, msg.Unread
	default:
		m.Notice = fmt.Sprintf("Sent the message to %s on %s.", f.Agent, f.IssueRef)
		m.AgentMessage = AgentMessageState{}
	}
	return m
}

// clearGoneAddresses clears what is left at the address of each agent this Pane started that has exited or whose block went, once
// per agent: nobody reads that address again. The record of the agents cleared keeps only those still mapped or gone.
func clearGoneAddresses(m Model) (Model, []Cmd) {
	var cmds []Cmd
	clearOnce := func(e AgentEntry) {
		project := m.projectOf(e)
		if e.Address == "" || project == 0 || slices.Contains(m.addressesCleared, e.Block) {
			return
		}
		m.addressesCleared = append(slices.Clip(m.addressesCleared), e.Block)
		cmds = append(cmds, ClearAgentAddress{Workspace: m.Workspace, ProjectID: project, UID: e.IssueUID, Actor: m.Actor, Address: e.Address})
	}
	for _, e := range m.AgentGone {
		clearOnce(e)
	}
	for _, e := range m.AgentMapping {
		if e.PaneBlock == m.PaneBlock && agentExited(m.Agents.States, e.Block) {
			clearOnce(e)
		}
	}
	if forgotten := m.addressForgotten; slices.ContainsFunc(m.addressesCleared, forgotten) {
		m.addressesCleared = slices.DeleteFunc(slices.Clone(m.addressesCleared), forgotten)
	}
	return m, cmds
}

// addressForgotten says the agent with block is neither mapped nor gone, so its cleared address needs no record.
func (m Model) addressForgotten(block string) bool {
	return !m.agentMapped(block) && !slices.ContainsFunc(m.AgentGone, func(g AgentEntry) bool { return g.Block == block })
}

func agentAddressCleared(m Model, msg AgentAddressCleared) Model {
	if msg.Err != nil {
		m.Notice = fmt.Sprintf("Cannot clear the message left for an agent that exited: %s", msg.Err)
	}
	return m
}
