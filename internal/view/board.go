package view

import (
	"strconv"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// BoardID is the Board's row of Lane columns; a Lane's column is its key under it, a card the Issue's key under the column's card list.
const BoardID = "main.columns.list.board"

// LaneHeaderID is the Lane view's header line over its list.
const LaneHeaderID = "main.columns.list.lane"

func laneKey(column int) string { return "lane" + strconv.Itoa(column) }

// CardID is the id of the Board card of uid, "" when the Board draws none.
func CardID(m model.Model, uid string) string {
	if col, _ := model.CardAt(m.BoardColumns(), uid); col >= 0 {
		return itemID(itemID(itemID(BoardID, laneKey(col)), "cards"), uid)
	}
	return ""
}

// SelectedRowID is the node of the selected Issue's row in the list, or its card on the Board.
func SelectedRowID(m model.Model) string {
	if m.Board.Mode == model.BoardColumns {
		return CardID(m, m.SelectedUID)
	}
	return IssueID(m.SelectedUID)
}

// board is the Board: one bordered column per Lane, No lane first and Done last, each a header over its cards. It takes the list column's place.
func board(m model.Model, now time.Time) tern.Element {
	columns := m.BoardColumns()
	lanes := make([]tern.Element, len(columns))
	for i, column := range columns {
		cards := make([]tern.Element, len(column.Cards))
		for j, card := range column.Cards {
			cards[j] = boardCard(m, card, now)
		}
		head := ui.Spans(ui.Span{T: column.Title, S: "strong"}, ui.Span{T: " " + strconv.Itoa(len(column.Cards)), S: "muted"})
		lanes[i] = ui.Card{
			Common:   ui.Common{Key: laneKey(i), Grow: 1, Basis: ui.BasisFrac(0), Min: &ui.Bounds{W: ui.Ch(0), H: ui.Lines(0)}, Max: &ui.Bounds{H: ui.Frac(1)}},
			Head:     head,
			Children: ui.Nodes(ui.List{CommonWithoutMax: scrollerCommon("cards"), Max: boardMax(m), Empty: ui.T("No cards"), Children: cards}),
		}
	}
	children := []tern.Element{}
	if m.Search.Open {
		children = append(children, searchInput(m))
	}
	children = append(children, ui.Row{Common: ui.Common{Key: "board", Grow: 1, Min: &ui.Bounds{H: ui.Lines(0)}}, Gap: ui.GapSM, Align: ui.AlignStretch, Children: lanes})
	return panel(m, model.ColumnList, ui.Common{Key: "list", Grow: 1}, "", children)
}

// boardCard is one Issue: its short id over its title clamped to two lines and its labels that are not Lane labels. The selected card
// is unclamped and adds the owner and every label.
func boardCard(m model.Model, card model.BoardCard, now time.Time) tern.Element {
	issue := card.Issue
	selected := issue.UID == m.SelectedUID
	tone := ui.ToneNeutral
	switch {
	case selected:
		tone = ui.ToneAccent
	case model.IsClosed(issue):
		tone = ui.ToneMuted
	}
	title := ui.Text{Common: ui.Common{Key: "title"}, Text: issue.Title, Lines: 2}
	labels := card.Labels
	if selected {
		title.Lines = 0
		labels = issue.Labels
	}
	body := ui.Nodes(title)
	if selected {
		owner := m.OwnerText(issue.Owner, issue.AssignmentExpiresOn, now).String()
		if owner == "" {
			owner = "unassigned"
		}
		body = append(body, ui.Text{Common: ui.Common{Key: "owner", Tone: ui.ToneMuted}, Text: owner})
	}
	if len(labels) > 0 {
		body = append(body, ui.Text{Common: ui.Common{Key: "labels", Tone: ui.ToneMuted}, Text: strings.Join(labels, ", ")})
	}
	if len(card.Also) > 0 {
		body = append(body, ui.Text{Common: ui.Common{Key: "also", Tone: ui.ToneWarning}, Text: "also: " + strings.Join(card.Also, ", ")})
	}
	if card.Reopen != "" {
		body = append(body, ui.Text{Common: ui.Common{Key: "reopen", Tone: ui.ToneMuted}, Text: "r → " + card.Reopen})
	}
	return ui.Card{
		Common:   ui.Common{Key: issue.UID, Tone: tone, Actions: &ui.Actions{Click: CardClick(issue.UID), DblClick: OpenRowClick(issue.UID)}},
		Head:     ui.Spans(ui.Span{T: issue.ShortID, S: "strong"}),
		Selected: selected,
		Children: body,
	}
}

// boardMax caps a Lane's card list so the Lane ends above the footer and the list scrolls inside it: the Pane has nothing to
// scroll, so the top bar stays. MaxFrac is a fraction of the window, so the room the chrome takes (top bar, Lane header, Search
// line, filter line, footer lines) is a share of an assumed 500px window, as in scrollMax.
// ponytail: below ~500px the Lane clips its lowest cards; above it the Lane leaves some of the window unused. Upgrade: a Tern prop for a card body that fills its card.
func boardMax(m model.Model) ui.ListMax {
	grid, _, _ := footerGrid(m)
	chrome := 120 + 16*len(grid)
	if m.FilterSummary() != "" {
		chrome += 16
	}
	if m.Search.Open {
		chrome += 40
	}
	return ui.MaxFrac(max(1-float64(chrome)/500, 0.3))
}
