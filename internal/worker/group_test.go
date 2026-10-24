//go:build !windows

package worker_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/worker"
	"github.com/Dangerboi/tern-kata/internal/worker/workertest"
)

// A command whose leader notes SIGTERM in term.txt and ends, and whose child ignores SIGTERM: the group outlives the
// first signal until SIGKILL.
const termNotingCommand = "(trap '' TERM; exec >/dev/null 2>&1; sleep 300 & echo $! > child.pid; wait) & trap 'echo term > term.txt; exit 0' TERM; wait"

// realPause lets the worker's goroutines react to the clock the test just moved; it stands for no virtual time.
func realPause() { time.Sleep(300 * time.Millisecond) }

func (h *harness) termNoted() bool {
	_, err := os.Stat(filepath.Join(h.repo, "term.txt"))
	return err == nil
}

// stopsStubbornGroupByThePolicy drives a group that ignores SIGTERM through the termination policy on the manual clock:
// still alive nine fake seconds after SIGTERM, and the run not settled; killed after ten.
func (h *harness) stopsStubbornGroupByThePolicy(uid string, pid int, trigger func()) {
	h.t.Helper()
	trigger()
	h.within("the leader to be signalled", h.termNoted)
	h.clock.Advance(9 * time.Second)
	realPause()
	if processGone(pid) {
		h.t.Fatal("the stubborn child was killed before ten seconds of fake time had passed")
	}
	if status := h.kataRun(uid).Status; status != "running" {
		h.t.Fatalf("run recorded %q while its group was still present", status)
	}
	h.clock.Advance(2 * time.Second)
	h.within("the stubborn child to be killed", func() bool { return processGone(pid) })
}

func TestATimedOutCommandStepIsKilledTenFakeSecondsAfterSigtermAndTheRunFails(t *testing.T) {
	h := newHarness(t)
	wf := h.kata.CreateWorkflow(t, h.proj, "stubborn", workertest.Step{Key: "wait", Command: termNotingCommand})
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "stubborn job", Workflow: wf.UID, Checkout: "repo", Timeout: 60})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	uid := h.runWorkflow(r, job)
	pid := h.stubbornChild()

	h.stopsStubbornGroupByThePolicy(uid, pid, func() { h.clock.Advance(61 * time.Second) })
	run := h.settled(uid)
	if run.Status != "failed" || !strings.Contains(run.Summary, "timed out") {
		t.Fatalf("run %q: %q", run.Status, run.Summary)
	}
}

func TestAnAbandonedCommandStepIsKilledTenFakeSecondsAfterSigtermAndTheRunIsCancelledOnlyThen(t *testing.T) {
	h := newHarness(t)
	wf := h.kata.CreateWorkflow(t, h.proj, "stubborn", workertest.Step{Key: "wait", Command: termNotingCommand})
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "stubborn job", Workflow: wf.UID, Checkout: "repo"})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	uid := h.runWorkflow(r, job)
	pid := h.stubbornChild()
	h.eventually("the run to be abandonable", func() bool { s, ok := h.runStatus(r, uid); return ok && s.State == "running" && s.Abandonable })

	h.stopsStubbornGroupByThePolicy(uid, pid, func() {
		if resp := h.ask(worker.Request{Op: "abandon", Run: uid, Actor: "ann"}); !resp.OK {
			t.Fatalf("abandon: %q", resp.Error)
		}
	})
	run := h.settled(uid)
	if run.Status != "cancelled" || !strings.HasPrefix(run.Summary, "abandoned by ann") {
		t.Fatalf("run %q: %q", run.Status, run.Summary)
	}
	if !processGone(pid) {
		t.Fatal("the run was recorded cancelled with its group still present")
	}
}

// leaverHarness is a harness whose agent leaves a sleeping descendant and exits; pidFile holds the descendant's pid.
func leaverHarness(t *testing.T) (h *harness, pidFile string) {
	pidFile = filepath.Join(t.TempDir(), "leftover.pid")
	return newHarness(t, "--stub-leave-child", pidFile, "--stub-text", "Done."), pidFile
}

func leftover(t *testing.T, h *harness, pidFile string) int {
	t.Helper()
	var pid int
	h.within("the agent to leave its descendant", func() bool {
		raw, err := os.ReadFile(pidFile)
		if err != nil {
			return false
		}
		pid, err = strconv.Atoi(string(raw))
		return err == nil && pid > 0
	})
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	return pid
}

