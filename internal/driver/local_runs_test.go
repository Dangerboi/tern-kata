package driver_test

import (
	"errors"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
)

// This machine's worker, as the Pane sees it over the control socket: its status and the local state of its runs.

func askedWorker(cmds []model.Cmd) (model.LoadWorker, bool) {
	for _, cmd := range cmds {
		if load, ok := cmd.(model.LoadWorker); ok {
			return load, true
		}
	}
	return model.LoadWorker{}, false
}

// answerWorker is the worker answering the status ask in out with running = status != nil.
func answerWorker(t *testing.T, out driver.Output, status *model.WorkerStatus) driver.Output {
	t.Helper()
	load, ok := askedWorker(out.Cmds)
	if !ok {
		t.Fatalf("no worker status asked for: %#v", out.Cmds)
	}
	msg := model.WorkerLoaded{Request: load.Request}
	if status != nil {
		msg.Running, msg.Status = true, *status
	}
	return step(t, out.State, msg)
}

func servingWorker(runs ...model.LocalRun) *model.WorkerStatus {
	return &model.WorkerStatus{Executor: "laptop", Projects: []model.WorkerProject{{Name: "work", Admitting: true, InUse: 1, Capacity: 2}}, Runs: runs}
}

func heldRun(uid string) model.LocalRun {
	return model.LocalRun{UID: uid, Project: "work", Job: "Triage", State: "held: interrupted", Attempt: 1, Held: true, Hold: "interrupted",
		Detail: "the worker stopped while the agent ran", Resumable: true, Abandonable: true}
}

// jobsPaneWithWorker is a Jobs Page whose job and worker status reads are answered.
func jobsPaneWithWorker(t *testing.T, status *model.WorkerStatus, jobs ...model.CronJob) driver.State {
	t.Helper()
	out := step(t, projectPane(t, 160, boardCard("parent-a")), key("3"))
	s := answerJobs(t, out, jobs...)
	return answerWorker(t, driver.Output{State: s, Cmds: out.Cmds}, status).State
}

func TestJobsPageSaysWhetherThisMachinesWorkerRunsAndServesTheProject(t *testing.T) {
	cases := []struct {
		name   string
		status *model.WorkerStatus
		want   string
	}{
		{"no worker", nil, "No worker is running on this machine"},
		{"serving", servingWorker(), `Worker laptop running and serving work: 1 of 2 slots in use`},
		{"not served", &model.WorkerStatus{Executor: "laptop", Projects: []model.WorkerProject{{Name: "other", Admitting: true}}}, "does not serve work"},
		{"lease held elsewhere", &model.WorkerStatus{Executor: "laptop", Projects: []model.WorkerProject{{Name: "work", Why: "its lease is held by worker desktop"}}}, "admits nothing: its lease is held by worker desktop"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := jobsPaneWithWorker(t, c.status, promptJob("A1", "Triage"))
			wantText(t, "Jobs Page", wireTextOf(t, s), c.want)
		})
	}
}

func TestRRunsTheSelectedAdapterJobOnTheWorker(t *testing.T) {
	s := jobsPaneWithWorker(t, servingWorker(), promptJob("A1", "Triage"))
	out := step(t, s, key("shift+r"))
	if len(out.Cmds) != 1 || out.Cmds[0] != (model.RunJobNow{JobUID: "A1", JobName: "Triage"}) {
		t.Fatalf("cmds %#v, want a run-now ask for the selected job", out.Cmds)
	}
	done := step(t, out.State, model.JobRunRequested{JobUID: "A1", JobName: "Triage", RunUID: "R9"})
	wantText(t, "notice", done.State.Model.Notice, "R9")
	refused := step(t, out.State, model.JobRunRequested{JobUID: "A1", JobName: "Triage", Err: errors.New("Project work admits nothing: no free slot")})
	wantText(t, "notice", refused.State.Model.Notice, "no free slot")
}

