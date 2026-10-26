package view

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// childrenTable is the Children heading, a header row and one row per child: reference,
// status, title, owner and updated time, as in kata's TUI. Rows are shaded as the Issue
// list's are. A click on any cell jumps to the child; a child
// that is not loaded shows only its reference.
func childrenTable(m model.Model, children []detailChild, now time.Time) tern.Element {
	cells := make([]model.ChildCells, len(children))
	for i, child := range children {
		cells[i] = m.ChildCells(child.ChildRow, now)
	}
	width := func(label string, text func(model.ChildCells) string) int {
		w := utf8.RuneCountInString(label)
		for i, child := range children {
			if child.Loaded {
				w = max(w, displayWidth(text(cells[i])))
			}
		}
		return w
	}
	refWidth := utf8.RuneCountInString("ticket")
	for _, child := range children {
		refWidth = max(refWidth, utf8.RuneCountInString(child.Ref))
	}
	statusW := width("status", func(c model.ChildCells) string { return c.Status })
	ownerW := width("owner", func(c model.ChildCells) string { return c.Owner })
	updatedW := width("updated", func(c model.ChildCells) string { return c.Updated })
	// row is the status, title, owner and updated cells after ref, in that order.
	row := func(c ui.Common, follow *ui.Actions, ref ui.Text, texts, styles [4]string) ui.Row {
		widths := [4]int{statusW, 0, ownerW, updatedW}
		keys := [4]string{"status", "title", "owner", "updated"}
		cells := ui.Nodes(ref)
		for i, key := range keys {
			text := texts[i]
			if key != "owner" { // the owner's box is its width; padding it by cell count would overshoot a wide glyph
				text = pad(text, widths[i]-displayWidth(text))
			} else if text == "" {
				text = "\u00a0" // an empty text draws no element, and every row keeps its owner cell
			}
			cell := ui.Text{Common: ui.Common{Key: key, Actions: follow, Shrink: &fixedShrink}, Spans: []ui.Span{{T: text, S: styles[i]}}, Wrap: ui.WrapNone}
			if key == "title" {
				cell.Grow, cell.Shrink = 1, &titleShrink
			} else {
				cell.Min = columnBox(widths[i])
			}
			cells = append(cells, cell)
		}
		return ui.Row{Common: c, Gap: ui.GapSM, Children: cells}
	}
	muted := [4]string{"muted", "muted", "muted", "muted"}
	ticket := ui.Text{Common: ui.Common{Key: "jump", Shrink: &fixedShrink}, Spans: []ui.Span{{T: pad("ticket", refWidth-utf8.RuneCountInString("ticket")), S: "muted"}}, Wrap: ui.WrapNone}
	ticket.Min = columnBox(refWidth)
	nodes := ui.Nodes(
		ui.Text{Common: ui.Common{Key: "heading"}, Spans: []ui.Span{{T: "Children", S: "strong"}}},
		row(ui.Common{Key: "header", Tone: ui.ToneMuted}, nil, ticket, [4]string{"status", "title", "owner", "updated"}, muted),
	)
	for i, child := range children {
		ref := jumpLink(child.UID, child.Ref)
		ref.Spans = append(ref.Spans, ui.Span{T: pad("", refWidth-utf8.RuneCountInString(child.Ref))})
		ref.Wrap, ref.Shrink = ui.WrapNone, &fixedShrink
		ref.Min = columnBox(refWidth)
		common := ui.Common{Key: child.Key, Role: stripeRole(i), Mark: child.mark}
		if !child.Loaded {
			nodes = append(nodes, ui.Row{Common: common, Gap: ui.GapSM, Children: ui.Nodes(ref)})
			continue
		}
		styles := [4]string{"accent", "", "muted", "muted"}
		if child.Closed {
			styles = muted
		}
		nodes = append(nodes, row(common, &ui.Actions{Click: JumpClick(child.UID)}, ref, [4]string{cells[i].Status, cells[i].Title, cells[i].Owner, cells[i].Updated}, styles))
	}
	return ui.Col{Common: ui.Common{Key: "children"}, Gap: ui.GapNone, Children: nodes}
}

// columnBox is the least width that makes a cell as wide as its column. Tern draws a glyph the font lacks, such as CJK,
// from a fallback font whose advance is not the character cell's, so padded text alone would leave its column narrower than
// the header's. A cell whose text outgrows the box widens instead of being cut with an ellipsis.
func columnBox(width int) *ui.Bounds { return &ui.Bounds{W: ui.Ch(float64(width))} }

// pad appends n no-break spaces, which Tern keeps where it collapses edge spaces.
func pad(text string, n int) string { return text + strings.Repeat("\u00a0", max(n, 0)) }

// titleShrink lets only the title give up width, so Tern cuts it with an ellipsis.
var titleShrink, fixedShrink = 1.0, 0.0
