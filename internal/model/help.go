package model

import (
	"cmp"
	"iter"
	"slices"
	"strings"
)

// coverMode is the mode of a card, a question or the help sheet that takes the Pane's keys, "" when none does.
func (m Model) coverMode() Mode {
	if mode := m.cardMode(); mode != "" {
		return mode
	}
	if q, ok := m.Question(); ok {
		return q.Mode
	}
	if m.HelpOpen {
		return ModeHelp
	}
	return ""
}

// Question is a y/n question waiting for its answer. Yes and No are the actions of its keys, and YesWords and NoWords what the
// buttons for them say; Danger marks a Yes that destroys something.
type Question struct {
	Mode              Mode
	Text              string
	Yes, No           Action
	YesWords, NoWords string
	Danger            bool
}

// Question is the question to answer, if any. Only one is shown at a time: the quit question wins over the delete-view one,
// which wins over the Docbank switch, which wins over the unlink one.
func (m Model) Question() (Question, bool) {
	switch {
	case m.AskQuit:
		return Question{Mode: ModeQuit, Text: "Quit tern-kata? y/n", Yes: ConfirmQuitAction, No: StayAction, YesWords: "Quit", NoWords: "Stay"}, true
	case m.Views.Deleting != "":
		return Question{Mode: ModeDeleteView, Text: "Delete view " + m.Views.Deleting + "? y/n", Yes: ConfirmDeleteViewAction, No: KeepViewAction, YesWords: "Delete", NoWords: "Keep", Danger: true}, true
	case m.ConfigPage.AskSwitch:
		return Question{Mode: ModeSwitchDocbank, Text: "Switch to kata's local Docbank? kata restarts its daemon. y/n", Yes: ConfirmSwitchDocbankAction, No: KeepDocbankAction, YesWords: "Switch", NoWords: "Keep"}, true
	case m.AskUnlink != nil:
		a := m.AskUnlink
		return Question{Mode: ModeUnlinkAttachment, Text: "Unlink " + a.Attachment.Name + " from " + a.Ref + "? y/n", Yes: ConfirmUnlinkAttachmentAction, No: KeepAttachmentAction, YesWords: "Unlink", NoWords: "Keep", Danger: true}, true
	}
	return Question{}, false
}

// covered reports whether a card, a question or the help sheet takes the Pane's keys: a context menu pick or a toast's button,
// which would act on the Issue beneath it, does nothing then.
func (m Model) covered() bool { return m.coverMode() != "" }

// OnErrorScreen reports whether the Pane draws its daemon error in place of the Page: an error with no help sheet over it.
// Cards and questions are still drawn over it; a search left open is not, as it lives in the list the error replaces.
func (m Model) OnErrorScreen() bool { return m.Err != nil && !m.HelpOpen }

func (m Model) CurrentMode() Mode {
	if mode := m.coverMode(); mode != "" {
		return mode
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
	if inputMode(mode) || mode == ModeQuit || mode == ModeDeleteView || mode == ModeSwitchDocbank || mode == ModeUnlinkAttachment {
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
