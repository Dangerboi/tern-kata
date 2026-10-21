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

// laneTones are the tones of the Board's columns, one per column: the default Lanes each have theirs, No lane is muted and Done
// success, and custom Lanes take the rest in turn.
func laneTones(columns []model.BoardColumn) []ui.Tone {
	named := map[string]ui.Tone{
		model.LaneNone:    ui.ToneMuted,
		"needs-triage":    ui.ToneWarning,
		"needs-info":      ui.ToneError,
		"ready-for-agent": ui.ToneInfo,
		"ready-for-human": ui.ToneUser,
		model.LaneDone:    ui.ToneSuccess,
	}
	rest := []ui.Tone{ui.ToneAccent, ui.TonePending, ui.ToneNeutral}
	tones := make([]ui.Tone, len(columns))
	custom := 0
	for i, column := range columns {
		if tone, ok := named[column.Lane]; ok {
			tones[i] = tone
			continue
		}
		tones[i] = rest[custom%len(rest)]
		custom++
	}
	return tones
}

// board is the Board: one column per Lane, No lane first and Done last, each a header over its cards. It takes the list column's
// place. A Lane is a column, not a Card: a Card's body sizes to its content, so only a column's list fills the Lane down to the
// bottom and scrolls inside it, which keeps the Pane itself from scrolling the top bar away.
func board(m model.Model, now time.Time) tern.Element {
	columns := m.BoardColumns()
	tones := laneTones(columns)
	lanes := make([]tern.Element, len(columns))
	for i, column := range columns {
		cards := make([]tern.Element, len(column.Cards))
		for j, card := range column.Cards {
			cards[j] = boardCard(m, card, tones[i], column.Lane == model.LaneDone, now)
		}
		count := strconv.Itoa(len(column.Cards))
		if column.Total > len(column.Cards) {
			count += " of " + strconv.Itoa(column.Total)
		}
		head := ui.Row{Common: ui.Common{Key: "head"}, Gap: ui.GapSM, Children: ui.Nodes(
			ui.Badge{Common: ui.Common{Key: "title", Tone: tones[i]}, Text: column.Title},
			ui.Text{Common: ui.Common{Key: "count"}, Spans: []ui.Span{{T: count, S: "muted"}}, Wrap: ui.WrapNone},
		)}
		lanes[i] = ui.Col{
			Common: ui.Common{Key: laneKey(i), Grow: 1, Basis: ui.BasisFrac(0), Min: &ui.Bounds{W: ui.Ch(0), H: ui.Lines(0)}, Max: &ui.Bounds{H: ui.Frac(1)}},
			Gap:    ui.GapSM,
			Children: ui.Nodes(
				head,
				ui.Rule{Common: ui.Common{Key: "rule", Tone: tones[i]}},
				ui.List{CommonWithoutMax: scrollerCommon("cards"), Max: laneMax(m), Empty: ui.T("No cards"), Children: cards},
			),
		}
	}
	children := []tern.Element{}
	if m.Search.Open {
		children = append(children, searchInput(m))
	}
	children = append(children, ui.Row{Common: ui.Common{Key: "board", Grow: 1, Basis: ui.BasisFrac(0), Min: &ui.Bounds{H: ui.Lines(0)}}, Gap: ui.GapMD, Align: ui.AlignStretch, Children: lanes})
	return panel(m, model.ColumnList, ui.Common{Key: "list", Grow: 1}, "", children)
}

// boardCard is one Issue: its short id over its title clamped to two lines and its labels that are not Lane labels. The selected card
// is unclamped and adds the owner and every label. The id is a badge in its Lane's tone; the title is the normal text colour, a Done
// card's muted. Labels are info badges, the owner a user badge, "also" warning and the reopen hint muted.
func boardCard(m model.Model, card model.BoardCard, tone ui.Tone, done bool, now time.Time) tern.Element {
	issue := card.Issue
	selected := issue.UID == m.SelectedUID
	titleStyle := ""
	if done {
		titleStyle = "muted"
	}
	title := ui.Text{Common: ui.Common{Key: "title"}, Spans: []ui.Span{{T: issue.Title, S: titleStyle}}, Lines: 2}
	labels := card.Labels
	if selected {
		title.Lines = 0
		labels = issue.Labels
	}
	body := ui.Nodes(
		ui.Row{Common: ui.Common{Key: "id"}, Gap: ui.GapSM, Wrap: true, Children: ui.Nodes(
			ui.Badge{Common: ui.Common{Key: "short", Tone: tone}, Text: issue.ShortID},
		)},
		title,
	)
	if selected {
		owner := m.OwnerText(issue.Owner, issue.AssignmentExpiresOn, now).String()
		if owner == "" {
			owner = "unassigned"
		}
		body = append(body, ui.Badge{Common: ui.Common{Key: "owner", Tone: ui.ToneUser}, Text: owner})
	}
	if len(labels) > 0 {
		chips := make([]tern.Element, len(labels))
		for i, label := range labels {
			chips[i] = ui.Badge{Common: ui.Common{Key: label, Tone: ui.ToneInfo}, Text: label}
		}
		body = append(body, ui.Row{Common: ui.Common{Key: "labels"}, Gap: ui.GapSM, Wrap: true, Children: chips})
	}
	if len(card.Also) > 0 {
		body = append(body, ui.Text{Common: ui.Common{Key: "also"}, Spans: []ui.Span{{T: "also: " + strings.Join(card.Also, ", "), S: "warning"}}})
	}
	if card.Reopen != "" {
		body = append(body, ui.Text{Common: ui.Common{Key: "reopen"}, Spans: []ui.Span{{T: "r → " + card.Reopen, S: "muted"}}})
	}
	cardTone := ui.ToneNeutral
	if done {
		cardTone = ui.ToneMuted
	}
	return ui.Card{
		Common:   ui.Common{Key: issue.UID, Tone: cardTone, Actions: &ui.Actions{Click: CardClick(issue.UID), DblClick: OpenRowClick(issue.UID)}},
		Selected: selected,
		Children: body,
	}
}

// laneMax caps a Lane's card list at the rows left in the Pane under the top bar and Lane header and above the footer, so the list
// fills the Lane and scrolls inside it and the Pane has nothing to scroll: the top bar stays. Tern has no fill for a list, only a
// height cap in list rows, each a text row and 6px tall; so the cap is the Pane's pixel height less its chrome in rows, rounded
// down, and the list ends up to one list row above the footer.
// ponytail: the chrome is measured at Tern's default density (the Pane's 6px padding, a text row for the top bar and for each gap, 46px of Lane header, a 24px footer margin, 24px for the Search field); a denser theme leaves more room unused, a looser one clips the lowest card. Upgrade: a Tern prop for a list that fills its column.
// Until Tern says the Pane's size there is nothing to measure and the list takes half the window.
func laneMax(m model.Model) ui.ListMax {
	if m.Rows == 0 || m.RowPx == 0 {
		return ui.MaxFrac(0.5)
	}
	grid, _, _ := footerGrid(m)
	chrome := 5*m.RowPx + 24 + m.RowPx*len(grid)
	if m.FilterSummary() != "" {
		chrome += 2 * m.RowPx
	}
	if m.Search.Open {
		chrome += 24
	}
	return ui.MaxLines(max((m.Rows*m.RowPx-chrome)/(m.RowPx+6), 1))
}
