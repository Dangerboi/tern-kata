package worker_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/worker"
	"github.com/Dangerboi/tern-kata/internal/worker/workertest"
)

// harness is the whole worker against kata in process, with a manual clock and the stub agent.
type harness struct {
	t      *testing.T
	kata   *workertest.Daemon
	proj   worker.Project
	dirs   worker.Dirs
	clock  *workertest.ManualClock
	repo   string
	log    string
	stub   string
	config string
}

// newHarness creates the Project "demo" and the files a worker needs; start launches a worker on them.
func newHarness(t *testing.T, stubArgs ...string) *harness {
	t.Helper()
	h := &harness{t: t, kata: workertest.StartKata(t), clock: workertest.NewManualClock(), repo: t.TempDir(), stub: workertest.BuildStub(t)}
	h.log = filepath.Join(t.TempDir(), "launches.jsonl")
	h.proj = h.kata.Project(t, "demo")
	args := append([]string{"--stub-log", h.log}, stubArgs...)
	quoted, _ := json.Marshal(args)
	h.config = `executor = "laptop"
actor = "robot"
[[projects]]
name = "demo"
[checkouts]
repo = ` + quote(h.repo) + `
[profiles.default]
command = ` + quote(h.stub) + `
args = ` + string(quoted) + "\n"
	h.dirs = workertest.WriteConfig(t, h.config)
	return h
}

func quote(s string) string { b, _ := json.Marshal(s); return string(b) }

// running is a started worker.
type running struct {
	t      *testing.T
	w      *worker.Worker
	cancel context.CancelFunc
	done   chan error
	ended  chan struct{} // closed when Run has returned
}

func (h *harness) start(hooks worker.Hooks, mutate ...func(*worker.Options)) *running {
	h.t.Helper()
	cfg, err := worker.LoadConfig(h.dirs.ConfigPath())
	if err != nil {
		h.t.Fatal(err)
	}
	opts := worker.Options{Config: cfg, Dirs: h.dirs, Kata: h.kata.Kata, Clock: h.clock, Actor: "fallback", Hooks: hooks}
	for _, m := range mutate {
		m(&opts)
	}
	w, err := worker.New(opts)
	if err != nil {
		h.t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &running{t: h.t, w: w, cancel: cancel, done: make(chan error, 1), ended: make(chan struct{})}
	go func() { r.done <- w.Run(ctx); close(r.ended) }()
	select {
	case <-w.Ready():
	case err := <-r.done:
		cancel()
		h.t.Fatalf("worker ended at start: %v", err)
	case <-time.After(10 * time.Second):
		h.t.Fatal("worker never became ready")
	}
	h.t.Cleanup(r.stop)
	return r
}

func (r *running) stop() {
	r.cancel()
	select {
	case <-r.ended:
	case <-time.After(10 * time.Second):
		r.t.Error("the worker did not stop within 10s")
	}
}

// eventually polls cond, advancing the manual clock a little each time so debounces and waits come due.
func (h *harness) eventually(what string, cond func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
		h.clock.Advance(300 * time.Millisecond)
	}
	h.t.Fatalf("timed out waiting for %s", what)
}

func (h *harness) admitting(r *running) {
	h.t.Helper()
	h.eventually("Project demo to admit", func() bool {
		s := r.w.Status()
		return len(s.Projects) == 1 && s.Projects[0].Admitting
	})
}

func (h *harness) ask(req worker.Request) worker.Response {
	h.t.Helper()
	resp, err := worker.Ask(context.Background(), h.dirs, req)
	if err != nil {
		h.t.Fatal(err)
	}
	return resp
}

// settled waits for kata to hold the run in a final status and returns it.
func (h *harness) settled(uid string) worker.Run {
	h.t.Helper()
	var run worker.Run
	h.eventually("run "+uid+" to settle", func() bool {
		var err error
		run, err = h.kata.Kata.ShowRun(context.Background(), h.proj.ID, uid)
		return err == nil && run.Status != "running"
	})
	return run
}

func (h *harness) launches() []map[string]any {
	raw, err := os.ReadFile(h.log)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		h.t.Fatal(err)
	}
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(line), &m) == nil {
			out = append(out, m)
		}
	}
	return out
}

