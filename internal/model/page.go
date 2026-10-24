package model

import "slices"

// Page is one of the Pane's top-level views, chosen from the Page bar. Model.Page is the current one.
//
// Adding a Page is one entry in pageDefs (its label, key, what its second column is, entry, clear and reset, its mode, the messages
// it owns, its list's movement keys and inspector's scroll keys, and its list node), a Page constant, an Action constant, and a
// state struct on Model for what the Page keeps while another is shown (selection, scroll, filters). The Page bar, the keys,
// the click targets, the help sheet line, the mode, the message routing, the Project reset and the scroll target all read the table.
type Page uint8

const (
	// PageIssues is the Issue list with the Issue detail, the Pane's main surface. Its state is the list's: the Collection,
	// the Issues, the selection and the filters, which the Board Page shares because it draws the same Issues.
	PageIssues Page = iota
	// PageBoard is the Project's Issues in Lanes; its state is Model.Board.
	PageBoard
	// PageJobs is the Project's Cron jobs; its state is Model.Jobs.
	PageJobs
	// PageRuns is the Project's Runs, newest first; its state is Model.Runs.
	PageRuns
	// PageWorkflows is the Project's kata workflows beside an inspector; its state is Model.Workflows.
	PageWorkflows
)

// SecondColumn says what a Page's second column is. A Page declares it in pageDefs, and the one leave rule reads it.
type SecondColumn uint8

const (
	// SecondIssueDetail is a second column that is the Issue detail (the Board and Workflows Pages, and the Issues Page itself):
	// focus on it survives leaving the Page.
	SecondIssueDetail SecondColumn = iota
	// SecondInspector is a second column that is the Page's own inspector (the Jobs and Runs Pages): leaving always returns focus
	// from it to the Issue list. The column's focus value is still ColumnDetail.
	SecondInspector
)

// SecondColumn is what the current Page's second column is.
func (m Model) SecondColumn() SecondColumn { return pageDefs[m.Page].second }

// ListNode names which of the Pane's list nodes a Page's list is, for the scroll keys: the view maps it to the node it draws.
type ListNode uint8

const (
	// ListNodeIssues is the Issue list's node, which the Lane view scrolls too.
	ListNodeIssues ListNode = iota
	// ListNodeJobs is the Jobs Page's list node.
	ListNodeJobs
	// ListNodeRuns is the Runs Page's list node.
	ListNodeRuns
	// ListNodeWorkflows is the Workflows Page's list node.
	ListNodeWorkflows
)

// ListNode is the current Page's list node.
func (m Model) ListNode() ListNode { return pageDefs[m.Page].list }

type pageDef struct {
	page Page
	// id names the Page bar's node; label is what it reads; chord is the key, which a click on the label sends too.
	id, label, chord string
	action           Action
	// second is what the Page's second column is; leavePage reads it.
	second SecondColumn
	// needs is the Page as the notice words it when the Pane has no Project; "" for the Issues Page, which needs none.
	needs string
	// enter is the Page's own work once switchPage has made it current (see there); nil for the Issues Page.
	enter func(Model) (Model, []Cmd)
	// clear undoes the Page's own state when it is left: its loading markers, inspector and mode. nil when there is nothing to undo.
	clear func(Model) Model
	// mode is the keymap mode the Page is in while it is the current Page, from its own state (focus, view); "" leaves the mode to the
	// Menu or list by focus. nil for a Page with no mode of its own. CurrentMode asks it after the popup card, help, search and error.
	mode func(Model) Mode
	// update takes the messages the Page owns, whichever Page is current (an answer can land after the Page is left); handled says it
	// did. nil for a Page that owns none. Update asks every Page in table order, ahead of the shared messages.
	update func(Model, Msg) (Model, []Cmd, bool)
	// filledFor is the Project, by uid, the Page's state was filled for; reset empties that state for the Pane's Project when it
	// differs, and records it. Both nil for a Page with no state of its own per Project.
	filledFor func(Model) string
	reset     func(Model, string) Model
	// keymap is the Page's contribution to the main actions, in the order its footer reads: its generated movement and scroll keys
	// and its own rows. Rows several Pages share are values the declarations reference. keymapRows assembles them in table order.
	keymap []keySegment
	// moves is the Page's list's six movement keys, generated into the main actions; the zero value binds none.
	moves pageMoves
	// scroll is the Page's inspector's four scroll keys, generated into the main actions; the zero value binds none.
	scroll pageScroll
	// list is the node the scroll keys of the Page's list go to.
	list ListNode
}

