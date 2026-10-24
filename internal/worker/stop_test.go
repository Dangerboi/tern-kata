package worker_test

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

// release lets the agent holding on --stub-wait-for file end its turn.
func release(t *testing.T, file string) {
	t.Helper()
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestAnIdleStopLetsTheRunFinishRefusesNewWorkAndThenExits(t *testing.T) {
	gate := filepath.Join(t.TempDir(), "release")
	h := newHarness(t, "--stub-wait-for", gate)
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
	release(t, gate)
	if run := h.settled(uid); run.Status != "succeeded" {
		t.Fatalf("the run in flight ended %q: %q", run.Status, run.Summary)
	}
	h.exits(r)
}

// A run whose agent finished but whose report kata refused for now is still owed: the drain waits for the report
// retry to settle it.
func TestAnIdleStopWaitsForARunWhoseReportKataRefusedUntilItIsSettled(t *testing.T) {
	gate := filepath.Join(t.TempDir(), "release")
	h := newHarness(t, "--stub-wait-for", gate)
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "slow", Prompt: "Do it."})
	r := h.start(worker.Hooks{}, func(o *worker.Options) { o.Supervisor = worker.SupervisorTern })
	h.admitting(r)
	uid := h.startedRun(r, job)

	var refused atomic.Int32
	restore := h.kata.Intercept(func(q *http.Request) bool {
		return q.Method != http.MethodGet && strings.Contains(q.URL.Path, "/cron/runs/")
	}, func(w http.ResponseWriter, _ *http.Request, _ http.Handler) {
		refused.Add(1)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	})
	if resp := h.ask(worker.Request{Op: "stop"}); !resp.OK || resp.Stop.InFlight != 1 {
		t.Fatalf("stop: %+v", resp)
	}
	release(t, gate)
	h.eventually("the report to be refused and retried", func() bool { return refused.Load() >= 2 })
	select {
	case <-r.ended:
		t.Fatal("the drain ended while kata did not hold the run settled")
	default:
	}
	if j, err := r.w.Store().Load(uid); err != nil || j.Phase == worker.PhaseSettled {
		t.Fatalf("journal %v, %v", j, err)
	}
	restore()
	if run := h.settled(uid); run.Status != "succeeded" {
		t.Fatalf("the run ended %q: %q", run.Status, run.Summary)
	}
	h.exits(r)
	j, err := r.w.Store().Load(uid)
	if err != nil || j.Phase != worker.PhaseSettled {
		t.Fatalf("the worker exited with the run %v (%v)", j.Phase, err)
	}
}

// A run request that passed the stopping check but has not reached its run goroutine holds an idle stop.
func TestAnIdleStopAskedWhileARunIsBeingAdmittedWaitsForThatRun(t *testing.T) {
	h := newHarness(t)
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "quick", Prompt: "Do it."})
	atPrepared, proceed := make(chan struct{}), make(chan struct{})
	var once sync.Once
	r := h.start(worker.Hooks{AfterSave: func(point string, _ *worker.Journal) error {
		if point == "prepared" {
			once.Do(func() { close(atPrepared); <-proceed })
		}
		return nil
	}})
	h.admitting(r)
	requested := h.askAsync(worker.Request{Op: "run", Job: job.UID})
	<-atPrepared

	resp := h.ask(worker.Request{Op: "stop"})
	if !resp.OK || resp.Stop.InFlight != 1 {
		t.Fatalf("stop: %+v", resp)
	}
	select {
	case <-r.ended:
		t.Fatal("the idle stop ended the worker while a run was being admitted")
	case <-time.After(200 * time.Millisecond):
	}
	close(proceed)
	accepted := <-requested
	if !accepted.resp.OK {
		t.Fatalf("the run request: %+v", accepted.resp)
	}
	if run := h.settled(accepted.resp.Run.RunUID); run.Status != "succeeded" {
		t.Fatalf("the run ended %q: %q", run.Status, run.Summary)
	}
	h.exits(r)
}

