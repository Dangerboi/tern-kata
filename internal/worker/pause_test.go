package worker_test

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/worker"
	"github.com/Dangerboi/tern-kata/internal/worker/workertest"
)

// limitsOf rewrites worker.toml with a [limits] table; the next start reads it.
func (h *harness) limitsOf(text string) {
	h.t.Helper()
	h.config = strings.Replace(h.config, "[[projects]]", "[limits]\n"+text+"\n[[projects]]", 1)
	if err := os.WriteFile(h.dirs.ConfigPath(), []byte(h.config), 0o600); err != nil {
		h.t.Fatal(err)
	}
}

// agentArgs rewrites the default profile's stub arguments; the next start reads them.
func (h *harness) agentArgs(stubArgs ...string) {
	h.t.Helper()
	raw, _ := json.Marshal(append([]string{"--stub-log", h.log}, stubArgs...))
	lines := strings.Split(h.config, "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "args = ") {
			lines[i] = "args = " + string(raw)
		}
	}
	h.config = strings.Join(lines, "\n")
	if err := os.WriteFile(h.dirs.ConfigPath(), []byte(h.config), 0o600); err != nil {
		h.t.Fatal(err)
	}
}

// runOnce asks for a run of the job and waits until its outcome is counted: kata holds it settled.
func (h *harness) runOnce(r *running, jobUID string) string {
	h.t.Helper()
	resp := h.ask(worker.Request{Op: "run", Job: jobUID})
	if resp.Error != "" {
		h.t.Fatalf("the run was refused: %s", resp.Error)
	}
	h.settledInKata(r, resp.Run.RunUID)
	return resp.Run.RunUID
}

func TestAJobPausesAfterThreeFailedRunsInARowUntilItIsResumed(t *testing.T) {
	h := newHarness(t, "--stub-exit", "3")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "flaky", Prompt: "Do it.", Checkout: "repo"})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	for range 2 {
		h.runOnce(r, job.UID)
	}
	if p := r.w.Status().Paused; len(p) != 0 {
		t.Fatalf("paused after two failed runs: %+v", p)
	}
	h.runOnce(r, job.UID)
	s := r.w.Status()
	if len(s.Paused) != 1 || s.Paused[0].JobUID != job.UID || s.Paused[0].Failed != 3 || s.Paused[0].Project != "demo" {
		t.Fatalf("paused jobs %+v, want flaky after 3 failures", s.Paused)
	}
	if got := s.String(); !strings.Contains(got, `"flaky"`) || !strings.Contains(got, "paused after 3 failed runs in a row") || !strings.Contains(got, "tern-kata worker resume "+job.UID) {
		t.Fatalf("status does not name the pause:\n%s", got)
	}
	if resp := h.ask(worker.Request{Op: "run", Job: job.UID}); !strings.Contains(resp.Error, "paused after 3 failed runs in a row") {
		t.Fatalf("a run of a paused job answered %+v", resp)
	}
	launched := len(h.launches())

	// The pause is the worker's own state: it outlives a restart.
	r.stop()
	r = h.start(worker.Hooks{})
	h.admitting(r)
	if p := r.w.Status().Paused; len(p) != 1 {
		t.Fatalf("the pause was lost over a restart: %+v", p)
	}

	if resp := h.ask(worker.Request{Op: "resume-job", Job: job.UID}); !resp.OK || resp.Resumed == nil || resp.Resumed.Name != "flaky" {
		t.Fatalf("resume answered %+v", resp)
	}
	if p := r.w.Status().Paused; len(p) != 0 {
		t.Fatalf("still paused after a resume: %+v", p)
	}
	h.runOnce(r, job.UID) // admitted again; the count starts from none, so one failure pauses nothing
	if p := r.w.Status().Paused; len(p) != 0 {
		t.Fatalf("paused again after one failure: %+v", p)
	}
	if got := len(h.launches()); got != launched+1 {
		t.Fatalf("agent launched %d times, want %d", got, launched+1)
	}
	if resp := h.ask(worker.Request{Op: "resume-job", Job: job.UID}); resp.OK || !strings.Contains(resp.Error, "not paused") {
		t.Fatalf("resuming a job that is not paused answered %+v", resp)
	}
}

func TestASucceededRunResetsTheCountOfFailures(t *testing.T) {
	h := newHarness(t, "--stub-exit", "3")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "flaky", Prompt: "Do it.", Checkout: "repo"})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	h.runOnce(r, job.UID)
	h.runOnce(r, job.UID)
	r.stop()

	h.agentArgs() // the agent works again
	r = h.start(worker.Hooks{})
	h.admitting(r)
	h.runOnce(r, job.UID)
	r.stop()

	h.agentArgs("--stub-exit", "3")
	r = h.start(worker.Hooks{})
	h.admitting(r)
	h.runOnce(r, job.UID)
	h.runOnce(r, job.UID) // four failures in all, two since the success
	if p := r.w.Status().Paused; len(p) != 0 {
		t.Fatalf("paused although a success came between: %+v", p)
	}
	h.runOnce(r, job.UID)
	if p := r.w.Status().Paused; len(p) != 1 {
		t.Fatalf("not paused after three failures since the success: %+v", p)
	}
}

