package model

import "slices"

// Key is one chord of a main-surface action and the label the footer and the help sheet show for it.
type Key struct{ Chord, Label string }

// actionEntry is everything the model knows about one main-surface action in the modes it is bound
// in: the list, the Issue detail, the Menu sidebar, the search modes and the keys that work
// everywhere. Keymap lookup, the help sheet and the footer read it, and the key dispatch calls handle.
// Adding a key to the main surface is adding one entry.
type actionEntry struct {
	action Action
	// modes is where the keys are bound; none means every mode, as for a global binding.
	modes []Mode
	// keys are the chords; the footer shows the first one only, the others are aliases.
	keys   []Key
	help   string
	footer bool
	// sheet are the help sheet lines the entry's keys appear on; none means the sheet does not list it.
	sheet  []sheetLine
	handle func(Model) (Model, []Cmd)
}

func keys(chordAndLabel ...string) []Key {
	list := make([]Key, 0, len(chordAndLabel)/2)
	for i := 0; i < len(chordAndLabel); i += 2 {
		list = append(list, Key{chordAndLabel[i], chordAndLabel[i+1]})
	}
	return list
}

// handler builders: a main-surface handler is a function of the model alone, the entry knows its action.
func pure(f func(Model) Model) func(Model) (Model, []Cmd) {
	return func(m Model) (Model, []Cmd) { return f(m), nil }
}

func selection(action Action) func(Model) (Model, []Cmd) {
	return pure(func(m Model) Model { return moveSelection(m, action) })
}

func searchMoving(action Action) func(Model) (Model, []Cmd) {
	return pure(func(m Model) Model { return searchMove(m, action) })
}

func scrolling(column Column, by ScrollBy) func(Model) (Model, []Cmd) {
	return func(m Model) (Model, []Cmd) { return m, []Cmd{Scroll{Column: column, By: by}} }
}

func hierarchy(action Action) func(Model) (Model, []Cmd) {
	return pure(func(m Model) Model { return updateHierarchy(m, action) })
}

func fieldFor(action Action) func(Model) (Model, []Cmd) {
	return func(m Model) (Model, []Cmd) { return openField(m, action) }
}

func sidebarMoving(action Action) func(Model) (Model, []Cmd) {
	return pure(func(m Model) Model { return moveSidebar(m, action) })
}

func cursorMoving(action Action) func(Model) (Model, []Cmd) {
	return pure(func(m Model) Model { return m.moveCursor(action) })
}

// moveSidebar moves the Menu sidebar's cursor by a row or to either end.
func moveSidebar(m Model, action Action) Model {
	entries := m.SidebarEntries()
	if len(entries) == 0 {
		return m
	}
	last := len(entries) - 1
	switch action {
	case MoveDown:
		m.CollectionCursor = min(m.CollectionCursor+1, last)
	case MoveUp:
		m.CollectionCursor = max(0, m.CollectionCursor-1)
	case MoveFirst:
		m.CollectionCursor = 0
	case MoveLast:
		m.CollectionCursor = last
	}
	return m
}

// sidebarOpen is enter in the Menu sidebar: it opens the entry under the cursor, or hands focus to the list when there is none.
func sidebarOpen(m Model) (Model, []Cmd) {
	entries := m.SidebarEntries()
	if len(entries) == 0 {
		m.Focus = ColumnList
		return m, nil
	}
	return openEntry(m, entries[min(m.CollectionCursor, len(entries)-1)], false)
}

// menuBack is esc in the Menu sidebar: it leaves the Projects level, and does nothing at the top.
func menuBack(m Model) Model {
	if m.SidebarProjects && len(m.SidebarEntries()) > 0 {
		return setMenuLevel(m, false, false)
	}
	return m
}

func reopenIssue(m Model) (Model, []Cmd) {
	issue := closeIssue(m)
	if issue == nil || m.Stale || m.CloseForm.Saving {
		return m, nil
	}
	m.CloseForm = CloseFormState{UID: issue.UID, Saving: true}
	return m, []Cmd{ReopenIssueCmd{Workspace: m.Workspace, UID: issue.UID, ProjectID: issue.ProjectID, Actor: m.Actor}}
}

var (
	listAndDetail = []Mode{ModeList, ModeDetail}
	searching     = []Mode{ModeSearch, ModeSearchResults}
)

