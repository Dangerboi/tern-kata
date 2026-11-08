package view

import (
	"strconv"

	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

type listColumns struct {
	ownerShown bool
	id, owner  int
}

// fit widens the columns to a row's id and owner widths, up to maxBoxWidth: a wider column would have no box rule, so its
// cells are cut to the cap instead (layoutRow and rightCells cut to the column's width).
func (c *listColumns) fit(id, owner int) {
	c.id = min(max(c.id, id), maxBoxWidth)
	c.owner = min(max(c.owner, owner), maxBoxWidth)
}

// role is the row's boxes in role tokens, which the style sheet turns into the columns' widths (see cellBoxCSS): the row
// kind, the id width and, when the owner shows, the owner width. Their widths are the Issues' own, so they cannot be static.
func (c listColumns) role() string {
	role := ListRowRole + " " + ListIDRole + strconv.Itoa(c.id)
	if c.ownerShown {
		role += " " + ListOwnerRole + strconv.Itoa(c.owner)
	}
	return role
}

type rowParts struct {
	id, prio, status ui.Span
	badges           []ui.Span // the Agent badge and the Work attention word, ahead of the title
	title            string
	date             ui.Span // the Date badge, after the title; empty when the Issue has none
	kids, owner      string
	updated          string
	cellStyle        string // the style of the kids and updated cells
	ownerStyle       string
}

type listRow struct {
	label, value []ui.Span
	dateFits     bool // whether the Date badge is among the label's spans
}

func layoutHeader(cols listColumns) listRow {
	return listRow{
		label: []ui.Span{
			{T: cell("#", cols.id), S: "muted mono"},
			{T: cellGap + cell("prio", prioWidth), S: "muted mono"},
			{T: cellGap + cell("status", statusWidth), S: "muted"},
			{T: cellGap + "title", S: "muted"},
		},
		value: rightCells("kids", "owner", "updated", cols, "muted mono", "muted"),
	}
}

// layoutRow measures a row once, by display width, against the list's width: the title has priority, so the Date badge, with
// its gap, is dropped when the whole row with it is wider than the list. The list's width less the row's padding and the
// one cell of the Detail between label and value is all the cells the row has.
func layoutRow(cols listColumns, width int, p rowParts) listRow {
	label := []ui.Span{
		{T: cell(p.id.T, cols.id), S: p.id.S},
		{T: cellGap + cell(p.prio.T, prioWidth), S: p.prio.S},
		{T: cellGap + cell(p.status.T, statusWidth), S: p.status.S},
	}
	// A badge or word takes the gap of cellGap and the title then one cell, so the title stays beside what it follows.
	gap := cellGap
	for _, badge := range p.badges {
		label = append(label, ui.Span{T: gap + badge.T, S: badge.S})
		gap = " "
	}
	label = append(label, ui.Span{T: gap + p.title})
	value := rightCells(p.kids, p.owner, p.updated, cols, p.cellStyle, p.ownerStyle)

	used := rowPadding + itemDetailWidth + cols.id + displayWidth(cellGap)*2 + prioWidth + statusWidth + kidsWidth + displayWidth(cellGap) + updatedWidth
	if cols.ownerShown {
		used += displayWidth(cellGap) + cols.owner // the owner box, however much of it the text fills
	}
	for _, span := range label[3:] {
		used += displayWidth(span.T)
	}
	laid := listRow{label: label, value: value}
	if p.date.T != "" {
		date := ui.Span{T: cellGap + p.date.T, S: p.date.S}
		if used+displayWidth(date.T) <= width {
			laid.label = append(laid.label, date)
			laid.dateFits = true
		}
	}
	return laid
}

func rightCells(kids, owner, updated string, cols listColumns, style, ownerStyle string) []ui.Span {
	spans := []ui.Span{{T: cell(kids, kidsWidth), S: style}}
	if cols.ownerShown {
		spans = append(spans, ui.Span{T: cellGap + cut(owner, cols.owner), S: ownerStyle})
	}
	return append(spans, ui.Span{T: cellGap + cell(updated, updatedWidth), S: style})
}
