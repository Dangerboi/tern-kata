package view

import (
	"fmt"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

func agentMessageLayer(m model.Model) tern.Element {
	f := m.AgentMessage
	if !f.Open {
		return nil
	}
	if f.Sweep {
		return sweepLayer(m)
	}
	if m.AgentMessageMode() == model.ModeAgentQuickReply {
		return quickReplyLayer(m)
	}
	if f.Outcome {
		return broadcastOutcomeLayer(m)
	}
	ref := m.RefOf(f.IssueRef)
	head, note := "Message "+f.Agent, "Delivered at the agent's next step; it stays until the agent clears it."
	if f.Broadcast {
		head = "Message the agents"
		note = "Sent to every running agent on " + ref + " and its open children. Agents with an unread message are skipped."
	}
	var children []tern.Element
	var foot tern.Element
	if m.AgentMessageMode() == model.ModeAgentMessageReplace {
		children = []tern.Element{toneText("held", ui.ToneNeutral, heldText(f.Agent, f.UnreadFrom))}
		switch {
		case f.Unread != "":
			children = append(children, toneText("unread", ui.ToneMuted, f.Unread))
		case f.UnreadFrom == "":
			children = append(children, toneText("unread", ui.ToneMuted, "(not readable as a message)"))
		}
		foot = formFoot(cardButton("replace", model.ModeAgentMessageReplace, model.AgentMessageReplace, "Replace", true), exitButton("keep", model.ModeAgentMessageReplace, model.AgentMessageKeep, "Keep editing"))
	} else {
		children = []tern.Element{
			ui.Input{Common: ui.Common{Key: inputLeaf, Tone: ui.ToneAccent, Aria: "Message"}, Field: ui.Field{Text: f.Text, Cursor: caretAt(f.Caret, "", f.Text), Mode: "insert", Placeholder: ui.T("Write a message"), Readonly: f.Sending}},
			toneText("note", ui.ToneMuted, note),
		}
		if f.Re != "" {
			children = append([]tern.Element{toneText("re", ui.ToneMuted, "Asks the agent to answer comment "+model.CommentHandle(f.Re)+".")}, children...)
		}
		send := cardButton("send", model.ModeAgentMessage, model.AgentMessageSend, "Send", true)
		if f.Sending {
			send = unavailable(send, model.Progress("Sending", ""))
		}
		foot = formFoot(send, exitButton("cancel", model.ModeAgentMessage, model.AgentMessageCancel, "Cancel"))
	}
	if f.Err != nil {
		children = append(children, toneText("error", ui.ToneError, model.Cannot("send", "the message", f.Err)))
	}
	if f.Sending {
		children = append(children, toneText("sending", ui.TonePending, model.Progress("Sending", "")))
	}
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardAgentMessage)}, Head: ui.T(head), Meta: refMeta(ref), Icon: ui.IconSend, Modal: true, Size: ui.SizeMD}, append(children, foot)))
}

func broadcastOutcomeLayer(m model.Model) tern.Element {
	f := m.AgentMessage
	summary := "No agents got the message."
	if reached := f.BroadcastReached(); reached > 0 {
		summary = model.Count(reached, "agent", "agents") + " got the message."
	}
	children := []tern.Element{ui.Text{Common: ui.Common{Key: "reached"}, Text: summary}}
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardAgentMessage)}, Head: ui.T("Message the agents"), Meta: refMeta(m.RefOf(f.IssueRef)), Icon: ui.IconSend, Modal: true, Size: ui.SizeMD}, skippedAndClose(m, children, true)))
}

// quickReplyLayer lists the three canned replies, each a button that sends it, with its number as the keycap, and what it sends beside it.
func quickReplyLayer(m model.Model) tern.Element {
	f := m.AgentMessage
	children := make([]tern.Element, 0, len(model.QuickReplies)+1)
	for _, q := range model.QuickReplies {
		button := cardButton("reply-"+q.Key, model.ModeAgentQuickReply, q.Action, q.Name, false)
		children = append(children, ui.Row{Common: ui.Common{Key: "row-" + q.Key}, Gap: ui.GapMD, Align: ui.AlignCenter, Children: ui.Nodes(
			button, toneText("text-"+q.Key, ui.ToneMuted, q.Text))})
	}
	children = append(children, formFootExit(exitButton("cancel", model.ModeAgentQuickReply, model.AgentMessageCancel, "Cancel")))
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardAgentMessage)}, Head: ui.T("Quick reply to " + f.Agent), Meta: refMeta(m.RefOf(f.IssueRef)), Icon: ui.IconSend, Modal: true, Size: ui.SizeMD}, children))
}

// formFootExit is the foot of a card whose only button is its way out.
func formFootExit(exit ui.Button) tern.Element { return buttonRow(exit) }

// sweepLayer follows the relay, so it changes as agents go idle or exit.
func sweepLayer(m model.Model) tern.Element {
	f := m.AgentMessage
	var children []tern.Element
	if f.Sending {
		children = append(children, toneText("sending", ui.TonePending, model.Progress("Sending", "the wrap-up")))
	} else {
		reached := f.BroadcastReached()
		children = append(children, ui.Text{Common: ui.Common{Key: "reached"}, Text: fmt.Sprintf("%d of %d %s finished the wrap-up.", m.SweepFinished(), reached, model.Noun(reached, "agent", "agents"))})
	}
	for i, t := range f.Targets {
		if t.Skipped != "" || !t.Answered {
			continue
		}
		standing, last := m.SweepStatus(t)
		parts := []string{t.Agent, m.RefOf(t.IssueRef)}
		if t.Remote() {
			parts = append(parts, t.Host)
		}
		parts = append(parts, standing.Text)
		if last != "" {
			parts = append(parts, last)
		}
		children = append(children, toneText(fmt.Sprintf("agent-%d", i), ui.ToneMuted, strings.Join(parts, " · ")))
	}
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardAgentMessage)}, Head: ui.T("Wrap-up sweep"), Icon: ui.IconSend, Modal: true, Size: ui.SizeMD}, skippedAndClose(m, children, !f.Sending)))
}

func skippedAndClose(m model.Model, children []tern.Element, withClose bool) []tern.Element {
	for i, t := range m.AgentMessage.Targets {
		if t.Skipped != "" {
			children = append(children, toneText(fmt.Sprintf("skipped-%d", i), ui.ToneMuted, fmt.Sprintf("Skipped %s on %s: %s", t.Agent, m.RefOf(t.IssueRef), t.Skipped)))
		}
	}
	if withClose {
		children = append(children, formFootExit(exitButton("close", model.ModeAgentBroadcastOutcome, model.AgentMessageCancel, "Close")))
	}
	return children
}

// heldText asks whether to replace the message the agent has not cleared.
func heldText(agent, from string) string {
	if from == "" {
		return agent + " has an unread message. Replace it?"
	}
	return agent + " has an unread message from " + from + ". Replace it?"
}
