package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// agentMessageLayer is the Message card: the message to the agent, or, while kata holds a message the agent has not cleared, that
// message and whether to replace it. kata's refusal of the last send and the sending marker follow either.
func agentMessageLayer(m model.Model) tern.Element {
	f := m.AgentMessage
	if !f.Open {
		return nil
	}
	if m.AgentMessageMode() == model.ModeAgentQuickReply {
		return quickReplyLayer(m)
	}
	var children []tern.Element
	var buttons []tern.Element
	if m.AgentMessageMode() == model.ModeAgentMessageReplace {
		unread := f.Unread
		if unread == "" {
			unread = "(a value that does not read as a message)"
		}
		children = []tern.Element{
			ui.Text{Common: ui.Common{Key: "held"}, Text: "The agent has not cleared its last message, and kata keeps one per Issue. Replace it?"},
			toneText("unread", ui.ToneMuted, unread),
		}
		buttons = []tern.Element{cardButton("replace", model.ModeAgentMessageReplace, model.AgentMessageReplace, "Replace it", true), cardButton("keep", model.ModeAgentMessageReplace, model.AgentMessageKeep, "Keep editing", false)}
	} else {
		children = []tern.Element{
			ui.Input{Common: ui.Common{Key: inputLeaf, Tone: ui.ToneAccent, Aria: "Message"}, Field: ui.Field{Text: f.Text, Cursor: caretAt(f.Caret, "", f.Text), Mode: "insert", Placeholder: ui.T("Message to " + f.Agent), Readonly: f.Sending}},
			toneText("note", ui.ToneMuted, "The agent gets it as a message from you at its next step, after you answer anything it asks you, and it stays on "+f.IssueRef+" until the agent clears it."),
		}
		if f.Re != "" {
			children = append([]tern.Element{toneText("re", ui.ToneMuted, "Asks the agent to answer comment "+f.Re+".")}, children...)
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
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardAgentMessage)}, Head: ui.T("Message " + f.Agent), Meta: refMeta(f.IssueRef), Icon: ui.IconSend, Modal: true, Size: ui.SizeMD}, children))
}

// quickReplyLayer is the picker of the three quick replies: each reply's text, a button to send it and a Cancel.
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
