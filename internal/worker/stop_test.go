package worker_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/worker"
	"github.com/Dangerboi/tern-kata/internal/worker/workertest"
)

// startedRun requests job and waits until its agent runs.
func (h *harness) startedRun(r *running, job worker.Job) string {
	h.t.Helper()
	uid := h.ask(worker.Request{Op: "run", Job: job.UID}).Run.RunUID
	h.eventually("the agent to start", func() bool {
		j, err := r.w.Store().Load(uid)
		return err == nil && j.Phase == worker.PhaseStarted
	})
	return uid
}

func (h *harness) exits(r *running) {
	h.t.Helper()
	h.eventually("the worker to exit", func() bool {
		select {
		case <-r.ended:
			return true
		default:
			return false
		}
	})
	if err := <-r.done; err != nil {
		h.t.Fatalf("the worker exited with %v", err)
	}
}

func TestAnIdleStopLetsTheRunFinishRefusesNewWorkAndThenExits(t *testing.T) {
	h := newHarness(t, "--stub-pause", "1500")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "slow", Prompt: "Do it."})
	r := h.start(worker.Hooks{}, func(o *worker.Options) { o.Supervisor = worker.SupervisorTern })
	h.admitting(r)
	uid := h.startedRun(r, job)

	resp := h.ask(worker.Request{Op: "stop"})
	if !resp.OK || resp.Stop == nil || resp.Stop.Mode != "idle" || resp.Stop.InFlight != 1 || !resp.Stop.Restarts {
		t.Fatalf("stop: %+v", resp)
	}
	if got := r.w.Status().String(); !strings.Contains(got, "Stopping") || !strings.Contains(got, "waiting for 1 run") {
		t.Fatalf("status does not say it is stopping:\n%s", got)
	}
	if refused := h.ask(worker.Request{Op: "run", Job: job.UID}); refused.OK || refused.Error != "the worker is stopping" {
		t.Fatalf("a run request during the drain: %+v", refused)
	}
	if refused := h.ask(worker.Request{Op: "resume", Run: uid}); refused.OK || refused.Error != "the worker is stopping" {
		t.Fatalf("a resume during the drain: %+v", refused)
	}
	if run := h.settled(uid); run.Status != "succeeded" {
		t.Fatalf("the run in flight ended %q: %q", run.Status, run.Summary)
	}
	h.exits(r)
}

func TestAnIdleStopWithNothingRunningExitsAtOnce(t *testing.T) {
	h := newHarness(t)
	r := h.start(worker.Hooks{})
	h.admitting(r)
	resp := h.ask(worker.Request{Op: "stop"})
	if !resp.OK || resp.Stop.InFlight != 0 || resp.Stop.Restarts {
		t.Fatalf("stop: %+v", resp)
	}
	h.exits(r)
}

func TestStopSaysItWillNotRestartWhenWorkerManualIsBesideTheConfig(t *testing.T) {
	h := newHarness(t)
	if err := os.WriteFile(filepath.Join(h.dirs.Config, "worker.manual"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	r := h.start(worker.Hooks{}, func(o *worker.Options) { o.Supervisor = worker.SupervisorTern })
	h.admitting(r)
	if resp := h.ask(worker.Request{Op: "stop"}); !resp.OK || resp.Stop.Restarts {
		t.Fatalf("stop: %+v", resp)
	}
	h.exits(r)
}

func TestAStopNowHoldsTheRunningRunInterruptedAtTheNextStart(t *testing.T) {
	h := newHarness(t, "--stub-hang")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "stuck", Prompt: "Do it."})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	uid := h.startedRun(r, job)

	resp := h.ask(worker.Request{Op: "stop", Mode: "now"})
	if !resp.OK || resp.Stop.Mode != "now" || resp.Stop.InFlight != 1 {
		t.Fatalf("stop now: %+v", resp)
	}
	h.exits(r)

	restarted := h.start(worker.Hooks{})
	h.admitting(restarted)
	held := h.settled(uid)
	if held.Status != "unknown" || !strings.HasPrefix(held.Summary, "interrupted: ") {
		t.Fatalf("run is %q: %q; want unknown, interrupted first", held.Status, held.Summary)
	}
}

func TestAStopNowDuringAnIdleDrainEndsItAtOnce(t *testing.T) {
	h := newHarness(t, "--stub-hang")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "stuck", Prompt: "Do it."})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	h.startedRun(r, job)

	if resp := h.ask(worker.Request{Op: "stop"}); !resp.OK || resp.Stop.Mode != "idle" {
		t.Fatalf("stop: %+v", resp)
	}
	select {
	case <-r.ended:
		t.Fatal("the idle drain ended with a run in flight")
	default:
	}
	if resp := h.ask(worker.Request{Op: "stop", Mode: "now"}); !resp.OK || resp.Stop.Mode != "now" {
		t.Fatalf("stop now: %+v", resp)
	}
	h.exits(r)
}

func TestAStopWithAnUnknownModeIsRefused(t *testing.T) {
	h := newHarness(t)
	r := h.start(worker.Hooks{})
	h.admitting(r)
	if resp := h.ask(worker.Request{Op: "stop", Mode: "soon"}); resp.OK || !strings.Contains(resp.Error, `"soon"`) {
		t.Fatalf("stop soon: %+v", resp)
	}
}
