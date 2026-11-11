package driver_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

// twoLinePane is a 47x42 Pane on a Project of twenty-five Issues, the newest first.
func twoLinePane(t *testing.T) driver.State {
	t.Helper()
	var issues []model.Issue
	for i := range 25 {
		uid := fmt.Sprintf("issue%02d", i)
		issues = append(issues, model.Issue{UID: uid, ShortID: uid, Title: "Title " + uid, Status: "open", ProjectName: "work", UpdatedAt: now.Add(-time.Duration(i) * time.Hour)})
	}
	project := model.Project{ID: 7, UID: "p", Name: "work"}
	s := feed(t, driver.State{}, model.Start{Workspace: "/repo"}, model.ResizeMsg{Cols: 47, Rows: 42}).State
	s.Model = loadFirst(s.Model, model.ProjectLoaded{Project: project}, model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: issues, Catalog: []model.CatalogProject{{Project: project}}}})
	return s
}

func selectedAfter(t *testing.T, s driver.State, in any) string {
	t.Helper()
	return step(t, s, in).State.Model.SelectedUID
}

func TestTwoLineRowsMoveBySingleRowsAndHalfPages(t *testing.T) {
	s := twoLinePane(t)
	rows := s.Model.VisibleRows()
	if len(rows) != 25 {
		t.Fatalf("fixture: %d rows", len(rows))
	}
	s = feed(t, s, key("g")).State
	if got := s.Model.SelectedUID; got != rows[0].UID {
		t.Fatalf("first row = %q, want %q", got, rows[0].UID)
	}
	out := step(t, s, key("j"))
	if got := out.State.Model.SelectedUID; got != rows[1].UID {
		t.Fatalf("j selected %q, want %q", got, rows[1].UID)
	}
	if got := selectedAfter(t, out.State, key("k")); got != rows[0].UID {
		t.Fatalf("k selected %q, want %q", got, rows[0].UID)
	}
	// A page is five two-line rows, so the rows a page skips fit the 42 lines the Pane has.
	if got := selectedAfter(t, s, key("page_down")); got != rows[5].UID {
		t.Fatalf("page_down selected %q, want %q", got, rows[5].UID)
	}
	down := feed(t, s, key("page_down"), key("page_down")).State
	if got := selectedAfter(t, down, key("page_up")); got != rows[5].UID {
		t.Fatalf("page_up selected %q, want %q", got, rows[5].UID)
	}
}

func TestClickSelectsTheTwoLineRowClicked(t *testing.T) {
	s := twoLinePane(t)
	rows := s.Model.VisibleRows()
	target := rows[7]
	id := view.IssueID(target.UID)
	out := step(t, s, click(id, view.CardClick(target.UID)))
	if got := out.State.Model.SelectedUID; got != target.UID {
		t.Fatalf("click selected %q, want %q", got, target.UID)
	}
	wantOps(t, out)
}
