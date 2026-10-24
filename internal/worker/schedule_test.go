package worker_test

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
	"github.com/Dangerboi/tern-kata/internal/worker"
	"github.com/Dangerboi/tern-kata/internal/worker/workertest"
)

// The scheduling seam: the whole worker on an injected clock. The Status' next run says when the worker will wake, so
// a test advances the clock to exactly that instant and then asks kata what ran.

func (h *harness) status(r *running) worker.ProjectStatus { return r.w.Status().Projects[0] }

// nextRun waits for the worker's earliest next occurrence after prev.
func (h *harness) nextRun(r *running, prev time.Time) time.Time {
	h.t.Helper()
	var at time.Time
	h.eventually("a next run after "+prev.Format(time.RFC3339), func() bool {
		at = h.status(r).NextRun
		return at.After(prev)
	})
	return at
}

func (h *harness) advanceTo(at time.Time) {
	h.t.Helper()
	if d := at.Sub(h.clock.Now()); d > 0 {
		h.clock.Advance(d)
	}
}

// fire advances the clock to the next occurrence after prev and returns it.
func (h *harness) fire(r *running, prev time.Time) time.Time {
	h.t.Helper()
	at := h.nextRun(r, prev)
	h.advanceTo(at)
	return at
}

// jobRuns is what kata holds of the job's runs, oldest occurrence first.
func (h *harness) jobRuns(job cronjob.Job) []worker.Run {
	h.t.Helper()
	runs, err := h.kata.Kata.Runs(context.Background(), h.proj.ID)
	if err != nil {
		h.t.Fatal(err)
	}
	var out []worker.Run
	for _, run := range runs {
		if run.JobUID == job.UID {
			out = append(out, run)
		}
	}
	slices.SortFunc(out, func(a, b worker.Run) int { return strings.Compare(a.OccurrenceKey, b.OccurrenceKey) })
	return out
}

func (h *harness) waitRuns(job cronjob.Job, n int) []worker.Run {
	h.t.Helper()
	var runs []worker.Run
	h.eventually("kata to hold the runs of "+job.Name, func() bool {
		runs = h.jobRuns(job)
		return len(runs) >= n
	})
	if len(runs) != n {
		h.t.Fatalf("kata holds %d runs of %s, want %d: %v", len(runs), job.Name, n, occurrenceKeys(runs))
	}
	return runs
}

func occurrenceKeys(runs []worker.Run) []string {
	var keys []string
	for _, r := range runs {
		keys = append(keys, r.OccurrenceKey)
	}
	return keys
}

// quiet fails when more runs of the job than want exist after the worker had every chance to start them.
func (h *harness) quiet(job cronjob.Job, want int) {
	h.t.Helper()
	time.Sleep(300 * time.Millisecond)
	h.clock.Advance(time.Millisecond) // wakes anything still waiting on the clock
	time.Sleep(300 * time.Millisecond)
	if runs := h.jobRuns(job); len(runs) != want {
		h.t.Fatalf("kata holds %d runs of %s, want %d: %v", len(runs), job.Name, want, occurrenceKeys(runs))
	}
}

// capacity serves the Project with n slots.
func (h *harness) capacity(n string) {
	h.t.Helper()
	cfg := strings.Replace(h.config, "name = \"demo\"\n", "name = \"demo\"\ncapacity = "+n+"\n", 1)
	if err := os.WriteFile(h.dirs.ConfigPath(), []byte(cfg), 0o600); err != nil {
		h.t.Fatal(err)
	}
}

// scheduled waits for the worker to have read the Project's jobs and planned n of them.
func (h *harness) scheduled(r *running, n int) {
	h.t.Helper()
	h.admitting(r)
	h.eventually("the worker to schedule", func() bool { return h.status(r).Scheduled == n })
}

// started waits for n agents of the job to be running.
func (h *harness) started(r *running, job cronjob.Job, n int) {
	h.t.Helper()
	h.eventually("agents of the job to run", func() bool {
		count := 0
		for _, rs := range r.w.Status().Runs {
			if rs.Job == job.Name && rs.State == "running" {
				count++
			}
		}
		return count == n
	})
	time.Sleep(100 * time.Millisecond) // the run's timeout timer is set just after its start is saved
}

