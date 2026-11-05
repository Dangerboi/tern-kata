package view_test

import (
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

// datedList is a Project list at cols with a dated open row, a dated closed row and an undated open row, today 2026-11-05.
func datedList(cols int) model.Model {
	day := "2026-11-05"
	dated := model.Issue{UID: "dated", ShortID: "dddd", Title: "Dated title", Status: "open", ProjectName: "work", DeadlineOnDate: &day}
	closed := model.Issue{UID: "closed", ShortID: "cccc", Title: "Closed title", Status: "closed", ProjectName: "work", DeadlineOnDate: &day}
	plain := model.Issue{UID: "plain", ShortID: "pppp", Title: "Plain title", Status: "open", ProjectName: "work"}
	m := apply(model.Model{},
		model.Start{Workspace: "/repo"},
		model.ResizeMsg{Cols: cols},
		model.ProjectLoaded{Project: model.Project{UID: "p", Name: "work"}},
		snap{model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: []model.Issue{dated, closed, plain}}}},
	)
	m.Calendar.Date = day
	return m
}

func TestListRowsShowDateBadgeOfOpenDatedIssues(t *testing.T) {
	nodes := render(t, datedList(160))
	if got := text(nodes[view.IssueID("dated")]); !strings.Contains(got, "due today") || !strings.Contains(got, "Dated title") {
		t.Errorf("dated open row %q, want its badge and title", got)
	}
	for _, uid := range []string{"closed", "plain"} {
		if got := text(nodes[view.IssueID(uid)]); strings.Contains(got, "due") {
			t.Errorf("%s row %q shows a Date badge", uid, got)
		}
	}
}

func TestListRowDropsDateBadgeBeforeTitleWhenNarrow(t *testing.T) {
	day, later := "2026-11-05", "2027-01-10"
	issue := model.Issue{UID: "dated", ShortID: "dddd", Title: "Dated title", Status: "open", ProjectName: "work", ScheduledOnDate: &later}
	load := func(cols int) model.Model {
		m := apply(model.Model{},
			model.Start{Workspace: "/repo"},
			model.ResizeMsg{Cols: cols},
			model.ProjectLoaded{Project: model.Project{UID: "p", Name: "work"}},
			snap{model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: []model.Issue{issue}}}},
		)
		m.Calendar.Date = day
		return m
	}
	// Beside the detail the list is about 71 columns (0.4 of the Pane plus half of the free tenth): the 20-cell badge leaves less than the least title, the title stays.
	side := text(render(t, load(160))[view.IssueID("dated")])
	if strings.Contains(side, "scheduled 2027-01-10") || !strings.Contains(side, "Dated title") {
		t.Errorf("side by side row %q, want the title without the badge", side)
	}
	// Alone in the Pane the list has room for both.
	m, _ := model.Update(load(160), model.KeyMsg{Chord: "shift+l"})
	if got := text(render(t, m)[view.IssueID("dated")]); !strings.Contains(got, "scheduled 2027-01-10") || !strings.Contains(got, "Dated title") {
		t.Errorf("list alone row %q, want badge and title", got)
	}
}