func TestRRefusesANativeJobAndAMissingOrNonServingWorker(t *testing.T) {
	native := jobsPaneWithWorker(t, servingWorker(), nativeJob("N1", "Hourly", jobParent("open", false)))
	out := step(t, native, key("shift+r"))
	if len(out.Cmds) != 0 {
		t.Errorf("a native job asked the worker: %#v", out.Cmds)
	}
	wantText(t, "native refusal", out.State.Model.Notice, "native job", "kata runs it")

	none := jobsPaneWithWorker(t, nil, promptJob("A1", "Triage"))
	out = step(t, none, key("shift+r"))
	if len(out.Cmds) != 0 {
		t.Errorf("no worker, yet the Pane asked: %#v", out.Cmds)
	}
	wantText(t, "no worker refusal", out.State.Model.Notice, "No worker is running")

	elsewhere := jobsPaneWithWorker(t, &model.WorkerStatus{Executor: "laptop", Projects: []model.WorkerProject{{Name: "other", Admitting: true}}}, promptJob("A1", "Triage"))
	out = step(t, elsewhere, key("shift+r"))
	if len(out.Cmds) != 0 {
		t.Errorf("a Project the worker does not serve was asked for: %#v", out.Cmds)
	}
	wantText(t, "not served refusal", out.State.Model.Notice, "does not serve work")
}

// localRunsPane is a Runs Page whose runs and worker status reads are answered. Runs of executor "laptop" are this machine's.
func localRunsPane(t *testing.T, status *model.WorkerStatus, runs ...model.Run) driver.Output {
	t.Helper()
	out := step(t, projectPane(t, 160, boardCard("parent-a")), key("4"))
	out.State = answerRuns(t, out, "", runs...)
	return answerWorker(t, out, status)
}

func heldKataRun(uid string) model.Run {
	r := finishedRun(uid)
	r.Status, r.EndedAt, r.Message = model.RunUnknown, nil, ""
	return r
}

func TestRunsPageShowsAHeldRunWithItsReasonAndOtherExecutorsRunsAsKataHoldsThem(t *testing.T) {
	other := finishedRun("R2")
	other.ExecutorLabel = "desktop"
	out := localRunsPane(t, servingWorker(heldRun("R1")), heldKataRun("R1"), other)

	wantText(t, "held row", runCardText(t, out.State, "R1"), "held", "interrupted", "on laptop")
	wantText(t, "other executor's row", runCardText(t, out.State, "R2"), "succeeded", "on desktop")
	if _, ok := askedLocalRun(out.Cmds); !ok {
		t.Fatalf("the selected run of this machine asked the worker for no detail: %#v", out.Cmds)
	}
	for _, cmd := range step(t, out.State, key("j")).Cmds {
		if _, ok := cmd.(model.LoadLocalRun); ok {
			t.Errorf("a run of another executor asked the worker for its detail")
		}
	}
}

func askedLocalRun(cmds []model.Cmd) (model.LoadLocalRun, bool) {
	for _, cmd := range cmds {
		if load, ok := cmd.(model.LoadLocalRun); ok {
			return load, true
		}
	}
	return model.LoadLocalRun{}, false
}

func localDetail(uid string, run model.LocalRun) model.LocalRunDetail {
	code := 1
	return model.LocalRunDetail{LocalRun: run, Cwd: "/home/me/checkout", SessionDir: "/state/runs/" + uid + "/session", SessionID: "sess-1",
		Steps:      []model.LocalStep{{Index: 1, Kind: "prompt", Attempt: 2, State: run.State, ExitCode: &code}},
		OutputTail: "I triaged four items and now wait for review", StderrTail: "warning: slow disk"}
}

