//go:build !windows

package worker_test

import (
	"context"
	"os/exec"
	"strings"
	"syscall"
	"testing"

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
