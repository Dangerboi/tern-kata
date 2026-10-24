package driver_test

import (
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
)

// twoProjectPane is a Pane on work with the catalog of work and infra, no Page asked for yet.
func twoProjectPane(t *testing.T) driver.State {
	t.Helper()
	return loadedWith(t, 160, model.Project{ID: 7, UID: "p", Name: "work"},
		model.Snapshot{Catalog: twoProjects(), Issues: []model.Issue{boardCard("parent-a")}})
}

// chooseInfra is the Menu choosing infra, the daemon's snapshot read of it answered with none.
func chooseInfra(t *testing.T, s driver.State) driver.Output {
	t.Helper()
	out := step(t, s, model.CollectionSelected{Collection: model.ProjectCollection("q", "infra")})
	if !readsProject(out.Cmds, "q") {
		t.Fatalf("cmds %#v, want a read of infra", out.Cmds)
	}
	return step(t, out.State, answeredReadMsg(t, out.Cmds))
}

func answeredReadMsg(t *testing.T, cmds []model.Cmd) model.SnapshotLoaded {
	t.Helper()
	loaded, ok := snapshotFor(cmds)
	if !ok {
		t.Fatalf("no snapshot read in %#v", cmds)
	}
	loaded.Snapshot.Catalog = twoProjects()
	return loaded
}

func TestChangingTheProjectOnTheRunsPageClearsItsFilterRunsSelectionAndCursor(t *testing.T) {
	s := twoProjectPane(t)
	s = answerJobs(t, step(t, s, key("3")), promptJob("J1", "Triage"))
	out := step(t, s, key("r"))
	s = answerRuns(t, out, "cursor-1", finishedRun("R1"))
	if got := s.Model.Runs; got.JobUID != "J1" || got.More == "" {
		t.Fatalf("setup: filter %q more %q", got.JobUID, got.More)
	}

	out = chooseInfra(t, s)
	got := out.State.Model.Runs
	if got.JobUID != "" || got.JobName != "" || len(got.Runs) != 0 || got.Selected != "" || got.More != "" {
		t.Fatalf("Runs after the Project change: %+v, want it empty", got)
	}
	if load, ok := askedRuns(out.Cmds); ok && (load.JobUID != "" || load.ProjectID != 8) {
		t.Fatalf("read %#v, want infra's Runs with no job", load)
	}
	// Whatever read the Page asks for next carries no job.
	next := step(t, out.State, key("R"))
	if load, ok := askedRuns(next.Cmds); ok && load.JobUID != "" {
		t.Fatalf("next read carries job %q", load.JobUID)
	}
}

func TestChangingTheProjectWhileAnotherPageIsShownClearsTheFilterBeforeTheRunsPageIsEntered(t *testing.T) {
	s := twoProjectPane(t)
	s = answerJobs(t, step(t, s, key("3")), promptJob("J1", "Triage"))
	s = answerRuns(t, step(t, s, key("r")), "", finishedRun("R1"))
	s = step(t, s, key("1")).State
	if s.Model.Page != model.PageIssues {
		t.Fatalf("setup: page %v", s.Model.Page)
	}
	s = chooseInfra(t, s).State

	out := step(t, s, key("4"))
	load, ok := askedRuns(out.Cmds)
	if !ok || load.JobUID != "" || load.ProjectID != 8 || load.Refresh {
		t.Fatalf("cmds %#v, want infra's newest Runs read with no job", out.Cmds)
	}
	if got := out.State.Model.Runs; got.JobUID != "" || len(got.Runs) != 0 {
		t.Fatalf("Runs %+v, want none of work's", got)
	}
}

func TestEnteringEveryPageAfterAProjectChangeShowsNothingOfTheOldProject(t *testing.T) {
	s := twoProjectPane(t)
	s = answerJobs(t, step(t, s, key("3")), promptJob("J1", "Triage"), promptJob("J2", "Other"))
	s = feed(t, s, key("j"), key("A")).State
	s = step(t, s, key("1")).State
	s = answerRuns(t, step(t, s, key("4")), "c", finishedRun("R1"))
	s = step(t, s, key("1")).State
	s = feed(t, s, key("b"), model.LanesLoaded{ProjectUID: "p", Lanes: []string{"todo", "done"}, DoneDays: 3}).State
	s = step(t, s, key("1")).State
	s = chooseInfra(t, s).State

	jobs := step(t, s, key("3"))
	if j := jobs.State.Model.Jobs; len(j.Jobs) != 0 || j.Selected != "" || j.ShowArchived {
		t.Errorf("Jobs %+v, want empty", j)
	}
	if load, ok := askedJobs(jobs.Cmds); !ok || load.Archived {
		t.Errorf("jobs read %#v, want one that leaves archived jobs out", jobs.Cmds)
	}
	runs := step(t, step(t, jobs.State, key("1")).State, key("4"))
	if r := runs.State.Model.Runs; len(r.Runs) != 0 || r.Selected != "" || r.JobUID != "" || r.More != "" {
		t.Errorf("Runs %+v, want empty", r)
	}
	flows := step(t, step(t, runs.State, key("1")).State, key("5"))
	if w := flows.State.Model.Workflows; len(w.List) != 0 || w.Selected != "" || w.ShowArchived {
		t.Errorf("Workflows %+v, want empty", w)
	}
	board := step(t, step(t, flows.State, key("1")).State, key("b"))
	if b := board.State.Model.Board; len(b.Lanes) != 0 || b.DoneDays != 0 {
		t.Errorf("Board %+v, want the default Lanes", b)
	}
}

