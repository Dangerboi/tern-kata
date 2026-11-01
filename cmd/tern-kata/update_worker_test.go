package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/worker"
)

// updateWithWorker is an update fixture on the config and state folders of a worker fixture, so the update sees the real
// worker the test serves.
func updateWithWorker(t *testing.T, w workerFixture) *updateFixture {
	t.Helper()
	f := newUpdateFixture(t, "v0.1.0", "0.2.0", nil)
	f.env.Dirs = w.env.Dirs
	return f
}

func workerAnswers(f *updateFixture) bool {
	_, err := worker.Ask(context.Background(), f.env.Dirs, worker.Request{Op: "status"})
	return !errors.Is(err, worker.ErrNoWorker)
}

func TestUpdateStopsASupervisedWorkerWhenIdleAndPrintsTheAnswerAsWorkerStopDoes(t *testing.T) {
	w := newWorkerFixture(t, "")
	w.env.Supervisor = worker.SupervisorTern
	w.serve(t)
	f := updateWithWorker(t, w)
	if err := f.run(LaunchPlan{Subcommand: "update"}); err != nil {
		t.Fatal(err)
	}
	if want := "Stopping when idle: 0 runs in flight, so the worker exits now. The plugin will start a fresh worker.\n"; !strings.Contains(f.out.String(), want) {
		t.Errorf("output lacks %q:\n%s", want, f.out.String())
	}
	eventually(t, "the worker to exit", func() bool { return !workerAnswers(f) })
}

func TestUpdateOnlyNamesAWorkerStartedByHand(t *testing.T) {
	w := newWorkerFixture(t, "")
	w.serve(t)
	f := updateWithWorker(t, w)
	if err := f.run(LaunchPlan{Subcommand: "update"}); err != nil {
		t.Fatal(err)
	}
	if want := "The worker was started by hand or supervision is off: restart it to use v0.2.0\n"; !strings.Contains(f.out.String(), want) || strings.Contains(f.out.String(), "Stopping") {
		t.Errorf("output:\n%s", f.out.String())
	}
	if !workerAnswers(f) {
		t.Error("the worker was stopped")
	}
}

func TestUpdateOnlyNamesASupervisedWorkerWhileSupervisionIsOff(t *testing.T) {
	w := newWorkerFixture(t, "")
	w.env.Supervisor = worker.SupervisorTern
	w.serve(t)
	if err := os.WriteFile(filepath.Join(w.env.Dirs.Config, worker.ManualFileName), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	f := updateWithWorker(t, w)
	if err := f.run(LaunchPlan{Subcommand: "update"}); err != nil {
		t.Fatal(err)
	}
	if want := "The worker was started by hand or supervision is off: restart it to use v0.2.0\n"; !strings.Contains(f.out.String(), want) || strings.Contains(f.out.String(), "Stopping") {
		t.Errorf("output:\n%s", f.out.String())
	}
	if !workerAnswers(f) {
		t.Error("the worker was stopped")
	}
}

func TestUpdateSaysNothingAboutAWorkerWhenNoneRuns(t *testing.T) {
	w := newWorkerFixture(t, "")
	f := updateWithWorker(t, w)
	if err := f.run(LaunchPlan{Subcommand: "update"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(f.out.String()), "worker") {
		t.Errorf("output mentions the worker:\n%s", f.out.String())
	}
}

func TestUpdateDoesNotStopTheWorkerAfterAForcedConfigRefusal(t *testing.T) {
	w := newWorkerFixture(t, "")
	w.env.Supervisor = worker.SupervisorTern
	w.serve(t)
	f := updateWithWorker(t, w)
	f.preflight(t, refusingConfig, 1, "")
	if err := f.run(LaunchPlan{Subcommand: "update", Force: true}); err != nil {
		t.Fatal(err)
	}
	if want := "Run `tern-kata worker upgrade` (or `worker edit`), then `tern-kata worker stop`."; !strings.Contains(f.out.String(), want) || strings.Contains(f.out.String(), "Stopping") {
		t.Errorf("output:\n%s", f.out.String())
	}
	if !workerAnswers(f) {
		t.Error("the worker was stopped")
	}
}

func TestUpToDateUpdateLeavesTheWorkerAlone(t *testing.T) {
	w := newWorkerFixture(t, "")
	w.env.Supervisor = worker.SupervisorTern
	w.serve(t)
	f := newUpdateFixture(t, "v0.2.0", "0.2.0", nil)
	f.env.Dirs = w.env.Dirs
	if err := f.run(LaunchPlan{Subcommand: "update"}); err != nil {
		t.Fatal(err)
	}
	if f.out.String() != "tern-kata v0.2.0 is up to date\n" || !workerAnswers(f) {
		t.Errorf("output %q, worker answering %v", f.out.String(), workerAnswers(f))
	}
}
