package view

import (
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const AttentionListID = "main.columns.list.attention"

func attention(m model.Model) tern.Element {
	address, refusal := m.InboxAddress()
	spans := []ui.Span{{T: "Attention", S: "strong"}}
	if refusal == nil {
		spans = append(spans, ui.Span{T: " " + m.AttentionHeader(), S: "muted"})
	}
	if m.Attention.Loading {
		spans = append(spans, loadingMarker)
	}
	children := ui.Nodes(heading(spans...))
	if refusal != nil {
		children = append(children, plainText("refused", ui.ToneError, "The Inbox address is refused: "+refusal.Error()))
		return listPanel(m, 1, children)
	}
	if m.Attention.Err != nil {
		children = append(children, plainText("error", ui.ToneError, m.Attention.Err.Error()))
	}
	cards := make([]tern.Element, len(m.Attention.Requests))
	for i, request := range m.Attention.Requests {
		cards[i] = attentionCard(request, request.IssueUID == m.Attention.Selected)
	}
	empty := "Loading requests…"
	if m.Attention.Err != nil {
		empty = "The requests could not be read."
	}
	list := ui.List{CommonWithoutMax: scrollerCommon("attention"), Max: fillMax, Empty: ui.T(empty), Children: cards}
	children = append(children, listAndEmpty(list, !m.Attention.Loaded, emptyState("No requests", "Nothing is waiting on "+address+"; a request shows here when someone asks it."))...)
	return listPanel(m, 1, children)
}

func attentionCard(request model.AttentionRequest, selected bool) tern.Element {
	head := headRow(
		ui.Text{Common: ui.Common{Key: "issue"}, Spans: []ui.Span{{T: request.QualifiedID(), S: toneStyle(ui.ToneInfo)}}, Wrap: ui.WrapNone},
		ui.Text{Common: ui.Common{Key: "title"}, Spans: []ui.Span{{T: request.Title, S: "strong"}}},
	)
	who := []string{request.Sender(), request.Tag()}
	if handle := request.CommentHandle(); handle != "" {
		who = append(who, handle)
	}
	tone := ui.ToneWarning
	if request.Slot.Kind != "" || request.Slot.Broadcast {
		tone = ui.ToneInfo
	}
	body := ui.Nodes(head, ui.Text{Common: ui.Common{Key: "who", Tone: tone}, Text: strings.Join(who, " · "), Wrap: ui.WrapNone})
	if selected {
		body = append(body, ui.Text{Common: ui.Common{Key: "message"}, Text: strings.TrimSpace(request.Slot.Message)})
	} else {
		body = append(body, ui.Text{Common: ui.Common{Key: "message"}, Spans: []ui.Span{{T: request.FirstLine(), S: "muted"}}, Wrap: ui.WrapNone})
	}
	return ui.Card{Common: ui.Common{Key: request.IssueUID, Actions: &ui.Actions{Click: AttentionClick(request.IssueUID), DblClick: AttentionOpenClick(request.IssueUID)}}, Selected: selected, Children: body}
}
