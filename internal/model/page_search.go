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
func (m Model) PageSearching() bool {
	return m.PageSearch.Text != "" && (m.Page == PageJobs || m.Page == PageRuns || m.Page == PageWorkflows)
}

// PageSearchNoun is what the current Page lists, for the search input's placeholder and the empty result.
func (m Model) PageSearchNoun() string {
	switch m.Page {
	case PageRuns:
		return "runs"
	case PageWorkflows:
		return "workflows"
	}
	return "jobs"
}

// pageSearchMatches is kata's match: the query is a case-insensitive substring of the row's words.
func pageSearchMatches(words, query string) bool {
	return strings.Contains(strings.ToLower(words), strings.ToLower(query))
}

// matchesSearch says a row with these words is listed under the current search.
func (m Model) matchesSearch(words ...string) bool {
	return pageSearchMatches(strings.Join(words, " "), m.PageSearch.Text)
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
	switch m.Page {
	case PageJobs:
		return m.settleJobSelection()
	case PageRuns:
		return m.settleRunSelection()
	case PageWorkflows:
		return m.selectWorkflow(row)
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
	return []string{job.Name, job.Pause(m.Jobs.ProjectArchived).StateText(), job.Kind(), triggerWords(job.Definition.Trigger), job.Definition.Profile, m.lastActivity(job, m.Now)}
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
	if m.Page != PageRuns || m.PageSearch.Text == "" {
		return m.Runs.Runs
	}
	rows := m.RunRows(m.Now)
	runs := make([]Run, len(rows))
	for i, row := range rows {
		runs[i] = row.Run
	}
	return runs
}
