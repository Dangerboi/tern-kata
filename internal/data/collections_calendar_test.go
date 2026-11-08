package data

import (
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// Every Collection is read in the Pane's calendar, not kata's UTC one: a deadline of 03:00 UTC on the 9th is still the 8th in Los
// Angeles, on the Board, the Project list and All Open as in Deadlines.
func TestEveryCollectionReadsPlanningDatesInTheLocalZone(t *testing.T) {
	t.Setenv("TZ", "America/Los_Angeles")
	now := systemCalendar.Now
	systemCalendar.Now = func() time.Time { return time.Date(2026, 11, 8, 13, 47, 0, 0, time.UTC) } // 05:47 on the 8th in LA
	t.Cleanup(func() { systemCalendar.Now = now })

	gateway := newGateway(t, newLocator(startKata(t, "0.21.0").target()))
	workspace, p := project(t, gateway, "work")
	created := as[model.IssueCreated](t, run(t, gateway, model.CreateIssueCmd{Workspace: workspace, Actor: "tester", ProjectID: p.ID, Title: "Late", DeadlineOn: "2026-11-09T03:00:00Z", ScheduledOn: "2026-11-09T03:00:00Z"}))
	if created.Err != nil {
		t.Fatal(created.Err)
	}
	for _, c := range []model.Collection{{View: "all-open"}, {View: "deadlines"}, model.ProjectCollection(p.UID, "work")} {
		loaded := as[model.SnapshotLoaded](t, run(t, gateway, model.ReadSnapshot{Workspace: workspace, Query: model.CollectionQuery(c, model.Filters{})}))
		if len(loaded.Snapshot.Issues) != 1 {
			t.Fatalf("%s: %d Issues", c.View, len(loaded.Snapshot.Issues))
		}
		issue := loaded.Snapshot.Issues[0]
		if issue.DeadlineOnDate == nil || *issue.DeadlineOnDate != "2026-11-08" || issue.ScheduledOnDate == nil || *issue.ScheduledOnDate != "2026-11-08" {
			t.Errorf("%s read deadline %v scheduled %v, want the local day 2026-11-08", c.View, issue.DeadlineOnDate, issue.ScheduledOnDate)
		}
	}
}