// pageMoves declares a list's movement keys: down, up, first, last, page up and page down, bound in mode and moving the selection
// with move. The footer shows down as ↑↓ and the help words name the noun (previous Issue, first run).
type pageMoves struct {
	mode Mode
	noun string
	move func(Model, Action) Model
	// sheet is the help sheet line of each key, in the order down, up, first, last, page up, page down; a zero line lists none.
	sheet [6]sheetLine
}

// pageScroll declares an inspector's scroll keys, bound in mode: scroll down and up by a line, page down and up. sheet is the line
// scroll down is listed on.
type pageScroll struct {
	mode  Mode
	sheet sheetLine
}

// issueMoves is the Issue list's movement keys, in the mode that shows the list: the Issues Page's list and the Board Page's Lane view.
func issueMoves(mode Mode) pageMoves {
	return pageMoves{mode: mode, noun: "Issue", move: moveSelection, sheet: [6]sheetLine{
		{sheetGraph, 2, "down"}, {sheetGraph, 1, "up"}, {sheetGraph, 5, "first"}, {sheetGraph, 6, "last"}, {sheetGraph, 3, "page up"}, {sheetGraph, 4, "page down"},
	}}
}

// pageDefs lists the Pages in Page bar order, indexed by Page. It is filled in init, as the Pages' rows reach back to the table.
var pageDefs []pageDef

// keySegment is one run of a Page's main actions.
type keySegment func(pageDef) []actionEntry

// movementKeys is the Page's generated movement rows.
func movementKeys(def pageDef) []actionEntry { return pageMoveActions(def) }

// scrollKeys is the Page's generated inspector scroll rows.
func scrollKeys(def pageDef) []actionEntry { return pageScrollActions(def) }

// rowsOf is a fixed run of rows.
func rowsOf(rows []actionEntry) keySegment { return func(pageDef) []actionEntry { return rows } }

// keymapRows is every Page's main actions, in table order.
func keymapRows() []actionEntry {
	var rows []actionEntry
	for _, def := range pageDefs {
		for _, segment := range def.keymap {
			rows = append(rows, segment(def)...)
		}
	}
	return rows
}

func init() {
	pageDefs = []pageDef{
		{page: PageIssues, id: "issues", label: "Issues", chord: "1", action: ShowIssuesPage, moves: issueMoves(ModeList),
			keymap: []keySegment{movementKeys}},
		{page: PageBoard, id: "board", label: "Board", chord: "2", action: ShowBoardPage, needs: "Board", second: SecondIssueDetail, enter: enterBoard, clear: clearBoard,
			mode: boardMode, update: updateBoard, moves: issueMoves(ModeLane),
			// listRows are the Issue list's and the Lane view's rows, which follow the Board's movement keys.
			keymap: []keySegment{movementKeys, rowsOf(listRows)}},
		{page: PageJobs, id: "jobs", label: "Jobs", chord: "3", action: ShowJobsPage, needs: "Jobs Page", second: SecondInspector, enter: enterJobs, clear: clearJobs,
			filledFor: jobsFilledFor, reset: resetJobs, mode: inspectorMode(ModeJobs, ModeJobInspector), update: updateJobs, list: ListNodeJobs,
			moves:  pageMoves{mode: ModeJobs, noun: "job", move: moveJob, sheet: [6]sheetLine{{sheetJobs, 1, "next job"}, {sheetJobs, 2, "previous job"}, {sheetJobs, 3, "first job"}, {sheetJobs, 4, "last job"}}},
			scroll: pageScroll{mode: ModeJobInspector, sheet: sheetLine{sheetJobs, 9, "scroll the inspector"}},
			keymap: []keySegment{movementKeys, rowsOf(jobsRows), scrollKeys, rowsOf(jobsRowsAfterScroll)}},
		{page: PageRuns, id: "runs", label: "Runs", chord: "4", action: ShowRunsPage, needs: "Runs Page", second: SecondInspector, enter: enterRuns, clear: clearRuns,
			filledFor: runsFilledFor, reset: resetRuns, mode: inspectorMode(ModeRuns, ModeRunInspector), update: updateRuns, list: ListNodeRuns,
			moves:  pageMoves{mode: ModeRuns, noun: "run", move: moveRun, sheet: [6]sheetLine{{sheetRuns, 1, "next run"}, {sheetRuns, 2, "previous run"}, {sheetRuns, 3, "first run"}, {sheetRuns, 4, "last run"}}},
			scroll: pageScroll{mode: ModeRunInspector, sheet: sheetLine{sheetRuns, 10, "scroll the detail"}},
			// mainRows are the Runs inspector's back key and every row after it, which no Page owns (layout, back, Menu, help, quit).
			keymap: []keySegment{rowsOf(runsRowsBeforeMovement), movementKeys, rowsOf(runsRows), scrollKeys, rowsOf(mainRows)}},
		{page: PageWorkflows, id: "workflows", label: "Workflows", chord: "5", action: ShowWorkflowsPage, needs: "Workflows Page", second: SecondIssueDetail, enter: enterWorkflows, clear: clearWorkflows,
			filledFor: workflowsFilledFor, reset: resetWorkflows, mode: workflowsMode, update: updateWorkflows, list: ListNodeWorkflows,
			moves:  pageMoves{mode: ModeWorkflows, noun: "workflow", move: moveWorkflow, sheet: [6]sheetLine{{sheetWorkflows, 1, "next workflow"}, {sheetWorkflows, 2, "previous workflow"}}},
			keymap: []keySegment{movementKeys, rowsOf(workflowActions())}},
	}
	buildKeymap()
}

