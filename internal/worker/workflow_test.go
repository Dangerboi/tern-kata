package worker_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/worker"
	"github.com/Dangerboi/tern-kata/internal/worker/workertest"
)

// lines is a checkout file's lines, empty when it does not exist.
func (h *harness) lines(name string) []string {
	raw, err := os.ReadFile(filepath.Join(h.repo, name))
	if err != nil || len(strings.TrimSpace(string(raw))) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(raw)), "\n")
}

// startWorkflow creates a job that runs the workflow in the checkout and starts a manual run of it.
func (h *harness) workflowJob(name string, steps ...workertest.Step) (worker.Workflow, worker.Job) {
	h.t.Helper()
	wf := h.kata.CreateWorkflow(h.t, h.proj, name, steps...)
	job := h.kata.CreateJob(h.t, h.proj, workertest.Job{Name: name + " job", Workflow: wf.UID, Checkout: "repo"})
	return wf, job
}

func (h *harness) runWorkflow(r *running, job worker.Job) string {
	h.t.Helper()
	resp := h.ask(worker.Request{Op: "run", Job: job.UID})
	if !resp.OK || resp.Run == nil {
		h.t.Fatalf("run request refused: %q", resp.Error)
	}
	return resp.Run.RunUID
}

func TestAWorkflowRunsItsPromptAndCommandStepsInOrderAsOneRun(t *testing.T) {
	h := newHarness(t, "--stub-text", "Reviewed.")
	wf, job := h.workflowJob("release",
		workertest.Step{Key: "build", Command: "echo build >> order.txt"},
		workertest.Step{Key: "review", Prompt: "Review what build made.", After: []string{"build"}},
		workertest.Step{Key: "publish", Command: "test -s '" + h.log + "' && echo publish >> order.txt", After: []string{"review"}},
	)
	r := h.start(worker.Hooks{})
	h.admitting(r)
	uid := h.runWorkflow(r, job)
	run := h.settled(uid)

	if run.Status != "succeeded" || run.WorkflowUID != wf.UID {
		t.Fatalf("run is %q for workflow %q, want succeeded for %s: %q", run.Status, run.WorkflowUID, wf.UID, run.Summary)
	}
	for _, want := range []string{`Workflow "release" succeeded (3 of 3 steps succeeded)`, "1. build [command]: succeeded", "2. review [prompt]: succeeded\n   Reviewed.", "3. publish [command]: succeeded"} {
		if !strings.Contains(run.Summary, want) {
			t.Errorf("summary lacks %q:\n%s", want, run.Summary)
		}
	}
	if got := h.lines("order.txt"); strings.Join(got, ",") != "build,publish" {
		t.Fatalf("the commands ran as %v, want build then publish (after the prompt step started the agent)", got)
	}
	launches := h.launches()
	if len(launches) != 1 {
		t.Fatalf("agent launched %d times, want once for the one prompt step", len(launches))
	}
	prompt := launches[0]["prompt"].(string)
	for _, want := range []string{"Workflow: release", "About: workflow about", "Step: 2 of 3 (review), attempt 1", "Review what build made."} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt lacks %q:\n%s", want, prompt)
		}
	}
	if runs, err := h.kata.Kata.Runs(context.Background(), h.proj.ID); err != nil || len(runs) != 1 {
		t.Fatalf("kata holds %d runs (%v), want the one for the workflow", len(runs), err)
	}
	resp := h.ask(worker.Request{Op: "run-detail", Run: uid})
	if !resp.OK || resp.Detail == nil || len(resp.Detail.Steps) != 3 {
		t.Fatalf("run-detail %+v", resp)
	}
	for i, want := range []struct{ key, kind string }{{"build", "command"}, {"review", "prompt"}, {"publish", "command"}} {
		s := resp.Detail.Steps[i]
		if s.Key != want.key || s.Kind != want.kind || s.State != "succeeded" || s.Attempt != 1 || s.ExitCode == nil {
			t.Errorf("step %d is %+v, want %s %s succeeded in one attempt", i+1, s, want.key, want.kind)
		}
	}
	// A Pane opens the session of the latest prompt step, from that step's own folder, in the checkout.
	d := resp.Detail
	rel, err := filepath.Rel(filepath.Join(h.dirs.Runs(), uid), d.SessionDir)
	if d.SessionID == "" || err != nil || filepath.Base(rel) != "session" || filepath.Dir(rel) == "." || d.Cwd != h.repo {
		t.Errorf("session %q in %q, cwd %q: want the review step's session in the checkout", d.SessionID, d.SessionDir, d.Cwd)
	}
}

