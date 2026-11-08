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

func (m Model) PageSearchShown() bool { return m.PageSearch.Open || m.PageSearch.Text != "" }

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

func (m Model) searchingPage(page Page) bool {
	return m.Page == page && m.PageSearch.Text != "" && pageDefs[page].settleSearch != nil
}

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

func pageSearchKeep(m Model) Model {
	m.PageSearch.Open = false
	return m
}

func clearPageSearch(m Model) Model {
	if m.PageSearch == (PageSearchState{}) {
		return m
	}
	row := m.selectedRow()
	m.PageSearch = PageSearchState{}
	return m.settlePageSelection(row)
}

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

func pageSearchMoving(action Action) func(Model) (Model, []Cmd) {
	return pure(func(m Model) Model { return pageDefs[m.Page].moves.move(m, action) })
}

func updatePageSearchText(m Model, msg KeyMsg) (Model, []Cmd) {
	text, edited := editText(m.PageSearch.Text, &m.PageSearch.Caret, "", false, msg)
	if !edited { // the caret may still have moved
		return m, nil
	}
	return m.pageSearchEdit(text), nil
}

func pageSearchEscape(m Model) (Model, []Cmd) {
	if m.PageSearching() {
		return clearPageSearch(m), nil
	}
	return drillOut(m)
}

func (m Model) jobSearchWords(job CronJob) []string {
	return []string{job.Name, m.JobPause(job).StateText(), job.Kind(), triggerWords(job.Definition.Trigger), job.Definition.Profile, m.lastActivity(job, m.Now)}
}

func runSearchWords(row RunRow) []string {
	return []string{row.State, row.Name, row.Issue, row.Executor, row.Start, row.Duration, row.Reason, row.Hold, row.Summary}
}

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

func (m Model) searchedRuns() []Run {
	if m.Runs.Timeline {
		return m.timelineSelectable()
	}
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