func TestAnAgentThatExitsLeavingADescendantHasItStoppedAndTheRunSettles(t *testing.T) {
	h, pidFile := leaverHarness(t)
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "review", Prompt: "Do it.", Checkout: "repo"})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	uid := h.ask(worker.Request{Op: "run", Job: job.UID}).Run.RunUID
	pid := leftover(t, h, pidFile)

	run := h.settled(uid)
	if run.Status != "succeeded" {
		t.Fatalf("run %q: %q", run.Status, run.Summary)
	}
	if !processGone(pid) {
		t.Fatal("the agent's descendant outlived the run")
	}
}

func TestADescendantThatSurvivesTheBoundedWindowHoldsTheRunGroupUnproven(t *testing.T) {
	h, pidFile := leaverHarness(t)
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "review", Prompt: "Do it.", Checkout: "repo"})
	r := h.start(worker.Hooks{GroupProof: func(*worker.LaunchRecord) (worker.GroupProof, bool) {
		return worker.GroupProof{State: worker.GroupPresent}, true
	}})
	h.admitting(r)
	uid := h.ask(worker.Request{Op: "run", Job: job.UID}).Run.RunUID
	leftover(t, h, pidFile)

	run := h.settled(uid)
	if run.Status != "unknown" || !strings.Contains(run.Summary, "still present after the agent exited") {
		t.Fatalf("run %q: %q", run.Status, run.Summary)
	}
}

// A group the platform cannot check holds the run and is said once per run per change of reason: the log, `worker status`
// and the resume and abandon refusals name the reason, and an answer that is not "cannot be checked" makes the next
// such answer be said again.
func TestAGroupThatCannotBeCheckedIsSaidOncePerReasonAndNamedInStatusAndRefusals(t *testing.T) {
	h, pidFile := leaverHarness(t)
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "review", Prompt: "Do it.", Checkout: "repo"})
	var answer atomic.Pointer[worker.GroupProof]
	answers := func(state worker.GroupState, reason string) {
		answer.Store(&worker.GroupProof{State: state, Reason: reason})
	}
	answers(worker.GroupUnchecked, "open failed")
	var log logLines
	r := h.start(worker.Hooks{GroupProof: func(*worker.LaunchRecord) (worker.GroupProof, bool) { return *answer.Load(), true }},
		func(o *worker.Options) { o.Log = log.add })
	h.admitting(r)
	uid := h.ask(worker.Request{Op: "run", Job: job.UID}).Run.RunUID
	leftover(t, h, pidFile)
	run := h.settled(uid)
	if run.Status != "unknown" || !strings.Contains(run.Summary, "cannot be checked after the agent exited: open failed") {
		t.Fatalf("run %q: %q", run.Status, run.Summary)
	}

	h.eventually("the hold to be reported", func() bool { s, ok := h.runStatus(r, uid); return ok && s.Held && s.Pending == "" })

	// look asks what status, resume and abandon say now; each asks the group again.
	look := func() (status, resume, abandon string) {
		s, _ := h.runStatus(r, uid)
		return s.ResumeRefusal, h.ask(worker.Request{Op: "resume", Run: uid}).Error, h.ask(worker.Request{Op: "abandon", Run: uid, Actor: "ann"}).Error
	}
	said := func(reason string) int { return log.count("cannot be checked: " + reason) }

	status, resume, abandon := look()
	for what, text := range map[string]string{"status": status, "resume": resume, "abandon": abandon} {
		if !strings.Contains(text, "cannot be checked: open failed") {
			t.Errorf("%s does not name the reason: %q", what, text)
		}
	}
	look()
	if n := said("open failed"); n != 1 {
		t.Errorf("the reason was said %d times for repeated answers, want once", n)
	}

	answers(worker.GroupUnchecked, "query failed")
	status, resume, abandon = look()
	if !strings.Contains(status, "query failed") || !strings.Contains(resume, "query failed") || !strings.Contains(abandon, "query failed") {
		t.Errorf("a changed reason is not named: %q / %q / %q", status, resume, abandon)
	}
	look()
	if said("query failed") != 1 || said("open failed") != 1 {
		t.Errorf("after the change: open failed said %d times, query failed %d; want once each", said("open failed"), said("query failed"))
	}

	answers(worker.GroupPresent, "")
	if _, resume, _ := look(); !strings.Contains(resume, "still present") {
		t.Errorf("a present group is refused as %q", resume)
	}
	answers(worker.GroupUnchecked, "query failed")
	look()
	if n := said("query failed"); n != 2 {
		t.Errorf("the reason was said %d times after an answer in between, want again once (2 in all)", n)
	}
}