func TestACommandOnlyWorkflowHasNoSessionToOpen(t *testing.T) {
	h := newHarness(t)
	_, job := h.workflowJob("plain", workertest.Step{Key: "build", Command: "echo hi"})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	uid := h.runWorkflow(r, job)
	h.settled(uid)
	resp := h.ask(worker.Request{Op: "run-detail", Run: uid})
	if !resp.OK || resp.Detail == nil || resp.Detail.SessionID != "" || resp.Detail.SessionDir != "" {
		t.Fatalf("detail %+v, want no session", resp.Detail)
	}
}

func TestAFailingStepIsRetriedThenFailsTheRunAndOnlyDependentStepsAreSkipped(t *testing.T) {
	h := newHarness(t)
	_, job := h.workflowJob("flaky",
		workertest.Step{Key: "prepare", Command: "echo prepare >> order.txt"},
		workertest.Step{Key: "test", Command: "echo try >> tries.txt; echo 'suite broke'; exit 3", After: []string{"prepare"}, Retries: 2},
		workertest.Step{Key: "ship", Command: "echo ship >> order.txt", After: []string{"test"}},
		workertest.Step{Key: "notes", Command: "echo notes >> order.txt"},
	)
	r := h.start(worker.Hooks{})
	h.admitting(r)
	uid := h.runWorkflow(r, job)
	run := h.settled(uid)

	if run.Status != "failed" {
		t.Fatalf("run is %q: %q", run.Status, run.Summary)
	}
	if got := len(h.lines("tries.txt")); got != 3 {
		t.Fatalf("the failing step ran %d times, want 1 + 2 retries", got)
	}
	if got := strings.Join(h.lines("order.txt"), ","); got != "prepare,notes" {
		t.Fatalf("steps that ran: %q; the step that depends on the failure must not, an independent one still does", got)
	}
	for _, want := range []string{`Workflow "flaky" failed (2 of 4 steps succeeded)`, "2. test [command]: failed after 3 attempts", "the command exited with status 3", "suite broke", "3. ship [command]: skipped"} {
		if !strings.Contains(run.Summary, want) {
			t.Errorf("summary lacks %q:\n%s", want, run.Summary)
		}
	}
	h.eventually("the failure to be commented on the Issue", func() bool { return len(h.issueOf(run).Comments) > 0 })
	if issue := h.issueOf(run); !issue.Open() || len(issue.Comments) != 1 || !strings.Contains(issue.Comments[0], "The run failed:") {
		t.Fatalf("Issue %+v: want one failure comment and the Issue left open", issue)
	}
	resp := h.ask(worker.Request{Op: "run-detail", Run: uid})
	states := []string{}
	for _, s := range resp.Detail.Steps {
		states = append(states, s.State)
	}
	if strings.Join(states, ",") != "succeeded,failed,skipped,succeeded" {
		t.Fatalf("step states %v", states)
	}
}

func TestARetriedStepThatThenSucceedsDoesNotFailTheRun(t *testing.T) {
	h := newHarness(t, "--stub-fail-once-when", "FLAKY", "--stub-text", "Fixed on the second go.")
	_, job := h.workflowJob("recovering",
		workertest.Step{Key: "warm", Command: "echo try >> warm.txt; test $(wc -l < warm.txt) -ge 2", Retries: 1},
		workertest.Step{Key: "agent", Prompt: "FLAKY task.", After: []string{"warm"}, Retries: 1},
	)
	r := h.start(worker.Hooks{})
	h.admitting(r)
	run := h.settled(h.runWorkflow(r, job))

	if run.Status != "succeeded" {
		t.Fatalf("run is %q: %q", run.Status, run.Summary)
	}
	for _, want := range []string{"1. warm [command]: succeeded after 2 attempts", "2. agent [prompt]: succeeded after 2 attempts"} {
		if !strings.Contains(run.Summary, want) {
			t.Errorf("summary lacks %q:\n%s", want, run.Summary)
		}
	}
	launches := h.launches()
	if len(launches) != 2 {
		t.Fatalf("agent launched %d times, want the failed attempt and its retry", len(launches))
	}
	first, second := launches[0]["prompt"].(string), launches[1]["prompt"].(string)
	if strings.Contains(first, "# Retry") || !strings.Contains(second, "# Retry") || !strings.Contains(second, "attempt 2 of this step") || !strings.Contains(second, "the agent exited with status 1") {
		t.Fatalf("the retry must say it is attempt 2 after a failure:\n--- first\n%s\n--- second\n%s", first, second)
	}
	if launches[1]["resume"] != "" {
		t.Errorf("a retry is a new session, but it resumed %v", launches[1]["resume"])
	}
}

