package view

import (
	"strconv"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// factLine is one label→value row of an inspector. Spans, when set, are the value and override Value and Style.
type factLine struct {
	Label, Value, Style string
	Spans               []ui.Span
	Mark                ui.Mark
}

// factInspector is an inspector: an optional head row (it carries the subject's hovercard), then the facts in the shared
// property grid, so the keys line up as they do in the Issue detail.
func factInspector(m model.Model, head tern.Element, lines []factLine) tern.Element {
	rows := make([]tern.Element, len(lines))
	for i, line := range lines {
		value := line.Spans
		if value == nil {
			value = []ui.Span{{T: line.Value, S: line.Style}}
		}
		rows[i] = factRow("fact"+strconv.Itoa(i), line.Mark, ui.Span{T: line.Label}, value...)
	}
	content := tern.Nodes{}
	if head != nil {
		content = append(content, head)
	}
	content = append(content, propGrid(rows...))
	list := ui.List{CommonWithoutMax: scrollerCommon("viewport"), Max: fillMax, Children: ui.Nodes(ui.Col{Common: ui.Common{Key: "content"}, Gap: ui.GapSM, Children: content})}
	return panel(m, model.ColumnDetail, ui.Common{Key: "detail", Grow: 1, Basis: ui.BasisFrac(0.5)}, ui.GapMD, ui.Nodes(list))
}

// factRow is a property row under key, keeping the mark; its value Text is keyed "value".
func factRow(key string, mark ui.Mark, label ui.Span, value ...ui.Span) tern.Element {
	valueText := ui.Text{Common: ui.Common{Key: "value", Grow: 1, Min: &ui.Bounds{W: ui.Ch(0)}}, Spans: value}
	row := propRow(label.T, valueText).(ui.Row)
	row.Key, row.Mark = key, mark
	return row
}

func emptyInspector(m model.Model, text string) tern.Element {
	return panel(m, model.ColumnDetail, ui.Common{Key: "detail", Grow: 1, Basis: ui.BasisFrac(0.5)}, ui.GapMD,
		ui.Nodes(ui.Text{Common: ui.Common{Key: "empty"}, Text: text}))
}
