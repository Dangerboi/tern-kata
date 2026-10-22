//go:build !windows

package worker_test

import (
	"errors"
	"context"
	"os/exec"
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

// A journal save counts only once its folder is synced: when the folder sync of the save before the launch fails, the
// launch does not happen, and the restarted worker does not start the agent either.
func TestAnAgentIsNotStartedWhenTheFolderSyncOfItsLaunchingSaveFails(t *testing.T) {
	h := newHarness(t, "--stub-text", "done")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "review", Prompt: "Do it.", Checkout: "repo"})
	hooks := worker.Hooks{FailDirSync: func(point string) error {
		if point == "launching" {
			return errors.New("folder sync failed")
		}
		return nil
	}}
	r := h.start(hooks)
	h.admitting(r)
	uid := h.ask(worker.Request{Op: "run", Job: job.UID}).Run.RunUID
	store := worker.NewStore(h.dirs, worker.Hooks{})
	h.eventually("the launching save to be replaced", func() bool { j, err := store.Load(uid); return err == nil && j.Phase == worker.PhaseLaunching })
	time.Sleep(200 * time.Millisecond) // a launch that wrongly went ahead would have started by now
	if n := len(h.launches()); n != 0 {
		t.Fatalf("agent launched %d times after a failed save, want none", n)
	}
	r.stop()

	restarted := h.start(worker.Hooks{})
	h.admitting(restarted)
	h.settled(uid)
	if n := len(h.launches()); n != 0 {
		t.Fatalf("agent launched %d times after the restart, want none", n)
	}
}
