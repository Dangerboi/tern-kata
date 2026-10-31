package worker_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
	"github.com/Dangerboi/tern-kata/internal/worker"
	"github.com/Dangerboi/tern-kata/internal/worker/workertest"
)

// An ordinary execute job on an existing Issue waits while the Issue is someday or scheduled for later; deadline_on
// never defers it, and a notify job has no readiness requirement.

// hourlyOn is an hourly execute job on an Issue, with the worker started and its first occurrence due.
func (h *harness) hourlyOn(issueUID string) (cronjob.Job, *running, time.Time) {
	h.t.Helper()
	job := h.kata.CreateJob(h.t, h.proj, workertest.Job{Name: "on issue", Prompt: "Do it.", ExistingUID: issueUID, Trigger: interval(3600)})
	r := h.start(worker.Hooks{})
	h.scheduled(r, 1)
	return job, r, h.fire(r, time.Time{})
}

func (h *harness) waitsFor(r *running, want string) {
	h.t.Helper()
	h.eventually("the occurrence to wait: "+want, func() bool {
		for _, n := range h.status(r).Waiting {
			if strings.Contains(n.Text, want) {
				return true
			}
		}
		return false
	})
	if p := h.status(r); p.InUse != 0 {
		h.t.Fatalf("a waiting occurrence holds %d slot(s)", p.InUse)
	}
}

func TestAnExecuteJobWaitsOnASomedayIssueWithoutASlotAndStartsOnceItIsUnset(t *testing.T) {
	h := newHarness(t)
	issue := h.kata.Issue(t, h.proj, "Parked")
	h.kata.SetSomeday(t, h.proj, issue.UID, true)
	job, r, _ := h.hourlyOn(issue.UID)

	h.waitsFor(r, "Issue is someday")
	if !strings.Contains(r.w.Status().String(), `Job "on issue" is waiting: Issue is someday`) {
		t.Fatalf("worker status does not name the wait:\n%s", r.w.Status().String())
	}
	h.quiet(job, 0)

	h.kata.SetSomeday(t, h.proj, issue.UID, false) // the Issue event wakes the check
	runs := h.waitRuns(job, 1)
	h.settled(runs[0].UID)
	h.eventually("the wait to be gone", func() bool { return len(h.status(r).Waiting) == 0 })
}

func TestAnExecuteJobWaitsOnAFutureScheduledIssueUntilItsInstant(t *testing.T) {
	h := newHarness(t)
	issue := h.kata.Issue(t, h.proj, "Later")
	instant := h.minuteIn(27 * time.Hour)
	h.kata.SetIssueDate(t, h.proj, issue.UID, "scheduled_on", instant)
	// Daily, so the next occurrence lands after the instant: a run can only be the parked one, kept past its grace
	// under the default catchup `skip`.
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "on issue", Prompt: "Do it.", ExistingUID: issue.UID, Trigger: interval(86400)})
	r := h.start(worker.Hooks{})
	h.scheduled(r, 1)
	first := h.fire(r, time.Time{})

	h.waitsFor(r, "Issue is scheduled for")
	h.quiet(job, 0)
	h.advanceTo(first.Add(2 * time.Hour)) // far past any grace, the instant still ahead
	h.quiet(job, 0)
	h.waitsFor(r, "Issue is scheduled for")

	h.advanceTo(instant) // the worker wakes at the instant itself, no Issue event involved
	runs := h.waitRuns(job, 1)
	if runs[0].OccurrenceKey != worker.OccurrenceKey(first) {
		t.Fatalf("the run is for %s, want the parked occurrence %s", runs[0].OccurrenceKey, worker.OccurrenceKey(first))
	}
	h.settled(runs[0].UID)
}

func TestAParkedOccurrenceSurvivesAWorkerRestartPastItsGrace(t *testing.T) {
	h := newHarness(t)
	issue := h.kata.Issue(t, h.proj, "Parked")
	h.kata.SetSomeday(t, h.proj, issue.UID, true)
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "on issue", Prompt: "Do it.", ExistingUID: issue.UID, Trigger: interval(86400)})
	r := h.start(worker.Hooks{})
	h.scheduled(r, 1)
	first := h.fire(r, time.Time{})
	h.waitsFor(r, "Issue is someday")
	r.stop()

	h.advanceTo(first.Add(2 * time.Hour)) // past any grace while the worker is down
	restarted := h.start(worker.Hooks{})
	h.scheduled(restarted, 1)
	h.waitsFor(restarted, "Issue is someday")
	h.quiet(job, 0)

	h.kata.SetSomeday(t, h.proj, issue.UID, false)
	runs := h.waitRuns(job, 1)
	if runs[0].OccurrenceKey != worker.OccurrenceKey(first) {
		t.Fatalf("the run is for %s, want the parked occurrence %s", runs[0].OccurrenceKey, worker.OccurrenceKey(first))
	}
	h.settled(runs[0].UID)
}

