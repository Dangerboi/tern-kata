package view

import (
	"fmt"

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
	head, placeholder, note := "Message "+f.Agent, "Message to "+f.Agent,
		"The agent gets it as a message from you at its next step, after you answer anything it asks you, and it stays on "+f.IssueRef+" until the agent clears it."
	if f.Broadcast {
		head, placeholder = "Message the agents", "Message to the agents"
		note = "Every running agent on " + f.IssueRef + " and its open children gets it as a message from you at its next step. An agent that has not cleared an earlier message is skipped, never replaced."
	}
	var children []tern.Element
	var buttons []tern.Element
	if m.AgentMessageMode() == model.ModeAgentMessageReplace {
		unread := f.Unread
		if unread == "" {
			unread = "(a value that does not read as a message)"
		}
		children = []tern.Element{
			ui.Text{Common: ui.Common{Key: "held"}, Text: heldText(f.UnreadFrom)},
			toneText("unread", ui.ToneMuted, unread),
		}
		buttons = []tern.Element{cardButton("replace", model.ModeAgentMessageReplace, model.AgentMessageReplace, "Replace it", true), cardButton("keep", model.ModeAgentMessageReplace, model.AgentMessageKeep, "Keep editing", false)}
	} else {
		children = []tern.Element{
			ui.Input{Common: ui.Common{Key: inputLeaf, Tone: ui.ToneAccent, Aria: "Message"}, Field: ui.Field{Text: f.Text, Cursor: caretAt(f.Caret, "", f.Text), Mode: "insert", Placeholder: ui.T(placeholder), Readonly: f.Sending}},
			toneText("note", ui.ToneMuted, note),
		}
		if f.Re != "" {
			children = append([]tern.Element{toneText("re", ui.ToneMuted, "Asks the agent to answer comment "+model.CommentHandle(f.Re)+".")}, children...)
		}
		buttons = []tern.Element{cardButton("send", model.ModeAgentMessage, model.AgentMessageSend, "Send", true), cardButton("cancel", model.ModeAgentMessage, model.AgentMessageCancel, "Cancel", false)}
	}
	if f.Err != nil {
		children = append(children, toneText("error", ui.ToneError, f.Err.Error()))
	}
	if f.Sending {
		children = append(children, toneText("sending", ui.TonePending, "Sending…"))
	} else {
		children = append(children, buttonRow(buttons...))
	}
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardAgentMessage)}, Head: ui.T(head), Meta: refMeta(f.IssueRef), Icon: ui.IconSend, Modal: true, Size: ui.SizeMD}, children))
}

func broadcastOutcomeLayer(m model.Model) tern.Element {
	f := m.AgentMessage
	reached := f.BroadcastReached()
	summary := "No agent got the message."
	switch {
	case reached == 1:
		summary = "1 agent got the message."
	case reached > 1:
		summary = fmt.Sprintf("%d agents got the message.", reached)
	}
	children := []tern.Element{ui.Text{Common: ui.Common{Key: "reached"}, Text: summary}}
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardAgentMessage)}, Head: ui.T("Message the agents"), Meta: refMeta(f.IssueRef), Icon: ui.IconSend, Modal: true, Size: ui.SizeMD}, skippedAndClose(m, children, true)))
}

func quickReplyLayer(m model.Model) tern.Element {
	f := m.AgentMessage
	children := make([]tern.Element, 0, len(model.QuickReplies)+1)
	buttons := make([]tern.Element, 0, len(model.QuickReplies)+1)
	for _, q := range model.QuickReplies {
		children = append(children, toneText("reply-"+q.Key, ui.ToneMuted, q.Key+"  "+q.Text))
		buttons = append(buttons, cardButton("reply-"+q.Key, model.ModeAgentQuickReply, q.Action, q.Name, q.Key == "1"))
	}
	buttons = append(buttons, cardButton("cancel", model.ModeAgentQuickReply, model.AgentMessageCancel, "Cancel", false))
	children = append(children, buttonRow(buttons...))
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardAgentMessage)}, Head: ui.T("Quick reply to " + f.Agent), Meta: refMeta(f.IssueRef), Icon: ui.IconSend, Modal: true, Size: ui.SizeMD}, children))
}

// sweepLayer follows the relay, so it changes as agents go idle or exit.
func sweepLayer(m model.Model) tern.Element {
	f := m.AgentMessage
	var children []tern.Element
	if f.Sending {
		children = append(children, toneText("sending", ui.TonePending, "Sending the wrap-up…"))
	} else {
		children = append(children, ui.Text{Common: ui.Common{Key: "reached"}, Text: fmt.Sprintf("%d of %d agents have finished the wrap-up.", m.SweepFinished(), f.BroadcastReached())})
	}
	for i, t := range f.Targets {
		if t.Skipped != "" || !t.Answered {
			continue
		}
		standing, last := m.SweepStatus(t)
		text := fmt.Sprintf("%s on %s: %s", t.Agent, t.IssueRef, standing.Text)
		if t.Remote() { // standing.Text already reads "on <host>: ..."
			text = fmt.Sprintf("%s on %s, %s", t.Agent, t.IssueRef, standing.Text)
		}
		if last != "" {
			text += " (" + last + ")"
		}
		children = append(children, toneText(fmt.Sprintf("agent-%d", i), ui.ToneMuted, text))
	}
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardAgentMessage)}, Head: ui.T("Wrap-up sweep"), Icon: ui.IconSend, Modal: true, Size: ui.SizeMD}, skippedAndClose(m, children, !f.Sending)))
}

func skippedAndClose(m model.Model, children []tern.Element, withClose bool) []tern.Element {
	for i, t := range m.AgentMessage.Targets {
		if t.Skipped != "" {
			children = append(children, toneText(fmt.Sprintf("skipped-%d", i), ui.ToneMuted, fmt.Sprintf("Skipped %s on %s: %s", t.Agent, t.IssueRef, t.Skipped)))
		}
	}
	if withClose {
		children = append(children, buttonRow(cardButton("close", model.ModeAgentBroadcastOutcome, model.AgentMessageCancel, "Close", true)))
	}
	return children
}

func heldText(from string) string {
	by := ""
	if from != "" {
		by = ", written by " + from + ","
	}
	return "The agent has not cleared its last message" + by + " and kata keeps one per Issue. Replace it?"
}