func TestACrashMidWorkflowHoldsAtThatStepAndResumeContinuesFromIt(t *testing.T) {
	h := newHarness(t, "--stub-hang-when", "HANG", "--stub-text", "Checked.")
	_, job := h.workflowJob("pipeline",
		workertest.Step{Key: "build", Command: "echo build >> order.txt"},
		workertest.Step{Key: "review", Prompt: "HANG until resumed.", After: []string{"build"}},
		workertest.Step{Key: "ship", Command: "echo ship >> order.txt", After: []string{"review"}},
	)
	uid, _ := h.killedAt("session", job)

	restarted := h.start(worker.Hooks{})
	h.admitting(restarted)
	held := h.settled(uid)
	if held.Status != "unknown" || !strings.HasPrefix(held.Summary, "interrupted: ") {
		t.Fatalf("held run is %q: %q", held.Status, held.Summary)
	}
	h.eventually("the hold to be resumable", func() bool { s, ok := h.runStatus(restarted, uid); return ok && s.Resumable })
	d := h.ask(worker.Request{Op: "run-detail", Run: uid}).Detail
	if d == nil || len(d.Steps) != 3 || d.Steps[0].State != "succeeded" || d.Steps[1].State != "held: interrupted" || d.Steps[2].State != "pending" || d.Steps[1].SessionID == "" {
		t.Fatalf("detail %+v: want build done, review held with a session, ship waiting", d)
	}
	if got := h.lines("order.txt"); strings.Join(got, ",") != "build" {
		t.Fatalf("before the resume %v ran; the step after the held one must wait", got)
	}

	if resp := h.ask(worker.Request{Op: "resume", Run: uid, Actor: "ann"}); !resp.OK {
		t.Fatalf("resume refused: %s", resp.Error)
	}
	var run worker.Run
	h.eventually("kata to hold the resumed workflow as succeeded", func() bool {
		run, _ = h.kata.Kata.ShowRun(context.Background(), h.proj.ID, uid)
		return run.Status == "succeeded"
	})
	if got := strings.Join(h.lines("order.txt"), ","); got != "build,ship" {
		t.Fatalf("commands ran as %q, want build once (not rerun by the resume) then ship", got)
	}
	launches := h.launches()
	if len(launches) != 2 || launches[1]["resume"] != d.Steps[1].SessionID {
		t.Fatalf("launches %v: want the held attempt and one resume of its session %s", launches, d.Steps[1].SessionID)
	}
	if !strings.Contains(run.Summary, "3. ship [command]: succeeded") || !strings.Contains(run.Summary, "2. review [prompt]: succeeded\n   Checked.") {
		t.Fatalf("summary %q", run.Summary)
	}
}

func TestAHeldCommandStepRerunsItsCommandOnResume(t *testing.T) {
	h := newHarness(t)
	_, job := h.workflowJob("slow",
		workertest.Step{Key: "wait", Command: "if [ -e ran ]; then echo again >> order.txt; else touch ran; sleep 60; fi"},
		workertest.Step{Key: "after", Command: "echo after >> order.txt", After: []string{"wait"}},
	)
	r := h.start(worker.Hooks{})
	h.admitting(r)
	uid := h.runWorkflow(r, job)
	h.eventually("the command to start", func() bool { _, err := os.Stat(filepath.Join(h.repo, "ran")); return err == nil })
	r.stop()

	restarted := h.start(worker.Hooks{})
	h.admitting(restarted)
	held := h.settled(uid)
	if held.Status != "unknown" || !strings.HasPrefix(held.Summary, "interrupted: the worker stopped while the command ran") {
		t.Fatalf("held run is %q: %q", held.Status, held.Summary)
	}
	h.eventually("the hold to be resumable", func() bool { s, ok := h.runStatus(restarted, uid); return ok && s.Resumable })
	if resp := h.ask(worker.Request{Op: "resume", Run: uid}); !resp.OK {
		t.Fatalf("resume refused: %s", resp.Error)
	}
	var run worker.Run
	h.eventually("the resumed workflow to succeed", func() bool {
		run, _ = h.kata.Kata.ShowRun(context.Background(), h.proj.ID, uid)
		return run.Status == "succeeded"
	})
	if got := strings.Join(h.lines("order.txt"), ","); got != "again,after" {
		t.Fatalf("commands ran as %q, want the held command rerun, then the next step", got)
	}
	if len(h.launches()) != 0 {
		t.Fatal("a command workflow must never start the agent")
	}
}