// A run the last worker left owing its report holds an idle stop asked before the Project has even resolved.
func TestAnIdleStopAskedAtOnceWaitsForTheReportTheLastWorkerOwed(t *testing.T) {
	h := newHarness(t)
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "review", Prompt: "Do it.", Checkout: "repo"})
	uid, _ := h.killedAt("finished", job)

	slow := make(chan struct{})
	h.kata.Intercept(func(q *http.Request) bool {
		return q.Method == http.MethodGet && strings.HasSuffix(q.URL.Path, "/projects")
	}, func(w http.ResponseWriter, q *http.Request, next http.Handler) {
		select {
		case <-slow:
			next.ServeHTTP(w, q)
		case <-q.Context().Done():
		}
	})
	r := h.start(worker.Hooks{})
	resp := h.ask(worker.Request{Op: "stop"})
	if !resp.OK || resp.Stop.InFlight != 1 {
		t.Fatalf("stop: %+v", resp)
	}
	select {
	case <-r.ended:
		t.Fatal("the idle stop ended the worker with a report still owed")
	case <-time.After(200 * time.Millisecond):
	}
	close(slow)
	if run := h.settled(uid); run.Status != "succeeded" {
		t.Fatalf("the owed run ended %q: %q", run.Status, run.Summary)
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

func TestStopSaysItWillNotRestartWhenTheSupervisionMarkerIsBesideTheConfig(t *testing.T) {
	h := newHarness(t)
	if err := os.WriteFile(h.dirs.ManualPath(), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	r := h.start(worker.Hooks{}, func(o *worker.Options) { o.Supervisor = worker.SupervisorTern })
	h.admitting(r)
	if resp := h.ask(worker.Request{Op: "stop"}); !resp.OK || resp.Stop.Restarts {
		t.Fatalf("stop: %+v", resp)
	}
	h.exits(r)
}

// The promise is read at each stop: a marker made or removed between two stops changes the second answer.
func TestAStopPromisesARestartFromTheSupervisionRuleAtThatMoment(t *testing.T) {
	gate := filepath.Join(t.TempDir(), "release")
	h := newHarness(t, "--stub-wait-for", gate)
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "slow", Prompt: "Do it."})
	r := h.start(worker.Hooks{}, func(o *worker.Options) { o.Supervisor = worker.SupervisorTern })
	h.admitting(r)
	uid := h.startedRun(r, job)
	restarts := func() bool {
		t.Helper()
		resp := h.ask(worker.Request{Op: "stop"})
		if !resp.OK || resp.Stop == nil {
			t.Fatalf("stop: %+v", resp)
		}
		return resp.Stop.Restarts
	}
	if !restarts() {
		t.Fatal("no restart promised with worker.toml present and supervision on")
	}
	if err := h.dirs.SetSupervision(false); err != nil {
		t.Fatal(err)
	}
	if restarts() {
		t.Fatal("a restart promised after the marker was created")
	}
	if err := h.dirs.SetSupervision(true); err != nil {
		t.Fatal(err)
	}
	if !restarts() {
		t.Fatal("no restart promised after the marker was removed")
	}
	release(t, gate)
	h.settled(uid)
	h.exits(r)
}

// A worker started by hand has no supervisor, so it promises no restart whatever the files say.
func TestAStopOfAWorkerStartedByHandPromisesNoRestart(t *testing.T) {
	h := newHarness(t)
	r := h.start(worker.Hooks{})
	h.admitting(r)
	if resp := h.ask(worker.Request{Op: "stop"}); !resp.OK || resp.Stop.Restarts {
		t.Fatalf("stop: %+v", resp)
	}
	h.exits(r)
}

// The plugin starts a worker only while worker.toml exists, so a stop after the file was removed promises no restart.
func TestStopSaysItWillNotRestartOnceWorkerTomlIsRemoved(t *testing.T) {
	h := newHarness(t)
	r := h.start(worker.Hooks{}, func(o *worker.Options) { o.Supervisor = worker.SupervisorTern })
	h.admitting(r)
	if err := os.Remove(h.dirs.ConfigPath()); err != nil {
		t.Fatal(err)
	}
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
