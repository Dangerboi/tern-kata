package model

// Page is one of the Pane's top-level views, chosen from the Page bar. Model.Page is the current one.
//
// Adding a Page is one entry in pageDefs (its label, key, what its second column is, entry and clear), a Page constant, an Action constant, and a state
// struct on Model for what the Page keeps while another is shown (selection, scroll, filters). The Page bar, the keys,
// the click targets and the help sheet line all read the table.
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
}

// pageDefs lists the Pages in Page bar order, indexed by Page.
var pageDefs = []pageDef{
	{page: PageIssues, id: "issues", label: "Issues", chord: "1", action: ShowIssuesPage},
	{page: PageBoard, id: "board", label: "Board", chord: "2", action: ShowBoardPage, needs: "Board", second: SecondIssueDetail, enter: enterBoard, clear: clearBoard},
	{page: PageJobs, id: "jobs", label: "Jobs", chord: "3", action: ShowJobsPage, needs: "Jobs Page", second: SecondInspector, enter: enterJobs, clear: clearJobs},
	{page: PageRuns, id: "runs", label: "Runs", chord: "4", action: ShowRunsPage, needs: "Runs Page", second: SecondInspector, enter: enterRuns, clear: clearRuns},
	{page: PageWorkflows, id: "workflows", label: "Workflows", chord: "5", action: ShowWorkflowsPage, needs: "Workflows Page", second: SecondIssueDetail, enter: enterWorkflows, clear: clearWorkflows},
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
