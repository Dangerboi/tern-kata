package view

import (
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
	refWidth := 0
	for _, child := range children {
		refWidth = max(refWidth, utf8.RuneCountInString(child.Ref))
	}
	nodes := ui.Nodes(ui.Text{Common: ui.Common{Key: "heading"}, Spans: []ui.Span{{T: "Children", S: "strong"}}})
	for i, child := range children {
		ref := jumpLink(child.UID, child.Ref)
		ref.Min, ref.Max = fixedWidth(refWidth)
		cells := ui.Nodes(ref)
		if child.Loaded {
			follow := &ui.Actions{Click: "jump=" + child.UID}
			cell := func(key, text, style string) ui.Text {
				return ui.Text{Common: ui.Common{Key: key, Actions: follow}, Spans: []ui.Span{{T: text, S: style}}}
			}
			statusStyle, titleStyle := "accent", ""
			if child.Closed {
				statusStyle, titleStyle = "muted", "muted"
			}
			status := cell("status", child.Status, statusStyle)
			status.Min, status.Max = fixedWidth(statusWidth)
			title := cell("title", child.Title, titleStyle)
			title.Grow = 1
			updated := cell("updated", relativeTime(child.UpdatedAt, now), "muted")
			updated.Min, updated.Max = fixedWidth(updatedWidth)
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

// fixedWidth pins a cell to n character cells so the columns line up across rows.
func fixedWidth(n int) (*ui.Bounds, *ui.Bounds) {
	bounds := ui.Bounds{W: ui.Ch(float64(n))}
	return &bounds, &bounds
}
