package view

import (
	"unicode/utf8"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// PageBarID is the Page bar's row; a Page's tab is its id under it.
const PageBarID = "main.topbar.left.pages"

// PageTabID is the id of the Page bar's tab for the Page called id.
func PageTabID(id string) string { return PageBarID + "." + id }

// barFit is how the Page bar is drawn: whole, compact, or compact on a line of its own under the top bar.
type barFit uint8

const (
	barFull barFit = iota
	barCompact
	barOwnLine
)

// The room the top bar's other cells keep when the Page bar is sized: the Menu button, and a few columns for the counts and
// Daemon, which Tern's flex then truncates. tabGap is the gap between two tabs.
const (
	menuCols  = 8
	slackCols = 6
	tabGap    = 2
)

// tabText is a tab's text: the key and the label, or the key alone in a compact Page bar for a Page that is not current.
func tabText(tab model.PageTab, compact bool) string {
	if compact && !tab.Current {
		return tab.Chord
	}
	return tab.Chord + " " + tab.Label
}

func tabsWidth(tabs []model.PageTab, compact bool) int {
	width := 0
	for i, tab := range tabs {
		if i > 0 {
			width += tabGap
		}
		width += utf8.RuneCountInString(tabText(tab, compact))
	}
	return width
}

// pageBarFit squeezes the top bar in the order the Daemon, the counts, the Page bar, the title: the Daemon and the counts are
// Tern's flex, so this only decides when the title and the Page bar leave them nothing. The Page bar then drops the labels of
// the Pages that are not current, and when the title, the compact bar and the Menu button still do not fit it moves to its own
// line, so the Menu button is never pushed off the bar. Width is unknown until Tern reports the Pane's size.
func pageBarFit(m model.Model, headingCols int) barFit {
	tabs := m.PageBar()
	fixed := headingCols + tabGap + menuCols + slackCols
	switch {
	case m.Cols == 0, fixed+tabsWidth(tabs, false) <= m.Cols:
		return barFull
	case fixed+tabsWidth(tabs, true) <= m.Cols:
		return barCompact
	}
	return barOwnLine
}

// pageBar is the Page bar: one tab per Page, "1 Issues  2 Board", the current one accented, each its own click target that
// presses its key.
func pageBar(m model.Model, flexCell func(string, ui.Tone, float64) ui.Common, compact bool) tern.Element {
	tabs := m.PageBar()
	cells := make([]tern.Element, 0, len(tabs))
	for _, tab := range tabs {
		tone, style := ui.ToneMuted, "muted"
		if tab.Current {
			tone, style = ui.ToneAccent, "accent strong"
		}
		cell := flexCell(tab.ID, tone, 0)
		cell.Title, cell.Actions = tab.Label+" page", &ui.Actions{Click: KeyClick(tab.Chord)}
		spans := []ui.Span{{T: tab.Chord, S: "muted"}}
		if !compact || tab.Current {
			spans = []ui.Span{{T: tab.Chord + " ", S: "muted"}, {T: tab.Label, S: style}}
		} else {
			spans[0].S = style
		}
		cells = append(cells, ui.Text{Common: cell, Spans: spans, Wrap: ui.WrapNone})
	}
	return ui.Row{Common: flexCell("pages", ui.ToneNeutral, 0), Gap: ui.GapMD, Children: cells}
}

// barTitle is the top bar's title: the Collection's, "Project: name" for a Project.
func barTitle(m model.Model) string {
	if m.Collection.IsProject() {
		return "Project: " + m.Collection.Title
	}
	return m.Collection.Title
}

// topBarFit is how the Page bar fits the top bar beside the title as drawn now.
func topBarFit(m model.Model) barFit {
	cols := utf8.RuneCountInString(barTitle(m))
	if m.Loading {
		cols += utf8.RuneCountInString(loadingMarker.T)
	}
	return pageBarFit(m, cols)
}

// pageLine is the Page bar on a line of its own under the top bar, for a Pane too narrow for it beside the title; nil otherwise.
func pageLine(m model.Model) []tern.Element {
	if topBarFit(m) != barOwnLine {
		return nil
	}
	flexCell := func(key string, tone ui.Tone, shrink float64) ui.Common {
		return ui.Common{Key: key, Tone: tone, Shrink: &shrink}
	}
	line := pageBar(m, flexCell, true).(ui.Row)
	line.Common.Key, line.Common.Hidden = "pagebar", m.HelpOpen
	return ui.Nodes(line)
}
