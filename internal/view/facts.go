package view

import (
	"strconv"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

type factLine struct {
	Label, Value, Style string
	Mark                ui.Mark
}

func factInspector(m model.Model, lines []factLine) tern.Element {
	rows := make([]tern.Element, len(lines))
	for i, line := range lines {
		rows[i] = factRow("fact"+strconv.Itoa(i), line.Mark, ui.Span{T: line.Label, S: "muted"}, ui.Span{T: line.Value, S: line.Style})
	}
	list := ui.List{CommonWithoutMax: scrollerCommon("viewport"), Max: fillMax, Children: ui.Nodes(ui.Col{Common: ui.Common{Key: "content"}, Gap: ui.GapSM, Children: rows})}
	return panel(m, model.ColumnDetail, ui.Common{Key: "detail", Grow: 1, Basis: ui.BasisFrac(0.5)}, ui.GapMD, ui.Nodes(list))
}

func factRow(key string, mark ui.Mark, label ui.Span, value ...ui.Span) tern.Element {
	labelText := ui.Text{Common: ui.Common{Key: "label", Shrink: new(float64), Min: &ui.Bounds{W: ui.Ch(18)}}, Spans: []ui.Span{label}, Wrap: ui.WrapNone}
	valueText := ui.Text{Common: ui.Common{Key: "value", Grow: 1, Min: &ui.Bounds{W: ui.Ch(0)}}, Spans: value}
	return ui.Row{Common: ui.Common{Key: key, Mark: mark}, Gap: ui.GapSM, Align: ui.AlignStart, Children: ui.Nodes(labelText, valueText)}
}

func emptyInspector(m model.Model, text string) tern.Element {
	return panel(m, model.ColumnDetail, ui.Common{Key: "detail", Grow: 1, Basis: ui.BasisFrac(0.5)}, ui.GapMD,
		ui.Nodes(ui.Text{Common: ui.Common{Key: "empty"}, Text: text}))
}
