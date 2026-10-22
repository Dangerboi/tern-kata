package model

// Page is one of the Pane's top-level views, chosen from the Page bar. Model.Page is the current one.
//
// Adding a Page is one entry in pageDefs (its label, key and entry), a Page constant, an Action constant, and a state
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

type pageDef struct {
	page Page
	// id names the Page bar's node; label is what it reads; chord is the key, which a click on the label sends too.
	id, label, chord string
	action           Action
	// enter shows the page. It may refuse (the Board Page without a Project) and then leaves the Issues Page current.
	// leave is run on the Page being left and lands on the Issues Page; nil when there is nothing to undo.
	enter func(Model) (Model, []Cmd)
	leave func(Model) Model
}

// pageDefs lists the Pages in Page bar order, indexed by Page.
var pageDefs = []pageDef{
	{page: PageIssues, id: "issues", label: "Issues", chord: "1", action: ShowIssuesPage},
	{page: PageBoard, id: "board", label: "Board", chord: "2", action: ShowBoardPage, enter: enterBoard, leave: Model.boardOff},
	{page: PageJobs, id: "jobs", label: "Jobs", chord: "3", action: ShowJobsPage, enter: enterJobs, leave: Model.jobsOff},
	{page: PageRuns, id: "runs", label: "Runs", chord: "4", action: ShowRunsPage, enter: enterRunsPage, leave: Model.runsOff},
	{page: PageWorkflows, id: "workflows", label: "Workflows", chord: "5", action: ShowWorkflowsPage, enter: enterWorkflows, leave: Model.workflowsOff},
}

// pageModes are the modes where the Page keys work: wherever the main surface has the keyboard and no text is being typed.
var pageModes = []Mode{ModeList, ModeBoard, ModeLane, ModeDetail, ModeCollections, ModeJobs, ModeJobInspector, ModeRuns, ModeRunInspector, ModeWorkflows}

// PageTab is one entry of the Page bar.
type PageTab struct {
	ID, Label, Chord string
	Current          bool
}

// PageBar is the Page bar's entries, left to right.
func (m Model) PageBar() []PageTab {
	tabs := make([]PageTab, len(pageDefs))
	for i, def := range pageDefs {
		tabs[i] = PageTab{ID: def.id, Label: def.label, Chord: def.chord, Current: def.page == m.Page}
	}
	return tabs
}

// showPage is a Page's key: it leaves the current Page (back to Issues) and enters the target, and does nothing when the
// target is already current.
func showPage(target Page) func(Model) (Model, []Cmd) {
	return func(m Model) (Model, []Cmd) {
		if m.Page == target {
			return m, nil
		}
		if leave := pageDefs[m.Page].leave; leave != nil {
			m = leave(m)
		}
		if enter := pageDefs[target].enter; enter != nil {
			return enter(m)
		}
		return m, nil
	}
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
