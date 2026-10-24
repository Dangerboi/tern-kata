package driver_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
)

// The six movement keys of every Page's list move its selection as that list always has: a row, either end, a page, and no
// further than the first or last row.
func TestMovementKeysMoveEachPagesSelection(t *testing.T) {
	const rows = 25
	uid := func(i int) string { return fmt.Sprintf("r%02d", i) }
	issues := make([]model.Issue, rows)
	jobs := make([]model.CronJob, rows)
	runs := make([]model.Run, rows)
	flows := make([]model.Workflow, rows)
	for i := range rows {
		issues[i] = boardCard(uid(i))
		jobs[i] = promptJob(uid(i), "Job "+uid(i))
		runs[i] = finishedRun(uid(i))
		flows[i] = nightlyFlow()
		flows[i].UID, flows[i].Name = uid(i), "Flow "+uid(i)
	}
	position := func(m model.Model) int { selected, _ := m.Position(); return selected - 1 }
	cases := []struct {
		name     string
		pane     func(t *testing.T) driver.State
		position func(model.Model) int
		total    func(model.Model) int
		page     func(model.Model) int
	}{
		{"Issues", func(t *testing.T) driver.State { return projectPane(t, 160, issues...) }, position, func(m model.Model) int { return len(m.VisibleRows()) }, func(model.Model) int { return 10 }},
		{"Board Lane view", func(t *testing.T) driver.State {
			s := feed(t, boardPane(t, issues...), key("g")).State
			s = step(t, s, key("enter")).State
			if !s.Model.LaneViewShown() {
				t.Fatal("enter on the Board did not open the Lane view")
			}
			return s
		}, position, func(m model.Model) int { return len(m.VisibleRows()) }, func(model.Model) int { return 10 }},
		{"Jobs", func(t *testing.T) driver.State { return jobsPane(t, 160, jobs...) }, position, func(m model.Model) int { return len(m.Jobs.Jobs) }, func(model.Model) int { return 10 }},
		{"Runs", func(t *testing.T) driver.State { return runsPane(t, 160, runs...) }, position, func(m model.Model) int { return len(m.Runs.Runs) }, func(model.Model) int { return 10 }},
		{"Workflows", func(t *testing.T) driver.State { return workflowsPane(t, 160, flows...) },
			func(m model.Model) int {
				return slices.IndexFunc(m.VisibleWorkflows(), func(w model.Workflow) bool { return w.UID == m.Workflows.Selected })
			},
			func(m model.Model) int { return len(m.VisibleWorkflows()) }, func(m model.Model) int { return max(m.Rows-2, 1) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := feed(t, c.pane(t), key("g")).State
			n, page := c.total(s.Model), c.page(s.Model)
			if n != rows {
				t.Fatalf("the Page lists %d rows, want %d", n, rows)
			}
			at := func(what string, want int) {
				t.Helper()
				if got := c.position(s.Model); got != want {
					t.Fatalf("%s: row %d, want %d", what, got, want)
				}
			}
			press := func(chord string) { t.Helper(); s = step(t, s, key(chord)).State }
			at("first", 0)
			press("k")
			at("up from the first row stays", 0)
			press("j")
			at("down", 1)
			press("down")
			at("arrow down", 2)
			press("k")
			at("up", 1)
			press("page_down")
			at("page down", 1+page)
			press("page_up")
			at("page up", 1)
			press("shift+g")
			at("last", n-1)
			press("j")
			at("down from the last row stays", n-1)
			press("page_down")
			at("page down at the last row stays", n-1)
			press("page_up")
			at("page up from the last row", n-1-page)
			for range n {
				press("page_up")
			}
			at("page up clamps at the first row", 0)
			press("g")
			at("first again", 0)
		})
	}
}