func interval(seconds int64) cronjob.Trigger {
	return cronjob.Trigger{Kind: cronjob.TriggerInterval, IntervalSeconds: seconds}
}

func TestAnIntervalJobRunsAtEachOccurrenceAnchoredAtItsCreation(t *testing.T) {
	h := newHarness(t)
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "hourly", Prompt: "Do it.", Trigger: interval(3600)})
	r := h.start(worker.Hooks{})
	h.scheduled(r, 1)

	first := h.fire(r, time.Time{})
	if want := job.CreatedAt.Add(time.Hour); !first.Equal(want) {
		t.Fatalf("first occurrence %s, want an hour after the job's creation, %s", first, want)
	}
	runs := h.waitRuns(job, 1)
	if runs[0].OccurrenceKey != worker.OccurrenceKey(first) || runs[0].ExecutorLabel != "laptop" {
		t.Fatalf("run %+v, want the occurrence %s", runs[0], first)
	}
	h.settled(runs[0].UID)

	second := h.fire(r, first)
	if !second.Equal(first.Add(time.Hour)) {
		t.Fatalf("second occurrence %s, want an hour after the first", second)
	}
	runs = h.waitRuns(job, 2)
	if runs[1].OccurrenceKey != worker.OccurrenceKey(second) {
		t.Fatalf("keys %v, want the second occurrence %s last", occurrenceKeys(runs), second)
	}
}

func TestACronJobRunsInItsTriggerZoneAndEveryIsAnAnchoredInterval(t *testing.T) {
	h := newHarness(t)
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("no time zone data")
	}
	cron := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "morning", Prompt: "Do it.", Trigger: cronjob.Trigger{Kind: cronjob.TriggerCron, Cron: "0 9 * * *", Timezone: "Europe/Berlin"}})
	every := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "every", Prompt: "Do it.", Trigger: cronjob.Trigger{Kind: cronjob.TriggerCron, Cron: "@every 2h"}})
	r := h.start(worker.Hooks{})
	h.scheduled(r, 2)

	now := h.clock.Now().In(berlin)
	nine := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, berlin)
	if !nine.After(now) {
		nine = nine.AddDate(0, 0, 1)
	}
	twoHours := every.CreatedAt.Add(2 * time.Hour)
	first, second := nine.UTC(), twoHours
	if second.Before(first) {
		first, second = second, first
	}
	if got := h.nextRun(r, time.Time{}); !got.Equal(first) {
		t.Fatalf("the worker's next run is %s, want %s", got, first)
	}
	h.advanceTo(first)
	if first.Equal(nine.UTC()) {
		h.settled(h.waitRuns(cron, 1)[0].UID)
		if got := h.waitRuns(cron, 1)[0].OccurrenceKey; got != worker.OccurrenceKey(nine.UTC()) {
			t.Fatalf("cron run %s, want 09:00 Berlin = %s", got, worker.OccurrenceKey(nine.UTC()))
		}
	} else {
		h.waitRuns(every, 1)
	}
	h.advanceTo(second)
	if second.Equal(twoHours) {
		if got := h.waitRuns(every, 1)[0].OccurrenceKey; got != worker.OccurrenceKey(twoHours) {
			t.Fatalf("@every run %s, want two hours after the job's creation, %s", got, worker.OccurrenceKey(twoHours))
		}
	} else {
		h.waitRuns(cron, 1)
	}
}