func TestARunIsRecordedRunningThenSucceededWithItsPerRunIssue(t *testing.T) {
	h := newHarness(t, "--stub-text", "Reviewed 3 pull requests.")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "review", Prompt: "Review the open pull requests.", Checkout: "repo", IssueTitle: "PR review"})
	var sawRunning worker.Run
	hooks := worker.Hooks{AfterSave: func(point string, j *worker.Journal) error {
		if point == "admitted" {
			sawRunning, _ = h.kata.Kata.ShowRun(context.Background(), j.ProjectID, j.RunUID)
		}
		return nil
	}}
	r := h.start(hooks)
	h.admitting(r)

	resp := h.ask(worker.Request{Op: "run", Job: job.UID})
	if !resp.OK || resp.Run == nil {
		t.Fatalf("run request refused: %q", resp.Error)
	}
	run := h.settled(resp.Run.RunUID)

	if run.Status != "succeeded" || run.Summary != "Reviewed 3 pull requests." {
		t.Fatalf("run is %q with summary %q", run.Status, run.Summary)
	}
	if run.InputTokens == nil || *run.InputTokens != 10 || run.OutputTokens == nil || *run.OutputTokens != 2 {
		t.Fatalf("tokens %v %v, want 10 and 2", run.InputTokens, run.OutputTokens)
	}
	if run.ExecutorLabel != "laptop" || !strings.HasPrefix(run.OccurrenceKey, "manual:") {
		t.Fatalf("run labelled %q with occurrence %q", run.ExecutorLabel, run.OccurrenceKey)
	}
	if sawRunning.Status != "running" || !strings.HasPrefix(sawRunning.Summary, "admitted ") {
		t.Fatalf("before the agent started kata held %q %q, want running and its admission note", sawRunning.Status, sawRunning.Summary)
	}
	issue, err := h.kata.Kata.ShowIssue(context.Background(), h.proj.ID, run.IssueUID)
	if err != nil || !strings.HasPrefix(issue.Title, "PR review — ") || issue.Body != "job body" {
		t.Fatalf("per-run Issue %+v, %v", issue, err)
	}
	launches := h.launches()
	if len(launches) != 1 {
		t.Fatalf("agent launched %d times, want once", len(launches))
	}
	l := launches[0]
	if l["cwd"] != h.repo && !sameDir(l["cwd"].(string), h.repo) {
		t.Errorf("agent ran in %v, want the checkout %s", l["cwd"], h.repo)
	}
	prompt := l["prompt"].(string)
	for _, want := range []string{"demo#" + issue.ShortID, "Review the open pull requests.", "kata close demo#" + issue.ShortID + " --done"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt lacks %q:\n%s", want, prompt)
		}
	}
	if l["author"] != "robot" || l["stdin"] != "eof" {
		t.Errorf("agent saw author %v and stdin %v, want robot and eof", l["author"], l["stdin"])
	}
	if args := strings.Join(anyStrings(l["args"]), " "); !strings.Contains(args, "-p --mode json --session-dir") || !strings.Contains(args, "--approval-mode yolo") {
		t.Errorf("agent args %q", args)
	}
	// The agent left the Issue open, so the worker comments the result and flags it for a reader.
	h.eventually("the closure check to label the Issue", func() bool {
		i, _ := h.kata.Kata.ShowIssue(context.Background(), h.proj.ID, run.IssueUID)
		return contains(i.Labels, "needs-review")
	})
}

func TestAFailingAgentIsRecordedFailedWithTheCause(t *testing.T) {
	h := newHarness(t, "--stub-exit", "3", "--stub-text", "Could not reach the server.")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "flaky", Prompt: "Do it.", Checkout: "repo"})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	run := h.settled(h.ask(worker.Request{Op: "run", Job: job.UID}).Run.RunUID)
	if run.Status != "failed" || !strings.HasPrefix(run.Summary, "the agent exited with status 3") || !strings.Contains(run.Summary, "Could not reach the server.") {
		t.Fatalf("run is %q: %q", run.Status, run.Summary)
	}
	issue, _ := h.kata.Kata.ShowIssue(context.Background(), h.proj.ID, run.IssueUID)
	if !issue.Open() {
		t.Error("the Issue of a failed run must stay open")
	}
}

func TestAnAgentThatNeverFinishesATurnIsFailedWhateverItsExitCode(t *testing.T) {
	h := newHarness(t, "--stub-no-end")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "quiet", Prompt: "Do it."})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	run := h.settled(h.ask(worker.Request{Op: "run", Job: job.UID}).Run.RunUID)
	if run.Status != "failed" || !strings.Contains(run.Summary, "no agent_end") {
		t.Fatalf("run is %q: %q", run.Status, run.Summary)
	}
}

