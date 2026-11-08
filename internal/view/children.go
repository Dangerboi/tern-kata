package view

import (
	"cmp"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// ChildrenTableID is the id of the Children table; a Select on it carries the chosen child's UID as the item.
const ChildrenTableID = DetailContentID + ".children.table"

func childrenTable(m model.Model, children []detailChild, now time.Time) tern.Element {
	rows := make([]ui.TableRow, len(children))
	selected := ""
	for i, child := range children {
		if child.mark == ui.MarkPick {
			selected = child.UID
		}
		cells := map[string]ui.Cell{"ticket": childTicketCell(m, child, now)}
		if child.Loaded {
			loadedChildCells(cells, m.ChildCells(child.ChildRow, now), child)
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

// childTicketCell is the reference cell. The table's cells take a tooltip and no hovercard, so the card's facts are its
// one-line Title.
func childTicketCell(m model.Model, child detailChild, now time.Time) ui.Cell {
	facts := refFacts{UID: child.UID, Qualified: cmp.Or(child.Qualified, child.Ref), Status: child.Status}
	return ui.TextCell{Text: ui.Spans(ui.Span{T: child.Ref, S: "accent"}), Title: refTooltip(summarize(m, facts, now))}
}

func loadedChildCells(cells map[string]ui.Cell, c model.ChildCells, child detailChild) {
	style := ""
	if child.Closed {
		style = "muted"
	}
	cells["status"] = ui.TextCell{Text: ui.Spans(ui.Span{T: c.Status, S: style}), Dot: statusTone(child.Status)}
	cells["title"] = ui.Spans(ui.Span{T: c.Title, S: style})
	cells["updated"] = ui.Spans(ui.Span{T: c.Updated, S: "muted"})
	if c.Owner != "" {
		cells["owner"] = ui.Spans(ui.Span{T: c.Owner, S: "muted"})
	}
}
