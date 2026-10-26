package worker_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/worker"
	"github.com/Dangerboi/tern-kata/internal/worker/workertest"
)

// Kata already holding the run in the final status is not delivery when it says something else: it is someone else's
// observation.
func TestAFinalObservationIsNotDeliveredWhenKataHoldsTheSameStatusWithAnotherSummary(t *testing.T) {
	h := newHarness(t)
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "review", Prompt: "Do it.", Checkout: "repo"})
	var once sync.Once
	r := h.start(worker.Hooks{AfterSave: func(point string, j *worker.Journal) error {
		if point != "finished" {
			return nil
		}
		once.Do(func() {
			_, _, err := h.kata.Kata.Observe(context.Background(), j.ProjectID, worker.Observation{
				RunUID: j.RunUID, JobUID: j.JobUID, DefinitionEventUID: j.DefinitionEventUID, OccurrenceKey: j.OccurrenceKey, IssueUID: j.Issue.UID,
				Executor: j.Executor, Actor: j.Actor, Status: j.Outcome.Status, Message: "carol wrote another summary", StartedAt: j.StartedAt, ExpectedRevision: j.Revision,
			})
			if err != nil {
				t.Error(err)
			}
		})
		return nil
	}})
	h.admitting(r)
	uid := h.ask(worker.Request{Op: "run", Job: job.UID}).Run.RunUID
	h.settledAndFree(r, uid)

	run, err := h.kata.Kata.ShowRun(context.Background(), h.proj.ID, uid)
	if err != nil || run.Summary != "carol wrote another summary" {
		t.Fatalf("kata holds %q (%v): the worker overwrote what carol recorded", run.Summary, err)
	}
	if status, _ := h.runStatus(r, uid); len(status.Inspect) != 1 {
		t.Fatalf("status %+v: the differing summary was counted as delivered", status)
	}
}

// stderr.log keeps the first bytes only; the detail shows the end the journal holds.
func TestRunDetailShowsTheEndOfALongStderrNotItsBeginning(t *testing.T) {
	h := newHarness(t, "--stub-stderr-bytes", "100000")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "noisy", Prompt: "Do it.", Checkout: "repo"})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	uid := h.ask(worker.Request{Op: "run", Job: job.UID}).Run.RunUID
	h.settledAndFree(r, uid)

	d, err := r.w.RunDetail(uid)
	if err != nil || !strings.Contains(d.StderrTail, "REAL-END-OF-STDERR") {
		t.Fatalf("stderr tail %q (%v), want the end of the stderr", d.StderrTail, err)
	}
}

// A concluded workflow step keeps the end of its stderr in the journal too.
func TestRunDetailShowsTheEndOfAConcludedStepsLongStderr(t *testing.T) {
	h := newHarness(t, "--stub-stderr-bytes", "100000")
	_, job := h.workflowJob("noisy flow", workertest.Step{Key: "agent", Prompt: "Do it."})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	uid := h.runWorkflow(r, job)
	h.settledAndFree(r, uid)

	d, err := r.w.RunDetail(uid)
	if err != nil || !strings.Contains(d.StderrTail, "REAL-END-OF-STDERR") {
		t.Fatalf("stderr tail %q (%v), want the end of the step's stderr", d.StderrTail, err)
	}
}