func TestTheSameOccurrenceIsTheSameRunBeforeAndAfterARestart(t *testing.T) {
	h := newHarness(t)
	h.kata.CreateJob(t, h.proj, workertest.Job{Name: "daily", Prompt: "Do it.", Checkout: "repo"})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	jobs, _ := h.kata.Kata.Jobs(context.Background(), h.proj.ID)
	job := jobs[0]
	occurrence := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)

	uid, created, err := r.w.RunOccurrence(context.Background(), "demo", job.UID, occurrence)
	if err != nil || !created {
		t.Fatalf("first request: %q created=%v err=%v", uid, created, err)
	}
	if want := worker.RunUID(job.UID, worker.OccurrenceKey(occurrence), occurrence); uid != want {
		t.Fatalf("uid %s, want the derived %s", uid, want)
	}
	h.settled(uid)
	again, created, err := r.w.RunOccurrence(context.Background(), "demo", job.UID, occurrence)
	if err != nil || created || again != uid {
		t.Fatalf("second request: %q created=%v err=%v; want the same run, not created", again, created, err)
	}
	r.stop()

	restarted := h.start(worker.Hooks{})
	h.admitting(restarted)
	again, created, err = restarted.w.RunOccurrence(context.Background(), "demo", job.UID, occurrence)
	if err != nil || created || again != uid {
		t.Fatalf("after a restart: %q created=%v err=%v; want the same run, not created", again, created, err)
	}
	if n := len(h.launches()); n != 1 {
		t.Fatalf("agent launched %d times for one occurrence", n)
	}
	// Local state loss never permits a replacement: kata's evidence vetoes it.
	restarted.stop()
	if err := os.RemoveAll(h.dirs.Runs()); err != nil {
		t.Fatal(err)
	}
	third := h.start(worker.Hooks{})
	h.admitting(third)
	if _, created, err := third.w.RunOccurrence(context.Background(), "demo", job.UID, occurrence); err == nil || created || !strings.Contains(err.Error(), "already holds run") {
		t.Fatalf("with the journal lost: created=%v err=%v; want kata's evidence to veto the run", created, err)
	}
	if n := len(h.launches()); n != 1 {
		t.Fatalf("agent launched %d times after the veto", n)
	}
}

func TestUnservedProjectsAndNativeJobsAreNeverRun(t *testing.T) {
	h := newHarness(t)
	other := h.kata.Project(t, "elsewhere")
	foreign := h.kata.CreateJob(t, other, workertest.Job{Name: "foreign", Prompt: "Do it."})
	parent := h.kata.Issue(t, h.proj, "Weekly")
	native := h.kata.CreateNativeJob(t, h.proj, "child maker", parent.UID)
	r := h.start(worker.Hooks{})
	h.admitting(r)

	if resp := h.ask(worker.Request{Op: "run", Job: foreign.UID}); resp.OK || !strings.Contains(resp.Error, "Project elsewhere, which this worker does not serve") {
		t.Fatalf("job of an unserved Project: ok=%v error=%q", resp.OK, resp.Error)
	}
	if resp := h.ask(worker.Request{Op: "run", Job: native.UID}); resp.OK || !strings.Contains(resp.Error, "native job") {
		t.Fatalf("native job: ok=%v error=%q", resp.OK, resp.Error)
	}
	for _, p := range []worker.Project{h.proj, other} {
		if runs, _ := h.kata.Kata.Runs(context.Background(), p.ID); len(runs) != 0 {
			t.Fatalf("kata holds %d runs in %s, want none", len(runs), p.Name)
		}
	}
	if n := len(h.launches()); n != 0 {
		t.Fatalf("agent launched %d times", n)
	}
	// The worker leaves native jobs to kata, and says so in its status.
	h.eventually("status to count the native job", func() bool { return r.w.Status().Projects[0].NativeJobs == 1 })
}

func TestASecondWorkerOnTheMachineIsRefused(t *testing.T) {
	h := newHarness(t)
	h.start(worker.Hooks{})
	cfg, _ := worker.LoadConfig(h.dirs.ConfigPath())
	second, err := worker.New(worker.Options{Config: cfg, Dirs: h.dirs, Kata: h.kata.Kata, Clock: h.clock})
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Run(context.Background()); !errors.Is(err, worker.ErrWorkerRunning) {
		t.Fatalf("second worker: %v, want ErrWorkerRunning", err)
	}
}

