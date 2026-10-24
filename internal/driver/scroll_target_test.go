package driver_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// Each Page's scroll key sends a scroll op naming a node the rendered view draws, and the node its list scrolls is one the Page
// draws too.
func TestEachPagesScrollsNameTheNodesItsViewDraws(t *testing.T) {
	panes := []struct {
		name string
		page model.Page
		pane func(t *testing.T) driver.State
		// list is whether the Page draws one list node its list scrolls; the Board's Lanes scroll inside their own columns.
		list bool
		// detail is whether the pane holds an Issue or an inspector for a scroll key to scroll.
		detail bool
	}{
		{"Issues", model.PageIssues, func(t *testing.T) driver.State { return browsing(t, 160) }, true, true},
		{"Board", model.PageBoard, func(t *testing.T) driver.State { return boardPane(t, boardCard("a")) }, false, false},
		{"Jobs", model.PageJobs, func(t *testing.T) driver.State { return jobsPane(t, 160, promptJob("j1", "Job")) }, true, true},
		{"Runs", model.PageRuns, func(t *testing.T) driver.State { return runsPane(t, 160, finishedRun("r1")) }, true, true},
		{"Workflows", model.PageWorkflows, func(t *testing.T) driver.State { return workflowsPane(t, 160, nightlyFlow()) }, true, false},
	}
	for _, c := range panes {
		t.Run(c.name, func(t *testing.T) {
			s := c.pane(t)
			if s.Model.Page != c.page {
				t.Fatalf("pane is on Page %v, want %v", s.Model.Page, c.page)
			}
			list := view.ScrollTarget(s.Model, model.ColumnList)
			if c.list && !drawnIDs(t, *driverView(t, s))[list] {
				t.Errorf("the list scrolls %q, which the %s Page does not draw", list, c.name)
			}
			if !c.list && list != "" {
				t.Errorf("the list scrolls %q, want no node", list)
			}
			if !c.detail {
				return
			}
			s = feed(t, s, click("", view.FocusDetailClick)).State
			out := step(t, s, key("page_down"))
			if len(out.Ops) != 1 || out.Ops[0][0] != "scroll" {
				t.Fatalf("page_down sent %v, want one scroll", out.Ops)
			}
			if id := out.Ops[0][1]; id != view.DetailContentID {
				t.Errorf("page_down scrolls %v, want %s", id, view.DetailContentID)
			}
		})
	}
}

func driverView(t *testing.T, s driver.State) *tern.View {
	t.Helper()
	v := view.Render(s.Model, now)
	return &v
}
