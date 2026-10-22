package worker_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/worker"
	"github.com/Dangerboi/tern-kata/internal/worker/workertest"
)

// The Issue-date seam: triggers that fire on an Issue's planning date, and notify jobs the worker delivers.

// minuteIn is a minute-aligned instant d from the harness clock, the resolution kata's dates have.
func (h *harness) minuteIn(d time.Duration) time.Time {
	return h.clock.Now().Add(d).Truncate(time.Minute)
}

func TestAnIssueScheduledJobRunsOncePerDateAtTheIssuesInstantAndMovesWithIt(t *testing.T) {
	h := newHarness(t)
	issue := h.kata.Issue(t, h.proj, "Ship it")
	first := h.minuteIn(time.Hour)
	h.kata.SetIssueDate(t, h.proj, issue.UID, "scheduled_on", first)
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "on date", Prompt: "Do it.", ExistingUID: issue.UID, Trigger: worker.Trigger{Kind: "issue-scheduled", IssueUID: issue.UID}})
	r := h.start(worker.Hooks{})
	h.scheduled(r, 1)

	if instant, _ := h.kata.PlanningDates(t, h.proj, issue.UID); !instant.Equal(first) {
		t.Fatalf("kata resolves the date to %s, want %s", instant, first)
	}
	at := h.fire(r, time.Time{})
	if !at.Equal(first) {
		t.Fatalf("the job wakes at %s, want the Issue's instant %s", at, first)
	}
	runs := h.waitRuns(job, 1)
	if runs[0].OccurrenceKey != worker.OccurrenceKey(first) || runs[0].IssueUID != issue.UID {
		t.Fatalf("run %+v, want the occurrence %s on the Issue", runs[0], first)
	}
	h.settled(runs[0].UID)
	h.clock.Advance(3 * time.Hour)
	h.quiet(job, 1) // the same date value never fires twice

	second := h.minuteIn(2 * time.Hour)
	h.kata.SetIssueDate(t, h.proj, issue.UID, "scheduled_on", second)
	if at := h.nextRun(r, first); !at.Equal(second) {
		t.Fatalf("after the date moved the job wakes at %s, want %s", at, second)
	}
	h.fire(r, first)
	runs = h.waitRuns(job, 2)
	if runs[1].OccurrenceKey != worker.OccurrenceKey(second) {
		t.Fatalf("keys %v, want the new date %s last", occurrenceKeys(runs), second)
	}
}

func TestAChangedDateBeforeItFiresReplacesTheOldInstant(t *testing.T) {
	h := newHarness(t)
	issue := h.kata.Issue(t, h.proj, "Moved")
	old := h.minuteIn(time.Hour)
	h.kata.SetIssueDate(t, h.proj, issue.UID, "scheduled_on", old)
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "on date", Prompt: "Do it.", ExistingUID: issue.UID, Trigger: worker.Trigger{Kind: "issue-scheduled", IssueUID: issue.UID}})
	r := h.start(worker.Hooks{})
	h.scheduled(r, 1)
	if at := h.nextRun(r, time.Time{}); !at.Equal(old) {
		t.Fatalf("first wake %s, want %s", at, old)
	}

	later := h.minuteIn(3 * time.Hour)
	h.kata.SetIssueDate(t, h.proj, issue.UID, "scheduled_on", later)
	h.eventually("the job to wait for the new date", func() bool { return h.status(r).NextRun.Equal(later) })
	h.advanceTo(old.Add(time.Minute))
	h.quiet(job, 0) // the old instant passed and nothing ran
	h.fire(r, old)
	runs := h.waitRuns(job, 1)
	if runs[0].OccurrenceKey != worker.OccurrenceKey(later) {
		t.Fatalf("keys %v, want only the new date %s", occurrenceKeys(runs), later)
	}
}

func TestAnIssueDeadlineNotifyJobSendsItsRequestLeadSecondsBeforeTheDeadline(t *testing.T) {
	h := newHarness(t)
	issue := h.kata.Issue(t, h.proj, "Due")
	deadline := h.minuteIn(2 * time.Hour)
	h.kata.SetIssueDate(t, h.proj, issue.UID, "deadline_on", deadline)
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "remind", Notify: &workertest.Notify{Recipient: "bob", Message: "Due in ten minutes"},
		Trigger: worker.Trigger{Kind: "issue-deadline", IssueUID: issue.UID, LeadSeconds: 600}})
	// Kata's own date sweeper already notifies a zero-lead current-owner-or-author job, so the worker leaves it alone.
	h.kata.CreateJob(t, h.proj, workertest.Job{Name: "swept", Notify: &workertest.Notify{Recipient: "current-owner-or-author"},
		Trigger: worker.Trigger{Kind: "issue-deadline", IssueUID: issue.UID}})
	r := h.start(worker.Hooks{})
	h.scheduled(r, 1)

	if _, _, ok := h.kata.InboxRequest(t, h.proj, issue.UID, "bob"); ok {
		t.Fatal("bob was notified before the lead time")
	}
	at := h.fire(r, time.Time{})
	if want := deadline.Add(-10 * time.Minute); !at.Equal(want) {
		t.Fatalf("the job wakes at %s, want the deadline less its lead, %s", at, want)
	}
	runs := h.waitRuns(job, 1)
	run := h.settled(runs[0].UID)
	if run.Status != "succeeded" || run.IssueUID != issue.UID {
		t.Fatalf("run %+v, want a succeeded run on the Issue", run)
	}
	from, message, ok := h.kata.InboxRequest(t, h.proj, issue.UID, "bob")
	if !ok || from != "robot" || message != "Due in ten minutes" {
		t.Fatalf("bob's inbox request is %q from %q (present %v), want the job's message from the worker's actor", message, from, ok)
	}
	if len(h.launches()) != 0 {
		t.Fatal("a notify run started an agent")
	}
	h.clock.Advance(time.Hour)
	h.quiet(job, 1)
}

