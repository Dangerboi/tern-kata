package worker_test

import (
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
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
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "on issue", Prompt: "Do it.", ExistingUID: issue.UID, Trigger: interval(3600)})
	r := h.start(worker.Hooks{})
	h.scheduled(r, 2)

	first := h.fire(r, time.Time{}) // the only slot is taken until the run times out
	h.started(r, busy, 1)
	startedAt := h.clock.Now()
	parked := h.fire(r, first) // the occurrence is due and its Issue is not ready
	h.eventually("the occurrence to wait for its Issue", func() bool { return len(h.status(r).Waiting) == 1 })
	h.advanceTo(instant) // the Issue is ready, the Project is full
	h.quiet(job, 0)

	h.advanceTo(startedAt.Add(6000*time.Second + 10*time.Second)) // the busy run times out and frees the slot
	runs := h.waitRuns(job, 1)
	if runs[0].OccurrenceKey != worker.OccurrenceKey(parked) {
		t.Fatalf("the run is for %s, want the parked occurrence %s", runs[0].OccurrenceKey, worker.OccurrenceKey(parked))
	}
}

// spinClock counts the waits too short to be anything but a loop with an instant already behind it.
type spinClock struct {
	*workertest.ManualClock
	spins atomic.Int64
}

func (c *spinClock) After(d time.Duration) <-chan time.Time {
	if d <= time.Millisecond {
		c.spins.Add(1)
	}
	return c.ManualClock.After(d)
}

func TestAParkedJobBehindACapacityRefusedJobNeverSpinsTheLoopAndStartsWhenASlotFrees(t *testing.T) {
	h := newHarness(t, "--stub-hang")
	h.capacity("1")
	laterIssue := h.kata.Issue(t, h.proj, "Later")
	someday := h.kata.Issue(t, h.proj, "Someday")
	h.kata.SetSomeday(t, h.proj, someday.UID, true)
	instant := h.minuteIn(5000 * time.Second)
	h.kata.SetIssueDate(t, h.proj, laterIssue.UID, "scheduled_on", instant)
	// The holder keeps the only slot until its run times out; its own occurrences wait on the overlap gate, not on capacity.
	holder := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "holder", Prompt: "Do it.", Trigger: interval(900), Overlap: "forbid", Timeout: 6000})
	h.kata.CreateJob(t, h.proj, workertest.Job{Name: "ahead", Prompt: "Do it.", ExistingUID: someday.UID, Trigger: interval(1800), Timeout: 60})
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "later", Prompt: "Do it.", ExistingUID: laterIssue.UID, Trigger: interval(3600), Timeout: 60})
	spin := &spinClock{ManualClock: h.clock}
	r := h.start(worker.Hooks{}, func(o *worker.Options) { o.Clock = spin })
	h.scheduled(r, 3)

	first := h.fire(r, time.Time{}) // the only slot is taken until the holder's run times out
	h.started(r, holder, 1)
	startedAt := h.clock.Now()
	// Each jump lands on the occurrences, so none is graced out before its first look.
	h.advanceTo(first.Add(901 * time.Second)) // the occurrence of "ahead" waits for its Issue
	h.eventually("the occurrence of ahead to wait", func() bool { return len(h.status(r).Waiting) == 1 })
	h.advanceTo(first.Add(2701 * time.Second)) // the occurrence of the parked job comes after it
	h.eventually("both occurrences to wait for their Issues", func() bool { return len(h.status(r).Waiting) == 2 })

	h.kata.SetSomeday(t, h.proj, someday.UID, false) // "ahead" is ready and refused for capacity, ahead of the parked job
	h.advanceTo(instant.Add(time.Minute))            // the parked job's wake instant is now behind the clock
	h.eventually("the refused job to stop waiting for its Issue", func() bool { return len(h.status(r).Waiting) <= 1 })
	time.Sleep(200 * time.Millisecond)
	before := spin.spins.Load()
	for range 20 {
		h.clock.Advance(time.Millisecond)
		time.Sleep(20 * time.Millisecond)
	}
	if n := spin.spins.Load() - before; n != 0 {
		t.Fatalf("the Project loop woke %d times in a row on an instant already past", n)
	}
	h.quiet(job, 0)

	h.advanceTo(startedAt.Add(6000*time.Second + 10*time.Second)) // the holder's run times out and frees the slot
	// The occurrences ahead run first, each until its 60 s timeout.
	var runs []cronjob.Run
	h.within("the parked job to run", func() bool {
		h.clock.Advance(61 * time.Second)
		runs = h.jobRuns(job)
		return len(runs) > 0
	})
	h.settled(runs[0].UID)
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

