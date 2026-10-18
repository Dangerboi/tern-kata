package model

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
	handle func(Model) (Model, []Cmd)
}

func keys(chordAndLabel ...string) []Key {
	list := make([]Key, 0, len(chordAndLabel)/2)
	for i := 0; i < len(chordAndLabel); i += 2 {
		list = append(list, Key{chordAndLabel[i], chordAndLabel[i+1]})
	}
	return list
}

func modes(list ...Mode) []Mode { return list }

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
	listAndDetail = modes(ModeList, ModeDetail)
	searching     = modes(ModeSearch, ModeSearchResults)
)

// The headless Tern the smoke harness drives delivers its Ctrl+<key> as meta+<key>, so the Emacs keys
// carry a meta+ twin with no label: the footer and the help sheet never show it. alt+v arrives as alt+v in both.
// mainActions lists the main-surface actions in the order the footer shows them.
var mainActions = []actionEntry{
	{action: PageUp, modes: modes(ModeList), keys: keys("page_up", "pgup", "alt+v", "M-v"), help: "page up", handle: selection(PageUp)},
	{action: PageDown, modes: modes(ModeList), keys: keys("page_down", "pgdn", "ctrl+v", "C-v", "meta+v", ""), help: "page down", handle: selection(PageDown)},
	{action: PageUp, modes: modes(ModeCollections), keys: keys("page_up", "pgup", "alt+v", "M-v"), help: "page up", handle: scrolling(ColumnCollections, ScrollPageUp)},
	{action: PageDown, modes: modes(ModeCollections), keys: keys("page_down", "pgdn", "ctrl+v", "C-v", "meta+v", ""), help: "page down", handle: scrolling(ColumnCollections, ScrollPageDown)},
	{action: PageUp, modes: modes(ModeDetail), keys: keys("page_up", "pgup", "alt+v", "M-v"), help: "scroll page up", handle: scrolling(ColumnDetail, ScrollPageUp)},
	{action: PageDown, modes: modes(ModeDetail), keys: keys("page_down", "pgdn", "ctrl+v", "C-v", "meta+v", ""), help: "scroll page down", handle: scrolling(ColumnDetail, ScrollPageDown)},
	{action: PageUp, modes: searching, keys: keys("page_up", "pgup", "alt+v", "M-v"), help: "page up", handle: searchMoving(PageUp)},
	{action: PageDown, modes: searching, keys: keys("page_down", "pgdn", "ctrl+v", "C-v", "meta+v", ""), help: "page down", handle: searchMoving(PageDown)},
	{action: NewIssue, modes: listAndDetail, keys: keys("n", "n"), help: "new Issue", footer: true, handle: func(m Model) (Model, []Cmd) { return openCreate(m, false) }},
	{action: NewChild, modes: listAndDetail, keys: keys("shift+n", "N"), help: "new child", footer: true, handle: func(m Model) (Model, []Cmd) { return openCreate(m, true) }},
	{action: MoveDown, modes: modes(ModeList), keys: keys("j", "j/↓", "down", "↓", "ctrl+n", "C-n", "meta+n", ""), help: "next Issue", footer: true, handle: selection(MoveDown)},
	{action: MoveUp, modes: modes(ModeList), keys: keys("k", "k/↑", "up", "↑", "ctrl+p", "C-p", "meta+p", ""), help: "previous Issue", footer: true, handle: selection(MoveUp)},
	{action: MoveFirst, modes: modes(ModeList), keys: keys("g", "g", "home", "home"), help: "first Issue", handle: selection(MoveFirst)},
	{action: MoveLast, modes: modes(ModeList), keys: keys("shift+g", "G", "end", "end"), help: "last Issue", handle: selection(MoveLast)},
	{action: EditBody, modes: modes(ModeDetail, ModeList), keys: keys("e", "e"), help: "edit body", footer: true, handle: pure(func(m Model) Model { return startDraft(m, false) })},
	{action: AddComment, modes: modes(ModeDetail), keys: keys("c", "c"), help: "add comment", footer: true, handle: pure(func(m Model) Model { return startDraft(m, true) })},
	{action: ClearFilters, modes: modes(ModeList), keys: keys("c", "c"), help: "clear filters", footer: true, handle: clearFilters},
	{action: OpenFilterForm, modes: modes(ModeList), keys: keys("f", "f"), help: "filters", footer: true, handle: pure(openFilterForm)},
	{action: CycleStatus, modes: modes(ModeList), keys: keys("s", "s"), help: "cycle status", footer: true, handle: cycleStatus},
	{action: SearchIssues, modes: modes(ModeList), keys: keys("/", "/"), help: "search", footer: true, handle: pure(openSearch)},
	{action: SearchEnter, modes: modes(ModeSearch), keys: keys("enter", "enter"), help: "results", footer: true, handle: pure(searchEnter)},
	{action: SearchBack, modes: modes(ModeSearch), keys: keys("escape", "esc"), help: "cancel search", footer: true, handle: searchBack},
	{action: MoveUp, modes: modes(ModeSearch), keys: keys("up", "up", "ctrl+p", "C-p", "meta+p", ""), help: "previous result", handle: searchMoving(MoveUp)},
	{action: MoveDown, modes: modes(ModeSearch), keys: keys("down", "down", "ctrl+n", "C-n", "meta+n", ""), help: "next result", handle: searchMoving(MoveDown)},
	{action: DeleteInput, modes: modes(ModeSearch), keys: keys("backspace", "backspace"), help: "delete", handle: func(m Model) (Model, []Cmd) { return searchEdit(m, deleteLastRune(m.Filters.Search)) }},
	{action: ClearInput, modes: modes(ModeSearch), keys: keys("ctrl+u", "ctrl+u"), help: "clear search", handle: func(m Model) (Model, []Cmd) { return searchEdit(m, "") }},
	{action: MoveUp, modes: modes(ModeSearchResults), keys: keys("up", "up", "ctrl+p", "C-p", "meta+p", ""), help: "previous result", footer: true, handle: searchMoving(MoveUp)},
	{action: MoveDown, modes: modes(ModeSearchResults), keys: keys("down", "down", "ctrl+n", "C-n", "meta+n", ""), help: "next result", footer: true, handle: searchMoving(MoveDown)},
	{action: SearchEnter, modes: modes(ModeSearchResults), keys: keys("enter", "enter"), help: "commit search", footer: true, handle: pure(searchEnter)},
	{action: SearchBack, modes: modes(ModeSearchResults), keys: keys("escape", "esc"), help: "edit query", footer: true, handle: searchBack},
	{action: SearchQuery, modes: modes(ModeSearchResults), keys: keys("/", "/"), help: "edit query", handle: pure(func(m Model) Model { m.Search.Results = false; return m })},
	{action: ToggleExpansion, modes: modes(ModeList), keys: keys("space", "space"), help: "expand/collapse", footer: true, handle: hierarchy(ToggleExpansion)},
	{action: ExpandIssue, modes: modes(ModeList), keys: keys("right", "right"), help: "expand", handle: hierarchy(ExpandIssue)},
	{action: CollapseIssue, modes: modes(ModeList), keys: keys("left", "left"), help: "collapse", handle: hierarchy(CollapseIssue)},
	{action: ToggleAllExpansion, modes: modes(ModeList), keys: keys("shift+e", "E"), help: "expand/collapse all", handle: hierarchy(ToggleAllExpansion)},
	{action: ToggleIssueView, modes: modes(ModeList), keys: keys("v", "v"), help: "flat/nested", footer: true, handle: hierarchy(ToggleIssueView)},
	{action: DrillOut, modes: modes(ModeList), keys: keys("escape", "esc"), help: "Menu", footer: true, handle: drillOut},
	{action: MoveDown, modes: modes(ModeCollections), keys: keys("j", "j/↓", "down", "↓", "ctrl+n", "C-n", "meta+n", ""), help: "next Collection", footer: true, handle: sidebarMoving(MoveDown)},
	{action: MoveUp, modes: modes(ModeCollections), keys: keys("k", "k/↑", "up", "↑", "ctrl+p", "C-p", "meta+p", ""), help: "previous Collection", footer: true, handle: sidebarMoving(MoveUp)},
	{action: MoveFirst, modes: modes(ModeCollections), keys: keys("g", "g", "home", "home"), help: "first Collection", handle: sidebarMoving(MoveFirst)},
	{action: MoveLast, modes: modes(ModeCollections), keys: keys("shift+g", "G", "end", "end"), help: "last Collection", handle: sidebarMoving(MoveLast)},
	{action: DrillIn, modes: modes(ModeCollections), keys: keys("enter", "enter"), help: "open Collection", footer: true, handle: sidebarOpen},
	{action: MenuBack, modes: modes(ModeCollections), keys: keys("escape", "esc"), help: "back", footer: true, handle: pure(menuBack)},
	{action: OpenProjects, modes: modes(ModeCollections, ModeList, ModeDetail), keys: keys("shift+p", "P"), help: "projects", handle: pure(openProjects)},
	{action: OpenInbox, modes: modes(ModeCollections, ModeList, ModeDetail), keys: keys("shift+i", "I"), help: "inbox", handle: openInbox},
	{action: QuitAction, keys: keys("q", "q", "ctrl+c", "ctrl+c"), help: "quit", footer: true, handle: pure(func(m Model) Model { m.Quitting = true; return m })},
	{action: DrillIn, modes: modes(ModeList), keys: keys("enter", "enter"), help: "open", footer: true, handle: pure(drillIn)},
	{action: FocusDetail, modes: modes(ModeList), keys: keys("tab", "tab"), help: "detail", footer: true, handle: pure(func(m Model) Model {
		if m.CanFocusDetail() {
			m.Focus = ColumnDetail
		}
		return m
	})},
	{action: ToggleDetail, modes: listAndDetail, keys: keys("shift+l", "L"), help: "toggle detail", footer: true, handle: pure(Model.toggleDetail)},
	{action: MoveDown, modes: modes(ModeDetail), keys: keys("j", "j/k"), help: "row", footer: true, handle: cursorMoving(MoveDown)},
	{action: MoveUp, modes: modes(ModeDetail), keys: keys("k", "k"), help: "previous row", handle: cursorMoving(MoveUp)},
	{action: ScrollDown, modes: modes(ModeDetail), keys: keys("down", "↓", "ctrl+n", "C-n", "meta+n", ""), help: "scroll down", footer: true, handle: scrolling(ColumnDetail, ScrollLineDown)},
	{action: ScrollUp, modes: modes(ModeDetail), keys: keys("up", "↑", "ctrl+p", "C-p", "meta+p", ""), help: "scroll up", footer: true, handle: scrolling(ColumnDetail, ScrollLineUp)},
	{action: MoveFirst, modes: modes(ModeDetail), keys: keys("g", "g", "home", "home"), help: "first row", handle: cursorMoving(MoveFirst)},
	{action: MoveLast, modes: modes(ModeDetail), keys: keys("shift+g", "G", "end", "end"), help: "last row", handle: cursorMoving(MoveLast)},
	{action: DrillIn, modes: modes(ModeDetail), keys: keys("enter", "↵"), help: "open", footer: true, handle: pure(drillIn)},
	{action: DrillOut, modes: modes(ModeDetail), keys: keys("escape", "esc", "left", "←"), help: "back", footer: true, handle: drillOut},
	{action: NextActivityTab, modes: modes(ModeDetail), keys: keys("tab", "↹"), help: "tab", footer: true, handle: pure(func(m Model) Model { return m.cycleTab(true) })},
	{action: PreviousActivityTab, modes: modes(ModeDetail), keys: keys("shift+tab", "shift+tab"), help: "previous tab", handle: pure(func(m Model) Model { return m.cycleTab(false) })},
	{action: RetryAction, modes: modes(ModeError), keys: keys("ctrl+r", "ctrl+r"), help: "retry daemon", footer: true, handle: func(m Model) (Model, []Cmd) { return updateLive(m, Retry{}) }},
	{action: ToggleHelp, keys: keys("?", "?", "shift+?", "?"), help: "help", footer: true, handle: pure(func(m Model) Model { m.HelpOpen = !m.HelpOpen; return m })},
	{action: CloseHelp, modes: modes(ModeHelp), keys: keys("escape", "esc"), help: "close help", footer: true, handle: pure(func(m Model) Model { m.HelpOpen = false; return m })},
	{action: OpenClose, modes: listAndDetail, keys: keys("x", "x"), help: "close Issue", footer: true, handle: pure(openClose)},
	{action: ReopenAction, modes: listAndDetail, keys: keys("r", "r"), help: "reopen Issue", footer: true, handle: reopenIssue},
	{action: AddLabelAction, modes: listAndDetail, keys: keys("+", "+"), help: "add label", handle: fieldFor(AddLabelAction)},
	{action: RemoveLabelAction, modes: listAndDetail, keys: keys("-", "-"), help: "remove label", handle: fieldFor(RemoveLabelAction)},
	{action: AssignOwnerAction, modes: listAndDetail, keys: keys("a", "a"), help: "assign owner", handle: fieldFor(AssignOwnerAction)},
	{action: ClearOwnerAction, modes: listAndDetail, keys: keys("shift+a", "A"), help: "clear owner", handle: fieldFor(ClearOwnerAction)},
	{action: PriorityAction, modes: listAndDetail, keys: keys("!", "!"), help: "priority", handle: fieldFor(PriorityAction)},
	{action: SetParentAction, modes: listAndDetail, keys: keys("p", "p"), help: "set parent", handle: fieldFor(SetParentAction)},
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
				Keymap = append(Keymap, Binding{Chord: key.Chord, Key: key.Label, Action: entry.action, Help: entry.help, Mode: mode, ShowInFooter: entry.footer && j == 0, entry: entry})
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
	return openEntry(m, PickerEntry{Collection: Collection{View: "inbox", Title: "Inbox"}}, false)
}
