package driver_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

func skippedStatus(skips ...model.JobSkips) *model.WorkerStatus {
	status := servingWorker()
	status.Projects[0].Skipped = skips
	return status
}

func dailyLimitSkips(uid, name string, count int) model.JobSkips {
	return model.JobSkips{UID: uid, Name: name, Count: count, LatestCondition: "runs_per_day",
		LatestReason: "2 of 2 runs today", LatestAt: now.Add(-30 * time.Minute)}
}

// skipRowNodes are the wire nodes of the skip row of job uid and of everything under it.
func skipRowNodes(t *testing.T, s driver.State, uid string) (text string, nodes int) {
	t.Helper()
	id := view.RunListID + ".skips-" + uid
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if n.ID == id || strings.HasPrefix(n.ID, id+".") {
			text += wireText(n)
			nodes++
		}
	}
	return text, nodes
}

func TestRunsPageShowsAJobsSkipsAsOneRowOfTheWorkersStatusWithItsCountAndLatestReason(t *testing.T) {
	skips := dailyLimitSkips("J1", "Triage", 40)
	out := localRunsPane(t, skippedStatus(skips, dailyLimitSkips("J2", "Other", 1)), finishedRun("R1"))

	text, _ := skipRowNodes(t, out.State, "J1")
	wantText(t, "skip row", text, "40 skips", "Triage", "runs_per_day", "2 of 2 runs today", out.State.Model.Calendar.Read(skips.LatestAt, now))
	other, _ := skipRowNodes(t, out.State, "J2")
	wantText(t, "single skip", other, "1 skip", "Other")
	wantText(t, "run row beside it", runCardText(t, out.State, "R1"), "succeeded", "Triage")

	rows := 0
	for _, n := range wireNodes(t, view.Render(out.State.Model, now)) {
		if strings.HasPrefix(n.ID, view.RunListID+".skips-J1") && strings.Count(n.ID, ".") == strings.Count(view.RunListID+".skips-J1", ".") {
			rows++
		}
	}
	if rows != 1 {
		t.Errorf("40 skips of one job draw %d rows, want 1", rows)
	}
}

func TestRunsPageSkipRowSitsWithTheRunRowsAndShowsWithoutAnyRun(t *testing.T) {
	out := localRunsPane(t, skippedStatus(dailyLimitSkips("J1", "Triage", 2)))
	if _, n := skipRowNodes(t, out.State, "J1"); n == 0 {
		t.Fatal("a job with skips and no Run draws no row")
	}
	wantNotShown(t, "the Runs Page", out.State, "No runs")
}

func TestRunsFilterToOneJobAlsoFiltersTheSkipRow(t *testing.T) {
	out := localRunsPane(t, skippedStatus(dailyLimitSkips("J1", "Triage", 3), dailyLimitSkips("J2", "Other", 5)), finishedRun("R1"))
	for _, uid := range []string{"J1", "J2"} {
		if _, n := skipRowNodes(t, out.State, uid); n == 0 {
			t.Fatalf("the unfiltered Page draws no skip row for %s", uid)
		}
	}

	narrowed := step(t, out.State, key("f"))
	if load, ok := askedRuns(narrowed.Cmds); !ok || load.JobUID != "J1" {
		t.Fatalf("f asks for %#v, want the runs of J1", narrowed.Cmds)
	}
	s := answerRuns(t, narrowed, "", finishedRun("R1"))
	if _, n := skipRowNodes(t, s, "J1"); n == 0 {
		t.Error("the filter to J1 hides J1's skip row")
	}
	if _, n := skipRowNodes(t, s, "J2"); n != 0 {
		t.Error("the filter to J1 still draws J2's skip row")
	}

	lifted := answerRuns(t, step(t, s, key("f")), "", finishedRun("R1"))
	if _, n := skipRowNodes(t, lifted, "J2"); n == 0 {
		t.Error("lifting the filter does not bring J2's skip row back")
	}
}

func TestRunsPageSkipRowFollowsTheSinceYouLastLookedModeAndItsFilter(t *testing.T) {
	out := localRunsPane(t, skippedStatus(dailyLimitSkips("J1", "Triage", 3), dailyLimitSkips("J2", "Other", 5)), finishedRun("R1"))
	s := step(t, out.State, key("n")).State
	if !s.Model.Runs.Timeline {
		t.Fatal("n does not switch to the since-you-last-looked mode")
	}
	for _, uid := range []string{"J1", "J2"} {
		if _, n := skipRowNodes(t, s, uid); n == 0 {
			t.Errorf("the mode draws no skip row for %s", uid)
		}
	}
	s.Model.Runs.JobUID = "J1"
	if _, n := skipRowNodes(t, s, "J2"); n != 0 {
		t.Error("the mode's job filter still draws J2's skip row")
	}
}

func TestRunsPageWithoutAReadableWorkerStatusShowsRunsOnly(t *testing.T) {
	for name, status := range map[string]*model.WorkerStatus{
		"no worker":         nil,
		"worker without it": {Executor: "laptop", Projects: []model.WorkerProject{{Name: "other", Skipped: []model.JobSkips{dailyLimitSkips("J1", "Triage", 4)}}}},
		"worker, no skips":  servingWorker(),
	} {
		t.Run(name, func(t *testing.T) {
			out := localRunsPane(t, status, finishedRun("R1"))
			if _, n := skipRowNodes(t, out.State, "J1"); n != 0 {
				t.Error("a skip row is drawn")
			}
			wantText(t, "run row", runCardText(t, out.State, "R1"), "succeeded", "Triage")
			wantNotShown(t, "the Runs Page", out.State, "skip")
		})
	}
}

func TestTheSkipRowIsNoRun(t *testing.T) {
	out := localRunsPane(t, skippedStatus(dailyLimitSkips("J1", "Triage", 3)))
	s := out.State
	text, _ := skipRowNodes(t, s, "J1")
	if strings.Contains(text, "run=") {
		t.Errorf("the skip row carries a click of a Run: %s", text)
	}
	if got := s.Model.Runs.Selected; got != "" {
		t.Fatalf("selected %q, want none: the skip row is no Run", got)
	}
	for _, chord := range []string{"enter", "u", "A", "o", "j", "k"} {
		out := step(t, s, key(chord))
		if len(out.Cmds) != 0 {
			t.Errorf("%s with only a skip row sent %#v", chord, out.Cmds)
		}
		if out.State.Model.Runs.Selected != "" {
			t.Errorf("%s selected %q", chord, out.State.Model.Runs.Selected)
		}
	}

	// With a Run beside it the selection stays on the Runs.
	withRun := localRunsPane(t, skippedStatus(dailyLimitSkips("J1", "Triage", 3)), finishedRun("R1")).State
	if got := withRun.Model.Runs.Selected; got != "R1" {
		t.Errorf("selected %q, want R1", got)
	}
	if again := step(t, withRun, key("k")).State.Model.Runs.Selected; again != "R1" {
		t.Errorf("k moved the selection to %q, want it on R1", again)
	}
	if _, n := skipRowNodes(t, withRun, "J1"); n == 0 {
		t.Fatal("no skip row")
	}
}