func TestAOnceJobRunsOnceAndAManualJobOnlyOnRequest(t *testing.T) {
	h := newHarness(t)
	at := h.clock.Now().Add(90 * time.Minute).Truncate(time.Second)
	once := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "once", Prompt: "Do it.", Trigger: cronjob.Trigger{Kind: cronjob.TriggerOnce, At: at.Format(time.RFC3339)}})
	manual := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "manual", Prompt: "Do it."})
	r := h.start(worker.Hooks{})
	h.scheduled(r, 1)

	if got := h.fire(r, time.Time{}); !got.Equal(at) {
		t.Fatalf("next run %s, want the instant %s", got, at)
	}
	runs := h.waitRuns(once, 1)
	h.settled(runs[0].UID)
	h.clock.Advance(30 * 24 * time.Hour)
	h.quiet(once, 1)
	h.quiet(manual, 0)
	if !h.status(r).NextRun.IsZero() {
		t.Fatalf("next run %s after the only occurrence passed", h.status(r).NextRun)
	}
	// A request still runs the manual job.
	resp := h.ask(worker.Request{Op: "run", Job: manual.UID})
	if !resp.OK {
		t.Fatalf("run request refused: %s", resp.Error)
	}
	h.waitRuns(manual, 1)
}

func TestForbiddenOverlapHoldsOccurrencesUntilTheEarlierRunSettlesAndTimesOut(t *testing.T) {
	h := newHarness(t, "--stub-hang")
	h.capacity("2") // a free slot does not open a forbidding job's gate
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "slow", Prompt: "Do it.", Trigger: interval(60), Overlap: "forbid", Catchup: "all", Timeout: 300})
	r := h.start(worker.Hooks{})
	h.scheduled(r, 1)

	first := h.fire(r, time.Time{})
	h.started(r, job, 1)
	for i := 1; i <= 2; i++ {
		h.advanceTo(first.Add(time.Duration(i) * time.Minute))
	}
	h.eventually("the held occurrences to queue", func() bool { return h.status(r).Queued == 2 })
	h.quiet(job, 1)

	h.advanceTo(first.Add(380 * time.Second)) // the first run's timeout passes, and four more occurrences
	h.started(r, job, 1)                      // the oldest waiting occurrence takes the gate, and only it
	runs := h.waitRuns(job, 2)
	if runs[0].Status != "failed" || !strings.Contains(runs[0].Summary, "timed out") {
		t.Fatalf("the first run is %q %q, want failed with a timeout", runs[0].Status, runs[0].Summary)
	}
	if want := worker.OccurrenceKey(first.Add(time.Minute)); runs[1].OccurrenceKey != want {
		t.Fatalf("the second run is %s, want the oldest waiting occurrence %s", runs[1].OccurrenceKey, want)
	}
	h.quiet(job, 2)
}

func TestForbiddenOverlapWithSkipDropsWhatItCouldNotRun(t *testing.T) {
	h := newHarness(t, "--stub-hang")
	h.capacity("2")
	grace := workertest.Seconds(0)
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "slow", Prompt: "Do it.", Trigger: interval(60), Overlap: "forbid", Catchup: "skip", GraceSeconds: grace, Timeout: 300})
	r := h.start(worker.Hooks{})
	h.scheduled(r, 1)

	first := h.fire(r, time.Time{})
	h.started(r, job, 1)
	h.advanceTo(first.Add(380 * time.Second)) // the run times out; the occurrences it blocked are over
	h.eventually("the first run to time out", func() bool {
		runs := h.jobRuns(job)
		return len(runs) == 1 && runs[0].Status == "failed"
	})
	h.quiet(job, 1)

	next := h.fire(r, first.Add(380*time.Second))
	runs := h.waitRuns(job, 2)
	if runs[1].OccurrenceKey != worker.OccurrenceKey(next) {
		t.Fatalf("keys %v, want only the first and the new occurrence %s", occurrenceKeys(runs), next)
	}
}