func TestANotifyJobOnATimerReachesItsRecipientAndAnOwnerAliasResolvesFromTheIssue(t *testing.T) {
	h := newHarness(t)
	issue := h.kata.Issue(t, h.proj, "Weekly")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "nudge", ExistingUID: issue.UID, Trigger: interval(3600),
		Notify: &workertest.Notify{Recipient: "current-owner-or-author"}})
	r := h.start(worker.Hooks{})
	h.scheduled(r, 1)

	h.fire(r, time.Time{})
	h.settled(h.waitRuns(job, 1)[0].UID)
	from, message, ok := h.kata.InboxRequest(t, h.proj, issue.UID, "tester")
	if !ok || from != "robot" || message == "" {
		t.Fatalf("the Issue's author got %q from %q (present %v), want a request with a default message", message, from, ok)
	}
}

func TestADateSetAfterTheJobWasFirstSeenIsCaughtUpUnderTheJobsCatchup(t *testing.T) {
	cases := []struct {
		name    string
		catchup string
		grace   int64
		runs    int
	}{
		{"all runs it", "all", 60, 1},
		{"skip within grace runs it", "skip", 600, 1},
		{"skip outside grace does not", "skip", 60, 0},
	}
	for _, tc := range cases {
		for _, clearFirst := range []bool{false, true} {
			name := tc.name
			if clearFirst {
				name += " after the date was cleared and set again"
			}
			t.Run(name, func(t *testing.T) {
				h := newHarness(t)
				issue := h.kata.Issue(t, h.proj, "Late date")
				job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "on date", Prompt: "Do it.", ExistingUID: issue.UID,
					Trigger: worker.Trigger{Kind: "issue-scheduled", IssueUID: issue.UID}, Catchup: tc.catchup, GraceSeconds: workertest.Seconds(tc.grace)})
				r := h.start(worker.Hooks{})
				h.admitting(r)
				h.quiet(job, 0) // the job is seen once while its Issue has no date
				if clearFirst {
					later := h.minuteIn(time.Hour)
					h.kata.SetIssueDate(t, h.proj, issue.UID, "scheduled_on", later)
					h.eventually("the job to wait for the date", func() bool { return h.status(r).NextRun.Equal(later) })
					h.kata.ClearIssueDate(t, h.proj, issue.UID, "scheduled_on")
					h.eventually("the job to stop waiting", func() bool { return h.status(r).NextRun.IsZero() })
				}
				past := h.minuteIn(-5 * time.Minute)
				h.kata.SetIssueDate(t, h.proj, issue.UID, "scheduled_on", past)
				if tc.runs == 0 {
					h.quiet(job, 0)
					return
				}
				runs := h.waitRuns(job, 1)
				if got := runs[len(runs)-1].OccurrenceKey; got != worker.OccurrenceKey(past) {
					t.Fatalf("keys %v, want the date %s last", occurrenceKeys(runs), past)
				}
				h.quiet(job, 1)
			})
		}
	}
}

// A planning-date read that failed when the job was first seen leaves no baseline: the Issue's date that was already
// in the past when the job was created is its first sight, not a change to replay under `all`.
func TestADateReadThatFailedAtFirstSightDoesNotMakeAnOldDateAChange(t *testing.T) {
	h := newHarness(t)
	issue := h.kata.Issue(t, h.proj, "Old date")
	past := h.minuteIn(-5 * time.Minute)
	h.kata.SetIssueDate(t, h.proj, issue.UID, "scheduled_on", past)
	isRead := func(r *http.Request) bool {
		return r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/planning-dates")
	}
	restore := h.kata.Intercept(isRead, func(w http.ResponseWriter, _ *http.Request, _ http.Handler) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	})
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "on date", Prompt: "Do it.", ExistingUID: issue.UID,
		Trigger: worker.Trigger{Kind: "issue-scheduled", IssueUID: issue.UID}, Catchup: "all"})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	h.eventually("the first date read to fail", func() bool { return h.kata.Requests("planning-dates") > 0 })
	failed := h.kata.Requests("planning-dates")
	restore() // the loop plans the job right after that read, before it can read again
	h.eventually("the date to be read again", func() bool { return h.kata.Requests("planning-dates") > failed })
	h.quiet(job, 0)
}
