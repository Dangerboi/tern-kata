package view

import (
	"strings"

	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// propRow is one row of a property grid, keyed by the lower-cased key word ("Scheduled" is "scheduled"): the key word in muted sans, with no colon (the column is the separator), then the
// value. PropertyKeyRole gives every key one box width so the values start at one x.
func propRow(key string, value ...tern.Element) tern.Element {
	noShrink := 0.0
	label := ui.Text{Common: ui.Common{Key: "k", Role: PropertyKeyRole, Tone: ui.ToneMuted, Shrink: &noShrink}, Spans: []ui.Span{{T: key, S: "muted"}}, Wrap: ui.WrapNone}
	children := append(ui.Nodes(label), value...)
	return ui.Row{Common: ui.Common{Key: strings.ReplaceAll(strings.ToLower(key), " ", "-")}, Gap: ui.GapMD, Align: ui.AlignStart, Children: children}
}

// propGrid stacks property rows at the tight gap and marks the grid with PropertiesRole.
func propGrid(rows ...tern.Element) tern.Element {
	return ui.Col{Common: ui.Common{Key: "properties", Role: PropertiesRole}, Gap: ui.GapXS, Children: rows}
}