// A failed read of the Issue says nothing about its readiness: the parked occurrence stays parked, so the grace
// cannot drop it, and it runs once the Issue is ready.
func TestAParkedOccurrenceSurvivesAnIssueReadFailurePastItsGrace(t *testing.T) {
	h := newHarness(t)
	issue := h.kata.Issue(t, h.proj, "Parked")
	h.kata.SetSomeday(t, h.proj, issue.UID, true)
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "on issue", Prompt: "Do it.", ExistingUID: issue.UID, Trigger: interval(86400)})
	r := h.start(worker.Hooks{})
	h.scheduled(r, 1)
	first := h.fire(r, time.Time{})
	h.waitsFor(r, "Issue is someday")

	h.kata.Fail(func(req *http.Request) bool {
		return req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/issues/"+issue.UID)
	})
	h.advanceTo(first.Add(2 * time.Hour)) // past any grace while the Issue cannot be read
	h.quiet(job, 0)
	h.quiet(job, 0)

	h.kata.Heal()
	h.kata.SetSomeday(t, h.proj, issue.UID, false)
	runs := h.waitRuns(job, 1)
	if runs[0].OccurrenceKey != worker.OccurrenceKey(first) {
		t.Fatalf("the run is for %s, want the parked occurrence %s", runs[0].OccurrenceKey, worker.OccurrenceKey(first))
	}
	h.settled(runs[0].UID)
}

// Under skip a long park keeps one missed occurrence, the newest, and not every occurrence that came due meanwhile.
func TestASkipJobParkedForHoursRunsOnlyTheNewestOccurrenceOnceItsIssueIsReady(t *testing.T) {
	h := newHarness(t)
	h.capacity("5")
	issue := h.kata.Issue(t, h.proj, "Parked")
	h.kata.SetSomeday(t, h.proj, issue.UID, true)
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "on issue", Prompt: "Do it.", ExistingUID: issue.UID, Trigger: interval(3600), Overlap: "allow"})
	r := h.start(worker.Hooks{})
	h.scheduled(r, 1)
	at := h.fire(r, time.Time{})
	h.waitsFor(r, "Issue is someday")
	for range 5 {
		at = h.fire(r, at)
		h.waitsFor(r, "Issue is someday")
		h.quiet(job, 0) // the occurrence is marked parked before the next one comes due
	}
	h.advanceTo(at.Add(5 * time.Minute)) // the newest occurrence is missed, not on time
	h.quiet(job, 0)

	h.kata.SetSomeday(t, h.proj, issue.UID, false)
	runs := h.waitRuns(job, 1)
	h.quiet(job, 1)
	if runs[0].OccurrenceKey != worker.OccurrenceKey(at) {
		t.Fatalf("the run is for %s, want the newest occurrence %s", runs[0].OccurrenceKey, worker.OccurrenceKey(at))
	}
}

// A failed read of kata's run evidence says nothing about the Issue's readiness either: the occurrence stays parked.
func TestAParkedOccurrenceSurvivesARunReadFailurePastItsGrace(t *testing.T) {
	h := newHarness(t)
	issue := h.kata.Issue(t, h.proj, "Parked")
	h.kata.SetSomeday(t, h.proj, issue.UID, true)
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "on issue", Prompt: "Do it.", ExistingUID: issue.UID, Trigger: interval(86400)})
	r := h.start(worker.Hooks{})
	h.scheduled(r, 1)
	first := h.fire(r, time.Time{})
	h.waitsFor(r, "Issue is someday")

	h.kata.Fail(func(req *http.Request) bool {
		return req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/runs/")
	})
	h.kata.SetSomeday(t, h.proj, issue.UID, false) // ready, but the worker cannot tell whether kata holds the run
	h.advanceTo(first.Add(2 * time.Hour))          // past any grace
	h.quiet(job, 0)
	h.quiet(job, 0)

	h.kata.Heal()
	runs := h.waitRuns(job, 1)
	if runs[0].OccurrenceKey != worker.OccurrenceKey(first) {
		t.Fatalf("the run is for %s, want the parked occurrence %s", runs[0].OccurrenceKey, worker.OccurrenceKey(first))
	}
	h.settled(runs[0].UID)
}

