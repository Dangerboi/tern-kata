package view

import (
	"strconv"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// factLine is one line of an inspector's fact grid: a muted label beside its value. Style names the value's span style, Mark the
// row's marker.
type factLine struct {
	Label, Value, Style string
	Mark                ui.Mark
}

// factInspector is the second column of the Jobs and Runs Pages: the lines in a scrolling grid of label and value, in the
// detail panel.
func factInspector(m model.Model, lines []factLine) tern.Element {
	rows := make([]tern.Element, len(lines))
	for i, line := range lines {
		rows[i] = factRow("fact"+strconv.Itoa(i), line.Mark, ui.Span{T: line.Label, S: "muted"}, ui.Span{T: line.Value, S: line.Style})
	}
	list := ui.List{CommonWithoutMax: scrollerCommon("viewport"), Max: scrollMax(m, 0), Children: ui.Nodes(ui.Col{Common: ui.Common{Key: "content"}, Gap: ui.GapSM, Children: rows})}
	return panel(m, model.ColumnDetail, ui.Common{Key: "detail", Grow: 1, Basis: ui.BasisFrac(0.5)}, ui.GapMD, ui.Nodes(list))
}

// factRow is one row of a fact grid: label in an 18-character column that never wraps, beside value, which wraps.
func factRow(key string, mark ui.Mark, label ui.Span, value ...ui.Span) tern.Element {
	labelText := ui.Text{Common: ui.Common{Key: "label", Shrink: new(float64), Min: &ui.Bounds{W: ui.Ch(18)}}, Spans: []ui.Span{label}, Wrap: ui.WrapNone}
	valueText := ui.Text{Common: ui.Common{Key: "value", Grow: 1, Min: &ui.Bounds{W: ui.Ch(0)}}, Spans: value}
	return ui.Row{Common: ui.Common{Key: key, Mark: mark}, Gap: ui.GapSM, Align: ui.AlignStart, Children: ui.Nodes(labelText, valueText)}
}

// emptyInspector is that column with one line of text, for a Page with nothing selected.
func emptyInspector(m model.Model, text string) tern.Element {
	return panel(m, model.ColumnDetail, ui.Common{Key: "detail", Grow: 1, Basis: ui.BasisFrac(0.5)}, ui.GapMD,
		ui.Nodes(ui.Text{Common: ui.Common{Key: "empty"}, Text: text}))
}