// pageMode is the current Page's mode from its declaration, "" when the Page has none or leaves it to the focus.
func pageMode(m Model) Mode {
	if mode := pageDefs[m.Page].mode; mode != nil {
		return mode(m)
	}
	return ""
}

// updatePages offers a message to each Page's update hook in table order and stops at the first that handles it.
func updatePages(m Model, msg Msg) (Model, []Cmd, bool) {
	for _, def := range pageDefs {
		if def.update != nil {
			if next, cmds, handled := def.update(m, msg); handled {
				return next, cmds, true
			}
		}
	}
	return m, nil, false
}

// boardMode is the Board Page's mode: the Board, or the Lane view unless the detail has focus.
func boardMode(m Model) Mode {
	switch {
	case m.BoardShown():
		return ModeBoard
	case m.LaneViewShown() && m.Focus != ColumnDetail:
		return ModeLane
	}
	return ""
}

// inspectorMode is the mode of a Page whose second column is its own inspector: the inspector's when it has focus, the list's otherwise.
func inspectorMode(list, inspector Mode) func(Model) Mode {
	return func(m Model) Mode {
		if m.Focus == ColumnDetail {
			return inspector
		}
		return list
	}
}

// workflowsMode is the Workflows Page's list mode; the detail beside it takes the Issue detail's by focus.
func workflowsMode(m Model) Mode {
	if m.Focus == ColumnList {
		return ModeWorkflows
	}
	return ""
}

// pageMoveActions is the main actions of a Page's movement keys, from its declaration.
func pageMoveActions(def pageDef) []actionEntry {
	p := def.moves
	if p.mode == "" {
		return nil
	}
	modes := []Mode{p.mode}
	rows := [6]struct {
		action Action
		keys   []Key
		help   string
	}{
		{MoveDown, slices.Concat(keys("j", "j/↓", "down", "↓"), emacsDown), "move"},
		{MoveUp, slices.Concat(keys("k", "k/↑", "up", "↑"), emacsUp), "previous " + p.noun},
		{MoveFirst, keys("g", "g", "home", "home"), "first " + p.noun},
		{MoveLast, keys("shift+g", "G", "end", "end"), "last " + p.noun},
		{PageUp, pageUpKeys, "page up"},
		{PageDown, pageDownKeys, "page down"},
	}
	entries := make([]actionEntry, len(rows))
	for i, row := range rows {
		entry := actionEntry{action: row.action, modes: modes, keys: row.keys, help: row.help,
			handle: pure(func(m Model) Model { return p.move(m, row.action) })}
		if p.sheet[i].text != "" {
			entry.sheet = []sheetLine{p.sheet[i]}
		}
		if row.action == MoveDown {
			entry.footer, entry.footerKey = modes, "↑↓"
		}
		entries[i] = entry
	}
	return entries
}