// An occurrence that comes due at the very instant the Issue becomes ready replaces the parked one under skip: one run.
func TestASkipJobWhoseOccurrenceFallsOnTheIssuesInstantRunsOnlyThatOccurrence(t *testing.T) {
	h := newHarness(t)
	h.capacity("5")
	issue := h.kata.Issue(t, h.proj, "Later")
	h.kata.SetSomeday(t, h.proj, issue.UID, true)
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "on issue", Prompt: "Do it.", ExistingUID: issue.UID, Trigger: cronjob.Trigger{Kind: cronjob.TriggerCron, Cron: "0 * * * *", Timezone: "UTC"}, Overlap: "allow"})
	r := h.start(worker.Hooks{})
	h.scheduled(r, 1)
	first := h.fire(r, time.Time{})
	h.waitsFor(r, "Issue is someday")
	instant := first.Add(3 * time.Hour) // exactly an occurrence of the job, which fires on the hour
	if !instant.Equal(instant.Truncate(time.Minute)) {
		t.Fatalf("occurrence %s is not minute-aligned", instant)
	}
	h.kata.SetIssueDate(t, h.proj, issue.UID, "scheduled_on", instant)
	h.kata.SetSomeday(t, h.proj, issue.UID, false)
	h.waitsFor(r, "Issue is scheduled for")
	at := first
	for range 2 {
		at = h.fire(r, at)
		h.waitsFor(r, "Issue is scheduled for")
		h.quiet(job, 0)
	}
	h.advanceTo(instant)
	runs := h.waitRuns(job, 1)
	h.quiet(job, 1)
	if want := worker.OccurrenceKey(instant); runs[0].OccurrenceKey != want {
		t.Fatalf("the run is for %s, want the occurrence on the instant %s", runs[0].OccurrenceKey, want)
	}
}

func TestABurstOfIssueEventsListsTheCronJobsOncePerDebounceWhileAnOccurrenceWaits(t *testing.T) {
	h := newHarness(t)
	issue := h.kata.Issue(t, h.proj, "Parked")
	h.kata.SetSomeday(t, h.proj, issue.UID, true)
	job, r, _ := h.hourlyOn(issue.UID)
	h.waitsFor(r, "Issue is someday")
	time.Sleep(200 * time.Millisecond)

	listings := func() int {
		return h.kata.Requests("GET /api/v1/projects/" + strconv.FormatInt(h.proj.ID, 10) + "/cron/jobs")
	}
	before := listings()
	for i := range 40 {
		h.kata.EditIssue(t, h.proj, issue.UID, "Parked "+strconv.Itoa(i), "")
	}
	time.Sleep(500 * time.Millisecond)
	if n := listings() - before; n > 1 {
		t.Fatalf("a burst of 40 Issue events listed the Cron jobs %d times inside one debounce interval, want at most 1", n)
	}
	h.quiet(job, 0)

	h.kata.SetSomeday(t, h.proj, issue.UID, false) // the Issue is ready: the wait ends without waiting for a listing interval
	h.settled(h.waitRuns(job, 1)[0].UID)
}

