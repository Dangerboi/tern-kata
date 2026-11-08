package view

import (
	"strconv"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const FederationListID = "main.columns.list.federation"

func federation(m model.Model, now time.Time) tern.Element {
	f := m.Federation
	children := ui.Nodes(heading(ui.Span{T: "Federation", S: "strong"}))
	if f.Screen == model.FederationList {
		children = append(children, ui.Text{Common: ui.Common{Key: "summary", Tone: ui.ToneMuted}, Text: f.Summary(), Wrap: ui.WrapNone})
	}
	children = append(children, plainText("header", ui.ToneMuted, f.Header()))
	switch f.Screen {
	case model.FederationLeavePreview:
		lines, warning := f.Leave.PreviewLines()
		children = append(children, federationScreen("Leave Federation", lines, warning)...)
		if f.Leaving {
			children = append(children, plainText("running", ui.TonePending, "leaving..."))
		}
		if f.LeaveErr != nil {
			children = append(children, plainText("error", ui.ToneError, f.LeaveErr.Error()))
		}
		return listPanel(m, 0.5, children)
	case model.FederationLeaveDone:
		lines, warning := f.Left.ResultLines()
		children = append(children, federationScreen("Leave Result", lines, warning)...)
		return listPanel(m, 0.5, children)
	}
	if more, shown := federationEnrollScreen(m); shown {
		children = append(children, more...)
		return listPanel(m, 0.5, children)
	}
	spoke := m.SelectedSpoke()
	if f.Screen == model.FederationDetail && spoke != nil {
		for i, line := range spoke.DetailLines() {
			children = append(children, ui.Text{Common: ui.Common{Key: "detail" + strconv.Itoa(i)}, Text: line, Wrap: ui.WrapNone})
		}
		return listPanel(m, 0.5, children)
	}
	if notice, shown := f.Notice(); shown {
		return listPanel(m, 0.5, append(children, noticeLines(notice.Err, !f.Loading, notice.Text, "")...))
	}
	cards := make([]tern.Element, len(f.Spokes))
	for i, s := range f.Spokes {
		cards[i] = federationCard(s, i == f.Cursor, now)
	}
	children = append(children, ui.List{CommonWithoutMax: scrollerCommon("federation"), Max: fillMax, Children: cards})
	if spoke != nil {
		children = append(children, plainText("footer", ui.ToneMuted, spoke.Footer()))
	}
	return listPanel(m, 0.5, children)
}

func federationCard(s model.FederationSpoke, selected bool, now time.Time) tern.Element {
	lines := s.Lines(now)
	body := make([]tern.Element, len(lines))
	for i, line := range lines {
		body[i] = ui.Text{Common: ui.Common{Key: "line" + strconv.Itoa(i)}, Text: line, Wrap: ui.WrapNone}
	}
	tone := ui.ToneNeutral
	if s.Badges() != "-" {
		tone = ui.ToneWarning
	}
	return ui.Card{Common: ui.Common{Key: s.ProjectName, Tone: tone}, Selected: selected, Children: body}
}

func federationScreen(title string, lines []string, warning int) []tern.Element {
	out := ui.Nodes(ui.Text{Common: ui.Common{Key: "title", Tone: ui.ToneMuted}, Spans: []ui.Span{{T: title, S: "strong"}}, Wrap: ui.WrapNone})
	return append(out, federationLines(lines, warning)...)
}

func federationLines(lines []string, warning int) []tern.Element {
	var out []tern.Element
	for i, line := range lines {
		if i == warning {
			out = append(out, ui.Text{Common: ui.Common{Key: "line" + strconv.Itoa(i), Tone: ui.ToneError}, Text: line})
			continue
		}
		out = append(out, ui.Text{Common: ui.Common{Key: "line" + strconv.Itoa(i)}, Text: line, Wrap: ui.WrapNone})
	}
	return out
}