func TestRunDetailShowsTheHeldReasonStepsAndTheOutputTail(t *testing.T) {
	run := heldRun("R1")
	out := localRunsPane(t, servingWorker(run), heldKataRun("R1"))
	s := step(t, out.State, model.LocalRunLoaded{UID: "R1", Detail: localDetail("R1", run)}).State
	s = step(t, s, key("enter")).State
	wantText(t, "run detail", inspectorText(t, s), "Held because", "interrupted", "the worker stopped while the agent ran", "u continues", "Step 1", "attempt 2", "exit 1",
		"I triaged four items and now wait for review", "warning: slow disk", "sess-1")
}

func TestRunsPageFollowsARunningRunUntilItSettles(t *testing.T) {
	running := model.LocalRun{UID: "R1", Project: "work", Job: "Triage", State: "running", Attempt: 1}
	live := heldKataRun("R1")
	live.Status = model.RunRunning
	out := localRunsPane(t, servingWorker(running), live)
	poll, ok := askedWorker(out.Cmds)
	if !ok || poll.After <= 0 {
		t.Fatalf("cmds %#v, want a status ask after a pause while a run runs", out.Cmds)
	}
	s := step(t, out.State, model.LocalRunLoaded{UID: "R1", Detail: localDetail("R1", running)}).State
	if text := runCardText(t, s, "R1"); text == "" {
		t.Fatal("no card")
	}
	// The pause ends: the worker answers again, the newest runs are read to follow the Run and the detail is read again.
	next := step(t, s, model.WorkerLoaded{Request: poll.Request, Running: true, Status: *servingWorker(running)})
	if _, ok := askedRuns(next.Cmds); !ok {
		t.Errorf("cmds %#v, want the newest runs read again", next.Cmds)
	}
	if _, ok := askedLocalRun(next.Cmds); !ok {
		t.Errorf("cmds %#v, want the run's detail read again", next.Cmds)
	}
	// Once the worker reports nothing running and kata agrees, the polling ends.
	settled := running
	settled.State = "succeeded"
	finished := live
	finished.Status = model.RunSucceeded
	s = step(t, next.State, model.RunsLoaded{Request: runsRequest(next.Cmds), Runs: []model.Run{finished}, Refresh: true}).State
	done := step(t, s, model.WorkerLoaded{Request: nextRequest(next.Cmds), Running: true, Status: *servingWorker(settled)})
	if _, ok := askedWorker(done.Cmds); ok {
		t.Errorf("cmds %#v, want the polling ended", done.Cmds)
	}
	wantText(t, "settled row", runCardText(t, done.State, "R1"), "succeeded")
}

func runsRequest(cmds []model.Cmd) int64 {
	load, _ := askedRuns(cmds)
	return load.Request
}

func nextRequest(cmds []model.Cmd) int64 {
	load, _ := askedWorker(cmds)
	return load.Request
}

func TestOOpensTheRunsSessionInANewTabForAFinishedOrHeldRunOfThisMachine(t *testing.T) {
	run := heldRun("R1")
	out := localRunsPane(t, servingWorker(run), heldKataRun("R1"))
	s := step(t, out.State, model.LocalRunLoaded{UID: "R1", Detail: localDetail("R1", run)}).State
	got := step(t, s, key("o"))
	want := model.OpenRunSession{Cwd: "/home/me/checkout", SessionDir: "/state/runs/R1/session", SessionID: "sess-1"}
	if len(got.Cmds) != 1 || got.Cmds[0] != want {
		t.Fatalf("cmds %#v, want %#v", got.Cmds, want)
	}

	// A run still running has nothing to open yet.
	running := model.LocalRun{UID: "R1", State: "running", Attempt: 1}
	s = step(t, s, model.LocalRunLoaded{UID: "R1", Detail: localDetail("R1", running)}).State
	refused := step(t, s, key("o"))
	if len(refused.Cmds) != 0 {
		t.Errorf("a running run opened a session: %#v", refused.Cmds)
	}
	wantText(t, "refusal", refused.State.Model.Notice, "still running")
}

