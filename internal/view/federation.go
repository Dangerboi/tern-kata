package view

import (
	"strconv"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const FederationListID = "main.columns.list.federation"

// federation is the Federation Page: the spokes of the active daemon as cards, a spoke's detail, and the enroll, leave and result
// screens. Every screen opens with the Page head, which names the active daemon and how it is authenticated.
func federation(m model.Model, now time.Time) tern.Element {
	f := m.Federation
	children := ui.Nodes(pageHead("heading", "Federation", f.Facts()...))
	switch f.Screen {
	case model.FederationLeavePreview:
		children = append(children, federationScreen("Leave federation", f.Leave.PreviewRows())...)
		if f.Leaving {
			children = append(children, plainText("running", ui.TonePending, model.Progress("Leaving", "")))
		}
		if failure := f.LeaveFailure(); failure != "" {
			children = append(children, toneText("error", ui.ToneError, failure))
		}
		return listPanel(m, 0.4, children)
	case model.FederationLeaveDone:
		children = append(children, federationScreen("Leave result", f.Left.ResultRows())...)
		return listPanel(m, 0.4, children)
	}
	if more, shown := federationEnrollScreen(m); shown {
		children = append(children, more...)
		return listPanel(m, 0.4, children)
	}
	spoke := m.SelectedSpoke()
	if f.Screen == model.FederationDetail && spoke != nil {
		children = append(children, factRows("", spoke.DetailRows(m.Calendar, now))...)
		return listPanel(m, 0.4, children)
	}
	if notice, shown := f.Notice(); shown {
		if notice.Empty {
			empty := emptyState(notice.Text, "Enroll a Project to sync it with a hub.", emptyButton(m, "enroll", model.StartFederationEnroll, "Enroll a Project", true)...)
			empty.Key = "empty"
			return listPanel(m, 0.4, append(children, empty))
		}
		return listPanel(m, 0.4, append(children, noticeLines(notice.Err, !notice.Pending, notice.Text, "")...))
	}
	cards := make([]tern.Element, len(f.Spokes))
	for i, s := range f.Spokes {
		cards[i] = federationCard(s, i == f.Cursor, now)
	}
	children = append(children, ui.List{CommonWithoutMax: scrollerCommon("federation"), Max: fillMax, Children: cards})
	if spoke != nil {
		children = append(children, ui.Text{Common: ui.Common{Key: "footer", Tone: ui.ToneMuted}, Spans: []ui.Span{
			monoSpan(spoke.HubURLFact(), "muted"), {T: " · " + spoke.CredentialFact(), S: "muted"}}, Wrap: ui.WrapNone})
	}
	return listPanel(m, 0.4, children)
}

// federationCard is a spoke in the list: its name, spoke name, hub host and actor on one line, the sync facts muted under it, and in
// the warning tone the conditions that need attention.
func federationCard(s model.FederationSpoke, selected bool, now time.Time) tern.Element {
	head := ui.Nodes(ui.Text{Common: ui.Common{Key: "name"}, Spans: []ui.Span{{T: s.ProjectName, S: "strong"}}, Wrap: ui.WrapNone})
	if s.SpokeName != "" {
		head = append(head, ui.Text{Common: ui.Common{Key: "spoke", Tone: ui.ToneMuted}, Text: s.SpokeName, Wrap: ui.WrapNone})
	}
	head = append(head,
		ui.Text{Common: ui.Common{Key: "hub"}, Spans: []ui.Span{monoSpan(s.HubHost(), "muted")}, Wrap: ui.WrapNone},
		ui.Text{Common: ui.Common{Key: "actor", Tone: ui.ToneMuted}, Text: "Actor " + dashActor(s), Wrap: ui.WrapNone})
	body := ui.Nodes(
		ui.Row{Common: ui.Common{Key: "head"}, Gap: ui.GapMD, Align: ui.AlignCenter, Children: head},
		ui.Text{Common: ui.Common{Key: "facts", Tone: ui.ToneMuted}, Text: s.CardFacts(now), Wrap: ui.WrapNone},
	)
	tone := ui.ToneNeutral
	if warnings := s.Warnings(); len(warnings) > 0 {
		tone = ui.ToneWarning
		body = append(body, toneText("warnings", ui.ToneWarning, capitalize(strings.Join(warnings, ", "))+"."))
	}
	return ui.Card{Common: ui.Common{Key: s.ProjectName, Tone: tone}, Selected: selected, Children: body}
}

// dashActor is the actor a spoke is bound as, or the one "no value" mark.
func dashActor(s model.FederationSpoke) string {
	if s.BoundActor == "" {
		return model.None
	}
	return s.BoundActor
}

// capitalize is text with its first letter in upper case.
func capitalize(text string) string {
	if text == "" {
		return text
	}
	return strings.ToUpper(text[:1]) + text[1:]
}

// federationScreen is a screen of the Page that is not the list: its title in strong text, then its rows.
func federationScreen(title string, rows []model.FactRow) []tern.Element {
	out := ui.Nodes(ui.Text{Common: ui.Common{Key: "title"}, Spans: []ui.Span{{T: title, S: "strong"}}, Wrap: ui.WrapNone})
	return append(out, factRows("", rows)...)
}

// factRows draws rows as property grids and sentences: a row with a key is a label→value row of the open grid, a row with only a
// value is a sentence (in the tone it names), and an empty row ends the open grid, so each group of rows aligns on its own.
func factRows(prefix string, rows []model.FactRow) []tern.Element {
	var out, grid []tern.Element
	groups := 0
	flush := func() {
		if len(grid) == 0 {
			return
		}
		out = append(out, keyedGrid(prefix+"rows"+strconv.Itoa(groups), grid...))
		groups++
		grid = nil
	}
	for i, row := range rows {
		switch {
		case row.Key != "":
			grid = append(grid, factPropRow(row))
		case row.Value == "":
			flush()
		default:
			flush()
			out = append(out, factSentence(prefix+"note"+strconv.Itoa(i), row))
		}
	}
	flush()
	return out
}

// factPropRows draws the keyed rows among rows as property rows for a grid the caller builds.
func factPropRows(rows []model.FactRow) []tern.Element {
	var out []tern.Element
	for _, row := range rows {
		if row.Key != "" {
			out = append(out, factPropRow(row))
		}
	}
	return out
}

func factPropRow(row model.FactRow) tern.Element {
	span := ui.Span{T: row.Value}
	if row.Mono {
		span = monoSpan(row.Value, "")
	}
	return propRow(row.Key, ui.Text{Common: ui.Common{Key: "v"}, Spans: []ui.Span{span}})
}

func factSentence(key string, row model.FactRow) tern.Element {
	switch row.Tone {
	case model.FactWarning:
		return toneText(key, ui.ToneWarning, row.Value)
	case model.FactError:
		return toneText(key, ui.ToneError, row.Value)
	}
	return ui.Text{Common: ui.Common{Key: key}, Text: row.Value}
}

// keyedGrid is a property grid under its own key, for a screen that draws more than one.
func keyedGrid(key string, rows ...tern.Element) tern.Element {
	grid := propGrid(rows...).(ui.Col)
	grid.Key = key
	return grid
}
