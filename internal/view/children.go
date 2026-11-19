package view

import (
	"cmp"
	"strconv"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// ChildrenTableID is the id of the Children table; a Select on it carries the chosen child's UID as the item.
const ChildrenTableID = DetailContentID + ".children.table"

func childrenTable(m model.Model, children []detailChild, now time.Time) tern.Element {
	rows := make([]ui.TableRow, len(children))
	selected, owners := "", false
	for i, child := range children {
		if child.mark == ui.MarkPick {
			selected = child.UID
		}
		cells := map[string]ui.Cell{"ticket": childTicketCell(m, child, now)}
		if child.Loaded {
			c := m.ChildCells(child.ChildRow, now)
			owners = owners || c.Owner != ""
			loadedChildCells(cells, c, child)
		}
		rows[i] = ui.TableRow{ID: child.UID, Cells: cells}
	}
	cols := []ui.Column{
		{ID: "ticket", Head: ui.T("Id"), Mono: true, Priority: 4},
		{ID: "status", Head: ui.T("Status"), Priority: 3},
		{ID: "title", Head: ui.T("Title"), Grow: 1, Priority: 5, Truncate: ui.TruncateEnd},
	}
	if owners {
		cols = append(cols, ui.Column{ID: "owner", Head: ui.T("Owner"), Priority: 1})
	}
	cols = append(cols, ui.Column{ID: "updated", Head: ui.T("Updated"), Priority: 2, Align: ui.AlignEnd})
	counts := m.DetailChildCounts()
	return ui.Col{Common: ui.Common{Key: "children"}, Gap: ui.GapXS, Children: ui.Nodes(
		ui.Text{Common: ui.Common{Key: "heading"}, Spans: []ui.Span{{T: "Children ", S: "strong"}, {T: strconv.Itoa(counts.Total), S: "muted"}}},
		ui.Table{
			Common:     ui.Common{Key: "table"},
			Cols:       cols,
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
	style := "accent link" // the style of every other Issue ref; a closed child's id is muted like the rest of its row
	if child.Closed {
		style = "muted"
	}
	return ui.TextCell{Text: ui.Spans(ui.Span{T: child.Ref, S: style}), Title: refTooltip(summarize(m, facts, now))}
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
