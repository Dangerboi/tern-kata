package model

import (
	"strconv"
	"strings"
)

// The Jobs, Runs and Workflows Pages search their rows with "/". The search is held in the Model, not in Filters: it narrows the
// rows already read, as the case-insensitive substring of a row's words, and it ends with the Page.

// PageSearchState is the search of the current Jobs, Runs or Workflows Page. The input is shown while it is Open or Text is not
// empty; Open says the keys type into it.
type PageSearchState struct {
	Open  bool
	Text  string
	Caret Caret
}

// PageSearchShown says the Page draws its search input above the list.
func (m Model) PageSearchShown() bool { return m.PageSearch.Open || m.PageSearch.Text != "" }

// PageSearching says the rows of the current Page are narrowed by a search text.
func (m Model) PageSearching() bool { return m.searchingPage(m.Page) }

// EscEndsSearch says esc on the current Page ends its search now: the rows are narrowed and no inner layer is open for esc to
// close first.
func (m Model) EscEndsSearch() bool {
	return m.PageSearching() && !m.innerEscOpen()
}

// innerEscOpen says the current Page has a layer open that esc closes before it ends a search or goes back: the Workflows
// inspector. A Page gaining such a layer is one case here and one close in its own esc handler.
func (m Model) innerEscOpen() bool {
	return m.Page == PageWorkflows && m.Workflows.Inspecting
}

// searchingPage says page is the current Page and its rows are narrowed by a search text.
func (m Model) searchingPage(page Page) bool {
	return m.Page == page && m.PageSearch.Text != "" && pageDefs[page].settleSearch != nil
}

// PageSearchNoun is what the current Page lists, for the search input's placeholder and the empty result.
func (m Model) PageSearchNoun() string { return pageDefs[m.Page].id }

// matchesSearch says a row with these words is listed under the current search: kata's match, the query is a case-insensitive
// substring of the row's words.
func (m Model) matchesSearch(words ...string) bool {
	return strings.Contains(strings.ToLower(strings.Join(words, " ")), strings.ToLower(m.PageSearch.Text))
}

func openPageSearch(m Model) Model {
	m.PageSearch.Open, m.PageSearch.Caret = true, Caret{}
	return m
}

// pageSearchKeep is enter in the input: the keys go back to the list, which stays narrowed.
func pageSearchKeep(m Model) Model {
	m.PageSearch.Open = false
	return m
}

// clearPageSearch ends the search: every row comes back.
func clearPageSearch(m Model) Model {
	if m.PageSearch == (PageSearchState{}) {
		return m
	}
	row := m.selectedRow()
	m.PageSearch = PageSearchState{}
	return m.settlePageSelection(row)
}

// pageSearchEdit sets the search text, as typing, backspace and ctrl+u do.
func (m Model) pageSearchEdit(text string) Model {
	row := m.selectedRow()
	m.PageSearch.Text = text
	return m.settlePageSelection(row)
}

// settlePageSelection keeps the current Page's selection on a row it lists after the search changed them. row is where the
// selected workflow stood, which a workflow takes its replacement from; a job or a Run falls back to the first row.
func (m Model) settlePageSelection(row int) Model {
	if settle := pageDefs[m.Page].settleSearch; settle != nil {
		return settle(m, row)
	}
	return m
}

func pageSearchErasing(action Action) func(Model) (Model, []Cmd) {
	return pure(func(m Model) Model {
		text, edited := eraseText(m.PageSearch.Text, &m.PageSearch.Caret, "", action)
		if !edited || text == m.PageSearch.Text {
			return m
		}
		return m.pageSearchEdit(text)
	})
}

// pageSearchMoving is a row or page key in the input: it moves through the rows the search lists.
func pageSearchMoving(action Action) func(Model) (Model, []Cmd) {
	return pure(func(m Model) Model { return pageDefs[m.Page].moves.move(m, action) })
}

// updatePageSearchText types a key into the search.
func updatePageSearchText(m Model, msg KeyMsg) (Model, []Cmd) {
	text, edited := editText(m.PageSearch.Text, &m.PageSearch.Caret, "", false, msg)
	if !edited { // the caret may still have moved
		return m, nil
	}
	return m.pageSearchEdit(text), nil
}

// pageSearchEscape is esc on a Page's list: with a search set it ends the search, else it goes back as esc does.
func pageSearchEscape(m Model) (Model, []Cmd) {
	if m.PageSearching() {
		return clearPageSearch(m), nil
	}
	return drillOut(m)
}

// jobSearchWords are the words of a job's row that the search reads: its name, state, kind, schedule, profile and last run.
func (m Model) jobSearchWords(job CronJob) []string {
	return []string{job.Name, m.JobPause(job).StateText(), job.Kind(), triggerWords(job.Definition.Trigger), job.Definition.Profile, m.lastActivity(job, m.Now)}
}

// runSearchWords are the words of a Run's row that the search reads.
func runSearchWords(row RunRow) []string {
	return []string{row.State, row.Name, row.Issue, row.Executor, row.Start, row.Duration, row.Reason, row.Hold, row.Summary}
}

// workflowSearchWords are the words of a workflow's row that the search reads: its name, steps, the jobs that use it and its
// latest run.
func (m Model) workflowSearchWords(w Workflow) []string {
	words := []string{w.Name, strconv.Itoa(len(w.Steps)) + " steps"}
	if w.Archived {
		words = append(words, "archived")
	}
	for _, job := range m.WorkflowJobs(w.UID) {
		words = append(words, job.Name)
	}
	if run, ok := m.LatestWorkflowRun(w.UID); ok {
		words = append(words, string(run.Status))
	}
	return words
}

// searchedRuns are the Runs the list shows: those the search leaves, every one without it.
func (m Model) searchedRuns() []Run {
	if !m.searchingPage(PageRuns) {
		return m.Runs.Runs
	}
	rows := m.RunRows(m.Now)
	runs := make([]Run, len(rows))
	for i, row := range rows {
		runs[i] = row.Run
	}
	return runs
}