func TestAllowedOverlapRunsTogetherUpToCapacityAndQueuesTheRestOldestFirst(t *testing.T) {
	h := newHarness(t, "--stub-hang")
	h.capacity("2")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "busy", Prompt: "Do it.", Trigger: interval(60), Overlap: "allow", Catchup: "all", Timeout: 600})
	r := h.start(worker.Hooks{})
	h.scheduled(r, 1)

	first := h.fire(r, time.Time{})
	h.started(r, job, 1)
	h.advanceTo(first.Add(time.Minute))
	h.started(r, job, 2)
	h.advanceTo(first.Add(2 * time.Minute))
	h.eventually("the third occurrence to queue", func() bool { return h.status(r).Queued == 1 })
	h.quiet(job, 2)

	h.advanceTo(first.Add(605 * time.Second)) // the first run times out and frees a slot
	runs := h.waitRuns(job, 3)
	if runs[0].Status != "failed" {
		t.Fatalf("the first run is %q, want failed by its timeout", runs[0].Status)
	}
	if want := worker.OccurrenceKey(first.Add(2 * time.Minute)); runs[2].OccurrenceKey != want {
		t.Fatalf("keys %v, want the oldest queued occurrence %s third", occurrenceKeys(runs), want)
	}
}

func TestCatchupAcrossADowntimeGap(t *testing.T) {
	gap := 5*time.Hour + 30*time.Minute // five hourly occurrences pass while the worker is down, the last 30 minutes ago
	cases := []struct {
		name    string
		catchup string
		grace   int64
		want    []int // hours after the job's creation of the runs that follow the restart
	}{
		{"all runs every missed occurrence in order", "all", 0, []int{1, 2, 3, 4, 5}},
		{"latest runs only the newest", "latest", 0, []int{5}},
		{"skip runs the newest within grace", "skip", 3600, []int{5}},
		{"skip with a shorter grace runs nothing missed", "skip", 600, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "hourly", Prompt: "Do it.", Trigger: interval(3600), Catchup: tc.catchup, GraceSeconds: workertest.Seconds(tc.grace)})
			r := h.start(worker.Hooks{})
			h.scheduled(r, 1)
			r.stop()

			h.advanceTo(job.CreatedAt.Add(gap))
			restarted := h.start(worker.Hooks{})
			h.scheduled(restarted, 1)

			var want []string
			for _, hours := range tc.want {
				want = append(want, worker.OccurrenceKey(job.CreatedAt.Add(time.Duration(hours)*time.Hour)))
			}
			if len(want) > 0 {
				h.waitRuns(job, len(want))
			}
			h.quiet(job, len(want))
			if got := occurrenceKeys(h.jobRuns(job)); !slices.Equal(got, want) {
				t.Fatalf("runs %v, want %v", got, want)
			}
			// The next occurrence still runs on time.
			next := h.fire(restarted, time.Time{})
			if !next.Equal(job.CreatedAt.Add(6 * time.Hour)) {
				t.Fatalf("next occurrence %s, want six hours after creation", next)
			}
			h.waitRuns(job, len(want)+1)
		})
	}
}

func TestCatchupKeepsAtMostAWeekAndAHundredOccurrences(t *testing.T) {
	h := newHarness(t, "--stub-hang")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "minutely", Prompt: "Do it.", Trigger: interval(60), Catchup: "all", Timeout: 3600})
	r := h.start(worker.Hooks{})
	h.scheduled(r, 1)
	r.stop()

	h.advanceTo(job.CreatedAt.Add(8*24*time.Hour + 30*time.Second))
	restarted := h.start(worker.Hooks{})
	h.started(restarted, job, 1) // one slot: the rest of the bounded backlog queues
	h.eventually("the backlog to queue", func() bool { return h.status(restarted).Queued == 99 })
	runs := h.waitRuns(job, 1)
	oldest := h.clock.Now().Add(-7 * 24 * time.Hour)
	key, err := time.Parse(time.RFC3339Nano, runs[0].OccurrenceKey)
	if err != nil || key.Before(oldest) {
		t.Fatalf("the oldest admitted occurrence is %s, want one within the last week (after %s)", runs[0].OccurrenceKey, oldest)
	}
}

