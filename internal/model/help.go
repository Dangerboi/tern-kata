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

type questionEntry struct {
	mode  Mode
	asked func(Model) (Question, bool)
}

// questions lists the y/n questions in the order they win when several are open: the quit question wins over the delete-view one,
// which wins over the Docbank switch, which wins over the unlink one, which wins over the daemon notice's restart. A new question is
// added here and nowhere else.
var questions = []questionEntry{
	{ModeQuit, func(m Model) (Question, bool) {
		return Question{Text: "Quit tern-kata? y/n", Yes: ConfirmQuitAction, No: StayAction, YesWords: "Quit", NoWords: "Stay"}, m.AskQuit
	}},
	{ModeDeleteView, func(m Model) (Question, bool) {
		return Question{Text: "Delete view " + m.Views.Deleting + "? y/n", Yes: ConfirmDeleteViewAction, No: KeepViewAction, YesWords: "Delete", NoWords: "Keep", Danger: true}, m.Views.Deleting != ""
	}},
	{ModeSwitchDocbank, func(m Model) (Question, bool) {
		return Question{Text: "Switch to kata's local Docbank? kata restarts its daemon.", Yes: ConfirmSwitchDocbankAction, No: KeepDocbankAction, YesWords: "Switch", NoWords: "Keep"}, m.ConfigPage.AskSwitch
	}},
	{ModeUnlinkAttachment, func(m Model) (Question, bool) {
		a := m.AskUnlink
		if a == nil {
			return Question{}, false
		}
		return Question{Text: "Unlink " + a.Attachment.Name + " from " + a.Ref + "? y/n", Yes: ConfirmUnlinkAttachmentAction, No: KeepAttachmentAction, YesWords: "Unlink", NoWords: "Keep", Danger: true}, true
	}},
	{ModeNoticeRestart, func(m Model) (Question, bool) {
		return Question{Text: RestartQuestion, Yes: ConfirmNoticeRestartAction, No: DeclineNoticeRestartAction, YesWords: "Restart now", NoWords: "Not now"}, m.Versions.askRestart
	}},
}

// isQuestionMode reports whether mode is the mode of a question, which takes every key for itself.
func isQuestionMode(mode Mode) bool {
	return slices.ContainsFunc(questions, func(q questionEntry) bool {
		return q.mode == mode
	})
}

// Question is the question to answer, if any. Only one is shown at a time, the first of questions.
func (m Model) Question() (Question, bool) {
	for _, q := range questions {
		if question, asked := q.asked(m); asked {
			question.Mode = q.mode
			return question, true
		}
	}
	return Question{}, false
}

// covered reports whether a card, a question or the help sheet takes the Pane's keys: a context menu pick or a toast's button,
// which would act on the Issue beneath it, does nothing then.
func (m Model) covered() bool { return m.coverMode() != "" }

// OnErrorScreen reports whether the Pane draws its daemon error in place of the Page: an error with no help sheet over it.
// Cards and questions are still drawn over it; a search left open is not, as it lives in the list the error replaces. The Daemons
// Page is the way out of the screen to another daemon, so it is drawn with the error still set.
func (m Model) OnErrorScreen() bool { return m.Err != nil && !m.HelpOpen && !m.OnDaemons() }

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
	if m.Err != nil && !m.OnDaemons() {
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
	if inputMode(mode) || isQuestionMode(mode) {
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
	if b.entry != nil && b.entry.footerLabel != nil {
		return b.entry.footerLabel(m)
	}
	if b.entry != nil && b.entry.label != nil {
		return b.entry.label(m)
	}
	return b.Help
}

func (b Binding) Offered(m Model) bool {
	// Tab completes a name only in the forms that list names (labels); the priority picker and the free-text forms have none.
	if b.Mode == ModeFieldForm && b.Action == FieldComplete {
		open := fieldSpecs[m.FieldForm.Action].open
		return open == openProjectLabels || open == openIssueLabels
	}
	// The Menu popup's view keys act on a saved view only, so its footer names them only while the cursor is on one.
	if b.Mode == ModePicker && (b.Action == EditViewAction || b.Action == AskDeleteView) && !m.CursorOnSavedView() {
		return false
	}
	return b.entry == nil || b.entry.when == nil || b.entry.when(m)
}

// run calls the binding's handler unless the binding is not on offer for m, which does nothing: no Cmd and no Notice.
func (b Binding) run(m Model) (Model, []Cmd) {
	if !b.Offered(m) {
		return m, nil
	}
	return b.entry.handle(m)
}

// footerRank puts the keys every Page ends its footer with last: the escape key (Back or Menu), then help, then quit. Everything
// else keeps the order of its entry.
func footerRank(b Binding) int {
	switch {
	case b.Action == QuitAction:
		return 3
	case b.Action == ToggleHelp:
		return 2
	case b.Chord == "escape":
		return 1
	}
	return 0
}

func BindingsForMode(mode Mode, footerOnly bool) iter.Seq[Binding] {
	return func(yield func(Binding) bool) {
		var active []Binding
		for _, binding := range Keymap {
			if binding.Mode != "" && binding.Mode != mode {
				continue
			}
			if footerOnly && !binding.ShowInFooter {
				continue
			}
			found, ok := LookupBindingForMode(binding.Chord, mode)
			if !ok || found != binding {
				continue
			}
			active = append(active, binding)
		}
		if footerOnly {
			slices.SortStableFunc(active, func(a, b Binding) int { return cmp.Compare(footerRank(a), footerRank(b)) })
		}
		for _, binding := range active {
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
	sheetMenu
	sheetBoard
	sheetJobs
	sheetWorkflows
	sheetRuns
	sheetAttention
	sheetLettered
)

// SheetEverywhere is the title of the group of keys that work on every Page, which the view adds the note on Emacs keys to.
const SheetEverywhere = "Everywhere"

// sheetTitles are the group titles the help sheet shows, in the order of the sheetGroup constants. The view reads "Cards" and
// "Filter and search" for the card lines it adds itself.
var sheetTitles = [...]string{SheetEverywhere, "Issue list", "Issue detail", "Children", "Cards", "Filter and search", "Menu", "Board", "Jobs", "Workflows", "Runs", "Attention", "More Pages"}

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
