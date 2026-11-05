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
	issue := model.Issue{UID: "dated", ShortID: "dddd", Title: "A rather long Dated title", Status: "open", ProjectName: "work", ScheduledOnDate: &later}
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
	// Beside the detail the list is about 71 columns (0.4 of the Pane plus half of the free tenth): the whole title and the 20-cell badge do not fit, so the badge goes and the title stays whole.
	side := text(render(t, load(160))[view.IssueID("dated")])
	if strings.Contains(side, "scheduled 2027-01-10") || !strings.Contains(side, "A rather long Dated title") {
		t.Errorf("side by side row %q, want the title without the badge", side)
	}
	// Alone in the Pane the list has room for both.
	m, _ := model.Update(load(160), model.KeyMsg{Chord: "shift+l"})
	if got := text(render(t, m)[view.IssueID("dated")]); !strings.Contains(got, "scheduled 2027-01-10") || !strings.Contains(got, "A rather long Dated title") {
		t.Errorf("list alone row %q, want badge and title", got)
	}
}

// An Agent badge before the title takes room the Date badge would otherwise have: the badged row drops the Date badge no later
// than the plain one does, and at some width only the badged row drops it. The title is never cut for the badge.
func TestListRowDateBadgeCountsAgentBadge(t *testing.T) {
	day := "2026-11-05"
	title := "A rather long Dated title"
	load := func(cols int, agent bool) model.Model {
		issue := model.Issue{UID: "dated", ShortID: "dddd", Title: title, Status: "open", ProjectName: "work", DeadlineOnDate: &day}
		m := apply(model.Model{},
			model.Start{Workspace: "/repo"},
			model.ResizeMsg{Cols: cols},
			model.ProjectLoaded{Project: model.Project{UID: "p", Name: "work"}},
			snap{model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: []model.Issue{issue}}}},
		)
		m.Calendar.Date = day
		if agent {
			m.AgentMapping = []model.AgentEntry{{IssueUID: "dated", Block: "b1"}}
		}
		m, _ = model.Update(m, model.KeyMsg{Chord: "shift+l"}) // the list alone in the Pane
		return m
	}
	onlyPlain := 0
	for cols := model.MinCols; cols <= 140; cols++ {
		plain := text(render(t, load(cols, false))[view.IssueID("dated")])
		badged := text(render(t, load(cols, true))[view.IssueID("dated")])
		if !strings.Contains(badged, "agent") || !strings.Contains(badged, title) || !strings.Contains(plain, title) {
			t.Fatalf("cols %d: rows %q and %q, want the Agent badge and the whole title", cols, badged, plain)
		}
		plainDate, badgedDate := strings.Contains(plain, "due today"), strings.Contains(badged, "due today")
		if badgedDate && !plainDate {
			t.Errorf("cols %d: the Agent-badged row keeps its Date badge where the plain row drops it", cols)
		}
		if plainDate && !badgedDate {
			onlyPlain++
		}
	}
	if onlyPlain == 0 {
		t.Error("no width dropped the Date badge only for the Agent-badged row")
	}
}