func TestOUAndAAreRefusedOnARunOfAnotherExecutor(t *testing.T) {
	other := finishedRun("R2")
	other.ExecutorLabel = "desktop"
	out := localRunsPane(t, servingWorker(), other)
	for _, k := range []string{"o", "u", "shift+a"} {
		got := step(t, out.State, key(k))
		if len(got.Cmds) != 0 {
			t.Errorf("%s on another executor's run: %#v", k, got.Cmds)
		}
		wantText(t, k+" refusal", got.State.Model.Notice, `executor "desktop"`)
	}
}

func TestUResumesAndAAbandonsTheHeldRunThroughTheWorker(t *testing.T) {
	run := heldRun("R1")
	out := localRunsPane(t, servingWorker(run), heldKataRun("R1"))
	resumed := step(t, out.State, key("u"))
	if len(resumed.Cmds) != 1 || resumed.Cmds[0] != (model.ResumeLocalRun{UID: "R1"}) {
		t.Fatalf("u: cmds %#v", resumed.Cmds)
	}
	abandoned := step(t, out.State, key("shift+a"))
	if len(abandoned.Cmds) != 1 || abandoned.Cmds[0] != (model.AbandonLocalRun{UID: "R1"}) {
		t.Fatalf("A: cmds %#v", abandoned.Cmds)
	}
	// The worker's refusal is shown; its acceptance reads the worker and the runs again.
	refused := step(t, resumed.State, model.LocalRunMoved{UID: "R1", Resumed: true, Err: errors.New("run R1 cannot be resumed: its Project's lease is held elsewhere")})
	wantText(t, "refusal", refused.State.Model.Notice, "Cannot resume", "lease is held elsewhere")
	accepted := step(t, resumed.State, model.LocalRunMoved{UID: "R1", Resumed: true, State: "resuming"})
	if _, ok := askedWorker(accepted.Cmds); !ok {
		t.Errorf("cmds %#v, want the worker's status read again", accepted.Cmds)
	}
}

func TestUOnARunThatIsNotHeldSaysSo(t *testing.T) {
	settled := model.LocalRun{UID: "R1", State: "succeeded", Attempt: 1}
	done := finishedRun("R1")
	out := localRunsPane(t, servingWorker(settled), done)
	got := step(t, out.State, key("u"))
	if len(got.Cmds) != 0 {
		t.Errorf("cmds %#v", got.Cmds)
	}
	wantText(t, "refusal", got.State.Model.Notice, "not held")
}

func TestACancelsARunningRunOfThisMachineAndUStillNeedsAHeldOne(t *testing.T) {
	running := model.LocalRun{UID: "R1", Project: "work", Job: "Triage", State: "running", Attempt: 1, Abandonable: true}
	kataRun := workflowRun("R1")
	kataRun.ExecutorLabel = "laptop"
	out := localRunsPane(t, servingWorker(running), kataRun)

	cancelled := step(t, out.State, key("shift+a"))
	if len(cancelled.Cmds) != 1 || cancelled.Cmds[0] != (model.AbandonLocalRun{UID: "R1"}) {
		t.Fatalf("A on a running run: cmds %#v, want the worker asked to cancel it", cancelled.Cmds)
	}
	wantText(t, "inspector", inspectorText(t, out.State), "A cancels the run")
	moved := step(t, cancelled.State, model.LocalRunMoved{UID: "R1", State: "cancelling"})
	wantText(t, "notice", moved.State.Model.Notice, "Cancelling the run")

	resumed := step(t, out.State, key("u"))
	if len(resumed.Cmds) != 0 {
		t.Errorf("u on a running run sent %#v", resumed.Cmds)
	}
	wantText(t, "refusal", resumed.State.Model.Notice, "not held")

	// A running run the worker would not cancel (a settled one) is refused.
	settled := model.LocalRun{UID: "R1", State: "succeeded", Attempt: 1}
	out = localRunsPane(t, servingWorker(settled), finishedRun("R1"))
	if got := step(t, out.State, key("shift+a")); len(got.Cmds) != 0 {
		t.Errorf("A on a settled run sent %#v", got.Cmds)
	}
}
