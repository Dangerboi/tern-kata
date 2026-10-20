package model

import (
	"cmp"
	"iter"
	"slices"
	"strings"
)

func (m Model) CurrentMode() Mode {
	if mode := m.cardMode(); mode != "" {
		return mode
	}
	if m.HelpOpen {
		return ModeHelp
	}
	if m.Search.Open {
		if m.Search.Results {
			return ModeSearchResults
		}
		return ModeSearch
	}
	if m.Err != nil {
		return ModeError
	}
	if m.Board.Mode == BoardColumns {
		return ModeBoard
	}
	if m.Board.Mode == BoardLane && m.Focus != ColumnDetail {
		return ModeLane
	}
	switch m.Focus {
	case ColumnDetail:
		return ModeDetail
	case ColumnCollections:
		return ModeCollections
	}
	return ModeList
}

func LookupBindingForMode(chord string, mode Mode) (Binding, bool) {
	for _, binding := range Keymap {
		if binding.Chord == chord && binding.Mode == mode {
			return binding, true
		}
	}
	if isCard, through := cardLetsThrough(mode, chord); isCard && !through {
		return Binding{}, false
	}
	if inputMode(mode) {
		return Binding{}, false
	}
	for _, binding := range Keymap {
		if binding.Chord == chord && binding.Mode == "" {
			return binding, true
		}
	}
	return Binding{}, false
}

func BindingsForMode(mode Mode, footerOnly bool) iter.Seq[Binding] {
	return func(yield func(Binding) bool) {
		for _, binding := range Keymap {
			if binding.Mode != "" && binding.Mode != mode {
				continue
			}
			if footerOnly && !binding.ShowInFooter {
				continue
			}
			active, ok := LookupBindingForMode(binding.Chord, mode)
			if !ok || active != binding {
				continue
			}
			if !yield(binding) {
				return
			}
		}
	}
}

type sheetGroup int

const (
	sheetGlobal sheetGroup = iota
	sheetGraph
	sheetDetail
	sheetChildren
	sheetForms
	sheetFilters
	sheetBoard
)

var sheetTitles = [...]string{"Global", "Graph", "Detail", "Children", "Forms", "Filters", "Board"}

// sheetLine places an entry's keys on one line of the help sheet. Rank orders the lines of a group; entries that
// share a group and text share the line, their keys in rank order.
type sheetLine struct {
	group sheetGroup
	rank  int
	text  string
}

// SheetLine is one line of the help sheet: what it does and the keys that do it.
type SheetLine struct {
	Text string
	Keys []string
}

// SheetGroup is a titled block of the help sheet.
type SheetGroup struct {
	Title string
	Lines []SheetLine
}

// HelpSheet is the main-surface lines of the help sheet, read from the action entries and grouped as the sheet shows
// them. A group no entry lists (Forms) comes back empty, and a line with no key to show is left out.
func HelpSheet() []SheetGroup {
	type listing struct {
		sheetLine
		keys []string
	}
	var listings []listing
	for _, entry := range mainActions {
		var keys []string
		for _, key := range entry.keys {
			for _, label := range strings.Split(key.Label, "/") {
				if label != "" && !slices.Contains(keys, label) {
					keys = append(keys, label)
				}
			}
		}
		for _, line := range entry.sheet {
			listings = append(listings, listing{line, keys})
		}
	}
	slices.SortStableFunc(listings, func(a, b listing) int {
		return cmp.Or(cmp.Compare(a.group, b.group), cmp.Compare(a.rank, b.rank))
	})
	sheet := make([]SheetGroup, len(sheetTitles))
	for i, title := range sheetTitles {
		sheet[i].Title = title
	}
	for _, l := range listings {
		lines := &sheet[l.group].Lines
		i := slices.IndexFunc(*lines, func(line SheetLine) bool { return line.Text == l.text })
		if i < 0 {
			*lines = append(*lines, SheetLine{Text: l.text})
			i = len(*lines) - 1
		}
		for _, k := range l.keys {
			if !slices.Contains((*lines)[i].Keys, k) {
				(*lines)[i].Keys = append((*lines)[i].Keys, k)
			}
		}
	}
	for i := range sheet {
		sheet[i].Lines = slices.DeleteFunc(sheet[i].Lines, func(line SheetLine) bool { return len(line.Keys) == 0 })
	}
	return sheet
}
