package model_test

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func wakes(cmds []model.Cmd) []model.WakeAt {
	var out []model.WakeAt
	for _, cmd := range cmds {
		if w, ok := cmd.(model.WakeAt); ok {
			out = append(out, w)
		}
	}
	return out
}

func TestTheModelAsksForOneWakeAtTheEndOfEachLocalDay(t *testing.T) {
	zone, _ := time.LoadLocation("America/Los_Angeles")
	m, cmds := loadFirst(model.Model{}, model.ProjectLoaded{}, model.SnapshotLoaded{LocalDate: "2026-03-08", TimeZone: "America/Los_Angeles"})
	got := wakes(cmds)
	if len(got) != 1 || got[0].Day != "2026-03-08" || !got[0].At.Equal(time.Date(2026, 3, 9, 0, 0, 0, 0, zone)) {
		t.Fatalf("first snapshot wakes = %+v, want one at the end of 2026-03-08 local", got)
	}

	_, cmds = step(m, snap{model.SnapshotLoaded{LocalDate: "2026-03-08", TimeZone: "America/Los_Angeles", ETag: "x"}})
	if len(wakes(cmds)) != 0 {
		t.Fatalf("a later snapshot of the same day woke again: %+v", cmds)
	}
}

func TestAReadStartedAfterTheWakeThatIsStillDatedTheOldDayDoesNotReadAgain(t *testing.T) {
	m, _ := loadFirst(model.Model{}, model.ProjectLoaded{}, model.SnapshotLoaded{LocalDate: "2026-10-08", TimeZone: "UTC", ETag: "e"})

	m, cmds := model.Update(m, model.DayEnded{Day: "2026-10-08"})
	if !readsCollection(cmds, m, "") {
		t.Fatalf("the wake did not read the snapshot without an ETag: %+v", cmds)
	}
	m, cmds = step(m, snap{model.SnapshotLoaded{LocalDate: "2026-10-08", TimeZone: "UTC", ETag: "e2"}})
	if len(cmds) != 0 {
		t.Fatalf("a result still dated the old day read again with no read in flight at the wake: %+v", cmds)
	}
	m, cmds = step(m, snap{model.SnapshotLoaded{LocalDate: "2026-10-09", TimeZone: "UTC", ETag: "e3"}})
	got := wakes(cmds)
	if m.Calendar.Date != "2026-10-09" || len(got) != 1 || got[0].Day != "2026-10-09" || len(cmds) != 1 {
		t.Fatalf("the new day should schedule the next wake alone, got %+v on %s", cmds, m.Calendar.Date)
	}
}

func TestAReadInFlightAtTheWakeReadsAgainWhileItIsStillDatedTheOldDay(t *testing.T) {
	m, _ := loadFirst(model.Model{}, model.ProjectLoaded{}, model.SnapshotLoaded{LocalDate: "2026-10-08", TimeZone: "UTC", ETag: "e"})
	m, _ = model.Update(m, model.StreamEvent{Generation: m.StreamGeneration, ID: 1, Type: "issue.commented"})

	m, cmds := model.Update(m, model.DayEnded{Day: "2026-10-08"})
	if len(cmds) != 0 {
		t.Fatalf("the wake started a second read beside the one in flight: %+v", cmds)
	}
	m, cmds = step(m, snap{model.SnapshotLoaded{LocalDate: "2026-10-08", TimeZone: "UTC", ETag: "e2", Snapshot: model.Snapshot{Cursor: 1}}})
	if !readsCollection(cmds, m, "") || len(wakes(cmds)) != 0 {
		t.Fatalf("a result in flight at the wake and still dated the old day should read again, got %+v", cmds)
	}
	_, cmds = step(m, snap{model.SnapshotLoaded{LocalDate: "2026-10-08", TimeZone: "UTC", ETag: "e3", Snapshot: model.Snapshot{Cursor: 1}}})
	if hasRead(cmds) {
		t.Fatalf("the second result read again: %+v", cmds)
	}
}

func TestAZoneChangeDuringTheDayWakesAtTheEndOfTheDayInTheNewZone(t *testing.T) {
	zone, _ := time.LoadLocation("America/Los_Angeles")
	m, _ := loadFirst(model.Model{}, model.ProjectLoaded{}, model.SnapshotLoaded{LocalDate: "2026-10-08", TimeZone: "UTC"})
	_, cmds := step(m, snap{model.SnapshotLoaded{LocalDate: "2026-10-08", TimeZone: "America/Los_Angeles"}})
	got := wakes(cmds)
	if len(got) != 1 || got[0].Day != "2026-10-08" || !got[0].At.Equal(time.Date(2026, 10, 9, 0, 0, 0, 0, zone)) {
		t.Fatalf("wakes after the zone moved = %+v, want one at the end of the day in Los Angeles", got)
	}
}

func TestAWakeForADayTheCalendarLeftChangesNothing(t *testing.T) {
	m, _ := loadFirst(model.Model{}, model.ProjectLoaded{}, model.SnapshotLoaded{LocalDate: "2026-10-09", TimeZone: "UTC"})
	next, cmds := model.Update(m, model.DayEnded{Day: "2026-10-08"})
	if len(cmds) != 0 || next.Refreshing != m.Refreshing {
		t.Fatalf("stale wake changed the model: %+v %+v", next, cmds)
	}
	_, cmds = step(next, snap{model.SnapshotLoaded{LocalDate: "2026-10-09", TimeZone: "UTC"}})
	if len(cmds) != 0 {
		t.Fatalf("a result after the stale wake did more: %+v", cmds)
	}
}

func TestDeadlinesMoveToTheNewDayAfterTheWakeWithoutADaemonEvent(t *testing.T) {
	issues := []model.Issue{
		{UID: "due-d", Status: "open", DeadlineOnDate: new("2026-10-08")},
		{UID: "due-next", Status: "open", DeadlineOnDate: new("2026-10-09")},
	}
	rows := func(m model.Model) map[string][]string {
		out := map[string][]string{}
		for _, g := range m.Groups {
			for _, issue := range g.Issues {
				out[g.Title] = append(out[g.Title], issue.UID)
			}
		}
		return out
	}
	m := model.Model{Collection: model.Collection{View: "deadlines", Title: "deadlines"}}
	m, _ = step(m, snap{model.SnapshotLoaded{LocalDate: "2026-10-08", TimeZone: "UTC", Snapshot: model.Snapshot{Issues: issues}}})
	if got := rows(m); !reflect.DeepEqual(got, map[string][]string{"Today": {"due-d"}, "2026-10-09": {"due-next"}}) {
		t.Fatalf("before midnight: %v", got)
	}

	m, _ = model.Update(m, model.DayEnded{Day: "2026-10-08"})
	m, _ = step(m, snap{model.SnapshotLoaded{LocalDate: "2026-10-09", TimeZone: "UTC", Snapshot: model.Snapshot{Issues: issues}}})
	if got := rows(m); !reflect.DeepEqual(got, map[string][]string{"Overdue": {"due-d"}, "Today": {"due-next"}}) {
		t.Fatalf("after midnight: %v", got)
	}
}

func hasRead(cmds []model.Cmd) bool {
	return slices.ContainsFunc(cmds, func(cmd model.Cmd) bool { _, ok := cmd.(model.ReadSnapshot); return ok })
}