func (h *harness) foreignLease(owner string, heartbeat time.Time, generation int64) {
	h.t.Helper()
	p, err := h.kata.Kata.Project(context.Background(), h.proj.ID)
	if err != nil {
		h.t.Fatal(err)
	}
	value := map[string]any{"version": 1, "owner": owner, "generation": generation, "heartbeat": heartbeat.UTC()}
	if _, err := h.kata.Kata.PatchProjectMetadata(context.Background(), h.proj.ID, p.Revision, "desktop-user", map[string]any{"tern_kata.lease": value}); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) lease() (owner string, generation float64) {
	h.t.Helper()
	p, err := h.kata.Kata.Project(context.Background(), h.proj.ID)
	if err != nil {
		h.t.Fatal(err)
	}
	m, _ := p.Metadata["tern_kata.lease"].(map[string]any)
	owner, _ = m["owner"].(string)
	generation, _ = m["generation"].(float64)
	return
}

func TestAProjectWhoseLeaseIsHeldElsewhereAdmitsNothingAndSaysSo(t *testing.T) {
	h := newHarness(t)
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "review", Prompt: "Do it."})
	h.foreignLease("desktop", h.clock.Now(), 4)
	r := h.start(worker.Hooks{})
	h.eventually("the lease to be read", func() bool { return strings.Contains(r.w.Status().Projects[0].Why, "another worker") })

	status := r.w.Status()
	if status.Projects[0].Admitting || !strings.Contains(status.String(), "admits nothing: another worker (desktop) owns this Project") {
		t.Fatalf("status:\n%s", status)
	}
	if resp := h.ask(worker.Request{Op: "run", Job: job.UID}); resp.OK || !strings.Contains(resp.Error, "another worker (desktop) owns this Project") {
		t.Fatalf("run while refused: ok=%v error=%q", resp.OK, resp.Error)
	}
	if owner, gen := h.lease(); owner != "desktop" || gen != 4 {
		t.Fatalf("the lease is %s generation %v; the refused worker must not touch it", owner, gen)
	}

	// Ten minutes without a renewal and the lease is the worker's, one generation later.
	h.clock.Advance(11 * time.Minute)
	h.admitting(r)
	if owner, gen := h.lease(); owner != "laptop" || gen != 5 {
		t.Fatalf("after expiry the lease is %s generation %v, want laptop generation 5", owner, gen)
	}
}

func TestTakeoverMovesALiveLeaseToThisWorker(t *testing.T) {
	h := newHarness(t)
	h.foreignLease("desktop", h.clock.Now(), 2)
	r := h.start(worker.Hooks{}, func(o *worker.Options) { o.Takeover = true })
	h.admitting(r)
	if owner, gen := h.lease(); owner != "laptop" || gen != 3 {
		t.Fatalf("lease %s generation %v, want laptop generation 3", owner, gen)
	}
}

func TestARunThatExceedsItsTimeoutIsStoppedAndFailed(t *testing.T) {
	h := newHarness(t, "--stub-hang")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "stuck", Prompt: "Do it.", Timeout: 60})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	uid := h.ask(worker.Request{Op: "run", Job: job.UID}).Run.RunUID
	h.eventually("the agent to start", func() bool {
		j, err := r.w.Store().Load(uid)
		return err == nil && j.Phase == worker.PhaseStarted
	})
	h.clock.Advance(61 * time.Second)
	run := h.settled(uid)
	if run.Status != "failed" || !strings.HasPrefix(run.Summary, "timed out after 1m0s") {
		t.Fatalf("run is %q: %q", run.Status, run.Summary)
	}
}

