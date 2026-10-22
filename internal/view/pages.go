package view

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// PageBarID is the Page bar's row; a Page's tab is its id under it.
const PageBarID = "main.topbar.left.pages"

// PageTabID is the id of the Page bar's tab for the Page called id.
func PageTabID(id string) string { return PageBarID + "." + id }

// pageBar is the Page bar: one tab per Page, "1 Issues  2 Board", the current one accented. A click on a tab presses its key.
func pageBar(m model.Model, flexCell func(string, ui.Tone, float64) ui.Common) []tern.Element {
	tabs := make([]tern.Element, 0, len(m.PageBar()))
	for _, tab := range m.PageBar() {
		tone, style := ui.ToneMuted, "muted"
		if tab.Current {
			tone, style = ui.ToneAccent, "accent strong"
		}
		cell := flexCell(tab.ID, tone, 0)
		cell.Title, cell.Actions = tab.Label+" page", &ui.Actions{Click: KeyClick(tab.Chord)}
		tabs = append(tabs, ui.Text{Common: cell, Spans: []ui.Span{{T: tab.Chord + " ", S: "muted"}, {T: tab.Label, S: style}}, Wrap: ui.WrapNone})
	}
	return ui.Nodes(ui.Row{Common: flexCell("pages", ui.ToneNeutral, 0), Gap: ui.GapMD, Children: tabs})
}