func TestSwitchingPagesWithinOneProjectKeepsEachPagesStateAndWorkflowsReadAgain(t *testing.T) {
	s := twoProjectPane(t)
	s = answerJobs(t, step(t, s, key("3")), promptJob("J1", "Triage"), promptJob("J2", "Other"))
	s = feed(t, s, key("j")).State
	s = answerRuns(t, step(t, s, key("r")), "", finishedRun("R1"))
	back := step(t, step(t, s, key("1")).State, key("4"))
	if got := back.State.Model.Runs; got.JobUID != "J2" || len(got.Runs) != 1 {
		t.Fatalf("Runs %+v, want the filter and Run kept", got)
	}
	jobs := step(t, step(t, back.State, key("1")).State, key("3"))
	if got := jobs.State.Model.Jobs; got.Selected != "J2" || len(got.Jobs) != 2 {
		t.Fatalf("Jobs %+v, want selection and jobs kept", got)
	}
	flows := step(t, step(t, jobs.State, key("1")).State, key("5"))
	if !flows.State.Model.Workflows.Loading {
		t.Fatalf("Workflows %+v, want a read on entry", flows.State.Model.Workflows)
	}
	again := step(t, step(t, flows.State, key("1")).State, key("5"))
	if !again.State.Model.Workflows.Loading {
		t.Fatalf("Workflows did not read again on re-entry")
	}
	lanes := feed(t, again.State, key("1"), key("b"), model.LanesLoaded{ProjectUID: "p", Lanes: []string{"todo", "done"}}).State
	lanes = step(t, step(t, lanes, key("1")).State, key("b")).State
	if got := lanes.Model.Board.Lanes; len(got) != 2 {
		t.Fatalf("Lanes %v, want them kept", got)
	}
}

func TestAReadAskedBeforeAProjectChangeDoesNotAppear(t *testing.T) {
	s := twoProjectPane(t)
	jobsOut := step(t, s, key("3"))
	oldJobs, _ := askedJobs(jobsOut.Cmds)
	s = chooseInfra(t, jobsOut.State).State
	s = step(t, s, model.JobsLoaded{Request: oldJobs.Request, Jobs: []model.CronJob{promptJob("J1", "Triage")}}).State
	if got := s.Model.Jobs.Jobs; len(got) != 0 {
		t.Fatalf("Jobs %v, want the late answer ignored", got)
	}

	s = twoProjectPane(t)
	runsOut := step(t, s, key("4"))
	oldRuns, _ := askedRuns(runsOut.Cmds)
	s = chooseInfra(t, runsOut.State).State
	s = step(t, s, model.RunsLoaded{Request: oldRuns.Request, Runs: []model.Run{finishedRun("R1")}}).State
	if got := s.Model.Runs.Runs; len(got) != 0 {
		t.Fatalf("Runs %v, want the late answer ignored", got)
	}
}

func TestAProjectInboxProjectRoundTripKeepsEachPagesState(t *testing.T) {
	s := twoProjectPane(t)
	s = answerJobs(t, step(t, s, key("3")), promptJob("J1", "Triage"), promptJob("J2", "Other"))
	s = feed(t, s, key("j")).State
	s = answerRuns(t, step(t, s, key("r")), "", finishedRun("R1"))
	s = step(t, s, key("1")).State

	s = step(t, s, model.CollectionSelected{Collection: model.InboxCollection()}).State
	s = step(t, s, model.CollectionSelected{Collection: model.ProjectCollection("p", "work")}).State

	jobs := step(t, s, key("3"))
	if got := jobs.State.Model.Jobs; got.Selected != "J2" || len(got.Jobs) != 2 {
		t.Fatalf("Jobs %+v, want selection and jobs kept across the Inbox", got)
	}
	runs := step(t, step(t, jobs.State, key("1")).State, key("4"))
	if got := runs.State.Model.Runs; got.JobUID != "J2" || len(got.Runs) != 1 {
		t.Fatalf("Runs %+v, want the filter and Run kept across the Inbox", got)
	}
}

func TestAReadingAskedBeforeAProjectChangeIsNotTakenForTheNewCardsAsk(t *testing.T) {
	five := 5
	text := workflowText(`{"key":"a","kind":"prompt","prompt":"p","options":{"on_fail":{}}}`)
	s := step(t, step(t, twoProjectPane(t), key("5")).State, key("n")).State
	typed := typeWorkflow(t, s, text)
	older, ok := readingAsk(t, typed)
	if !ok {
		t.Fatalf("typing sent %#v, want an ask", typed.Cmds)
	}

	s = chooseInfra(t, typed.State).State
	s = step(t, step(t, s, key("5")).State, key("n")).State
	typed = typeWorkflow(t, s, text)
	newer, ok := readingAsk(t, typed)
	if !ok || newer.Request == older.Request {
		t.Fatalf("asks %d then %d (found %v), want distinct numbers across the Project change", older.Request, newer.Request, ok)
	}
	late := step(t, typed.State, workersAnswer(older, &five, "", false)).State
	if strings.Contains(screen(t, late), "Worker") {
		t.Error("the answer for the other Project's text was drawn")
	}
}
