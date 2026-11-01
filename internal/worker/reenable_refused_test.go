package worker_test

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/worker"
	"github.com/Dangerboi/tern-kata/internal/worker/workertest"
)

// The Project refuses (recovery blocked). A catch-up-all hourly job is disabled, enabled again at +1h30m, and the
// refusal lasts until +5h30m: the job starts from the enable, and the occurrences missed while the Project refused
// (+2h to +5h) follow the catch-up rules.
func TestAJobEnabledAgainWhileTheProjectRefusesWorkCatchesUpFromTheEnableWhenTheRefusalEnds(t *testing.T) {
	h := newHarness(t)
	def := workertest.Job{Name: "paused", Prompt: "Do it.", Trigger: interval(3600), Catchup: "all", GraceSeconds: workertest.Seconds(3600)}
	job := h.kata.CreateJob(t, h.proj, def)
	r := h.start(worker.Hooks{})
	h.scheduled(r, 1)
	r.stop()

	runs := h.dirs.Runs()
	_ = os.RemoveAll(runs)
	if err := os.WriteFile(runs, nil, 0o600); err != nil {
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
	for range 3 {
		time.Sleep(300 * time.Millisecond)
		h.clock.Advance(3 * time.Second)
	}
	h.eventually("the worker to list the disabled job", func() bool { return listings() > before })
	time.Sleep(200 * time.Millisecond)
	h.advanceTo(job.CreatedAt.Add(time.Hour + 30*time.Minute))
	time.Sleep(300 * time.Millisecond)

	before = listings()
	job = h.kata.ReplaceJob(t, h.proj, job, def)
	for range 3 { // the job event's listing waits on the worker's clock
		time.Sleep(300 * time.Millisecond)
		h.clock.Advance(3 * time.Second)
	}
	h.eventually("the worker to list the enabled job", func() bool { return listings() > before })
	time.Sleep(300 * time.Millisecond)
	h.advanceTo(job.CreatedAt.Add(5*time.Hour + 30*time.Minute))
	time.Sleep(300 * time.Millisecond)
	if err := os.Remove(runs); err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(15 * time.Second)
	h.admitting(r)
	h.waitRuns(job, 4)
}

// A corrupt checkpoint of an enabled job is reported once, not on every tick of a refusal.
func TestACorruptCheckpointIsReportedOnceWhileTheProjectRefusesWork(t *testing.T) {
	h := newHarness(t)
	def := workertest.Job{Name: "j", Prompt: "Do it.", Trigger: interval(3600), Catchup: "all"}
	job := h.kata.CreateJob(t, h.proj, def)
	r := h.start(worker.Hooks{})
	h.scheduled(r, 1)
	r.stop()
	path := h.dirs.ProjectDir(h.proj.UID) + "/schedule/" + job.UID + ".json"
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	runs := h.dirs.Runs()
	_ = os.RemoveAll(runs)
	if err := os.WriteFile(runs, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	log := &logLines{}
	r = h.start(worker.Hooks{}, func(o *worker.Options) { o.Log = log.add })
	h.eventually("the Project to refuse", func() bool {
		s := r.w.Status()
		return len(s.Projects) == 1 && strings.Contains(s.Projects[0].Why, "not all recovered yet")
	})
	h.eventually("the event stream to open", func() bool { return h.status(r).Following })
	for range 30 { // 7.5 minutes of the fake clock
		time.Sleep(30 * time.Millisecond)
		h.clock.Advance(15 * time.Second)
	}
	if n := log.count("unreadable"); n > 1 {
		t.Fatalf("the corrupt checkpoint was reported %d times during the refusal, want once", n)
	}
}
