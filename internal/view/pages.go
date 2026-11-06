package view

import (
	"strconv"
	"unicode/utf8"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// PageBarID is the Page bar's row; a Page's entry is its id under it.
const PageBarID = "main.topbar.left.pages"

// PageBarEntryID is the id of the Page bar's entry for the Page called id.
func PageBarEntryID(id string) string { return PageBarID + "." + id }

// barFit is how the Page bar is drawn: whole, compact, or compact on a line of its own under the top bar.
type barFit uint8

const (
	barFull barFit = iota
	barCompact
	barOwnLine
)

// The room the top bar's other cells keep when the Page bar is sized: the Menu button, and a few columns for the counts and
// Daemon, which Tern's flex then truncates. entryGap is the gap between two entries.
const (
	menuCols  = 8
	slackCols = 6
	entryGap  = 2
)

// keycapPadCols is the room a keycap span's pill takes beyond its text; the compact bar must still fit at the widths the tests walk.
const keycapPadCols = 1

// badgePadCols is the room a badge's pill takes beyond its text, and badgeGapCols the gap between an entry's label and its count badge.
const (
	badgePadCols = 2
	badgeGapCols = 1
)

// entryCols is the width an entry draws: its keycap, then its label unless the Page bar is compact and the Page not current,
// then its count badge when it has one, which the compact bar keeps.
func entryCols(entry model.PageBarEntry, compact bool) int {
	cols := utf8.RuneCountInString(entry.Chord) + keycapPadCols
	if !compact || entry.Current {
		cols += 1 + displayWidth(entry.Label)
	}
	if entry.Count > 0 {
		cols += badgeGapCols + len(strconv.Itoa(entry.Count)) + badgePadCols
	}
	return cols
}

func entriesWidth(entries []model.PageBarEntry, compact bool) int {
	width := 0
	for i, entry := range entries {
		if i > 0 {
			width += entryGap
		}
		width += entryCols(entry, compact)
	}
	return width
}

// pageBarFit squeezes the top bar in the order the Daemon, the counts, the Page bar, the title: the Daemon and the counts are
// Tern's flex, so this only decides when the title and the Page bar leave them nothing. The Page bar then drops the labels of
// the Pages that are not current, and when the title, the compact bar and the Menu button still do not fit it moves to its own
// line, so the Menu button is never pushed off the bar. Width is unknown until Tern reports the Pane's size.
func pageBarFit(m model.Model, headingCols int) barFit {
	entries := m.PageBar()
	fixed := headingCols + entryGap + menuCols + slackCols
	switch {
	case m.Cols == 0, fixed+entriesWidth(entries, false) <= m.Cols:
		return barFull
	case fixed+entriesWidth(entries, true) <= m.Cols:
		return barCompact
	}
	return barOwnLine
}

// pageBar is the Page bar: one entry per Page, a keycap and the label, "[1] Issues  [2] Jobs", the current label accented, each its
// own click target that presses its key. An entry is a row of its label text and, while the Page has a count, a count badge.
func pageBar(m model.Model, flexCell func(string, ui.Tone, float64) ui.Common, compact bool) tern.Element {
	entries := m.PageBar()
	cells := make([]tern.Element, 0, len(entries))
	for _, entry := range entries {
		tone, style := ui.ToneMuted, "muted"
		if entry.Current {
			tone, style = ui.ToneAccent, "accent strong"
		}
		cell := flexCell(entry.ID, tone, 0)
		cell.Title, cell.Actions = entry.Label+" page", &ui.Actions{Click: KeyClick(entry.Press)}
		spans := []ui.Span{{T: keycap(entry.Chord, entry.Chord), S: "key"}}
		if !compact || entry.Current {
			spans = append(spans, ui.Span{T: " " + entry.Label, S: style})
		}
		parts := ui.Nodes(ui.Text{Common: ui.Common{Key: "label", Tone: tone}, Spans: spans, Wrap: ui.WrapNone})
		if entry.Count > 0 {
			parts = append(parts, ui.Badge{Common: ui.Common{Key: "count", Tone: ui.ToneAccent, Role: MonoRole}, Text: strconv.Itoa(entry.Count)})
		}
		cells = append(cells, ui.Row{Common: cell, Gap: ui.GapXS, Align: ui.AlignCenter, Children: parts})
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
	cols := displayWidth(barTitle(m))
	if m.Loading {
		cols += displayWidth(loadingMarker.T)
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
