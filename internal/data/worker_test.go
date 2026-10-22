package data

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"slices"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/worker"
)

// fakeWorker answers the control socket of dirs the way the worker does, from answers keyed by op, and records the requests.
func fakeWorker(t *testing.T, answers map[string]worker.Response) (worker.Dirs, *[]worker.Request) {
	t.Helper()
	root, err := os.MkdirTemp("", "tk")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	dirs := worker.Dirs{Config: root, State: root}
	ln, err := net.Listen("unix", dirs.Socket())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	var seen []worker.Request
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			var req worker.Request
			_ = json.NewDecoder(conn).Decode(&req)
			seen = append(seen, req)
			_ = json.NewEncoder(conn).Encode(answers[req.Op])
			conn.Close()
		}
	}()
	return dirs, &seen
}

func workerGateway(dirs worker.Dirs) *Gateway {
	g := NewGateway(nil)
	g.workerDirs = func() (worker.Dirs, error) { return dirs, nil }
	return g
}

func TestWorkerStatusReachesThePaneAndNoWorkerIsNotAnError(t *testing.T) {
	status := worker.Status{Executor: "laptop", Projects: []worker.ProjectStatus{{Name: "work", Admitting: true, Capacity: 2, InUse: 1}},
		Runs: []worker.RunStatus{{UID: "R1", State: "held: interrupted", Held: true, Hold: "interrupted", Detail: "stopped", Resumable: true, Abandonable: true}}}
	dirs, _ := fakeWorker(t, map[string]worker.Response{"status": {OK: true, Status: &status}})
	got := as[model.WorkerLoaded](t, run(t, workerGateway(dirs), model.LoadWorker{Request: 4}))
	if !got.Running || got.Request != 4 || got.Err != nil || got.Status.Executor != "laptop" || len(got.Status.Projects) != 1 || !got.Status.Projects[0].Admitting {
		t.Fatalf("status %+v", got)
	}
	if r := got.Status.Runs; len(r) != 1 || !r[0].Held || r[0].Hold != "interrupted" || !r[0].Resumable || !r[0].Abandonable {
		t.Fatalf("runs %+v", r)
	}

	empty, err := os.MkdirTemp("", "tk")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(empty)
	none := as[model.WorkerLoaded](t, run(t, workerGateway(worker.Dirs{Config: empty, State: empty}), model.LoadWorker{Request: 5}))
	if none.Running || none.Err != nil || none.Request != 5 {
		t.Fatalf("no worker: %+v", none)
	}
}

func TestRunNowResumeAndAbandonCarryTheWorkersAnswerAndRefusal(t *testing.T) {
	dirs, seen := fakeWorker(t, map[string]worker.Response{
		"run":     {OK: true, Run: &worker.RunAccepted{RunUID: "R9", JobUID: "J1"}},
		"resume":  {OK: true, Transition: &worker.RunTransition{RunUID: "R1", State: "resuming"}},
		"abandon": {Error: "run R1 cannot be abandoned: it is not held"},
	})
	g := workerGateway(dirs)
	if got := as[model.JobRunRequested](t, run(t, g, model.RunJobNow{JobUID: "J1", JobName: "Triage"})); got.Err != nil || got.RunUID != "R9" {
		t.Errorf("run now: %+v", got)
	}
	if got := as[model.LocalRunMoved](t, run(t, g, model.ResumeLocalRun{UID: "R1"})); got.Err != nil || !got.Resumed || got.State != "resuming" {
		t.Errorf("resume: %+v", got)
	}
	got := as[model.LocalRunMoved](t, run(t, g, model.AbandonLocalRun{UID: "R1"}))
	if got.Resumed || got.Err == nil || got.Err.Error() != "run R1 cannot be abandoned: it is not held" {
		t.Errorf("abandon: %+v", got)
	}
	if len(*seen) != 3 || (*seen)[0].Job != "J1" || (*seen)[1].Run != "R1" || (*seen)[2].Op != "abandon" {
		t.Errorf("requests %+v", *seen)
	}
	missing := as[model.JobRunRequested](t, run(t, workerGateway(worker.Dirs{Config: "/nonexistent", State: "/nonexistent"}), model.RunJobNow{JobUID: "J1"}))
	if !errors.Is(missing.Err, worker.ErrNoWorker) {
		t.Errorf("no worker: %v", missing.Err)
	}
}

func TestRunDetailCarriesStepsOutputTailAndWhereToResume(t *testing.T) {
	code := 1
	detail := worker.RunDetail{RunStatus: worker.RunStatus{UID: "R1", State: "failed"}, OutputTail: "said it", Cwd: "/work", SessionDir: "/runs/R1/session", SessionID: "s1",
		Steps: []worker.StepDetail{{Index: 1, Kind: "prompt", Attempt: 1, State: "failed", ExitCode: &code}}}
	dirs, _ := fakeWorker(t, map[string]worker.Response{"run-detail": {OK: true, Detail: &detail}})
	got := as[model.LocalRunLoaded](t, run(t, workerGateway(dirs), model.LoadLocalRun{UID: "R1"}))
	if got.Err != nil || got.Detail.OutputTail != "said it" || got.Detail.SessionID != "s1" || got.Detail.Cwd != "/work" || got.Detail.SessionDir != "/runs/R1/session" ||
		len(got.Detail.Steps) != 1 || got.Detail.Steps[0].ExitCode == nil || *got.Detail.Steps[0].ExitCode != 1 {
		t.Fatalf("detail %+v", got)
	}
}

func TestOpeningASessionRunsTernNewTabInTheRunsFolderWithTheSessionResumed(t *testing.T) {
	var ran []string
	g := NewGateway(nil)
	g.runTern = func(_ context.Context, args []string) ([]byte, error) { ran = args; return nil, nil }
	got := as[model.RunSessionOpened](t, run(t, g, model.OpenRunSession{Cwd: "/home/me/my repo", SessionDir: "/state/runs/R1/session", SessionID: "sess-1"}))
	want := []string{"new", "tab", "--cwd", "/home/me/my repo", "--", "'omp'", "'--resume'", "'sess-1'", "'--session-dir'", "'/state/runs/R1/session'"}
	if got.Err != nil || !slices.Equal(ran, want) {
		t.Fatalf("ran %q (err %v), want %q", ran, got.Err, want)
	}
	g.runTern = func(context.Context, []string) ([]byte, error) {
		return []byte("tern: there is no session\n"), errors.New("exit status 1")
	}
	if failed := as[model.RunSessionOpened](t, run(t, g, model.OpenRunSession{})); failed.Err == nil || failed.Err.Error() != "exit status 1: tern: there is no session" {
		t.Errorf("failure %v", failed.Err)
	}
}
