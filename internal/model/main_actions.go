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
	keys []Key
	// help is the footer's text for the entry, kata tui's word for it where kata has one.
	help string
	// footer is the modes whose footer shows the entry's first key and help; global for an entry with no modes.
	// footerKey, when set, is what the footer names instead of the first key's label: one hint for a pair, as ↑↓.
	footer    []Mode
	footerKey string
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

func boardMoving(action Action) func(Model) (Model, []Cmd) {
	return pure(func(m Model) Model { return moveOnBoard(m, action) })
}

// laneMoving is `←` and `→`: the selection to the neighbouring Lane's card on the Board, the neighbouring Lane in the Lane view.
func laneMoving(action Action, step int) func(Model) (Model, []Cmd) {
	return func(m Model) (Model, []Cmd) {
		if m.LaneViewShown() {
			return switchLane(m, step), nil
		}
		return boardMoving(action)(m)
	}
}

// cardMoving is `<` and `>`: a Move of the selected card, which in the Lane view also picks the next selection.
func cardMoving(dir int) func(Model) (Model, []Cmd) {
	return func(m Model) (Model, []Cmd) {
		if m.LaneViewShown() {
			return laneCardMove(dir)(m)
		}
		return MoveCard(m, m.SelectedUID, dir)
	}
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
	// issueModes are the modes whose keys act on the selected Issue: the list, the Board and the detail.
	issueModes   = []Mode{ModeList, ModeBoard, ModeLane, ModeDetail}
	listAndBoard = []Mode{ModeList, ModeBoard, ModeLane}
	// boardAndDetail are the modes whose footer lists the label and owner keys.
	boardAndDetail = []Mode{ModeBoard, ModeLane, ModeDetail}
	// listModes are the modes with the list's rows: the list and the Lane view.
	listModes = []Mode{ModeList, ModeLane}
	// laneModes are the modes that show Lanes: the Board and the Lane view.
	laneModes = []Mode{ModeBoard, ModeLane}
	searching = []Mode{ModeSearch, ModeSearchResults}
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

// global is the one mode an entry without modes binds in; footer: global puts the entry in every footer it is bound in.
var global = []Mode{""}

// mainActions lists the main-surface actions. A mode's footer shows its entries in this order, after kata tui's footers:
// moving and opening first, then what acts on the Issues, then layout, back, help and quit.
var mainActions = []actionEntry{
	// Moving and opening in the Issue list.
	{action: MoveDown, modes: listModes, keys: slices.Concat(keys("j", "j/↓", "down", "↓"), emacsDown), help: "move", footer: listModes, footerKey: "↑↓", sheet: []sheetLine{{sheetGraph, 2, "down"}}, handle: selection(MoveDown)},
	{action: MoveUp, modes: listModes, keys: slices.Concat(keys("k", "k/↑", "up", "↑"), emacsUp), help: "previous Issue", sheet: []sheetLine{{sheetGraph, 1, "up"}}, handle: selection(MoveUp)},
	{action: DrillIn, modes: listModes, keys: keys("enter", "enter"), help: "open", footer: listModes, footerKey: "↵", sheet: []sheetLine{{sheetGraph, 7, "open detail"}}, handle: drillIn},
	{action: FocusDetail, modes: listModes, keys: keys("tab", "tab"), help: "detail", footer: listModes, handle: pure(func(m Model) Model {
		if m.CanFocusDetail() {
			m.Focus = ColumnDetail
		}
		return m
	})},
	{action: ToggleExpansion, modes: []Mode{ModeList}, keys: keys("space", "space"), help: "expand", footer: []Mode{ModeList}, sheet: []sheetLine{{sheetGraph, 8, "expand/collapse"}}, handle: hierarchy(ToggleExpansion)},
	{action: ToggleAllExpansion, modes: []Mode{ModeList}, keys: keys("shift+e", "E"), help: "all", footer: []Mode{ModeList}, sheet: []sheetLine{{sheetGraph, 11, "expand/collapse all"}}, handle: hierarchy(ToggleAllExpansion)},
	{action: ExpandIssue, modes: []Mode{ModeList}, keys: keys("right", "right"), help: "expand", sheet: []sheetLine{{sheetGraph, 9, "expand"}}, handle: hierarchy(ExpandIssue)},
	{action: CollapseIssue, modes: []Mode{ModeList}, keys: keys("left", "left"), help: "collapse", sheet: []sheetLine{{sheetGraph, 10, "collapse"}}, handle: hierarchy(CollapseIssue)},
	{action: MoveFirst, modes: listModes, keys: keys("g", "g", "home", "home"), help: "first Issue", sheet: []sheetLine{{sheetGraph, 5, "first"}}, handle: selection(MoveFirst)},
	{action: MoveLast, modes: listModes, keys: keys("shift+g", "G", "end", "end"), help: "last Issue", sheet: []sheetLine{{sheetGraph, 6, "last"}}, handle: selection(MoveLast)},
	{action: PageUp, modes: listModes, keys: pageUpKeys, help: "page up", sheet: []sheetLine{{sheetGraph, 3, "page up"}}, handle: selection(PageUp)},
	{action: PageDown, modes: listModes, keys: pageDownKeys, help: "page down", sheet: []sheetLine{{sheetGraph, 4, "page down"}}, handle: selection(PageDown)},
	// Board Page: the Board takes the list's place, with the cards in Lanes.
	{action: ToggleBoard, modes: []Mode{ModeList, ModeCollections}, keys: keys("b", "b"), help: "Board", footer: []Mode{ModeList}, sheet: []sheetLine{{sheetBoard, 1, "toggle Issues and Board Pages"}}, handle: toggleBoard},
	{action: ToggleBoard, modes: laneModes, keys: keys("b", "b"), help: "list", footer: laneModes, sheet: []sheetLine{{sheetBoard, 1, "toggle Issues and Board Pages"}}, handle: toggleBoard},
	{action: OpenLanes, modes: laneModes, keys: keys("shift+b", "B"), help: "Lanes", sheet: []sheetLine{{sheetBoard, 9, "edit the Lane list (Lanes card)"}}, handle: pure(openLanesForm)},
	{action: MoveDown, modes: []Mode{ModeBoard}, keys: slices.Concat(keys("j", "j/↓", "down", "↓"), emacsDown), help: "move", footer: []Mode{ModeBoard}, footerKey: "↑↓", sheet: []sheetLine{{sheetBoard, 2, "next card in the Lane"}}, handle: boardMoving(MoveDown)},
	{action: MoveUp, modes: []Mode{ModeBoard}, keys: slices.Concat(keys("k", "k/↑", "up", "↑"), emacsUp), help: "previous card", sheet: []sheetLine{{sheetBoard, 3, "previous card in the Lane"}}, handle: boardMoving(MoveUp)},
	{action: NextLane, modes: laneModes, keys: keys("right", "→"), help: "Lane", footer: laneModes, footerKey: "←→", sheet: []sheetLine{{sheetBoard, 5, "next Lane"}}, handle: laneMoving(NextLane, 1)},
	{action: PreviousLane, modes: laneModes, keys: keys("left", "←"), help: "previous Lane", sheet: []sheetLine{{sheetBoard, 4, "previous Lane"}}, handle: laneMoving(PreviousLane, -1)},
	{action: MoveCardLeft, modes: laneModes, keys: keys("<", "<"), help: "previous Lane", sheet: []sheetLine{{sheetBoard, 6, "move card to the previous Lane"}}, handle: cardMoving(-1)},
	{action: MoveCardRight, modes: laneModes, keys: keys(">", ">"), help: "move card", footer: laneModes, footerKey: "<>", sheet: []sheetLine{{sheetBoard, 7, "move card to the next Lane"}}, handle: cardMoving(1)},
	{action: SelectParent, modes: laneModes, keys: keys("^", "^", "shift+^", "^"), help: "parent", sheet: []sheetLine{{sheetBoard, 11, "select the parent's card"}}, handle: pure(selectParent)},
	{action: OpenLane, modes: []Mode{ModeBoard}, keys: keys("enter", "↵"), help: "open", footer: []Mode{ModeBoard}, sheet: []sheetLine{{sheetBoard, 8, "open the Lane view"}}, handle: pure(openLane)},
	// Jobs Page: the Project's Cron jobs, with the selected job's inspector beside them.
	{action: MoveDown, modes: []Mode{ModeJobs}, keys: slices.Concat(keys("j", "j/↓", "down", "↓"), emacsDown), help: "move", footer: []Mode{ModeJobs}, footerKey: "↑↓", sheet: []sheetLine{{sheetJobs, 1, "next job"}}, handle: pure(func(m Model) Model { return moveJob(m, MoveDown) })},
	{action: MoveUp, modes: []Mode{ModeJobs}, keys: slices.Concat(keys("k", "k/↑", "up", "↑"), emacsUp), help: "previous job", sheet: []sheetLine{{sheetJobs, 2, "previous job"}}, handle: pure(func(m Model) Model { return moveJob(m, MoveUp) })},
	{action: MoveFirst, modes: []Mode{ModeJobs}, keys: keys("g", "g", "home", "home"), help: "first job", sheet: []sheetLine{{sheetJobs, 3, "first job"}}, handle: pure(func(m Model) Model { return moveJob(m, MoveFirst) })},
	{action: MoveLast, modes: []Mode{ModeJobs}, keys: keys("shift+g", "G", "end", "end"), help: "last job", sheet: []sheetLine{{sheetJobs, 4, "last job"}}, handle: pure(func(m Model) Model { return moveJob(m, MoveLast) })},
	{action: PageUp, modes: []Mode{ModeJobs}, keys: pageUpKeys, help: "page up", handle: pure(func(m Model) Model { return moveJob(m, PageUp) })},
	{action: PageDown, modes: []Mode{ModeJobs}, keys: pageDownKeys, help: "page down", handle: pure(func(m Model) Model { return moveJob(m, PageDown) })},
	{action: FocusJobInspector, modes: []Mode{ModeJobs}, keys: keys("enter", "↵", "tab", "tab"), help: "inspect", footer: []Mode{ModeJobs}, footerKey: "↵", sheet: []sheetLine{{sheetJobs, 5, "inspect the job (definition and parent)"}}, handle: pure(focusInspector)},
	{action: ToggleArchivedJobs, modes: []Mode{ModeJobs}, keys: keys("a", "a"), help: "archived", footer: []Mode{ModeJobs}, sheet: []sheetLine{{sheetJobs, 6, "show or hide archived jobs"}}, handle: pure(toggleArchivedJobs)},
	{action: ReloadJobs, modes: []Mode{ModeJobs, ModeJobInspector}, keys: keys("ctrl+r", "ctrl+r", "meta+r", ""), help: "reload", sheet: []sheetLine{{sheetJobs, 7, "read the jobs again"}}, handle: pure(reloadJobs)},
	{action: OpenJobParent, modes: []Mode{ModeJobInspector}, keys: keys("enter", "↵"), help: "open parent", footer: []Mode{ModeJobInspector}, sheet: []sheetLine{{sheetJobs, 8, "open a native job's parent Issue"}}, handle: pure(openJobParent)},
	{action: ScrollDown, modes: []Mode{ModeJobInspector}, keys: slices.Concat(keys("down", "↓", "j", "j/↓"), emacsDown), help: "scroll", footer: []Mode{ModeJobInspector}, footerKey: "↑↓", sheet: []sheetLine{{sheetJobs, 9, "scroll the inspector"}}, handle: scrolling(ColumnDetail, ScrollLineDown)},
	{action: ScrollUp, modes: []Mode{ModeJobInspector}, keys: slices.Concat(keys("up", "↑", "k", "k/↑"), emacsUp), help: "scroll up", handle: scrolling(ColumnDetail, ScrollLineUp)},
	{action: PageDown, modes: []Mode{ModeJobInspector}, keys: pageDownKeys, help: "page", handle: scrolling(ColumnDetail, ScrollPageDown)},
	{action: PageUp, modes: []Mode{ModeJobInspector}, keys: pageUpKeys, help: "page up", handle: scrolling(ColumnDetail, ScrollPageUp)},
	{action: JobInspectorBack, modes: []Mode{ModeJobInspector}, keys: keys("escape", "esc", "tab", "tab", "left", "←"), help: "back", footer: []Mode{ModeJobInspector}, sheet: []sheetLine{{sheetJobs, 10, "back to the job list"}}, handle: pure(inspectorBack)},
	{action: RunsOfJob, modes: []Mode{ModeJobs, ModeJobInspector}, keys: keys("r", "r"), help: "runs", footer: []Mode{ModeJobs}, sheet: []sheetLine{{sheetJobs, 11, "open the Runs Page on the job's runs"}}, handle: runsOfJob},
	// Runs Page: the Project's Runs newest first, with the selected Run's detail beside them.
	{action: MoveDown, modes: []Mode{ModeRuns}, keys: slices.Concat(keys("j", "j/↓", "down", "↓"), emacsDown), help: "move", footer: []Mode{ModeRuns}, footerKey: "↑↓", sheet: []sheetLine{{sheetRuns, 1, "next run"}}, handle: pure(func(m Model) Model { return moveRun(m, MoveDown) })},
	{action: MoveUp, modes: []Mode{ModeRuns}, keys: slices.Concat(keys("k", "k/↑", "up", "↑"), emacsUp), help: "previous run", sheet: []sheetLine{{sheetRuns, 2, "previous run"}}, handle: pure(func(m Model) Model { return moveRun(m, MoveUp) })},
	{action: MoveFirst, modes: []Mode{ModeRuns}, keys: keys("g", "g", "home", "home"), help: "first run", sheet: []sheetLine{{sheetRuns, 3, "first run"}}, handle: pure(func(m Model) Model { return moveRun(m, MoveFirst) })},
	{action: MoveLast, modes: []Mode{ModeRuns}, keys: keys("shift+g", "G", "end", "end"), help: "last run", sheet: []sheetLine{{sheetRuns, 4, "last run"}}, handle: pure(func(m Model) Model { return moveRun(m, MoveLast) })},
	{action: PageUp, modes: []Mode{ModeRuns}, keys: pageUpKeys, help: "page up", handle: pure(func(m Model) Model { return moveRun(m, PageUp) })},
	{action: PageDown, modes: []Mode{ModeRuns}, keys: pageDownKeys, help: "page down", handle: pure(func(m Model) Model { return moveRun(m, PageDown) })},
	{action: FocusJobInspector, modes: []Mode{ModeRuns}, keys: keys("enter", "↵", "tab", "tab"), help: "detail", footer: []Mode{ModeRuns}, footerKey: "↵", sheet: []sheetLine{{sheetRuns, 5, "show the run's detail"}}, handle: inspectRun},
	{action: OpenRunIssue, modes: []Mode{ModeRuns, ModeRunInspector}, keys: keys("i", "i"), help: "issue", footer: []Mode{ModeRuns, ModeRunInspector}, sheet: []sheetLine{{sheetRuns, 6, "open the run's Issue"}}, handle: pure(openRunIssue)},
	{action: FilterRunsByJob, modes: []Mode{ModeRuns, ModeRunInspector}, keys: keys("f", "f"), help: "job filter", footer: []Mode{ModeRuns}, sheet: []sheetLine{{sheetRuns, 7, "show only the selected run's job, or all runs again"}}, handle: pure(toggleRunJobFilter)},
	{action: OlderRuns, modes: []Mode{ModeRuns}, keys: keys("m", "m"), help: "older", footer: []Mode{ModeRuns}, sheet: []sheetLine{{sheetRuns, 8, "read the next older runs"}}, handle: olderRuns},
	{action: ReloadRuns, modes: []Mode{ModeRuns, ModeRunInspector}, keys: keys("ctrl+r", "ctrl+r", "meta+r", ""), help: "reload", sheet: []sheetLine{{sheetRuns, 9, "read the runs again"}}, handle: pure(reloadRuns)},
	{action: ScrollDown, modes: []Mode{ModeRunInspector}, keys: slices.Concat(keys("down", "↓", "j", "j/↓"), emacsDown), help: "scroll", footer: []Mode{ModeRunInspector}, footerKey: "↑↓", sheet: []sheetLine{{sheetRuns, 10, "scroll the detail"}}, handle: scrolling(ColumnDetail, ScrollLineDown)},
	{action: ScrollUp, modes: []Mode{ModeRunInspector}, keys: slices.Concat(keys("up", "↑", "k", "k/↑"), emacsUp), help: "scroll up", handle: scrolling(ColumnDetail, ScrollLineUp)},
	{action: PageDown, modes: []Mode{ModeRunInspector}, keys: pageDownKeys, help: "page", handle: scrolling(ColumnDetail, ScrollPageDown)},
	{action: PageUp, modes: []Mode{ModeRunInspector}, keys: pageUpKeys, help: "page up", handle: scrolling(ColumnDetail, ScrollPageUp)},
	{action: RunInspectorBack, modes: []Mode{ModeRunInspector}, keys: keys("escape", "esc", "tab", "tab", "left", "←"), help: "back", footer: []Mode{ModeRunInspector}, sheet: []sheetLine{{sheetRuns, 11, "back to the run list"}}, handle: pure(runInspectorBack)},
	// Moving and opening in the Issue detail.
	{action: ScrollDown, modes: []Mode{ModeDetail}, keys: keys("down", "↓"), help: "scroll", footer: []Mode{ModeDetail}, footerKey: "↑↓", sheet: []sheetLine{{sheetChildren, 5, "scroll detail viewport"}}, handle: scrolling(ColumnDetail, ScrollLineDown)},
	{action: ScrollUp, modes: []Mode{ModeDetail}, keys: keys("up", "↑"), help: "scroll up", sheet: []sheetLine{{sheetChildren, 4, "scroll detail viewport"}}, handle: scrolling(ColumnDetail, ScrollLineUp)},
	{action: MoveDown, modes: []Mode{ModeDetail}, keys: slices.Concat(keys("j", "j/k"), emacsDown), help: "row", footer: []Mode{ModeDetail}, sheet: []sheetLine{{sheetChildren, 2, "move child cursor"}}, handle: cursorMoving(MoveDown)},
	{action: MoveUp, modes: []Mode{ModeDetail}, keys: slices.Concat(keys("k", "k"), emacsUp), help: "previous row", sheet: []sheetLine{{sheetChildren, 3, "move child cursor"}}, handle: cursorMoving(MoveUp)},
	{action: NextActivityTab, modes: []Mode{ModeDetail}, keys: keys("tab", "↹"), help: "tab", footer: []Mode{ModeDetail}, sheet: []sheetLine{{sheetDetail, 1, "next tab"}}, handle: pure(func(m Model) Model { return m.cycleTab(true) })},
	{action: PreviousActivityTab, modes: []Mode{ModeDetail}, keys: keys("shift+tab", "shift+tab"), help: "previous tab", sheet: []sheetLine{{sheetDetail, 2, "prev tab"}}, handle: pure(func(m Model) Model { return m.cycleTab(false) })},
	{action: DrillIn, modes: []Mode{ModeDetail}, keys: keys("enter", "↵"), help: "open", footer: []Mode{ModeDetail}, sheet: []sheetLine{{sheetDetail, 3, "jump to referenced issue or open the Cron job"}, {sheetChildren, 6, "open child"}}, handle: drillIn},
	{action: PageUp, modes: []Mode{ModeDetail}, keys: pageUpKeys, help: "scroll page up", handle: scrolling(ColumnDetail, ScrollPageUp)},
	{action: PageDown, modes: []Mode{ModeDetail}, keys: pageDownKeys, help: "page", footer: []Mode{ModeDetail}, footerKey: "pgup/pgdn", handle: scrolling(ColumnDetail, ScrollPageDown)},
	{action: MoveFirst, modes: []Mode{ModeDetail}, keys: keys("g", "g", "home", "home"), help: "first row", handle: cursorMoving(MoveFirst)},
	{action: MoveLast, modes: []Mode{ModeDetail}, keys: keys("shift+g", "G", "end", "end"), help: "last row", handle: cursorMoving(MoveLast)},
	// Moving and opening in the Menu sidebar.
	{action: MoveDown, modes: []Mode{ModeCollections}, keys: slices.Concat(keys("j", "j/↓", "down", "↓"), emacsDown), help: "next Collection", footer: []Mode{ModeCollections}, handle: sidebarMoving(MoveDown)},
	{action: MoveUp, modes: []Mode{ModeCollections}, keys: slices.Concat(keys("k", "k/↑", "up", "↑"), emacsUp), help: "previous Collection", footer: []Mode{ModeCollections}, handle: sidebarMoving(MoveUp)},
	{action: MoveFirst, modes: []Mode{ModeCollections}, keys: keys("g", "g", "home", "home"), help: "first Collection", handle: sidebarMoving(MoveFirst)},
	{action: MoveLast, modes: []Mode{ModeCollections}, keys: keys("shift+g", "G", "end", "end"), help: "last Collection", handle: sidebarMoving(MoveLast)},
	{action: PageUp, modes: []Mode{ModeCollections}, keys: pageUpKeys, help: "page up", handle: scrolling(ColumnCollections, ScrollPageUp)},
	{action: PageDown, modes: []Mode{ModeCollections}, keys: pageDownKeys, help: "page down", handle: scrolling(ColumnCollections, ScrollPageDown)},
	{action: DrillIn, modes: []Mode{ModeCollections}, keys: keys("enter", "enter"), help: "open Collection", footer: []Mode{ModeCollections}, handle: sidebarOpen},
	{action: MenuBack, modes: []Mode{ModeCollections}, keys: keys("escape", "esc"), help: "back", footer: []Mode{ModeCollections}, handle: pure(menuBack)},
	// Searching.
	{action: SearchEnter, modes: []Mode{ModeSearch}, keys: keys("enter", "enter"), help: "results", footer: []Mode{ModeSearch}, handle: pure(searchEnter)},
	{action: SearchBack, modes: []Mode{ModeSearch}, keys: keys("escape", "esc"), help: "cancel search", footer: []Mode{ModeSearch}, handle: searchBack},
	{action: MoveUp, modes: []Mode{ModeSearch}, keys: slices.Concat(keys("up", "up"), emacsUp), help: "previous result", handle: searchMoving(MoveUp)},
	{action: MoveDown, modes: []Mode{ModeSearch}, keys: slices.Concat(keys("down", "down"), emacsDown), help: "next result", handle: searchMoving(MoveDown)},
	{action: DeleteInput, modes: []Mode{ModeSearch}, keys: keys("backspace", "backspace"), help: "delete", handle: searchErasing(DeleteInput)},
	{action: ClearInput, modes: []Mode{ModeSearch}, keys: keys("ctrl+u", "ctrl+u"), help: "clear search", handle: searchErasing(ClearInput)},
	{action: PageUp, modes: searching, keys: pageUpKeys, help: "page up", handle: searchMoving(PageUp)},
	{action: PageDown, modes: searching, keys: pageDownKeys, help: "page down", handle: searchMoving(PageDown)},
	{action: MoveUp, modes: []Mode{ModeSearchResults}, keys: slices.Concat(keys("up", "up"), emacsUp), help: "previous result", footer: []Mode{ModeSearchResults}, handle: searchMoving(MoveUp)},
	{action: MoveDown, modes: []Mode{ModeSearchResults}, keys: slices.Concat(keys("down", "down"), emacsDown), help: "next result", footer: []Mode{ModeSearchResults}, handle: searchMoving(MoveDown)},
	{action: SearchEnter, modes: []Mode{ModeSearchResults}, keys: keys("enter", "enter"), help: "commit search", footer: []Mode{ModeSearchResults}, handle: pure(searchEnter)},
	{action: SearchBack, modes: []Mode{ModeSearchResults}, keys: keys("escape", "esc"), help: "edit query", footer: []Mode{ModeSearchResults}, handle: searchBack},
	{action: SearchQuery, modes: []Mode{ModeSearchResults}, keys: keys("/", "/"), help: "edit query", handle: pure(func(m Model) Model { m.Search.Results = false; return m })},
	// Acting on the Issues.
	{action: NewIssue, modes: issueModes, keys: keys("n", "n"), help: "new", footer: listAndBoard, sheet: []sheetLine{{sheetGraph, 12, "new issue (form)"}}, handle: func(m Model) (Model, []Cmd) { return openCreate(m, false) }},
	{action: SearchIssues, modes: listAndBoard, keys: keys("/", "/"), help: "search", footer: listAndBoard, sheet: []sheetLine{{sheetFilters, 1, "search"}}, handle: pure(openSearch)},
	{action: OpenFilterForm, modes: listAndBoard, keys: keys("f", "f"), help: "filter", footer: listAndBoard, sheet: []sheetLine{{sheetFilters, 3, "filter (form)"}}, handle: pure(openFilterForm)},
	{action: CycleStatus, modes: listAndBoard, keys: keys("s", "s"), help: "status", footer: listAndBoard, sheet: []sheetLine{{sheetFilters, 2, "cycle status filter"}}, handle: cycleStatus},
	{action: ToggleIssueView, modes: []Mode{ModeList}, keys: keys("v", "v"), help: "view", footer: []Mode{ModeList}, sheet: []sheetLine{{sheetGraph, 13, "toggle flat/nested view"}}, handle: hierarchy(ToggleIssueView)},
	{action: ClearFilters, modes: listAndBoard, keys: keys("c", "c"), help: "clear", footer: listAndBoard, sheet: []sheetLine{{sheetFilters, 4, "clear filters"}}, handle: clearFilters},
	{action: EditBody, modes: issueModes, keys: keys("e", "e"), help: "edit", footer: []Mode{ModeDetail}, sheet: []sheetLine{{sheetDetail, 5, "edit body"}}, handle: pure(func(m Model) Model { return startDraft(m, false) })},
	{action: AddComment, modes: []Mode{ModeDetail}, keys: keys("c", "c"), help: "comment", footer: []Mode{ModeDetail}, sheet: []sheetLine{{sheetDetail, 6, "new comment"}}, handle: pure(func(m Model) Model { return startDraft(m, true) })},
	{action: ReplyAction, modes: []Mode{ModeDetail}, keys: keys("shift+r", "R"), help: "reply to comment", sheet: []sheetLine{{sheetDetail, 15, "reply to comment"}}, handle: pure(startReply)},
	{action: AddLabelAction, modes: issueModes, keys: keys("+", "+"), help: "label", footer: boardAndDetail, sheet: []sheetLine{{sheetDetail, 8, "add label"}}, handle: fieldFor(AddLabelAction)},
	{action: RemoveLabelAction, modes: issueModes, keys: keys("-", "-"), help: "unlabel", footer: boardAndDetail, sheet: []sheetLine{{sheetDetail, 9, "remove label"}}, handle: fieldFor(RemoveLabelAction)},
	{action: AttachFileAction, modes: issueModes, keys: keys("u", "u"), help: "attach", footer: []Mode{ModeDetail}, sheet: []sheetLine{{sheetDetail, 10, "attach file"}}, handle: fieldFor(AttachFileAction)},
	{action: DetachFileAction, modes: issueModes, keys: keys("shift+u", "U"), help: "detach", footer: []Mode{ModeDetail}, sheet: []sheetLine{{sheetDetail, 11, "remove attachment"}}, handle: fieldFor(DetachFileAction)},
	{action: AssignOwnerAction, modes: issueModes, keys: keys("a", "a"), help: "owner", footer: boardAndDetail, sheet: []sheetLine{{sheetDetail, 12, "assign owner"}}, handle: fieldFor(AssignOwnerAction)},
	{action: ClearOwnerAction, modes: issueModes, keys: keys("shift+a", "A"), help: "unassign", footer: boardAndDetail, sheet: []sheetLine{{sheetDetail, 13, "clear owner"}}, handle: fieldFor(ClearOwnerAction)},
	{action: OpenClose, modes: issueModes, keys: keys("x", "x"), help: "close", footer: issueModes, sheet: []sheetLine{{sheetGraph, 14, "close"}}, handle: pure(openClose)},
	{action: ReopenAction, modes: issueModes, keys: keys("r", "r"), help: "reopen", footer: issueModes, sheet: []sheetLine{{sheetGraph, 15, "reopen"}}, handle: reopenIssue},
	{action: SetParentAction, modes: issueModes, keys: keys("p", "p"), help: "parent", footer: []Mode{ModeDetail}, sheet: []sheetLine{{sheetDetail, 7, "set parent"}}, handle: fieldFor(SetParentAction)},
	{action: PriorityAction, modes: issueModes, keys: keys("!", "!"), help: "priority", footer: issueModes, sheet: []sheetLine{{sheetDetail, 14, "set priority"}}, handle: fieldFor(PriorityAction)},
	{action: OpenDates, modes: issueModes, keys: keys("d", "d"), help: "dates", footer: issueModes, sheet: []sheetLine{{sheetDetail, 17, "set scheduled date and deadline (Dates card)"}}, handle: pure(openDates)},
	{action: OpenRecurrenceAction, modes: issueModes, keys: keys("shift+o", "O"), help: "make or change recurrence", sheet: []sheetLine{{sheetDetail, 16, "make or change recurrence"}}, handle: pure(openRecurrence)},
	{action: NewChild, modes: issueModes, keys: keys("shift+n", "N"), help: "child", footer: issueModes, sheet: []sheetLine{{sheetChildren, 1, "new child"}}, handle: func(m Model) (Model, []Cmd) { return openCreate(m, true) }},
	// Layout, back, help and quit.
	{action: ToggleDetail, modes: []Mode{ModeList, ModeLane, ModeDetail}, keys: keys("shift+l", "L"), help: "layout", footer: []Mode{ModeList, ModeLane, ModeDetail}, sheet: []sheetLine{{sheetGlobal, 6, "toggle layout"}}, handle: pure(Model.toggleDetail)},
	{action: DrillOut, modes: []Mode{ModeList, ModeBoard, ModeJobs, ModeRuns}, keys: keys("escape", "esc"), help: "Menu", footer: []Mode{ModeList, ModeBoard, ModeJobs, ModeRuns}, sheet: []sheetLine{{sheetGlobal, 5, "menu"}}, handle: drillOut},
	{action: DrillOut, modes: []Mode{ModeLane}, keys: keys("escape", "esc"), help: "back", footer: []Mode{ModeLane}, sheet: []sheetLine{{sheetBoard, 10, "back to the Board, or the Menu in a narrow Pane"}}, handle: laneBack},
	{action: DrillOut, modes: []Mode{ModeDetail}, keys: keys("escape", "esc", "left", "←"), help: "back", footer: []Mode{ModeDetail}, sheet: []sheetLine{{sheetDetail, 4, "back"}}, handle: drillOut},
	{action: OpenProjects, modes: []Mode{ModeCollections, ModeList, ModeBoard, ModeLane, ModeDetail, ModeJobs, ModeJobInspector, ModeRuns, ModeRunInspector}, keys: keys("shift+p", "P"), help: "projects", sheet: []sheetLine{{sheetGlobal, 4, "projects"}}, handle: pure(openProjects)},
	{action: OpenInbox, modes: []Mode{ModeCollections, ModeList, ModeBoard, ModeLane, ModeDetail, ModeJobs, ModeJobInspector, ModeRuns, ModeRunInspector}, keys: keys("shift+i", "I"), help: "inbox", sheet: []sheetLine{{sheetGlobal, 3, "inbox"}}, handle: openInbox},
	{action: RetryAction, modes: []Mode{ModeError}, keys: keys("ctrl+r", "ctrl+r"), help: "retry daemon", footer: []Mode{ModeError}, handle: func(m Model) (Model, []Cmd) { return updateLive(m, Retry{}) }},
	{action: CloseHelp, modes: []Mode{ModeHelp}, keys: keys("escape", "esc"), help: "close help", footer: []Mode{ModeHelp}, handle: pure(func(m Model) Model { m.HelpOpen = false; return m })},
	{action: ToggleHelp, keys: keys("?", "?", "shift+?", "?"), help: "help", footer: global, sheet: []sheetLine{{sheetGlobal, 1, "help"}}, handle: pure(func(m Model) Model { m.HelpOpen = !m.HelpOpen; return m })},
	{action: QuitAction, keys: keys("q", "q", "ctrl+c", "ctrl+c"), help: "quit", footer: global, sheet: []sheetLine{{sheetGlobal, 2, "quit"}}, handle: pure(func(m Model) Model { m.Quitting = true; return m })},
}

// Keymap is every binding: the Popup cards' keys, then the main-surface actions' keys, expanded from
// mainActions. Empty Mode is global; otherwise the binding is active only in that mode.
var Keymap []Binding

func init() {
	mainActions = append(mainActions, workflowActions()...)
	mainActions = append(mainActions, pageActions()...)
	Keymap = append([]Binding(nil), cardBindings...)
	for i := range mainActions {
		entry := &mainActions[i]
		for _, mode := range orGlobal(entry.modes) {
			for j, key := range entry.keys {
				help := entry.help
				if key.Label == "" {
					help += headlessNote
				}
				label := key.Label
				if j == 0 && entry.footerKey != "" {
					label = entry.footerKey
				}
				Keymap = append(Keymap, Binding{Chord: key.Chord, Key: label, Action: entry.action, Help: help, Mode: mode, ShowInFooter: slices.Contains(entry.footer, mode) && j == 0, entry: entry})
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
