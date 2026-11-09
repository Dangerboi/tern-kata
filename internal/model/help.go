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
	if m.AskQuit {
		return ModeQuit
	}
	if m.Views.Deleting != "" {
		return ModeDeleteView
	}
	if m.ConfigPage.AskSwitch {
		return ModeSwitchDocbank
	}
	if m.HelpOpen {
		return ModeHelp
	}
	if m.PageSearch.Open {
		return ModePageSearch
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
	if mode := pageMode(m); mode != "" {
		return mode
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
	if inputMode(mode) || mode == ModeQuit || mode == ModeDeleteView || mode == ModeSwitchDocbank {
		return Binding{}, false
	}
	for _, binding := range Keymap {
		if binding.Chord == chord && binding.Mode == "" {
			return binding, true
		}
	}
	return Binding{}, false
}

func (b Binding) Described(m Model) string {
	if b.entry != nil && b.entry.label != nil {
		return b.entry.label(m)
	}
	return b.Help
}

func (b Binding) Offered(m Model) bool {
	return b.entry == nil || b.entry.when == nil || b.entry.when(m)
}

// run calls the binding's handler unless the binding is not on offer for m, which does nothing: no Cmd and no Notice.
func (b Binding) run(m Model) (Model, []Cmd) {
	if !b.Offered(m) {
		return m, nil
	}
	return b.entry.handle(m)
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
	sheetJobs
	sheetWorkflows
	sheetRuns
	sheetAttention
	sheetLettered
)

var sheetTitles = [...]string{"Global", "Graph", "Detail", "Children", "Forms", "Filters", "Board", "Jobs", "Workflows", "Runs", "Attention", "Lettered Pages"}

// sheetLine places an entry's keys on one line of the help sheet. Rank orders the lines of a group; entries that
// share a group and text share the line, their keys in rank order.
type sheetLine struct {
	group sheetGroup
	rank  int
	text  string
}

type SheetLine struct {
	Text string
	Keys []string
}

type SheetGroup struct {
	Title string
	Lines []SheetLine
}

// pageHelpModes are the modes whose keys the help sheet lists for a Page, besides the global ones: the Page's list and inspector
// modes, and the modes of what opens from them or sits beside them (search and the Menu sidebar on the Issues Page, the Issue
// detail beside the Board and Workflows Pages).
var pageHelpModes = map[Page][]Mode{
	PageIssues:      {ModeList, ModeDetail, ModeCollections, ModeSearch, ModeSearchResults},
	PageBoard:       {ModeBoard, ModeLane, ModeDetail},
	PageJobs:        {ModeJobs, ModeJobInspector, ModePageSearch},
	PageRuns:        {ModeRuns, ModeRunInspector, ModePageSearch},
	PageWorkflows:   {ModeWorkflows, ModeDetail, ModePageSearch},
	PageAttention:   {ModeAttention},
	PageCredentials: {ModeCredentials},
	PageFederation: {ModeFederation, ModeFederationLeave, ModeFederationResult, ModeFederationPick, ModeFederationBrowse,
		ModeFederationPreview, ModeFederationAdopt, ModeFederationName, ModeFederationRecovery},
	PageDaemons: {ModeDaemons},
	PageConfig:  {ModeConfig},
}

type helpListing struct {
	sheetLine
	keys  []string
	modes []Mode
	when  func(Model) bool
	label func(Model) string
}

func helpListings() []helpListing {
	var listings []helpListing
	for _, entry := range mainActions {
		var keys []string
		for _, key := range entry.keys {
			parts := strings.Split(key.Label, "/")
			if !slices.ContainsFunc(parts, func(p string) bool { return p != "" }) {
				parts = []string{key.Label}
			}
			for _, label := range parts {
				if label != "" && !slices.Contains(keys, label) {
					keys = append(keys, label)
				}
			}
		}
		for _, line := range entry.sheet {
			listings = append(listings, helpListing{line, keys, entry.modes, entry.when, entry.label})
		}
	}
	return listings
}

// HelpSheet is the main-surface lines of the help sheet for every Page, read from the action entries and grouped as the
// sheet shows them. A group no entry lists (Forms) comes back empty, and a line with no key to show is left out.
func HelpSheet() []SheetGroup { return sheetOf(helpListings()) }

// HelpSheetOn is the help sheet opened over a Page: the lines whose entry is global or bound in one of the Page's modes. The
// groups keep their places (and so their positions in the full sheet), those left empty come back empty.
func HelpSheetOn(page Page, m Model) []SheetGroup {
	modes := pageHelpModes[page]
	listings := slices.DeleteFunc(helpListings(), func(l helpListing) bool {
		if l.when != nil && !l.when(m) {
			return true
		}
		return len(l.modes) > 0 && !slices.ContainsFunc(l.modes, func(mode Mode) bool { return slices.Contains(modes, mode) })
	})
	for i, l := range listings {
		if l.label != nil {
			listings[i].text = l.label(m)
		}
	}
	return sheetOf(listings)
}

func sheetOf(listings []helpListing) []SheetGroup {
	slices.SortStableFunc(listings, func(a, b helpListing) int {
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
