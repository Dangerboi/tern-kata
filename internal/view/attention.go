package view

import (
	"strconv"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const AttentionListID = "main.columns.list.attention"

func attention(m model.Model, now time.Time) tern.Element {
	address, refusal := m.InboxAddress()
	if refusal != nil {
		children := ui.Nodes(pageHead("heading", "Attention"), toneText("refused", ui.ToneError, model.Cannot("read", "your inbox", refusal)))
		return listPanel(m, 1, children)
	}
	// The count waits for the first answer: a zero before it would read as an empty inbox.
	count, loading := "", ""
	if m.Attention.Loaded {
		count = strconv.Itoa(len(m.Attention.Requests))
	}
	if m.Attention.Loading {
		loading = "loading…"
	}
	children := ui.Nodes(pageHead("heading", "Attention", count, m.AttentionHeader(), loading))
	if m.Attention.Err != nil {
		children = append(children, toneText("error", ui.ToneError, model.Cannot("read", "the requests", m.Attention.Err)))
	}
	cards := make([]tern.Element, len(m.Attention.Requests))
	for i, request := range m.Attention.Requests {
		cards[i] = attentionCard(m, request, request.IssueUID == m.Attention.Selected, now)
	}
	empty := "Loading requests…"
	if m.Attention.Err != nil {
		empty = "The requests could not be read."
	}
	list := ui.List{CommonWithoutMax: scrollerCommon("attention"), Max: fillMax, Empty: ui.T(empty), Children: cards}
	children = append(children, listAndEmpty(list, !m.Attention.Loaded, emptyState("No requests", "Requests addressed to "+address+" appear here."))...)
	return listPanel(m, 1, children)
}

// attentionCard is one request: the Issue's title and ref on the left, who asked and how long ago on the right, then the
// message, whole on the selected card and its first line on the others. The age carries the absolute time in a hovercard.
func attentionCard(m model.Model, request model.AttentionRequest, selected bool, now time.Time) tern.Element {
	left := ui.Nodes(
		ui.Text{Common: ui.Common{Key: "title"}, Spans: []ui.Span{{T: request.Title, S: "strong"}}},
		ui.Text{Common: ui.Common{Key: "issue"}, Spans: []ui.Span{refSpan(request.QualifiedID(), true)}, Wrap: ui.WrapNone},
	)
	if tag := request.Tag(); tag != "" {
		left = append(left, ui.Badge{Common: ui.Common{Key: "tag", Tone: ui.ToneMuted}, Text: tag})
	}
	if request.Slot.Re != "" {
		left = append(left, ui.Text{Common: ui.Common{Key: "comment", Tone: ui.ToneMuted}, Text: "on a comment", Wrap: ui.WrapNone})
	}
	who := ui.Text{Common: ui.Common{Key: "who", Tone: ui.ToneMuted}, Text: request.Sender() + " · " + model.RelativeTime(request.Updated, now), Wrap: ui.WrapNone}
	if !request.Updated.IsZero() {
		who.Hovercard = ui.Hovercard{Element: ui.Text{Common: ui.Common{Key: "stamp"}, Spans: []ui.Span{timeSpan(m.Calendar.Stamp(request.Updated))}}}
	}
	head := ui.Row{Common: ui.Common{Key: "head"}, Gap: ui.GapMD, Justify: ui.JustifyBetween, Children: ui.Nodes(
		ui.Row{Common: ui.Common{Key: "subject"}, Gap: ui.GapMD, Wrap: true, Children: left}, who)}
	body := ui.Nodes(head)
	if selected {
		body = append(body, ui.Text{Common: ui.Common{Key: "message"}, Text: strings.TrimSpace(request.Slot.Message)})
	} else {
		body = append(body, ui.Text{Common: ui.Common{Key: "message"}, Spans: []ui.Span{{T: request.FirstLine(), S: "muted"}}, Wrap: ui.WrapNone})
	}
	return ui.Card{Common: ui.Common{Key: request.IssueUID, Actions: &ui.Actions{Click: AttentionClick(request.IssueUID), DblClick: AttentionOpenClick(request.IssueUID)}}, Selected: selected, Children: body}
}