func TestEditingOrDisablingAJobChangesTheScheduleWithoutARestart(t *testing.T) {
	h := newHarness(t)
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "edited", Prompt: "Do it.", Trigger: interval(3600), GraceSeconds: workertest.Seconds(0)})
	r := h.start(worker.Hooks{})
	h.scheduled(r, 1)
	h.eventually("the event stream to open", func() bool { return h.status(r).Following })
	if got := h.nextRun(r, time.Time{}); !got.Equal(job.CreatedAt.Add(time.Hour)) {
		t.Fatalf("next run %s, want an hour after creation", got)
	}

	job = h.kata.ReplaceJob(t, h.proj, job, workertest.Job{Name: "edited", Prompt: "Do it.", Trigger: interval(3600), Disabled: true, GraceSeconds: workertest.Seconds(0)})
	h.eventually("the disabled job to leave the schedule", func() bool { return h.status(r).Scheduled == 0 })
	h.advanceTo(job.CreatedAt.Add(3*time.Hour + 5*time.Minute))
	h.quiet(job, 0)

	// Re-enabled with a new interval, it plans from the new definition; the hours it was disabled are not run.
	job = h.kata.ReplaceJob(t, h.proj, job, workertest.Job{Name: "edited", Prompt: "Do it.", Trigger: interval(600), GraceSeconds: workertest.Seconds(0)})
	h.eventually("the edited job to be scheduled", func() bool { return h.status(r).Scheduled == 1 })
	next := h.nextRun(r, time.Time{})
	if want := job.CreatedAt.Add(3*time.Hour + 10*time.Minute); !next.Equal(want) {
		t.Fatalf("next run %s, want %s under the new interval", next, want)
	}
	h.quiet(job, 0)
	h.advanceTo(next)
	runs := h.waitRuns(job, 1)
	if runs[0].OccurrenceKey != worker.OccurrenceKey(next) {
		t.Fatalf("run %s, want the occurrence %s of the edited job", runs[0].OccurrenceKey, next)
	}

	// Changed to manual, it no longer fires.
	job = h.kata.ReplaceJob(t, h.proj, job, workertest.Job{Name: "edited", Prompt: "Do it."})
	h.eventually("the manual job to leave the schedule", func() bool { return h.status(r).Scheduled == 0 })
	h.advanceTo(next.Add(2 * time.Hour))
	h.quiet(job, 1)
}

func TestAPerRunIssueGetsItsDateOffsetsFromTheOccurrence(t *testing.T) {
	h := newHarness(t)
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "dated", Prompt: "Do it.", ScheduledOffset: workertest.Seconds(-120), DeadlineOffset: workertest.Seconds(2 * 86400)})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	uid := h.ask(worker.Request{Op: "run", Job: job.UID}).Run.RunUID
	run := h.settled(uid)
	issue, err := h.kata.Kata.ShowIssue(context.Background(), h.proj.ID, run.IssueUID)
	if err != nil {
		t.Fatal(err)
	}
	_, stamp, _ := strings.Cut(issue.Title, " — ")
	occurrence, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		t.Fatalf("the Issue title %q does not end in the occurrence: %v", issue.Title, err)
	}
	scheduled, deadline := h.kata.PlanningDates(t, h.proj, run.IssueUID)
	if want := occurrence.Add(-2 * time.Minute).Truncate(time.Second); !scheduled.Equal(want) {
		t.Errorf("scheduled %s, want two minutes before the occurrence, %s", scheduled, want)
	}
	if want := occurrence.Add(48 * time.Hour).Truncate(time.Second); !deadline.Equal(want) {
		t.Errorf("deadline %s, want two days after the occurrence, %s", deadline, want)
	}

	plain := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "plain", Prompt: "Do it."})
	var accepted worker.Response
	h.eventually("the first run to free its slot", func() bool {
		accepted = h.ask(worker.Request{Op: "run", Job: plain.UID})
		return accepted.OK
	})
	run = h.settled(accepted.Run.RunUID)
	if scheduled, deadline := h.kata.PlanningDates(t, h.proj, run.IssueUID); !scheduled.IsZero() || !deadline.IsZero() {
		t.Errorf("an Issue of a job with no offsets has dates %s and %s", scheduled, deadline)
	}
}