func TestPauseAfterFailuresSetsTheCountAndZeroTurnsPausingOff(t *testing.T) {
	h := newHarness(t, "--stub-exit", "3")
	h.limitsOf("pause_after_failures = 1")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "flaky", Prompt: "Do it.", Checkout: "repo"})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	h.runOnce(r, job.UID)
	if p := r.w.Status().Paused; len(p) != 1 || p[0].Failed != 1 {
		t.Fatalf("one failure with pause_after_failures = 1 gave %+v", p)
	}
	r.stop()

	h2 := buildHarness(t, "--stub-exit", "3")
	h2.limitsOf("pause_after_failures = 0")
	job2 := h2.kata.CreateJob(t, h2.proj, workertest.Job{Name: "flaky", Prompt: "Do it.", Checkout: "repo"})
	r2 := h2.start(worker.Hooks{})
	h2.admitting(r2)
	for range 5 {
		h2.runOnce(r2, job2.UID)
	}
	if p := r2.w.Status().Paused; len(p) != 0 {
		t.Fatalf("paused with pause_after_failures = 0: %+v", p)
	}
}

func TestARunThatTimesOutCountsAsFailed(t *testing.T) {
	h := newHarness(t, "--stub-hang")
	h.limitsOf("pause_after_failures = 1")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "stuck", Prompt: "Do it.", Timeout: 60})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	uid := h.ask(worker.Request{Op: "run", Job: job.UID}).Run.RunUID
	h.eventually("the agent to start", func() bool { return h.isRunning(r, uid) })
	h.clock.Advance(61 * time.Second)
	h.settledInKata(r, uid)
	if p := r.w.Status().Paused; len(p) != 1 {
		t.Fatalf("a timeout did not count: %+v", p)
	}
}

func TestACancelledRunDoesNotCountAsFailed(t *testing.T) {
	h := newHarness(t, "--stub-hang")
	h.limitsOf("pause_after_failures = 1")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "stuck", Prompt: "Do it."})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	uid := h.ask(worker.Request{Op: "run", Job: job.UID}).Run.RunUID
	h.eventually("the run to be abandonable", func() bool { s, ok := h.runStatus(r, uid); return ok && s.State == "running" && s.Abandonable })
	if resp := h.ask(worker.Request{Op: "abandon", Run: uid, Actor: "ann"}); !resp.OK {
		t.Fatalf("abandon answered %+v", resp)
	}
	if run := h.settledInKata(r, uid); run.Status != "cancelled" {
		t.Fatalf("run is %q, want cancelled", run.Status)
	}
	if p := r.w.Status().Paused; len(p) != 0 {
		t.Fatalf("a cancelled run paused the job: %+v", p)
	}
	if resp := h.ask(worker.Request{Op: "run", Job: job.UID}); resp.Error != "" {
		t.Fatalf("the job admits nothing after a cancel: %s", resp.Error)
	}
}

func TestAHeldRunDoesNotCountAsFailed(t *testing.T) {
	h := newHarness(t, "--stub-hang")
	h.limitsOf("pause_after_failures = 1")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "stuck", Prompt: "Do it."})
	var once sync.Once
	killed := make(chan struct{})
	hooks := worker.Hooks{AfterSave: func(point string, j *worker.Journal) error {
		if point == "session" {
			var err error
			once.Do(func() { close(killed); err = errors.New("crash") })
			return err
		}
		return nil
	}}
	r := h.start(hooks)
	h.admitting(r)
	uid := h.ask(worker.Request{Op: "run", Job: job.UID}).Run.RunUID
	<-killed
	r.stop()

	restarted := h.start(worker.Hooks{})
	h.admitting(restarted)
	if run := h.settledInKata(restarted, uid); run.Status != "unknown" {
		t.Fatalf("run is %q, want the interrupted hold", run.Status)
	}
	if p := restarted.w.Status().Paused; len(p) != 0 {
		t.Fatalf("a held run paused the job: %+v", p)
	}
}

func TestAReportReplayedAfterACrashCountsItsFailedRunOnce(t *testing.T) {
	h := newHarness(t, "--stub-exit", "3")
	h.limitsOf("pause_after_failures = 2")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "flaky", Prompt: "Do it.", Checkout: "repo"})
	uid, _ := h.killedAt("intent-comment", job)

	restarted := h.start(worker.Hooks{})
	h.admitting(restarted)
	h.settledInKata(restarted, uid)
	if p := restarted.w.Status().Paused; len(p) != 0 {
		t.Fatalf("one failed run replayed twice paused the job: %+v", p)
	}
	raw, err := os.ReadFile(h.dirs.ProjectDir(h.proj.UID) + "/failures/" + job.UID + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Failed int `json:"failed"`
	}
	if err := json.Unmarshal(raw, &saved); err != nil || saved.Failed != 1 {
		t.Fatalf("failure count file %s (err %v), want a count of 1", raw, err)
	}
}

func TestAnUnreadableFailureCountDoesNotRefuseTheJobsRuns(t *testing.T) {
	h := newHarness(t, "--stub-exit", "3")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "flaky", Prompt: "Do it.", Checkout: "repo"})
	dir := h.dirs.ProjectDir(h.proj.UID) + "/failures"
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/"+job.UID+".json", []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := h.start(worker.Hooks{})
	h.admitting(r)
	if resp := h.ask(worker.Request{Op: "run", Job: job.UID}); resp.Error != "" {
		t.Fatalf("a run was refused over an unreadable failure count: %s", resp.Error)
	}
}
