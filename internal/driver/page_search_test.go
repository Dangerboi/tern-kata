package driver_test

import (
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

// searchPage is one Page with two rows, for the checks every searchable Page shares.
type searchPage struct {
	name string
	// pane is the Page open on its two rows.
	pane func(t *testing.T) driver.State
	// rows are the uids the Page lists, in order; selected is the uid its selection is on.
	rows     func(m model.Model) []string
	selected func(m model.Model) string
	// query matches only the second row, as typed (the row's words are in another case); empty matches no row.
	query, second, empty string
}

func searchPages() []searchPage {
	return []searchPage{
		{
			name: "Jobs",
			pane: func(t *testing.T) driver.State {
				return jobsPane(t, 160, promptJob("J1", "Triage"), nativeJob("J2", "Weekly report", jobParent("open", false)))
			},
			rows: func(m model.Model) []string {
				var uids []string
				for _, row := range m.JobRows(now) {
					uids = append(uids, row.Job.UID)
				}
				return uids
			},
			selected: func(m model.Model) string { return m.Jobs.Selected },
			query:    "weekly", second: "J2", empty: "zzz",
		},
		{
			name: "Runs",
			pane: func(t *testing.T) driver.State { return runsPane(t, 160, finishedRun("R1"), workflowRun("R2")) },
			rows: func(m model.Model) []string {
				var uids []string
				for _, row := range m.RunRows(now) {
					uids = append(uids, row.Run.UID)
				}
				return uids
			},
			selected: func(m model.Model) string { return m.Runs.Selected },
			query:    "nightly", second: "R2", empty: "zzz",
		},
		{
			name: "Workflows",
			pane: func(t *testing.T) driver.State { return workflowsPane(t, 160, reviewFlow(), nightlyFlow()) },
			rows: func(m model.Model) []string {
				var uids []string
				for _, w := range m.VisibleWorkflows() {
					uids = append(uids, w.UID)
				}
				return uids
			},
			selected: func(m model.Model) string { return m.Workflows.Selected },
			query:    "night", second: "W2", empty: "zzz",
		},
	}
}

func searchInputDrawn(t *testing.T, s driver.State) bool {
	t.Helper()
	return hasNode(wireNodes(t, view.Render(s.Model, now)), view.SearchID)
}

func TestSlashSearchesEachPagesRowsAndTheSelectionStaysOnAVisibleRow(t *testing.T) {
	for _, p := range searchPages() {
		t.Run(p.name, func(t *testing.T) {
			s := p.pane(t)
			all := p.rows(s.Model)
			if len(all) != 2 {
				t.Fatalf("the Page lists %v, want two rows", all)
			}
			if p.selected(s.Model) != all[0] {
				t.Fatalf("selected %q before the search, want the first row %q", p.selected(s.Model), all[0])
			}
			if searchInputDrawn(t, s) {
				t.Fatal("the search input is drawn before / is pressed")
			}
			s = step(t, s, key("/")).State
			if s.Model.CurrentMode() != model.ModePageSearch || !searchInputDrawn(t, s) {
				t.Fatalf("/ gave mode %s, input drawn %v; want the search input open", s.Model.CurrentMode(), searchInputDrawn(t, s))
			}
			s = typed(t, s, p.query)
			if got := p.rows(s.Model); len(got) != 1 || got[0] != p.second {
				t.Fatalf("%q lists %v, want only %s", p.query, got, p.second)
			}
			if got := p.selected(s.Model); got != p.second {
				t.Errorf("selected %q after the search narrowed the rows, want the visible %s", got, p.second)
			}
			s = step(t, s, key("enter")).State
			if s.Model.CurrentMode() == model.ModePageSearch || !searchInputDrawn(t, s) || len(p.rows(s.Model)) != 1 {
				t.Fatalf("enter: mode %s, input drawn %v, rows %v; want the list kept narrowed under the shown search", s.Model.CurrentMode(), searchInputDrawn(t, s), p.rows(s.Model))
			}
			s = step(t, s, key("escape")).State
			if got := p.rows(s.Model); len(got) != 2 || s.Model.Page == model.PageIssues || s.Model.PageSearch != (model.PageSearchState{}) || searchInputDrawn(t, s) {
				t.Fatalf("esc: rows %v, page %v, search %+v; want both rows back on the same Page with no search", got, s.Model.Page, s.Model.PageSearch)
			}
			if got := p.selected(s.Model); got != p.second {
				t.Errorf("selected %q after clearing, want it kept on %s", got, p.second)
			}
		})
	}
}

func TestEscInTheSearchInputClearsItAndStaysOnThePage(t *testing.T) {
	for _, p := range searchPages() {
		t.Run(p.name, func(t *testing.T) {
			s := typed(t, step(t, p.pane(t), key("/")).State, p.query)
			s = step(t, s, key("escape")).State
			if len(p.rows(s.Model)) != 2 || s.Model.PageSearch != (model.PageSearchState{}) || s.Model.Page == model.PageIssues {
				t.Fatalf("esc in the input: rows %v, search %+v, page %v", p.rows(s.Model), s.Model.PageSearch, s.Model.Page)
			}
		})
	}
}

func TestAnEmptySearchResultSaysSoAndKeepsTheSearchShown(t *testing.T) {
	for _, p := range searchPages() {
		t.Run(p.name, func(t *testing.T) {
			s := typed(t, step(t, p.pane(t), key("/")).State, p.empty)
			if len(p.rows(s.Model)) != 0 {
				t.Fatalf("%q lists %v, want no row", p.empty, p.rows(s.Model))
			}
			if !strings.Contains(screen(t, s), " match") || !searchInputDrawn(t, s) {
				t.Errorf("an empty result must say nothing matches and keep the search drawn: %q", screen(t, s))
			}
			s = step(t, s, key("enter")).State
			if !strings.Contains(screen(t, s), " match") || !searchInputDrawn(t, s) {
				t.Error("the empty result and the search must stay after enter")
			}
		})
	}
}

func TestSwitchingPageEndsTheSearch(t *testing.T) {
	s := typed(t, step(t, runsPane(t, 160, finishedRun("R1"), workflowRun("R2")), key("/")).State, "nightly")
	s = step(t, s, key("enter")).State
	out := step(t, s, key("2"))
	if out.State.Model.PageSearch != (model.PageSearchState{}) {
		t.Errorf("switching Page keeps the search %+v", out.State.Model.PageSearch)
	}
}

func TestARunReadThatLeavesTheSearchMovesTheSelectionToAListedRun(t *testing.T) {
	s := runsPane(t, 160, finishedRun("R1"), finishedRun("R3"))
	s = typed(t, step(t, s, key("/")).State, "triage")
	s = step(t, s, key("enter")).State
	if got := s.Model.Runs.Selected; got != "R1" {
		t.Fatalf("selected %q before the read, want R1", got)
	}
	changed := finishedRun("R1")
	changed.JobName, changed.Summary = "Other", "none"
	s = step(t, s, model.RunShown{UID: "R1", Run: changed}).State
	if got := s.Model.Runs.Selected; got != "R3" {
		t.Errorf("selected %q after R1 left the search, want the listed R3", got)
	}
}

func TestFooterOffersEndSearchWhileAKeptSearchIsActive(t *testing.T) {
	s := runsPane(t, 160, finishedRun("R1"), workflowRun("R2"))
	if strings.Contains(screen(t, s), "end search") {
		t.Fatal("the footer offers end search before any search")
	}
	s = step(t, typed(t, step(t, s, key("/")).State, "nightly"), key("enter")).State
	if !strings.Contains(screen(t, s), "end search") {
		t.Errorf("the footer does not offer esc as end search while the search is kept:\n%s", screen(t, s))
	}
}

func TestWorkflowsFooterOffersEndSearchOnlyWhileEscEndsIt(t *testing.T) {
	s := workflowsPane(t, 160, reviewFlow(), nightlyFlow())
	s = step(t, typed(t, step(t, s, key("/")).State, "nightly"), key("enter")).State
	if !strings.Contains(screen(t, s), "end search") {
		t.Fatalf("the footer does not offer esc as end search with a kept search:\n%s", screen(t, s))
	}
	s = step(t, s, key("enter")).State
	if !s.Model.Workflows.Inspecting {
		t.Fatal("enter did not open the steps")
	}
	if strings.Contains(screen(t, s), "end search") {
		t.Errorf("the footer offers end search while esc closes the steps first:\n%s", screen(t, s))
	}
}

func TestAJobsReadThatLeavesTheWorkflowSearchMovesTheSelectionToAListedWorkflow(t *testing.T) {
	out := feed(t, projectPane(t, 160, boardCard("a")), key("4"))
	var load model.LoadJobs
	for _, cmd := range out.Cmds {
		if c, ok := cmd.(model.LoadJobs); ok {
			load = c
		}
	}
	use := model.CronJob{UID: "J1", Name: "Nightly review", Definition: cronjob.Definition{Enabled: true, Action: cronjob.Action{Kind: "execute", WorkflowUID: "W1"}}}
	s := step(t, out.State, model.WorkflowsLoaded{ProjectUID: "p", Workflows: []model.Workflow{reviewFlow(), nightlyFlow()}}).State
	s = step(t, s, model.JobsLoaded{Request: load.Request, Jobs: []model.CronJob{use}}).State
	s = step(t, typed(t, step(t, s, key("/")).State, "nightly"), key("enter")).State
	if got := s.Model.Workflows.Selected; got != "W1" {
		t.Fatalf("selected %q with both workflows listed, want W1", got)
	}
	s = step(t, s, model.JobsLoaded{Request: load.Request}).State
	if got := s.Model.Workflows.Selected; got != "W2" {
		t.Errorf("selected %q after W1 left the search, want the listed W2", got)
	}
}
