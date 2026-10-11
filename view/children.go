package view

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Dangerboi/tern-kata/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// childrenTable is the Children heading and one row per child: reference, status,
// title and updated time, as in kata's TUI. A child's row index is its place in the
// detail's j/k cursor, which walks these rows before the Activity rows. A click on
// any cell jumps to the child; a child that is not loaded shows only its reference.
func childrenTable(m model.Model, children []model.ChildRow, now time.Time) tern.Element {
	refWidth, statusW, updatedW := 0, 0, 0
	for _, child := range children {
		refWidth = max(refWidth, utf8.RuneCountInString(child.Ref))
		if child.Loaded {
			statusW = max(statusW, utf8.RuneCountInString(child.Status))
			updatedW = max(updatedW, utf8.RuneCountInString(relativeTime(child.UpdatedAt, now)))
		}
	}
	nodes := ui.Nodes(ui.Text{Common: ui.Common{Key: "heading"}, Spans: []ui.Span{{T: "Children", S: "strong"}}})
	for i, child := range children {
		ref := jumpLink(child.UID, child.Ref)
		ref.Spans = append(ref.Spans, ui.Span{T: pad("", refWidth-utf8.RuneCountInString(child.Ref))})
		ref.Wrap, ref.Shrink = ui.WrapNone, &fixedShrink
		cells := ui.Nodes(ref)
		if child.Loaded {
			follow := &ui.Actions{Click: JumpClick(child.UID)}
			cell := func(key, text, style string, width int) ui.Text {
				return ui.Text{Common: ui.Common{Key: key, Actions: follow}, Spans: []ui.Span{{T: pad(text, width-utf8.RuneCountInString(text)), S: style}}, Wrap: ui.WrapNone}
			}
			statusStyle, titleStyle := "accent", ""
			if child.Closed {
				statusStyle, titleStyle = "muted", "muted"
			}
			status := cell("status", child.Status, statusStyle, statusW)
			title := cell("title", child.Title, titleStyle, 0)
			title.Grow, title.Shrink = 1, &titleShrink
			updated := cell("updated", relativeTime(child.UpdatedAt, now), "muted", updatedW)
			status.Shrink, updated.Shrink = &fixedShrink, &fixedShrink
			cells = append(cells, status, title, updated)
		}
		mark := ui.Mark("")
		if m.Focus == model.ColumnDetail && m.DetailIndex == i {
			mark = ui.MarkPick
		}
		nodes = append(nodes, ui.Row{Common: ui.Common{Key: child.Key, Mark: mark}, Gap: ui.GapSM, Children: cells})
	}
	return ui.Col{Common: ui.Common{Key: "children"}, Gap: ui.GapSM, Children: nodes}
}

// pad appends n no-break spaces, which Tern keeps where it collapses edge spaces.
func pad(text string, n int) string { return text + strings.Repeat("\u00a0", max(n, 0)) }

// titleShrink lets only the title give up width, so Tern cuts it with an ellipsis.
var titleShrink, fixedShrink = 1.0, 0.0
