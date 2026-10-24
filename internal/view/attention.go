package view

import (
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// AttentionListID is the Attention Page's list of request cards; a card is the request's Issue uid under it.
const AttentionListID = "main.columns.list.attention"

// AttentionID is the id of the card of the request on the Issue with uid.
func AttentionID(uid string) string { return itemID(AttentionListID, uid) }

// attention is the Attention Page's one full-width list: a heading, then one card per request, newest first.
func attention(m model.Model) tern.Element {
	address, refusal := m.InboxAddress()
	heading := []ui.Span{{T: "Attention", S: "strong"}}
	if refusal == nil {
		heading = append(heading, ui.Span{T: " " + m.AttentionHeader(), S: "muted"})
	}
	if m.Attention.Loading {
		heading = append(heading, loadingMarker)
	}
	children := ui.Nodes(ui.Text{Common: ui.Common{Key: "heading"}, Spans: heading, Wrap: ui.WrapNone})
	if refusal != nil {
		children = append(children, ui.Text{Common: ui.Common{Key: "refused", Tone: ui.ToneError}, Text: "The Inbox address is refused: " + refusal.Error()})
		return panel(m, model.ColumnList, ui.Common{Key: "list", Grow: 1, Basis: ui.BasisFrac(1)}, ui.GapSM, children)
	}
	if m.Attention.Err != nil {
		children = append(children, ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: m.Attention.Err.Error()})
	}
	cards := make([]tern.Element, len(m.Attention.Requests))
	for i, request := range m.Attention.Requests {
		cards[i] = attentionCard(request, request.IssueUID == m.Attention.Selected)
	}
	empty := "No requests for " + address
	switch {
	case !m.Attention.Loaded && m.Attention.Err != nil:
		empty = "The requests could not be read."
	case !m.Attention.Loaded:
		empty = "Loading requests…"
	}
	children = append(children, ui.List{CommonWithoutMax: scrollerCommon("attention"), Max: scrollMax(m, m.RowPx+6+8), Empty: ui.T(empty), Children: cards})
	return panel(m, model.ColumnList, ui.Common{Key: "list", Grow: 1, Basis: ui.BasisFrac(1)}, ui.GapSM, children)
}

// attentionCard is one request: the Issue and who asked, then the message, its first line, or all of it wrapped on the selected card.
func attentionCard(request model.AttentionRequest, selected bool) tern.Element {
	head := ui.Row{Common: ui.Common{Key: "head"}, Gap: ui.GapSM, Wrap: true, Children: ui.Nodes(
		ui.Text{Common: ui.Common{Key: "issue"}, Spans: []ui.Span{{T: request.QualifiedID(), S: toneStyle(ui.ToneInfo)}}, Wrap: ui.WrapNone},
		ui.Text{Common: ui.Common{Key: "title"}, Spans: []ui.Span{{T: request.Title, S: "strong"}}},
	)}
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
	return ui.Card{Common: ui.Common{Key: request.IssueUID, Actions: &ui.Actions{Click: AttentionClick(request.IssueUID)}}, Selected: selected, Children: body}
}