func TestAWorkflowTheWorkerDoesNotRunIsRefusedWithItsReason(t *testing.T) {
	h := newHarness(t)
	loop := h.kata.CreateWorkflow(t, h.proj, "loop",
		workertest.Step{Key: "make", Prompt: "Make it."},
		workertest.Step{Key: "check", Prompt: "Check it.", After: []string{"make"}, Options: map[string]any{"on_fail": "make"}},
	)
	park := h.kata.CreateWorkflow(t, h.proj, "park", workertest.Step{Key: "only", Command: "true", Options: map[string]any{"on_failure": "park"}})
	many := h.kata.CreateWorkflow(t, h.proj, "many", workertest.Step{Key: "a", Prompt: "A.", Retries: 100})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	for name, want := range map[string]string{"loop": "on_fail", "park": "park", "many": "prompt_budget 101 > 12"} {
		wf := map[string]worker.Workflow{"loop": loop, "park": park, "many": many}[name]
		job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: name + " job", Workflow: wf.UID, Checkout: "repo"})
		resp := h.ask(worker.Request{Op: "run", Job: job.UID})
		if resp.OK || !strings.Contains(resp.Error, want) {
			t.Errorf("workflow %s: ok=%v %q, want a refusal naming %q", name, resp.OK, resp.Error, want)
		}
	}
	if runs, _ := h.kata.Kata.Runs(context.Background(), h.proj.ID); len(runs) != 0 {
		t.Fatalf("a refused workflow recorded %d runs", len(runs))
	}
}

func TestACommandStepKeepsTheStartAndEndOfItsOutputWithAMarkerBetween(t *testing.T) {
	h := newHarness(t)
	_, job := h.workflowJob("chatty", workertest.Step{Key: "noise", Command: "echo START; head -c 2000000 /dev/zero | tr '\\0' x; echo; echo END"})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	uid := h.runWorkflow(r, job)
	if run := h.settled(uid); run.Status != "succeeded" {
		t.Fatalf("run is %q: %q", run.Status, run.Summary)
	}
	raw, err := os.ReadFile(filepath.Join(h.dirs.Runs(), uid, "steps", "1-1", "output.log"))
	if err != nil {
		t.Fatal(err)
	}
	out := string(raw)
	if !strings.HasPrefix(out, "START\n") || !strings.HasSuffix(out, "END\n") || !strings.Contains(out, "bytes omitted") {
		t.Fatalf("output is %d bytes starting %q ending %q", len(out), out[:10], out[len(out)-10:])
	}
	if max := (1 << 20) + (256 << 10) + 100; len(out) > max {
		t.Fatalf("output is %d bytes, over the %d it may keep", len(out), max)
	}
}

// A workflow run past its expiry starts no step: not the first one after a restart, nor the next one between two.
func TestAWorkflowRunPastItsExpiryStartsNoStepAfterARestart(t *testing.T) {
	for _, point := range []string{"admitted", "step-concluded"} {
		t.Run(point, func(t *testing.T) {
			h := newHarness(t)
			wf := h.kata.CreateWorkflow(t, h.proj, "pipeline",
				workertest.Step{Key: "build", Command: "echo build >> order.txt"},
				workertest.Step{Key: "ship", Command: "echo ship >> order.txt", After: []string{"build"}},
			)
			job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "pipeline job", Workflow: wf.UID, Checkout: "repo", Timeout: 3600})
			uid, _ := h.killedAt(point, job)
			before := strings.Join(h.lines("order.txt"), ",")

			h.clock.Advance(2 * time.Hour)
			h.start(worker.Hooks{})
			h.within("the run to fail", func() bool { return h.kataRun(uid).Status == "failed" })

			run := h.kataRun(uid)
			if !strings.Contains(run.Summary, "timed out") || !strings.Contains(run.Summary, "ship [command]: skipped") {
				t.Fatalf("summary %q: want timed out with the unstarted step skipped", run.Summary)
			}
			if got := strings.Join(h.lines("order.txt"), ","); got != before {
				t.Fatalf("commands ran as %q after the expiry, were %q before", got, before)
			}
			if len(h.launches()) != 0 {
				t.Fatal("a process was launched past the expiry")
			}
		})
	}
}
