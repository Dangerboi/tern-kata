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
	width := func(label string, text func(detailChild) string) int {
		w := utf8.RuneCountInString(label)
		for _, child := range children {
			if child.Loaded {
				w = max(w, utf8.RuneCountInString(text(child)))
			}
		}
		return w
	}
	refWidth := utf8.RuneCountInString("ticket")
	for _, child := range children {
		refWidth = max(refWidth, utf8.RuneCountInString(child.Ref))
	}
	statusW := width("status", func(c detailChild) string { return c.Status })
	ownerW := width("owner", func(c detailChild) string { return m.OwnerDisplay(&c.Owner, c.OwnerExpires, now) })
	updatedW := width("updated", func(c detailChild) string { return relativeTime(c.UpdatedAt, now) })
	// row is the status, title, owner and updated cells after ref, in that order.
	row := func(c ui.Common, follow *ui.Actions, ref ui.Text, texts, styles [4]string) ui.Row {
		widths := [4]int{statusW, 0, ownerW, updatedW}
		keys := [4]string{"status", "title", "owner", "updated"}
		cells := ui.Nodes(ref)
		for i, key := range keys {
			cell := ui.Text{Common: ui.Common{Key: key, Actions: follow, Shrink: &fixedShrink}, Spans: []ui.Span{{T: pad(texts[i], widths[i]-utf8.RuneCountInString(texts[i])), S: styles[i]}}, Wrap: ui.WrapNone}
			if key == "title" {
				cell.Grow, cell.Shrink = 1, &titleShrink
			}
			cells = append(cells, cell)
		}
		return ui.Row{Common: c, Gap: ui.GapSM, Children: cells}
	}
	muted := [4]string{"muted", "muted", "muted", "muted"}
	ticket := ui.Text{Common: ui.Common{Key: "jump", Shrink: &fixedShrink}, Spans: []ui.Span{{T: pad("ticket", refWidth-utf8.RuneCountInString("ticket")), S: "muted"}}, Wrap: ui.WrapNone}
	nodes := ui.Nodes(
		ui.Text{Common: ui.Common{Key: "heading"}, Spans: []ui.Span{{T: "Children", S: "strong"}}},
		row(ui.Common{Key: "header", Tone: ui.ToneMuted}, nil, ticket, [4]string{"status", "title", "owner", "updated"}, muted),
	)
	for i, child := range children {
		ref := jumpLink(child.UID, child.Ref)
		ref.Spans = append(ref.Spans, ui.Span{T: pad("", refWidth-utf8.RuneCountInString(child.Ref))})
		ref.Wrap, ref.Shrink = ui.WrapNone, &fixedShrink
		common := ui.Common{Key: child.Key, Role: stripeRole(i), Mark: child.mark}
		if !child.Loaded {
			nodes = append(nodes, ui.Row{Common: common, Gap: ui.GapSM, Children: ui.Nodes(ref)})
			continue
		}
		styles := [4]string{"accent", "", "muted", "muted"}
		if child.Closed {
			styles = muted
		}
		nodes = append(nodes, row(common, &ui.Actions{Click: JumpClick(child.UID)}, ref, [4]string{child.Status, child.Title, m.OwnerDisplay(&child.Owner, child.OwnerExpires, now), relativeTime(child.UpdatedAt, now)}, styles))
	}
	return ui.Col{Common: ui.Common{Key: "children"}, Gap: ui.GapNone, Children: nodes}
}

// pad appends n no-break spaces, which Tern keeps where it collapses edge spaces.
func pad(text string, n int) string { return text + strings.Repeat("\u00a0", max(n, 0)) }

// titleShrink lets only the title give up width, so Tern cuts it with an ellipsis.
var titleShrink, fixedShrink = 1.0, 0.0
