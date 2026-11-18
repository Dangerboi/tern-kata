package view

import (
	"strconv"
	"strings"

	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// The roles that tell the style sheet which optional columns of an Issue list row are not drawn, so their boxes take no room, and
// how wide the Date column is. A hidden column's spans stay in the row, one-space, so every other cell keeps its child index.
const (
	ListNoPrioRole   = "tern-kata.nopr"
	ListNoStatusRole = "tern-kata.nost"
	ListNoKidsRole   = "tern-kata.nokd"
	ListDateRole     = "tern-kata.dt"
)

// maxDateWidth caps the Date column: a badge such as "scheduled 2026-12-01" is the longest the Board's words give.
const maxDateWidth = 20

// listColumns are the columns an Issue list shows. Priority, Status, Children and Owner are drawn only while some visible row has
// a value; date is the width of the Date column, 0 while no visible row has a Date badge.
type listColumns struct {
	prioShown, statusShown, kidsShown, ownerShown bool
	id, owner, date                               int
}

// fit widens the columns to a row's id, owner and Date badge widths, up to maxBoxWidth: a wider column would have no box rule,
// so its cells are cut to the cap instead (layoutRow and rightCells cut to the column's width). The Date column stops at
// maxDateWidth.
func (c *listColumns) fit(id, owner, date int) {
	c.id = min(max(c.id, id), maxBoxWidth)
	c.owner = min(max(c.owner, owner), maxBoxWidth)
	c.date = min(max(c.date, date), maxDateWidth)
}

// role is the row's boxes in role tokens, which the style sheet turns into the columns' widths (see cellBoxCSS): the row
// kind, the id width, the Date width when the column shows, the owner width when it shows, and a token for each optional
// column that does not. Their widths are the Issues' own, so they cannot be static.
func (c listColumns) role() string {
	role := ListRowRole + " " + ListIDRole + strconv.Itoa(c.id)
	if c.date > 0 {
		role += " " + ListDateRole + strconv.Itoa(c.date)
	}
	if c.ownerShown {
		role += " " + ListOwnerRole + strconv.Itoa(c.owner)
	}
	for _, hidden := range []struct {
		shown bool
		role  string
	}{{c.prioShown, ListNoPrioRole}, {c.statusShown, ListNoStatusRole}, {c.kidsShown, ListNoKidsRole}} {
		if !hidden.shown {
			role += " " + hidden.role
		}
	}
	return role
}

type rowParts struct {
	id, prio, status ui.Span
	// flags follow the title: the agentFlags Agent badge spans (the badge, then "message unread" when one waits), then the Work attention word.
	flags      []ui.Span
	agentFlags int
	title      string
	date       ui.Span // the Date badge; empty when the Issue has none
	kids       string
	owner      string
	updated    string
	cellStyle  string // the style of the kids and updated cells
	// ownerStyle is the owner's style: plain text.
	ownerStyle string
}

// plainStyle is the span style of text in the default colour that still makes an element. Tern draws an unstyled span as a bare
// text node, which is no child element, so the style sheet's nth-child boxes (cellBoxCSS) would count the cells after it one
// place off; the "default" token draws no element either, and "dim" dims the text. "em" draws a span in the sans face with no
// colour of its own, so it takes the colour of its container: PlainCellCSS gives the right-hand cells the default text colour.
const plainStyle = "em"

// cellStyleOrDefault is style, or plainStyle when it is empty.
func cellStyleOrDefault(style string) string {
	if style == "" {
		return plainStyle
	}
	return style
}

// hiddenCell is what a hidden column's span holds: Tern drops an empty span, which would shift the child index of every cell
// after it, so the span holds a zero-width space instead, which is a real glyph, takes no room and is styled (see cellStyleOrDefault).
const hiddenCell = "\u200b"

type listRow struct {
	label, value []ui.Span
}

// headerStyle is the style of a header cell over a mono cell: muted, and in the mono face. A box is sized in em, and a span's em
// is its face's, so a sans label would start its column at another x than the mono cells under it.
const headerStyle = "muted mono"

func layoutHeader(cols listColumns) listRow {
	label := []ui.Span{{T: "ID", S: headerStyle}, {T: hiddenCell, S: headerStyle}, {T: hiddenCell, S: "muted"}, {T: cellGap + "Title", S: "muted"}}
	if cols.prioShown {
		label[1].T = cellGap + "Priority"
	}
	if cols.statusShown {
		label[2].T = cellGap + "Status"
	}
	// The header's cells are not padded: its sans spaces are wider than the mono cells the style sheet's boxes are sized in, so
	// padding would push a header cell past its box and shift every cell after it. Each box gives the cell its width.
	// The status, date and owner cells are sans in the rows, so their header cells are sans too.
	value := []ui.Span{{T: hiddenCell, S: "muted"}, {T: hiddenCell, S: headerStyle}}
	if cols.kidsShown {
		value[1].T = "Children"
	}
	if cols.date > 0 {
		value[0].T = cellGap
	}
	if cols.ownerShown {
		value = append(value, ui.Span{T: cellGap + "Owner", S: "muted"})
	}
	value = append(value, ui.Span{T: cellGap + "Updated", S: headerStyle})
	return listRow{label: label, value: value}
}

// cellText is the text of an optional column's cell: its gap and text padded to width, or nothing while the column is hidden.
func cellText(shown bool, text string, width int) string {
	if !shown {
		return hiddenCell
	}
	return cellGap + cell(text, width)
}

// layoutRow measures a row once, by display width, against the list's width. The title has priority over the Agent badge, which
// goes when the whole row is wider than the list; the Work attention word is never dropped, so a title that still does not fit
// is cut instead. The list's width less the row's padding and the one cell of the Detail between label and value is all the
// cells the row has.
func layoutRow(cols listColumns, width int, p rowParts) listRow {
	label := []ui.Span{
		{T: cell(p.id.T, cols.id), S: p.id.S},
		{T: cellText(cols.prioShown, p.prio.T, prioWidth), S: p.prio.S},
		{T: cellText(cols.statusShown, p.status.T, statusWidth), S: p.status.S},
	}
	value := rightCells(cols, p.date.T, p.kids, p.date.S, p.owner, p.updated, p.cellStyle, p.ownerStyle)

	room := width - rowPadding - itemDetailWidth - cols.id
	if cols.prioShown {
		room -= displayWidth(cellGap) + prioWidth
	}
	if cols.statusShown {
		room -= displayWidth(cellGap) + statusWidth
	}
	for i, span := range value {
		if cols.ownerShown && i == len(value)-2 {
			room -= displayWidth(cellGap) + cols.owner // the owner box, however much of it the text fills
			continue
		}
		room -= displayWidth(span.T)
	}
	flags := p.flags
	flagsWidth := func() int {
		total := 0
		for _, flag := range flags {
			total += displayWidth(cellGap + flag.T)
		}
		return total
	}
	title := p.title
	room -= displayWidth(cellGap)
	if p.agentFlags > 0 && len(flags) >= p.agentFlags && displayWidth(title)+flagsWidth() > room {
		flags = flags[p.agentFlags:]
	}
	// With no flag after it the title is left whole: Tern ellipsizes a label that overflows, up to the first shown right-hand cell, so
	// a cut made here from the estimate above would only lose room that is there.
	if over := displayWidth(title) + flagsWidth() - room; over > 0 && len(flags) > 0 {
		title = cut(title, max(displayWidth(title)-over, 1))
	}
	label = append(label, ui.Span{T: cellGap + title})
	for _, flag := range flags {
		label = append(label, ui.Span{T: cellGap + flag.T, S: flag.S})
	}
	return listRow{label: label, value: value}
}

// rightCells are the cells at a row's right end: the Date badge (empty while the column is hidden), Children, the owner when it
// shows, and Updated, right-aligned under its header. Hidden columns keep a one-space span so the others keep their child index.
func rightCells(cols listColumns, date, kids, dateStyle, owner, updated, style, ownerStyle string) []ui.Span {
	dateText := hiddenCell
	if cols.date > 0 {
		dateText = cellGap + cell(date, cols.date)
	}
	kidsText := hiddenCell
	if cols.kidsShown {
		kidsText = cell(kids, kidsWidth)
	}
	dateStyle, ownerStyle = cellStyleOrDefault(dateStyle), cellStyleOrDefault(ownerStyle)
	spans := []ui.Span{{T: dateText, S: dateStyle}, {T: kidsText, S: style}}
	if cols.ownerShown {
		if strings.TrimSpace(owner) == "" {
			owner, ownerStyle = "—", "muted" // no value, as in every other table
		}
		spans = append(spans, ui.Span{T: cellGap + cut(owner, cols.owner), S: ownerStyle})
	}
	return append(spans, ui.Span{T: cellGap + alignRight(updated, updatedWidth), S: style})
}

// alignRight is text cut to width and padded on the left.
func alignRight(text string, width int) string {
	text = cut(text, width)
	return strings.Repeat(" ", max(width-displayWidth(text), 0)) + text
}

type twoLineParts struct {
	head, meta, title []ui.Span
}

// layoutTwoLine splits a row for a Pane under model.TwoLineRowCols: line one holds the id, a two-cell priority slot and the
// status word when the Issue is not open, with `date · children · owner` at its end (the updated cell is dropped); line two the
// title and its flags.
func layoutTwoLine(p rowParts) twoLineParts {
	head := []ui.Span{{T: p.id.T, S: p.id.S}, {T: " " + cell(p.prio.T, 2), S: p.prio.S}}
	if p.status.T != "open" {
		head = append(head, ui.Span{T: " " + p.status.T, S: p.status.S})
	}
	var meta []ui.Span
	add := func(text, style string) {
		if text == "" {
			return
		}
		if len(meta) > 0 {
			meta = append(meta, ui.Span{T: " · ", S: "muted"})
		}
		meta = append(meta, ui.Span{T: text, S: style})
	}
	add(p.date.T, p.date.S)
	add(p.kids, p.cellStyle)
	add(p.owner, p.ownerStyle)
	title := []ui.Span{{T: p.title}}
	for _, flag := range p.flags {
		title = append(title, ui.Span{T: " " + flag.T, S: flag.S})
	}
	return twoLineParts{head: head, meta: meta, title: title}
}
