//go:build !windows

package worker_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/worker"
	"github.com/Dangerboi/tern-kata/internal/worker/workertest"
)

// A held run whose recorded process group is still present can be neither resumed nor abandoned (the worker never
// stops a process to make that possible), keeps its slot, and becomes resumable once the group is gone.
func TestAHeldRunWhoseProcessGroupIsStillPresentWaitsUntilItIsGone(t *testing.T) {
	h := newHarness(t, "--stub-hang-first")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "review", Prompt: "Do it.", Checkout: "repo"})
	uid, _ := h.killedAt("session", job)
	first := h.start(worker.Hooks{})
	h.admitting(first)
	h.settled(uid)
	h.eventually("the hold to be reported", func() bool { s, ok := h.runStatus(first, uid); return ok && s.Resumable })
	first.stop()

	// Another process the agent left behind: a group of its own, recorded as the run's.
	sleeper := exec.Command("sleep", "300")
	sleeper.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := sleeper.Start(); err != nil {
		t.Fatal(err)
	}
	gone := false
	killSleeper := func() {
		if !gone {
			gone = true
			_ = syscall.Kill(-sleeper.Process.Pid, syscall.SIGKILL)
			_ = sleeper.Wait()
		}
	}
	t.Cleanup(killSleeper)
	store := worker.NewStore(h.dirs, worker.Hooks{})
	if _, err := store.Update(uid, "test", func(j *worker.Journal) error {
		j.Launch.PID, j.Launch.PGID, j.Launch.StartTicks = sleeper.Process.Pid, sleeper.Process.Pid, 0
		j.Outcome.Hold = worker.HoldGroupUnproven
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	r := h.start(worker.Hooks{})
	h.admitting(r)
	s, _ := h.runStatus(r, uid)
	if s.Resumable || s.Abandonable || !strings.Contains(s.ResumeRefusal, "is still present") {
		t.Fatalf("status %+v", s)
	}
	for _, op := range []string{"resume", "abandon"} {
		if resp := h.ask(worker.Request{Op: op, Run: uid}); resp.OK || !strings.Contains(resp.Error, "is still present") {
			t.Fatalf("%s while the group lives: ok=%v %q", op, resp.OK, resp.Error)
		}
	}
	if r.w.Status().Projects[0].InUse != 1 {
		t.Fatal("a held run with a live process group must keep its slot")
	}

	killSleeper()
	h.eventually("the slot to be freed", func() bool { return r.w.Status().Projects[0].InUse == 0 })
	h.eventually("the hold to be resumable", func() bool { s, ok := h.runStatus(r, uid); return ok && s.Resumable })
	if resp := h.ask(worker.Request{Op: "resume", Run: uid}); !resp.OK {
		t.Fatalf("resume after the group is gone: %q", resp.Error)
	}
	h.eventually("kata to hold the run as succeeded", func() bool {
		run, _ := h.kata.Kata.ShowRun(context.Background(), h.proj.ID, uid)
		return run.Status == "succeeded"
	})
}

// A command step whose leader ends on SIGTERM but whose child ignores it and has let go of the output pipes: the group
// outlives the first signal, and the command's wait ends without it.
const stubbornCommand = "(trap '' TERM; exec >/dev/null 2>&1; sleep 300 & echo $! > child.pid; wait) & exec sleep 300"

// stubbornChild is the pid the stubborn command's child recorded; it kills it when the test ends.
func (h *harness) stubbornChild() int {
	h.t.Helper()
	var pid int
	h.eventually("the command's child to record its pid", func() bool {
		raw, err := os.ReadFile(filepath.Join(h.repo, "child.pid"))
		pid, err = strconv.Atoi(strings.TrimSpace(string(raw)))
		return err == nil && pid > 0
	})
	h.t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	return pid
}

// processGone is true once pid no longer runs; a zombie nobody has reaped yet is gone too.
func processGone(pid int) bool {
	if err := syscall.Kill(pid, 0); err != nil {
		return true
	}
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return false
	}
	_, after, _ := strings.Cut(string(stat), ") ")
	return strings.HasPrefix(after, "Z")
}

// A command step that runs out of time is stopped completely: SIGTERM, then SIGKILL for a child that ignores it, and
// only then is the run recorded failed. It is not held as a process group the worker cannot prove gone.
func TestATimedOutCommandStepWhoseChildIgnoresSigtermIsKilledAndTheRunFails(t *testing.T) {
	h := newHarness(t)
	wf := h.kata.CreateWorkflow(t, h.proj, "stubborn", workertest.Step{Key: "wait", Command: stubbornCommand})
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "stubborn job", Workflow: wf.UID, Checkout: "repo", Timeout: 3600})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	uid := h.runWorkflow(r, job)
	pid := h.stubbornChild()

	h.clock.Advance(2 * time.Hour)
	var run worker.Run
	h.eventually("the run to be recorded failed", func() bool { run = h.kataRun(uid); return run.Status == "failed" })
	if !strings.Contains(run.Summary, "timed out") {
		t.Fatalf("run %q: %q", run.Status, run.Summary)
	}
	if !processGone(pid) {
		t.Fatal("the command's child outlived the run")
	}
}

// An abandoned command step is stopped the same way, and the run ends cancelled.
func TestAnAbandonedCommandStepWhoseChildIgnoresSigtermIsKilledAndTheRunIsCancelled(t *testing.T) {
	h := newHarness(t)
	wf := h.kata.CreateWorkflow(t, h.proj, "stubborn", workertest.Step{Key: "wait", Command: stubbornCommand})
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "stubborn job", Workflow: wf.UID, Checkout: "repo"})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	uid := h.runWorkflow(r, job)
	pid := h.stubbornChild()
	h.eventually("the run to be abandonable", func() bool { s, ok := h.runStatus(r, uid); return ok && s.State == "running" && s.Abandonable })

	if resp := h.ask(worker.Request{Op: "abandon", Run: uid, Actor: "ann"}); !resp.OK {
		t.Fatalf("abandon: %q", resp.Error)
	}
	var run worker.Run
	h.eventually("the run to be recorded cancelled", func() bool { run = h.kataRun(uid); return run.Status == "cancelled" })
	if !strings.HasPrefix(run.Summary, "abandoned by ann") {
		t.Fatalf("run %q: %q", run.Status, run.Summary)
	}
	if !processGone(pid) {
		t.Fatal("the command's child outlived the run")
	}
}

// A command that exits by itself and leaves a process behind is held, not killed: the leftover may still be doing what
// the command began.
func TestACommandStepThatExitsLeavingAProcessBehindIsHeldWithItRunning(t *testing.T) {
	h := newHarness(t)
	wf := h.kata.CreateWorkflow(t, h.proj, "leaver", workertest.Step{Key: "leave", Command: "(sleep 300 >/dev/null 2>&1 & echo $! > child.pid) ; exit 0"})
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "leaver job", Workflow: wf.UID, Checkout: "repo"})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	uid := h.runWorkflow(r, job)
	pid := h.stubbornChild()

	h.eventually("the run to be held", func() bool { return h.kataRun(uid).Status == "unknown" })
	h.eventually("the hold to name the group", func() bool {
		s, ok := h.runStatus(r, uid)
		return ok && strings.Contains(s.ResumeRefusal, "is still present")
	})
	if processGone(pid) {
		t.Fatal("the leftover process was killed")
	}
}