func TestAnExecuteJobStartsWhenTheIssuesScheduledInstantIsPast(t *testing.T) {
	h := newHarness(t)
	issue := h.kata.Issue(t, h.proj, "Due")
	h.kata.SetIssueDate(t, h.proj, issue.UID, "scheduled_on", h.minuteIn(-time.Hour))
	job, _, _ := h.hourlyOn(issue.UID)
	h.settled(h.waitRuns(job, 1)[0].UID)
}

func TestADeadlineNeverDefersAnExecuteJob(t *testing.T) {
	h := newHarness(t)
	issue := h.kata.Issue(t, h.proj, "Deadline")
	h.kata.SetIssueDate(t, h.proj, issue.UID, "deadline_on", h.minuteIn(48*time.Hour))
	job, _, _ := h.hourlyOn(issue.UID)
	h.settled(h.waitRuns(job, 1)[0].UID)
}

func TestAParkedExecuteJobWhoseInstantPassedOnAFullProjectStartsWhenASlotFrees(t *testing.T) {
	h := newHarness(t, "--stub-hang")
	h.capacity("1")
	issue := h.kata.Issue(t, h.proj, "Later")
	instant := h.minuteIn(2 * time.Hour)
	h.kata.SetIssueDate(t, h.proj, issue.UID, "scheduled_on", instant)
	busy := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "busy", Prompt: "Do it.", Trigger: interval(2400), Overlap: "allow", Catchup: "all", Timeout: 6000})
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "on issue", Prompt: "Do it.", ExistingUID: issue.UID, Trigger: interval(3600), Catchup: "all"})
	r := h.start(worker.Hooks{})
	h.scheduled(r, 2)

	first := h.fire(r, time.Time{}) // the only slot is taken until the run times out
	h.started(r, busy, 1)
	startedAt := h.clock.Now()
	h.fire(r, first) // the occurrence is due and its Issue is not ready
	h.eventually("the occurrence to wait for its Issue", func() bool { return len(h.status(r).Waiting) == 1 })
	h.advanceTo(instant) // the Issue is ready, the Project is full
	h.quiet(job, 0)

	h.advanceTo(startedAt.Add(6000*time.Second + 10*time.Second)) // the busy run times out and frees the slot
	h.waitRuns(job, 1)
}

func TestAManualRunOnAParkedIssueIsRefusedAndTakesNoSlot(t *testing.T) {
	h := newHarness(t)
	issue := h.kata.Issue(t, h.proj, "Parked")
	h.kata.SetSomeday(t, h.proj, issue.UID, true)
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "on issue", Prompt: "Do it.", ExistingUID: issue.UID})
	r := h.start(worker.Hooks{})
	h.admitting(r)

	resp := h.ask(worker.Request{Op: "run", Job: job.UID})
	if resp.OK || !strings.Contains(resp.Error, "Issue is someday") {
		t.Fatalf("run on a someday Issue: ok=%v %q", resp.OK, resp.Error)
	}
	if p := h.status(r); p.InUse != 0 || len(h.jobRuns(job)) != 0 {
		t.Fatalf("a refused run left %d slot(s) and %d run(s)", p.InUse, len(h.jobRuns(job)))
	}
}

func TestANotifyJobOnASomedayIssueStillNotifies(t *testing.T) {
	h := newHarness(t)
	issue := h.kata.Issue(t, h.proj, "Parked")
	h.kata.SetSomeday(t, h.proj, issue.UID, true)
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "nudge", ExistingUID: issue.UID, Notify: &workertest.Notify{Recipient: "bob", Message: "Still due"}})
	r := h.start(worker.Hooks{})
	h.admitting(r)

	resp := h.ask(worker.Request{Op: "run", Job: job.UID})
	if !resp.OK {
		t.Fatalf("run: %q", resp.Error)
	}
	if run := h.settled(resp.Run.RunUID); run.Status != "succeeded" {
		t.Fatalf("run %+v", run)
	}
}
