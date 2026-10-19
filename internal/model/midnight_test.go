package model_test

import (
	"reflect"
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

	m, cmds = step(m, snap{model.SnapshotLoaded{LocalDate: "2026-03-08", TimeZone: "America/Los_Angeles", ETag: "x"}})
	if len(wakes(cmds)) != 0 {
		t.Fatalf("a later snapshot of the same day woke again: %+v", cmds)
	}
	_ = m
}

func TestTheWakeReadsTheSnapshotAndAsksAgainWhileItIsStillDatedTheOldDay(t *testing.T) {
	m, _ := loadFirst(model.Model{}, model.ProjectLoaded{}, model.SnapshotLoaded{LocalDate: "2026-10-08", TimeZone: "UTC", ETag: "e"})

	m, cmds := model.Update(m, model.DayEnded{Day: "2026-10-08"})
	if !readsCollection(cmds, m, "") {
		t.Fatalf("the wake did not read the snapshot without an ETag: %+v", cmds)
	}
	m, cmds = step(m, snap{model.SnapshotLoaded{LocalDate: "2026-10-08", TimeZone: "UTC", ETag: "e2"}})
	if !readsCollection(cmds, m, "") || len(wakes(cmds)) != 0 {
		t.Fatalf("a result still dated the old day should read again, got %+v", cmds)
	}
	m, cmds = step(m, snap{model.SnapshotLoaded{LocalDate: "2026-10-09", TimeZone: "UTC", ETag: "e3"}})
	got := wakes(cmds)
	if m.Calendar.Date != "2026-10-09" || len(got) != 1 || got[0].Day != "2026-10-09" || len(cmds) != 1 {
		t.Fatalf("the new day should schedule the next wake alone, got %+v on %s", cmds, m.Calendar.Date)
	}
}

func TestAWakeForADayTheCalendarLeftChangesNothing(t *testing.T) {
	m, _ := loadFirst(model.Model{}, model.ProjectLoaded{}, model.SnapshotLoaded{LocalDate: "2026-10-09", TimeZone: "UTC"})
	next, cmds := model.Update(m, model.DayEnded{Day: "2026-10-08"})
	if len(cmds) != 0 || next.DayEnded || next.Refreshing != m.Refreshing {
		t.Fatalf("stale wake changed the model: %+v %+v", next, cmds)
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
