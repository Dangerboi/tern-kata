package view

import (
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// ChildrenTableID is the id of the Children table; a Select on it carries the chosen child's UID as the item.
const ChildrenTableID = DetailContentID + ".children.table"

// childrenTable is the Children heading over a native table of one row per child: reference, status, title, owner and
// updated time, as in kata's TUI. Tern sizes the columns, cuts the title and hides low-priority columns when narrow. A
// click on a row jumps to the child, and the row under the detail's cursor is the table's selected row. A child that is
// not loaded shows only its reference.
func childrenTable(m model.Model, children []detailChild, now time.Time) tern.Element {
	rows := make([]ui.TableRow, len(children))
	selected := ""
	for i, child := range children {
		if child.mark == ui.MarkPick {
			selected = child.UID
		}
		cells := map[string]ui.Cell{"ticket": ui.Spans(ui.Span{T: child.Ref, S: "accent"})}
		if child.Loaded {
			cells = loadedChildCells(m.ChildCells(child.ChildRow, now), child)
		}
		rows[i] = ui.TableRow{ID: child.UID, Cells: cells}
	}
	return ui.Col{Common: ui.Common{Key: "children"}, Gap: ui.GapXS, Children: ui.Nodes(
		ui.Text{Common: ui.Common{Key: "heading"}, Spans: []ui.Span{{T: "Children", S: "strong"}}},
		ui.Table{
			Common: ui.Common{Key: "table"},
			Cols: []ui.Column{
				{ID: "ticket", Head: ui.T("ticket"), Mono: true, Priority: 4},
				{ID: "status", Head: ui.T("status"), Priority: 3},
				{ID: "title", Head: ui.T("title"), Grow: 1, Priority: 5, Truncate: ui.TruncateEnd},
				{ID: "owner", Head: ui.T("owner"), Priority: 1},
				{ID: "updated", Head: ui.T("updated"), Mono: true, Priority: 2},
			},
			Rows:       rows,
			Selectable: true,
			Selected:   selected,
		},
	)}
}

// loadedChildCells are the cells of a loaded child; a closed child's status and title are muted, as its owner and updated always are, and an empty owner is left out.
func loadedChildCells(c model.ChildCells, child detailChild) map[string]ui.Cell {
	title := ""
	if child.Closed {
		title = "muted"
	}
	cells := map[string]ui.Cell{
		"ticket":  ui.Spans(ui.Span{T: child.Ref, S: "accent"}),
		"status":  ui.TextCell{Text: ui.Spans(ui.Span{T: c.Status, S: title}), Dot: statusTone(child.Status)},
		"title":   ui.Spans(ui.Span{T: c.Title, S: title}),
		"updated": ui.Spans(ui.Span{T: c.Updated, S: "muted"}),
	}
	if c.Owner != "" {
		cells["owner"] = ui.Spans(ui.Span{T: c.Owner, S: "muted"})
	}
	return cells
}