// The Emacs key sets every mode shares. The headless Tern the smoke harness drives delivers its Ctrl+<key> as
// meta+<key>, so each set carries a meta+ twin with no label; a chord with no label is Tern headless only and neither
// the footer nor the help sheet shows it. alt+v arrives as alt+v in both.
var (
	pageUpKeys   = keys("page_up", "pgup", "alt+v", "M-v")
	pageDownKeys = keys("page_down", "pgdn", "ctrl+v", "C-v", "meta+v", "")
	emacsDown    = keys("ctrl+n", "C-n", "meta+n", "")
	emacsUp      = keys("ctrl+p", "C-p", "meta+p", "")
)

// mainActions lists the main-surface actions in the order the footer shows them.
var mainActions = []actionEntry{
	{action: PageUp, modes: []Mode{ModeList}, keys: pageUpKeys, help: "page up", sheet: []sheetLine{{sheetGraph, 3, "page up"}}, handle: selection(PageUp)},
	{action: PageDown, modes: []Mode{ModeList}, keys: pageDownKeys, help: "page down", sheet: []sheetLine{{sheetGraph, 4, "page down"}}, handle: selection(PageDown)},
	{action: PageUp, modes: []Mode{ModeCollections}, keys: pageUpKeys, help: "page up", handle: scrolling(ColumnCollections, ScrollPageUp)},
	{action: PageDown, modes: []Mode{ModeCollections}, keys: pageDownKeys, help: "page down", handle: scrolling(ColumnCollections, ScrollPageDown)},
	{action: PageUp, modes: []Mode{ModeDetail}, keys: pageUpKeys, help: "scroll page up", handle: scrolling(ColumnDetail, ScrollPageUp)},
	{action: PageDown, modes: []Mode{ModeDetail}, keys: pageDownKeys, help: "scroll page down", handle: scrolling(ColumnDetail, ScrollPageDown)},
	{action: PageUp, modes: searching, keys: pageUpKeys, help: "page up", handle: searchMoving(PageUp)},
	{action: PageDown, modes: searching, keys: pageDownKeys, help: "page down", handle: searchMoving(PageDown)},
	{action: NewIssue, modes: listAndDetail, keys: keys("n", "n"), help: "new Issue", footer: true, sheet: []sheetLine{{sheetGraph, 12, "new issue (form)"}}, handle: func(m Model) (Model, []Cmd) { return openCreate(m, false) }},
	{action: NewChild, modes: listAndDetail, keys: keys("shift+n", "N"), help: "new child", footer: true, sheet: []sheetLine{{sheetChildren, 1, "new child"}}, handle: func(m Model) (Model, []Cmd) { return openCreate(m, true) }},
	{action: MoveDown, modes: []Mode{ModeList}, keys: slices.Concat(keys("j", "j/↓", "down", "↓"), emacsDown), help: "next Issue", footer: true, sheet: []sheetLine{{sheetGraph, 2, "down"}}, handle: selection(MoveDown)},
	{action: MoveUp, modes: []Mode{ModeList}, keys: slices.Concat(keys("k", "k/↑", "up", "↑"), emacsUp), help: "previous Issue", footer: true, sheet: []sheetLine{{sheetGraph, 1, "up"}}, handle: selection(MoveUp)},
	{action: MoveFirst, modes: []Mode{ModeList}, keys: keys("g", "g", "home", "home"), help: "first Issue", sheet: []sheetLine{{sheetGraph, 5, "first"}}, handle: selection(MoveFirst)},
	{action: MoveLast, modes: []Mode{ModeList}, keys: keys("shift+g", "G", "end", "end"), help: "last Issue", sheet: []sheetLine{{sheetGraph, 6, "last"}}, handle: selection(MoveLast)},
	{action: EditBody, modes: []Mode{ModeDetail, ModeList}, keys: keys("e", "e"), help: "edit body", footer: true, sheet: []sheetLine{{sheetDetail, 5, "edit body"}}, handle: pure(func(m Model) Model { return startDraft(m, false) })},
	{action: AddComment, modes: []Mode{ModeDetail}, keys: keys("c", "c"), help: "add comment", footer: true, sheet: []sheetLine{{sheetDetail, 6, "new comment"}}, handle: pure(func(m Model) Model { return startDraft(m, true) })},
	{action: ClearFilters, modes: []Mode{ModeList}, keys: keys("c", "c"), help: "clear filters", footer: true, sheet: []sheetLine{{sheetFilters, 4, "clear filters"}}, handle: clearFilters},
	{action: OpenFilterForm, modes: []Mode{ModeList}, keys: keys("f", "f"), help: "filters", footer: true, sheet: []sheetLine{{sheetFilters, 3, "filter (form)"}}, handle: pure(openFilterForm)},
	{action: CycleStatus, modes: []Mode{ModeList}, keys: keys("s", "s"), help: "cycle status", footer: true, sheet: []sheetLine{{sheetFilters, 2, "cycle status filter"}}, handle: cycleStatus},
	{action: SearchIssues, modes: []Mode{ModeList}, keys: keys("/", "/"), help: "search", footer: true, sheet: []sheetLine{{sheetFilters, 1, "search"}}, handle: pure(openSearch)},
	{action: SearchEnter, modes: []Mode{ModeSearch}, keys: keys("enter", "enter"), help: "results", footer: true, handle: pure(searchEnter)},
	{action: SearchBack, modes: []Mode{ModeSearch}, keys: keys("escape", "esc"), help: "cancel search", footer: true, handle: searchBack},
	{action: MoveUp, modes: []Mode{ModeSearch}, keys: slices.Concat(keys("up", "up"), emacsUp), help: "previous result", handle: searchMoving(MoveUp)},
	{action: MoveDown, modes: []Mode{ModeSearch}, keys: slices.Concat(keys("down", "down"), emacsDown), help: "next result", handle: searchMoving(MoveDown)},
	{action: DeleteInput, modes: []Mode{ModeSearch}, keys: keys("backspace", "backspace"), help: "delete", handle: func(m Model) (Model, []Cmd) { return searchEdit(m, deleteLastRune(m.Filters.Search)) }},
	{action: ClearInput, modes: []Mode{ModeSearch}, keys: keys("ctrl+u", "ctrl+u"), help: "clear search", handle: func(m Model) (Model, []Cmd) { return searchEdit(m, "") }},
	{action: MoveUp, modes: []Mode{ModeSearchResults}, keys: slices.Concat(keys("up", "up"), emacsUp), help: "previous result", footer: true, handle: searchMoving(MoveUp)},
	{action: MoveDown, modes: []Mode{ModeSearchResults}, keys: slices.Concat(keys("down", "down"), emacsDown), help: "next result", footer: true, handle: searchMoving(MoveDown)},
	{action: SearchEnter, modes: []Mode{ModeSearchResults}, keys: keys("enter", "enter"), help: "commit search", footer: true, handle: pure(searchEnter)},
	{action: SearchBack, modes: []Mode{ModeSearchResults}, keys: keys("escape", "esc"), help: "edit query", footer: true, handle: searchBack},
	{action: SearchQuery, modes: []Mode{ModeSearchResults}, keys: keys("/", "/"), help: "edit query", handle: pure(func(m Model) Model { m.Search.Results = false; return m })},
	{action: ToggleExpansion, modes: []Mode{ModeList}, keys: keys("space", "space"), help: "expand/collapse", footer: true, sheet: []sheetLine{{sheetGraph, 8, "expand/collapse"}}, handle: hierarchy(ToggleExpansion)},
	{action: ExpandIssue, modes: []Mode{ModeList}, keys: keys("right", "right"), help: "expand", sheet: []sheetLine{{sheetGraph, 9, "expand"}}, handle: hierarchy(ExpandIssue)},
	{action: CollapseIssue, modes: []Mode{ModeList}, keys: keys("left", "left"), help: "collapse", sheet: []sheetLine{{sheetGraph, 10, "collapse"}}, handle: hierarchy(CollapseIssue)},
	{action: ToggleAllExpansion, modes: []Mode{ModeList}, keys: keys("shift+e", "E"), help: "expand/collapse all", sheet: []sheetLine{{sheetGraph, 11, "expand/collapse all"}}, handle: hierarchy(ToggleAllExpansion)},
	{action: ToggleIssueView, modes: []Mode{ModeList}, keys: keys("v", "v"), help: "flat/nested", footer: true, sheet: []sheetLine{{sheetGraph, 13, "toggle flat/nested view"}}, handle: hierarchy(ToggleIssueView)},
	{action: DrillOut, modes: []Mode{ModeList}, keys: keys("escape", "esc"), help: "Menu", footer: true, sheet: []sheetLine{{sheetGlobal, 5, "menu"}}, handle: drillOut},
	{action: MoveDown, modes: []Mode{ModeCollections}, keys: slices.Concat(keys("j", "j/↓", "down", "↓"), emacsDown), help: "next Collection", footer: true, handle: sidebarMoving(MoveDown)},
	{action: MoveUp, modes: []Mode{ModeCollections}, keys: slices.Concat(keys("k", "k/↑", "up", "↑"), emacsUp), help: "previous Collection", footer: true, handle: sidebarMoving(MoveUp)},
	{action: MoveFirst, modes: []Mode{ModeCollections}, keys: keys("g", "g", "home", "home"), help: "first Collection", handle: sidebarMoving(MoveFirst)},
	{action: MoveLast, modes: []Mode{ModeCollections}, keys: keys("shift+g", "G", "end", "end"), help: "last Collection", handle: sidebarMoving(MoveLast)},
	{action: DrillIn, modes: []Mode{ModeCollections}, keys: keys("enter", "enter"), help: "open Collection", footer: true, handle: sidebarOpen},
	{action: MenuBack, modes: []Mode{ModeCollections}, keys: keys("escape", "esc"), help: "back", footer: true, handle: pure(menuBack)},
	{action: OpenProjects, modes: []Mode{ModeCollections, ModeList, ModeDetail}, keys: keys("shift+p", "P"), help: "projects", sheet: []sheetLine{{sheetGlobal, 4, "projects"}}, handle: pure(openProjects)},
	{action: OpenInbox, modes: []Mode{ModeCollections, ModeList, ModeDetail}, keys: keys("shift+i", "I"), help: "inbox", sheet: []sheetLine{{sheetGlobal, 3, "inbox"}}, handle: openInbox},
	{action: QuitAction, keys: keys("q", "q", "ctrl+c", "ctrl+c"), help: "quit", footer: true, sheet: []sheetLine{{sheetGlobal, 2, "quit"}}, handle: pure(func(m Model) Model { m.Quitting = true; return m })},
	{action: DrillIn, modes: []Mode{ModeList}, keys: keys("enter", "enter"), help: "open", footer: true, sheet: []sheetLine{{sheetGraph, 7, "open detail"}}, handle: pure(drillIn)},
	{action: FocusDetail, modes: []Mode{ModeList}, keys: keys("tab", "tab"), help: "detail", footer: true, handle: pure(func(m Model) Model {
		if m.CanFocusDetail() {
			m.Focus = ColumnDetail
		}
		return m
	})},
	{action: ToggleDetail, modes: listAndDetail, keys: keys("shift+l", "L"), help: "toggle detail", footer: true, sheet: []sheetLine{{sheetGlobal, 6, "toggle layout"}}, handle: pure(Model.toggleDetail)},
	{action: MoveDown, modes: []Mode{ModeDetail}, keys: slices.Concat(keys("j", "j/k"), emacsDown), help: "row", footer: true, sheet: []sheetLine{{sheetChildren, 2, "move child cursor"}}, handle: cursorMoving(MoveDown)},
	{action: MoveUp, modes: []Mode{ModeDetail}, keys: slices.Concat(keys("k", "k"), emacsUp), help: "previous row", sheet: []sheetLine{{sheetChildren, 3, "move child cursor"}}, handle: cursorMoving(MoveUp)},
	{action: ScrollDown, modes: []Mode{ModeDetail}, keys: keys("down", "↓"), help: "scroll down", footer: true, sheet: []sheetLine{{sheetChildren, 5, "scroll detail viewport"}}, handle: scrolling(ColumnDetail, ScrollLineDown)},
	{action: ScrollUp, modes: []Mode{ModeDetail}, keys: keys("up", "↑"), help: "scroll up", footer: true, sheet: []sheetLine{{sheetChildren, 4, "scroll detail viewport"}}, handle: scrolling(ColumnDetail, ScrollLineUp)},
	{action: MoveFirst, modes: []Mode{ModeDetail}, keys: keys("g", "g", "home", "home"), help: "first row", handle: cursorMoving(MoveFirst)},
	{action: MoveLast, modes: []Mode{ModeDetail}, keys: keys("shift+g", "G", "end", "end"), help: "last row", handle: cursorMoving(MoveLast)},
	{action: DrillIn, modes: []Mode{ModeDetail}, keys: keys("enter", "↵"), help: "open", footer: true, sheet: []sheetLine{{sheetDetail, 3, "jump to referenced issue"}, {sheetChildren, 6, "open child"}}, handle: pure(drillIn)},
	{action: DrillOut, modes: []Mode{ModeDetail}, keys: keys("escape", "esc", "left", "←"), help: "back", footer: true, sheet: []sheetLine{{sheetDetail, 4, "back"}}, handle: drillOut},
	{action: NextActivityTab, modes: []Mode{ModeDetail}, keys: keys("tab", "↹"), help: "tab", footer: true, sheet: []sheetLine{{sheetDetail, 1, "next tab"}}, handle: pure(func(m Model) Model { return m.cycleTab(true) })},
	{action: PreviousActivityTab, modes: []Mode{ModeDetail}, keys: keys("shift+tab", "shift+tab"), help: "previous tab", sheet: []sheetLine{{sheetDetail, 2, "prev tab"}}, handle: pure(func(m Model) Model { return m.cycleTab(false) })},
	{action: RetryAction, modes: []Mode{ModeError}, keys: keys("ctrl+r", "ctrl+r"), help: "retry daemon", footer: true, handle: func(m Model) (Model, []Cmd) { return updateLive(m, Retry{}) }},
	{action: ToggleHelp, keys: keys("?", "?", "shift+?", "?"), help: "help", footer: true, sheet: []sheetLine{{sheetGlobal, 1, "help"}}, handle: pure(func(m Model) Model { m.HelpOpen = !m.HelpOpen; return m })},
	{action: CloseHelp, modes: []Mode{ModeHelp}, keys: keys("escape", "esc"), help: "close help", footer: true, handle: pure(func(m Model) Model { m.HelpOpen = false; return m })},
	{action: OpenClose, modes: listAndDetail, keys: keys("x", "x"), help: "close Issue", footer: true, sheet: []sheetLine{{sheetGraph, 14, "close"}}, handle: pure(openClose)},
	{action: OpenRecurrenceAction, modes: listAndDetail, keys: keys("shift+o", "O"), help: "make recurring", sheet: []sheetLine{{sheetDetail, 13, "make recurring"}}, handle: pure(openRecurrence)},
	{action: ReopenAction, modes: listAndDetail, keys: keys("r", "r"), help: "reopen Issue", footer: true, sheet: []sheetLine{{sheetGraph, 15, "reopen"}}, handle: reopenIssue},
	{action: AddLabelAction, modes: listAndDetail, keys: keys("+", "+"), help: "add label", sheet: []sheetLine{{sheetDetail, 8, "add label"}}, handle: fieldFor(AddLabelAction)},
	{action: RemoveLabelAction, modes: listAndDetail, keys: keys("-", "-"), help: "remove label", sheet: []sheetLine{{sheetDetail, 9, "remove label"}}, handle: fieldFor(RemoveLabelAction)},
	{action: AssignOwnerAction, modes: listAndDetail, keys: keys("a", "a"), help: "assign owner", sheet: []sheetLine{{sheetDetail, 10, "assign owner"}}, handle: fieldFor(AssignOwnerAction)},
	{action: ClearOwnerAction, modes: listAndDetail, keys: keys("shift+a", "A"), help: "clear owner", sheet: []sheetLine{{sheetDetail, 11, "clear owner"}}, handle: fieldFor(ClearOwnerAction)},
	{action: PriorityAction, modes: listAndDetail, keys: keys("!", "!"), help: "priority", sheet: []sheetLine{{sheetDetail, 12, "set priority"}}, handle: fieldFor(PriorityAction)},
	{action: SetParentAction, modes: listAndDetail, keys: keys("p", "p"), help: "set parent", sheet: []sheetLine{{sheetDetail, 7, "set parent"}}, handle: fieldFor(SetParentAction)},
}

// Keymap is every binding: the Popup cards' keys, then the main-surface actions' keys, expanded from
// mainActions. Empty Mode is global; otherwise the binding is active only in that mode.
var Keymap []Binding

func init() {
	Keymap = append([]Binding(nil), cardBindings...)
	for i := range mainActions {
		entry := &mainActions[i]
		for _, mode := range orGlobal(entry.modes) {
			for j, key := range entry.keys {
				help := entry.help
				if key.Label == "" {
					help += headlessNote
				}
				Keymap = append(Keymap, Binding{Chord: key.Chord, Key: key.Label, Action: entry.action, Help: help, Mode: mode, ShowInFooter: entry.footer && j == 0, entry: entry})
			}
		}
	}
}

func orGlobal(list []Mode) []Mode {
	if len(list) == 0 {
		return []Mode{""}
	}
	return list
}

// openInbox is `I`: it opens the Inbox Collection wherever the Menu is, like picking it in the sidebar.
func openInbox(m Model) (Model, []Cmd) {
	return openEntry(m, PickerEntry{Collection: InboxCollection()}, false)
}
