package worker_test

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
	"github.com/Dangerboi/tern-kata/internal/worker"
	"github.com/Dangerboi/tern-kata/internal/worker/workertest"
)

// The Project refuses work (its runs are not all recovered) while a catch-up-all job is disabled for 5.5 hours and
// enabled again: the time off is never run.
func TestAJobDisabledAndEnabledWhileTheProjectRefusesWorkRunsNothingOfTheTimeOff(t *testing.T) {
	h := newHarness(t)
	def := workertest.Job{Name: "paused", Prompt: "Do it.", Trigger: interval(3600), Catchup: "all", GraceSeconds: workertest.Seconds(3600)}
	job := h.kata.CreateJob(t, h.proj, def)
	r := h.start(worker.Hooks{})
	h.scheduled(r, 1) // checkpoint saved
	r.stop()

	runs := h.dirs.Runs()
	_ = os.RemoveAll(runs)
	if err := os.WriteFile(runs, nil, 0o600); err != nil { // listing the runs fails: recovery cannot finish
		t.Fatal(err)
	}
	r = h.start(worker.Hooks{})
	h.eventually("the Project to refuse", func() bool {
		s := r.w.Status()
		return len(s.Projects) == 1 && strings.Contains(s.Projects[0].Why, "not all recovered yet")
	})
	h.eventually("the event stream to open", func() bool { return h.status(r).Following })
	listings := func() int {
		return h.kata.Requests("GET /api/v1/projects/" + strconv.FormatInt(h.proj.ID, 10) + "/cron/jobs")
	}
	off := def
	off.Disabled = true
	before := listings()
	job = h.kata.ReplaceJob(t, h.proj, job, off)
	h.eventually("the worker to list the disabled job", func() bool { return listings() > before })
	time.Sleep(200 * time.Millisecond)
	h.advanceTo(job.CreatedAt.Add(5*time.Hour + 30*time.Minute))
	time.Sleep(300 * time.Millisecond)

	before = listings()
	job = h.kata.ReplaceJob(t, h.proj, job, def)
	h.eventually("the worker to list the enabled job", func() bool { return listings() > before })
	for range 3 {
		time.Sleep(300 * time.Millisecond)
		h.clock.Advance(3 * time.Second)
	}
	time.Sleep(300 * time.Millisecond)
	if err := os.Remove(runs); err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(15 * time.Second)
	h.admitting(r)
	h.eventually("the re-enabled job to be scheduled", func() bool { return h.status(r).Scheduled == 1 })
	h.quiet(job, 0)
}

// A restart between the re-enable of an Issue-date job and the first successful read of its date keeps the date as
// the baseline: the date that moved into the time off is not caught up.
func TestARestartBetweenAReEnableAndTheFirstDateReadDoesNotCatchUpTheTimeOff(t *testing.T) {
	h := newHarness(t)
	issue := h.kata.Issue(t, h.proj, "Dated")
	h.kata.SetIssueDate(t, h.proj, issue.UID, "scheduled_on", h.minuteIn(3*time.Hour))
	def := workertest.Job{Name: "on date", Prompt: "Do it.", ExistingUID: issue.UID,
		Trigger: cronjob.Trigger{Kind: cronjob.TriggerIssueScheduled, IssueUID: issue.UID}, Catchup: "skip", GraceSeconds: workertest.Seconds(7200)}
	job := h.kata.CreateJob(t, h.proj, def)
	r := h.start(worker.Hooks{})
	h.scheduled(r, 1)
	h.eventually("the event stream to open", func() bool { return h.status(r).Following })

	off := def
	off.Disabled = true
	job = h.kata.ReplaceJob(t, h.proj, job, off)
	h.eventually("the disabled job to leave the schedule", func() bool { return h.status(r).Scheduled == 0 })
	h.advanceTo(h.clock.Now().Add(time.Hour))
	h.kata.SetIssueDate(t, h.proj, issue.UID, "scheduled_on", h.minuteIn(-10*time.Minute))
	h.quiet(job, 0)

	restore := h.kata.Intercept(func(r *http.Request) bool {
		return r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/planning-dates")
	}, func(w http.ResponseWriter, _ *http.Request, _ http.Handler) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	})
	path := h.dirs.ProjectDir(h.proj.UID) + "/schedule/" + job.UID + ".json"
	job = h.kata.ReplaceJob(t, h.proj, job, def)
	h.eventually("the resume to be saved", func() bool {
		raw, _ := os.ReadFile(path)
		return len(raw) > 0 && !strings.Contains(string(raw), "paused") && !strings.Contains(string(raw), "date_seen")
	})
	r.stop()
	restore()
	h.clock.Advance(10 * time.Second)
	r = h.start(worker.Hooks{})
	h.scheduled(r, 1)
	h.quiet(job, 0)
}