func TestARunInterruptedByACrashIsHeldAndNeverSentAgain(t *testing.T) {
	h := newHarness(t, "--stub-hang")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "stuck", Prompt: "Do it."})
	var once sync.Once
	killed := make(chan struct{})
	hooks := worker.Hooks{AfterSave: func(point string, j *worker.Journal) error {
		if point == "started" {
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
	run := h.settled(uid)
	if run.Status != "unknown" || !strings.HasPrefix(run.Summary, "interrupted: ") {
		t.Fatalf("run is %q: %q; want unknown, interrupted first", run.Status, run.Summary)
	}
	if n := len(h.launches()); n != 1 {
		t.Fatalf("agent launched %d times, want once", n)
	}
	if got := restarted.w.Status().String(); !strings.Contains(got, "held: interrupted") {
		t.Fatalf("status lacks the held run:\n%s", got)
	}
}

func anyStrings(v any) []string {
	var out []string
	for _, e := range v.([]any) {
		out = append(out, e.(string))
	}
	return out
}

func contains(list []string, s string) bool {
	for _, e := range list {
		if e == s {
			return true
		}
	}
	return false
}

func sameDir(a, b string) bool {
	ra, _ := filepath.EvalSymlinks(a)
	rb, _ := filepath.EvalSymlinks(b)
	return ra == rb
}

func TestSecretsReachTheAgentButNeitherKataNorTheJournal(t *testing.T) {
	h := newHarness(t)
	t.Setenv("TK_TEST_SECRET", "s3cret-value")
	h.config += "[secrets]\ntoken = { env = \"TK_TEST_SECRET\" }\n"
	if err := os.WriteFile(h.dirs.ConfigPath(), []byte(h.config), 0o600); err != nil {
		t.Fatal(err)
	}
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "needs secret", Prompt: "Do it.", Secrets: map[string]string{"STUB_SECRET": "token"}})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	uid := h.ask(worker.Request{Op: "run", Job: job.UID}).Run.RunUID
	run := h.settled(uid)
	if got := h.launches()[0]["secret"]; got != "s3cret-value" {
		t.Fatalf("the agent saw secret %v", got)
	}
	journal, err := os.ReadFile(filepath.Join(h.dirs.Runs(), uid, "run.json"))
	if err != nil {
		t.Fatal(err)
	}
	issue, _ := h.kata.Kata.ShowIssue(context.Background(), h.proj.ID, run.IssueUID)
	for what, text := range map[string]string{"journal": string(journal), "run summary": run.Summary, "Issue comments": strings.Join(issue.Comments, "\n")} {
		if strings.Contains(text, "s3cret-value") {
			t.Errorf("the secret leaked into the %s", what)
		}
	}
}

func TestAMissingSecretHoldsTheRunBeforeAnythingStarts(t *testing.T) {
	h := newHarness(t)
	h.config += "[secrets]\ntoken = { env = \"TK_NEVER_SET\" }\n"
	_ = os.WriteFile(h.dirs.ConfigPath(), []byte(h.config), 0o600)
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "needs secret", Prompt: "Do it.", Secrets: map[string]string{"STUB_SECRET": "token"}})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	run := h.settled(h.ask(worker.Request{Op: "run", Job: job.UID}).Run.RunUID)
	if run.Status != "unknown" || !strings.HasPrefix(run.Summary, "secret_unavailable: ") || len(h.launches()) != 0 {
		t.Fatalf("run is %q: %q; launches %d", run.Status, run.Summary, len(h.launches()))
	}
}

func TestARunOnAnExistingIssueOnlyComments(t *testing.T) {
	h := newHarness(t, "--stub-text", "Report written.")
	issue := h.kata.Issue(t, h.proj, "Weekly report")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "report", Prompt: "Write it.", ExistingUID: issue.UID})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	run := h.settled(h.ask(worker.Request{Op: "run", Job: job.UID}).Run.RunUID)
	if run.IssueUID != issue.UID || run.Status != "succeeded" {
		t.Fatalf("run %+v", run)
	}
	h.eventually("the comment", func() bool {
		i, _ := h.kata.Kata.ShowIssue(context.Background(), h.proj.ID, issue.UID)
		return len(i.Comments) == 1 && i.Comments[0] == "Report written."
	})
	if i, _ := h.kata.Kata.ShowIssue(context.Background(), h.proj.ID, issue.UID); !i.Open() || contains(i.Labels, "needs-review") {
		t.Fatalf("the existing Issue was closed or labelled: %+v", i)
	}
	if len(h.launches()) != 1 || !strings.Contains(h.launches()[0]["prompt"].(string), "comment only") {
		t.Fatal("the prompt of an existing-Issue run must say comment only")
	}
}

func TestEditingAJobReachesTheWorkerThroughTheEventStream(t *testing.T) {
	h := newHarness(t)
	r := h.start(worker.Hooks{})
	h.admitting(r)
	h.eventually("the stream to open", func() bool { return r.w.Status().Projects[0].Following })
	h.kata.CreateJob(t, h.proj, workertest.Job{Name: "late", Prompt: "Do it."})
	h.eventually("the new job to be counted", func() bool { return r.w.Status().Projects[0].AdapterJobs == 1 })
}