// pageScrollActions is the main actions of a Page's inspector's scroll keys, from its declaration.
func pageScrollActions(def pageDef) []actionEntry {
	p := def.scroll
	if p.mode == "" {
		return nil
	}
	modes := []Mode{p.mode}
	return []actionEntry{
		{action: ScrollDown, modes: modes, keys: slices.Concat(keys("down", "↓", "j", "j/↓"), emacsDown), help: "scroll", footer: modes, footerKey: "↑↓",
			sheet: []sheetLine{p.sheet}, handle: scrolling(ColumnDetail, ScrollLineDown)},
		{action: ScrollUp, modes: modes, keys: slices.Concat(keys("up", "↑", "k", "k/↑"), emacsUp), help: "scroll up", handle: scrolling(ColumnDetail, ScrollLineUp)},
		{action: PageDown, modes: modes, keys: pageDownKeys, help: "page", handle: scrolling(ColumnDetail, ScrollPageDown)},
		{action: PageUp, modes: modes, keys: pageUpKeys, help: "page up", handle: scrolling(ColumnDetail, ScrollPageUp)},
	}
}

// pageModes are the modes where the Page keys work: wherever the main surface has the keyboard and no text is being typed.
var pageModes = []Mode{ModeList, ModeBoard, ModeLane, ModeDetail, ModeCollections, ModeJobs, ModeJobInspector, ModeRuns, ModeRunInspector, ModeWorkflows}

// PageBarEntry is one entry of the Page bar.
type PageBarEntry struct {
	ID, Label, Chord string
	Current          bool
}

// PageBar is the Page bar's entries, left to right.
func (m Model) PageBar() []PageBarEntry {
	entries := make([]PageBarEntry, len(pageDefs))
	for i, def := range pageDefs {
		entries[i] = PageBarEntry{ID: def.id, Label: def.label, Chord: def.chord, Current: def.page == m.Page}
	}
	return entries
}

// showPage is a Page's key: it leaves the current Page (back to Issues) and enters the target, and does nothing when the
// target is already current.
func showPage(target Page) func(Model) (Model, []Cmd) {
	return func(m Model) (Model, []Cmd) {
		if m.Page == target {
			return m, nil
		}
		before := liveQuery(m)
		m, cmds := switchPage(m, target)
		if !sameCollectionQuery(before, liveQuery(m)) { // the Board reads without the status filter, the Issues Page with it
			var more []Cmd
			m, more = snapshotChanged(m)
			cmds = append(cmds, more...)
		}
		return m, cmds
	}
}

// switchPage is the one way onto a Page, the Issues Page included. It leaves the current Page, and for a Page that acts on the
// Pane's Project it then resolves the Project (see enterProjectPage), makes the Page current with the detail column's hidden
// flag remembered, the column hidden and focus on the list, and runs the Page's own enter. With no Project it stops on the Issues
// Page with the Menu's Projects open.
func switchPage(m Model, target Page) (Model, []Cmd) {
	m = m.leavePage()
	def := pageDefs[target]
	if def.enter == nil {
		return m, nil
	}
	m, cmds, ok := enterProjectPage(m, def.needs)
	if !ok {
		return m, nil
	}
	m.Page = target
	m.pageDetailHidden = m.DetailHidden()
	m.hidden = true
	m.Focus = ColumnList
	m, own := def.enter(m)
	return m, append(cmds, own...)
}

// leavePage is the one leave rule: it lands on the Issues Page from whichever Page is current (and does nothing on the Issues
// Page). The Page's own state is cleared. The detail column's hidden flag goes back to what it was before the Page, and the
// keys follow what the Page declares its second column to be: an inspector returns focus to the list, while the Issue detail
// keeps focus when it has it and then stays shown. The list scrolls to the selection and the selected Issue's detail loads again,
// as it was not kept fresh while the Page hid it.
func (m Model) leavePage() Model {
	if m.Page == PageIssues {
		return m
	}
	def := pageDefs[m.Page]
	if def.clear != nil {
		m = def.clear(m)
	}
	m.Page = PageIssues
	switch {
	case def.second == SecondInspector:
		if m.Focus == ColumnDetail {
			m.Focus = ColumnList
		}
		m.hidden = m.pageDetailHidden
	case m.Focus != ColumnDetail:
		m.hidden = m.pageDetailHidden
	}
	m.stale = true
	return m.revealInList(m.SelectedUID).WithVisibleSelection()
}

// pageActions are the Page keys as main-surface entries, so keys, help sheet and click targets come from one place.
func pageActions() []actionEntry {
	entries := make([]actionEntry, len(pageDefs))
	for i, def := range pageDefs {
		entries[i] = actionEntry{action: def.action, modes: pageModes, keys: keys(def.chord, def.chord), help: def.label + " page",
			sheet: []sheetLine{{sheetGlobal, 7, "switch Page"}}, handle: showPage(def.page)}
	}
	return entries
}
