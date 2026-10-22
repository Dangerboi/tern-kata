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
		label := ui.Text{Common: ui.Common{Key: "label", Shrink: new(float64), Min: &ui.Bounds{W: ui.Ch(18)}}, Spans: []ui.Span{{T: line.Label, S: "muted"}}, Wrap: ui.WrapNone}
		value := ui.Text{Common: ui.Common{Key: "value", Grow: 1, Min: &ui.Bounds{W: ui.Ch(0)}}, Spans: []ui.Span{{T: line.Value, S: line.Style}}}
		rows[i] = ui.Row{Common: ui.Common{Key: "fact" + strconv.Itoa(i), Mark: line.Mark}, Gap: ui.GapSM, Align: ui.AlignStart, Children: ui.Nodes(label, value)}
	}
	list := ui.List{CommonWithoutMax: scrollerCommon("viewport"), Max: scrollMax(m, 0), Children: ui.Nodes(ui.Col{Common: ui.Common{Key: "content"}, Gap: ui.GapSM, Children: rows})}
	return panel(m, model.ColumnDetail, ui.Common{Key: "detail", Grow: 1, Basis: ui.BasisFrac(0.5)}, ui.GapMD, ui.Nodes(list))
}

// emptyInspector is that column with one line of text, for a Page with nothing selected.
func emptyInspector(m model.Model, text string) tern.Element {
	return panel(m, model.ColumnDetail, ui.Common{Key: "detail", Grow: 1, Basis: ui.BasisFrac(0.5)}, ui.GapMD,
		ui.Nodes(ui.Text{Common: ui.Common{Key: "empty"}, Text: text}))
}
