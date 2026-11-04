package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// agentMessageLayer is the Message card: the message to the agent, or, while kata holds a message the agent has not cleared, that
// message and whether to replace it.
func agentMessageLayer(m model.Model) tern.Element {
	f := m.AgentMessage
	if !f.Open {
		return nil
	}
	var children []tern.Element
	if m.AgentMessageMode() == model.ModeAgentMessageReplace {
		children = agentMessageReplacing(f)
	} else {
		children = agentMessageTyping(f)
	}
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardAgentMessage)}, Head: ui.T("Message " + f.Agent), Meta: refMeta(f.IssueRef), Icon: ui.IconSend, Modal: true, Size: ui.SizeMD}, children))
}

// agentMessageTyping is the card while the message is typed: the input, where it goes, and kata's refusal of the last send.
func agentMessageTyping(f model.AgentMessageState) []tern.Element {
	children := []tern.Element{
		ui.Input{Common: ui.Common{Key: inputLeaf, Tone: ui.ToneAccent, Aria: "Message"}, Field: ui.Field{Text: f.Text, Cursor: caretAt(f.Caret, "", f.Text), Mode: "insert", Placeholder: ui.T("Message to " + f.Agent), Readonly: f.Sending}},
		toneText("note", ui.ToneMuted, "It reaches the agent at once as a message from you, and stays on "+f.IssueRef+" until the agent clears it."),
	}
	if f.Err != nil {
		children = append(children, toneText("error", ui.ToneError, f.Err.Error()))
	}
	if f.Sending {
		return append(children, toneText("sending", ui.TonePending, "Sending…"))
	}
	return append(children, buttonRow(cardButton("send", model.ModeAgentMessage, model.AgentMessageSend, "Send", true), cardButton("cancel", model.ModeAgentMessage, model.AgentMessageCancel, "Cancel", false)))
}

// agentMessageReplacing is the card asking before it replaces the message kata holds for the agent: kata keeps one per Issue and address.
func agentMessageReplacing(f model.AgentMessageState) []tern.Element {
	unread := f.Unread
	if unread == "" {
		unread = "(a value that does not read as a message)"
	}
	children := []tern.Element{
		ui.Text{Common: ui.Common{Key: "held"}, Text: "The agent has not cleared its last message, and kata keeps one per Issue. Replace it?"},
		toneText("unread", ui.ToneMuted, unread),
	}
	if f.Err != nil {
		children = append(children, toneText("error", ui.ToneError, f.Err.Error()))
	}
	if f.Sending {
		return append(children, toneText("sending", ui.TonePending, "Sending…"))
	}
	return append(children, buttonRow(cardButton("replace", model.ModeAgentMessageReplace, model.AgentMessageReplace, "Replace it", true), cardButton("keep", model.ModeAgentMessageReplace, model.AgentMessageKeep, "Keep editing", false)))
}