// A job event the worker takes while the clock stands still is answered by a listing that reads the same clock. That listing
// still answers it: a later burst of Issue events reuses it instead of listing the jobs on every event.
func TestAJobEventAnsweredAtTheSameClockReadingStillLetsIssueEventsReuseTheListing(t *testing.T) {
	h := newHarness(t)
	issue := h.kata.Issue(t, h.proj, "Parked")
	h.kata.SetSomeday(t, h.proj, issue.UID, true)
	_, r, _ := h.hourlyOn(issue.UID)
	h.waitsFor(r, "Issue is someday")
	time.Sleep(200 * time.Millisecond)

	listings := func() int {
		return h.kata.Requests("GET /api/v1/projects/" + strconv.FormatInt(h.proj.ID, 10) + "/cron/jobs")
	}
	before := listings()
	h.kata.CreateWorkflow(t, h.proj, "unrelated", workertest.Step{Key: "noop", Command: "true"})
	// Wait in real time only: the clock must not move between the event and the listing that answers it.
	for deadline := time.Now().Add(10 * time.Second); listings() == before; time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the workflow event never made the worker list the jobs")
		}
	}
	time.Sleep(200 * time.Millisecond)
	before = listings()
	for i := range 40 {
		h.kata.EditIssue(t, h.proj, issue.UID, "Parked "+strconv.Itoa(i), "")
	}
	time.Sleep(500 * time.Millisecond)
	if n := listings() - before; n > 1 {
		t.Fatalf("after a job event answered at the same clock reading, 40 Issue events listed the Cron jobs %d times, want at most 1", n)
	}
}

// A job disabled less than the refresh debounce after the last listing must not be admitted by the next Issue event.
func TestAnIssueEventDoesNotAdmitADisabledJobFromAListingTakenBeforeItWasDisabled(t *testing.T) {
	h := newHarness(t)
	issue := h.kata.Issue(t, h.proj, "Late date")
	def := workertest.Job{Name: "on date", Prompt: "Do it.", ExistingUID: issue.UID,
		Trigger: cronjob.Trigger{Kind: cronjob.TriggerIssueScheduled, IssueUID: issue.UID}, Catchup: "all", GraceSeconds: workertest.Seconds(600)}
	job := h.kata.CreateJob(t, h.proj, def)
	r := h.start(worker.Hooks{})
	h.admitting(r)
	h.eventually("the event stream to open", func() bool { return h.status(r).Following })
	h.quiet(job, 0)

	listings := func() int {
		return h.kata.Requests("GET /api/v1/projects/" + strconv.FormatInt(h.proj.ID, 10) + "/cron/jobs")
	}
	before := listings()
	edit := def
	edit.Prompt = "Do it now."
	job = h.kata.ReplaceJob(t, h.proj, job, edit)
	h.within("a fresh listing", func() bool {
		h.clock.Advance(100 * time.Millisecond)
		return listings() > before
	})

	off := edit
	off.Disabled = true
	job = h.kata.ReplaceJob(t, h.proj, job, off)
	time.Sleep(400 * time.Millisecond) // the job event reaches the worker; the clock does not move

	h.kata.SetIssueDate(t, h.proj, issue.UID, "scheduled_on", h.minuteIn(-5*time.Minute))
	time.Sleep(800 * time.Millisecond)
	if runs := h.jobRuns(job); len(runs) != 0 {
		t.Fatalf("a disabled job ran %d time(s) from a listing taken before it was disabled: %v", len(runs), occurrenceKeys(runs))
	}
}

// Enabling a job again supersedes the occurrence that was parked before it was disabled, as any edit does.
func TestAParkedOccurrenceIsDroppedWhenTheJobIsDisabledAndEnabledAgain(t *testing.T) {
	h := newHarness(t)
	issue := h.kata.Issue(t, h.proj, "Parked")
	h.kata.SetSomeday(t, h.proj, issue.UID, true)
	job, r, _ := h.hourlyOn(issue.UID)
	h.waitsFor(r, "Issue is someday")

	def := workertest.Job{Name: "on issue", Prompt: "Do it.", ExistingUID: issue.UID, Trigger: interval(3600)}
	off := def
	off.Disabled = true
	job = h.kata.ReplaceJob(t, h.proj, job, off)
	h.eventually("the disabled job to leave the schedule", func() bool { return h.status(r).Scheduled == 0 })
	job = h.kata.ReplaceJob(t, h.proj, job, def)
	h.eventually("the re-enabled job to be scheduled", func() bool { return h.status(r).Scheduled == 1 })

	h.kata.SetSomeday(t, h.proj, issue.UID, false)
	h.quiet(job, 0)
}
