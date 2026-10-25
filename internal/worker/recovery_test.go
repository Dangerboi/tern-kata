package worker_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
	"github.com/Dangerboi/tern-kata/internal/worker"
	"github.com/Dangerboi/tern-kata/internal/worker/workertest"
)

// crash is a Hooks.AfterSave that kills the worker once, at one named journal point, and remembers the run's uid.
type crash struct {
	point  string
	once   sync.Once
	killed chan struct{}
	mu     sync.Mutex
	uid    string
	// before runs at every save, ahead of the kill check, for tests that need the world changed at a point.
	before func(point string, j *worker.Journal)
}

func newCrash(point string) *crash { return &crash{point: point, killed: make(chan struct{})} }

func (c *crash) hooks() worker.Hooks {
	return worker.Hooks{AfterSave: func(point string, j *worker.Journal) error {
		c.mu.Lock()
		c.uid = j.RunUID
		c.mu.Unlock()
		if c.before != nil {
			c.before(point, j)
		}
		var err error
		if point == c.point {
			c.once.Do(func() { close(c.killed); err = errors.New("killed by the test") })
		}
		return err
	}}
}

// wait blocks until the kill happened and returns the run that was killed.
func (c *crash) wait(t *testing.T) string {
	t.Helper()
	select {
	case <-c.killed:
	case <-time.After(15 * time.Second):
		t.Fatalf("the worker never reached journal point %q", c.point)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.uid
}

// killedAt starts a worker that dies at point while running a manual run of job, stops it, and returns the run's uid.
func (h *harness) killedAt(point string, job cronjob.Job) (uid string, c *crash) {
	h.t.Helper()
	c = newCrash(point)
	r := h.start(c.hooks())
	h.admitting(r)
	_ = h.ask(worker.Request{Op: "run", Job: job.UID}) // a kill at "prepared" refuses the request; the journal is what counts
	uid = c.wait(h.t)
	r.stop()
	return uid, c
}

func (h *harness) runStatus(r *running, uid string) (worker.RunStatus, bool) {
	for _, s := range r.w.Status().Runs {
		if s.UID == uid {
			return s, true
		}
	}
	return worker.RunStatus{}, false
}

// issueOf is the run's per-run Issue as kata holds it.
func (h *harness) issueOf(run cronjob.Run) worker.Issue {
	h.t.Helper()
	i, err := h.kata.Kata.ShowIssue(context.Background(), h.proj.ID, run.IssueUID)
	if err != nil {
		h.t.Fatal(err)
	}
	return i
}

func count(list []string, containing string) int {
	n := 0
	for _, s := range list {
		if strings.Contains(s, containing) {
			n++
		}
	}
	return n
}

// Every point the journal saves at: a worker killed there and restarted on the same state folder never sends the
// prompt again. A run the worker can prove nothing about is held with its reason; one it can finish, it finishes,
// reporting each write to kata once.
func TestAWorkerKilledAtAnyJournalPointNeverSendsThePromptAgain(t *testing.T) {
	type want struct {
		status   cronjob.RunStatus // kata's final status
		summary  string            // the start of its summary
		launches int               // agent processes started by both workers together, -1 for "at most one"
		stub     []string
	}
	done := want{status: "succeeded", summary: "done", launches: 1}
	cases := map[string]want{
		"prepared":                 {status: "succeeded", summary: "done", launches: 1},
		"issue-resolved":           done,
		"admitted":                 done,
		"launching":                {status: "unknown", summary: "launch_unproven: ", launches: 0},
		"started":                  {status: "unknown", summary: "interrupted: ", launches: -1, stub: []string{"--stub-hang"}},
		"session":                  {status: "unknown", summary: "interrupted: ", launches: 1, stub: []string{"--stub-hang"}},
		"exited":                   done,
		"finished":                 done,
		"intents-planned":          done,
		"intent-final-observation": done,
		"intent-comment":           done,
		"intent-label":             done,
		"settled":                  done,
	}
	for point, w := range cases {
		t.Run(point, func(t *testing.T) {
			h := newHarness(t, w.stub...)
			job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "review", Prompt: "Do it.", Checkout: "repo"})
			uid, _ := h.killedAt(point, job)

			restarted := h.start(worker.Hooks{})
			h.admitting(restarted)
			run := h.settled(uid)
			if run.Status != w.status || !strings.HasPrefix(run.Summary, w.summary) {
				t.Fatalf("after a kill at %s the run is %q: %q; want %s %q", point, run.Status, run.Summary, w.status, w.summary)
			}
			n := len(h.launches())
			if w.launches >= 0 && n != w.launches || w.launches < 0 && n > 1 {
				t.Fatalf("the agent was started %d times, want %d", n, w.launches)
			}
			if w.status == "unknown" {
				h.eventually("status to show the held run", func() bool {
					s, ok := h.runStatus(restarted, uid)
					return ok && s.Held && strings.HasPrefix(s.Detail, strings.TrimSuffix(w.summary, ": "))
				})
				return
			}
			// Each write the run owed went out once: one comment, one label, however often the restart replayed it.
			h.eventually("the closure check to label the Issue", func() bool { return contains(h.issueOf(run).Labels, "needs-review") })
			if i := h.issueOf(run); len(i.Comments) != 1 {
				t.Fatalf("the Issue holds %d comments, want exactly one: %q", len(i.Comments), i.Comments)
			}
			h.eventually("the run to settle", func() bool {
				s, ok := h.runStatus(restarted, uid)
				return ok && s.State == "succeeded" && s.Pending == ""
			})
		})
	}
}

// A hold the worker made is itself journaled before it is reported: killed between the two, the next start still
// reports it, and the agent that was never started stays unstarted.
func TestAHoldKilledBeforeItIsReportedIsStillReportedAfterARestart(t *testing.T) {
	h := newHarness(t)
	h.config = strings.Replace(h.config, "name = \"demo\"\n", "name = \"demo\"\nsecrets = [\"token\"]\n", 1) + "[secrets]\ntoken = { env = \"TK_NEVER_SET\" }\n"
	_ = os.WriteFile(h.dirs.ConfigPath(), []byte(h.config), 0o600)
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "needs secret", Prompt: "Do it.", Secrets: map[string]string{"STUB_SECRET": "token"}})
	uid, _ := h.killedAt("held", job)
	if _, err := h.kata.Kata.ShowRun(context.Background(), h.proj.ID, uid); err != nil {
		t.Fatalf("kata should hold the admitted run: %v", err)
	}
	restarted := h.start(worker.Hooks{})
	h.admitting(restarted)
	run := h.settled(uid)
	if run.Status != "unknown" || !strings.HasPrefix(run.Summary, "secret_unavailable: ") || len(h.launches()) != 0 {
		t.Fatalf("run is %q: %q; launches %d", run.Status, run.Summary, len(h.launches()))
	}
}

// A run held before kata ever saw it writes nothing to kata, survives a restart as held, and abandoning it closes it locally.
func TestAHoldBeforeAdmissionNeedsNoKataWriteAndAbandonsLocally(t *testing.T) {
	h := newHarness(t)
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "review", Prompt: "Do it.", Checkout: "repo"})
	c := newCrash("held-before-admission")
	c.before = func(point string, j *worker.Journal) {
		if point == "prepared" { // another worker takes the lease between preparing and admitting
			h.foreignLease("desktop", h.clock.Now(), j.Generation+1)
		}
	}
	r := h.start(c.hooks())
	h.admitting(r)
	_ = h.ask(worker.Request{Op: "run", Job: job.UID})
	uid := c.wait(t)
	r.stop()

	restarted := h.start(worker.Hooks{})
	h.eventually("the held run to show", func() bool { s, ok := h.runStatus(restarted, uid); return ok && s.Hold == "admission_rejected" })
	s, _ := h.runStatus(restarted, uid)
	if s.Resumable || !s.Abandonable || !strings.Contains(s.ResumeRefusal, "before kata saw it") {
		t.Fatalf("status %+v: a pre-admission hold abandons but never resumes", s)
	}
	if resp := h.ask(worker.Request{Op: "resume", Run: uid}); resp.OK || !strings.Contains(resp.Error, "before kata saw it") {
		t.Fatalf("resume: ok=%v %q", resp.OK, resp.Error)
	}
	h.eventually("the lease to be read", func() bool { return strings.Contains(restarted.w.Status().Projects[0].Why, "another worker") })
	// Another worker still owns the Project: abandon fails at once with the owner named, like run and resume.
	if resp := h.ask(worker.Request{Op: "abandon", Run: uid, Actor: "ann"}); resp.OK || !strings.Contains(resp.Error, "another worker (desktop) owns this Project") {
		t.Fatalf("abandon while the lease is elsewhere: ok=%v %q", resp.OK, resp.Error)
	}
	h.clock.Advance(11 * time.Minute) // its lease expires and this worker takes the Project
	h.admitting(restarted)
	if resp := h.ask(worker.Request{Op: "abandon", Run: uid, Actor: "ann"}); !resp.OK || resp.Transition.State != "cancelled" {
		t.Fatalf("abandon: %+v", resp)
	}
	if _, err := h.kata.Kata.ShowRun(context.Background(), h.proj.ID, uid); !errors.Is(err, worker.ErrNotFound) {
		t.Fatalf("kata holds evidence of a run it never saw: %v", err)
	}
	if got, _ := h.runStatus(restarted, uid); got.Held || got.State != "cancelled" || len(h.launches()) != 0 {
		t.Fatalf("after abandon: %+v, launches %d", got, len(h.launches()))
	}
}

// The held run is recorded in kata, shown with its reason, its session and the agent's last words over the control
// socket, resumed into the same omp session with a prompt that warns of partial work, and ends recorded by that attempt.
func TestResumeContinuesTheOmpSessionAndRecordsTheOutcome(t *testing.T) {
	h := newHarness(t, "--stub-hang-first", "--stub-text", "All checked.")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "review", Prompt: "Review the open pull requests.", Checkout: "repo"})
	uid, _ := h.killedAt("session", job)

	restarted := h.start(worker.Hooks{})
	h.admitting(restarted)
	held := h.settled(uid)
	if held.Status != "unknown" || !strings.HasPrefix(held.Summary, "interrupted: ") {
		t.Fatalf("held run is %q: %q", held.Status, held.Summary)
	}
	h.eventually("the hold to be resumable", func() bool { s, ok := h.runStatus(restarted, uid); return ok && s.Resumable })
	resp := h.ask(worker.Request{Op: "run-detail", Run: uid})
	if !resp.OK || resp.Detail == nil {
		t.Fatalf("run-detail: %+v", resp)
	}
	d := resp.Detail
	if !d.Held || d.Hold != "interrupted" || !d.Resumable || !d.Abandonable || len(d.Steps) != 1 || d.Steps[0].SessionID == "" || d.Steps[0].Kind != "prompt" || d.Steps[0].Attempt != 1 ||
		d.SessionID != d.Steps[0].SessionID || filepath.Base(d.SessionDir) != "session" || d.Cwd == "" {
		t.Fatalf("detail %+v", d)
	}

	resp = h.ask(worker.Request{Op: "resume", Run: uid, Actor: "ann"})
	if !resp.OK || resp.Transition == nil || resp.Transition.State != "resuming" {
		t.Fatalf("resume: %+v", resp)
	}
	if again := h.ask(worker.Request{Op: "resume", Run: uid}); again.OK {
		t.Fatal("a second resume of the same hold was accepted")
	}
	var run cronjob.Run
	h.eventually("kata to hold the resumed run as succeeded", func() bool {
		run, _ = h.kata.Kata.ShowRun(context.Background(), h.proj.ID, uid)
		return run.Status == "succeeded"
	})
	if run.Summary != "All checked." || run.StartedAt == nil || held.StartedAt == nil || !run.StartedAt.Equal(*held.StartedAt) || run.EndedAt == nil {
		t.Fatalf("resumed run %+v, held run started %v", run, held.StartedAt)
	}
	launches := h.launches()
	if len(launches) != 2 {
		t.Fatalf("agent launched %d times, want the original and one resume", len(launches))
	}
	first, second := launches[0], launches[1]
	if first["resume"] != "" || second["resume"] != d.Steps[0].SessionID {
		t.Fatalf("resume ids %v then %v, want none then the held session %s", first["resume"], second["resume"], d.Steps[0].SessionID)
	}
	args := strings.Join(anyStrings(second["args"]), " ")
	if !strings.Contains(args, "-p --mode json --session-dir") || !strings.Contains(args, "--resume "+d.Steps[0].SessionID) || strings.Contains(args, "--restore") {
		t.Errorf("resumed agent args %q", args)
	}
	prompt := second["prompt"].(string)
	for _, want := range []string{"Step: 1, attempt 2", "may have partly run", "Review the open pull requests.", "interrupted: "} {
		if !strings.Contains(prompt, want) {
			t.Errorf("continuation prompt lacks %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(first["prompt"].(string), "partly run") {
		t.Error("the first attempt's prompt carries the resume warning")
	}
	h.eventually("the held and the final comment", func() bool { return len(h.issueOf(run).Comments) == 2 })
	if c := h.issueOf(run).Comments; !strings.Contains(c[0], "The run is held: interrupted: ") || c[1] != "All checked." {
		t.Fatalf("comments %q", c)
	}
	if s, _ := h.runStatus(restarted, uid); s.Attempt != 2 || s.Held {
		t.Fatalf("status %+v", s)
	}
	// The held attempt's output stays readable beside the new one.
	if _, err := os.Stat(filepath.Join(h.dirs.Runs(), uid, "output.attempt1.jsonl")); err != nil {
		t.Errorf("the held attempt's output was not kept: %v", err)
	}
}

// A resume killed after it was accepted, before the agent starts, neither loses the acceptance nor starts twice.
func TestAResumeKilledAfterAcceptanceIsContinuedByTheNextStart(t *testing.T) {
	h := newHarness(t, "--stub-hang-first")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "review", Prompt: "Do it.", Checkout: "repo"})
	uid, _ := h.killedAt("session", job)

	c := newCrash("resume-accepted")
	second := h.start(c.hooks())
	h.admitting(second)
	h.settled(uid)
	h.eventually("the hold to be resumable", func() bool { s, ok := h.runStatus(second, uid); return ok && s.Resumable })
	_ = h.ask(worker.Request{Op: "resume", Run: uid})
	c.wait(t)
	second.stop()
	if n := len(h.launches()); n != 1 {
		t.Fatalf("the agent was started %d times before the resume was continued", n)
	}

	third := h.start(worker.Hooks{})
	h.admitting(third)
	h.eventually("the resumed run to succeed", func() bool {
		run, _ := h.kata.Kata.ShowRun(context.Background(), h.proj.ID, uid)
		return run.Status == "succeeded"
	})
	if n := len(h.launches()); n != 2 {
		t.Fatalf("the agent was started %d times, want the original and exactly one resume", n)
	}
	if s, _ := h.runStatus(third, uid); s.Attempt != 2 {
		t.Fatalf("attempt %d, want 2", s.Attempt)
	}
}

// Abandon records cancelled with who and why, frees the overlap gate and the slot, and comments on the Issue; resume
// of a launch nobody can vouch for is refused, and runs that are not held refuse both.
func TestAbandonRecordsCancelledAndFreesTheGate(t *testing.T) {
	h := newHarness(t)
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "review", Prompt: "Do it.", Checkout: "repo", Overlap: "forbid"})
	uid, _ := h.killedAt("launching", job)

	r := h.start(worker.Hooks{})
	h.admitting(r)
	held := h.settled(uid)
	if held.Status != "unknown" {
		t.Fatalf("held run is %q", held.Status)
	}
	h.eventually("the hold to be reported", func() bool { s, ok := h.runStatus(r, uid); return ok && s.Held && s.Abandonable })
	if resp := h.ask(worker.Request{Op: "run", Job: job.UID}); resp.OK || !strings.Contains(resp.Error, "forbids overlap and run "+uid) {
		t.Fatalf("a second run beside the held one: ok=%v %q", resp.OK, resp.Error)
	}
	if resp := h.ask(worker.Request{Op: "resume", Run: uid}); resp.OK || !strings.Contains(resp.Error, "cannot prove the agent was never started") {
		t.Fatalf("resume of an unproven launch: ok=%v %q", resp.OK, resp.Error)
	}
	if st := r.w.Status().Projects[0]; st.InUse != 1 {
		t.Fatalf("a launch that may exist holds a slot: in use %d", st.InUse)
	}

	resp := h.ask(worker.Request{Op: "abandon", Run: uid, Actor: "ann"})
	if !resp.OK || resp.Transition == nil {
		t.Fatalf("abandon: %+v", resp)
	}
	var run cronjob.Run
	h.eventually("kata to hold the run as cancelled", func() bool {
		run, _ = h.kata.Kata.ShowRun(context.Background(), h.proj.ID, uid)
		return run.Status == "cancelled"
	})
	if !strings.HasPrefix(run.Summary, "abandoned by ann (was held: launch_unproven: ") || run.EndedAt == nil {
		t.Fatalf("cancelled run %+v", run)
	}
	h.eventually("the abandon comment", func() bool { return count(h.issueOf(run).Comments, "The run was abandoned: abandoned by ann") == 1 })
	h.eventually("the run to settle", func() bool { s, ok := h.runStatus(r, uid); return ok && s.State == "cancelled" && s.Pending == "" })
	if resp := h.ask(worker.Request{Op: "abandon", Run: uid}); resp.OK || !strings.Contains(resp.Error, "cancelled, not held") {
		t.Fatalf("abandoning twice: ok=%v %q", resp.OK, resp.Error)
	}
	if resp := h.ask(worker.Request{Op: "resume", Run: uid}); resp.OK || !strings.Contains(resp.Error, "cancelled, not held") {
		t.Fatalf("resuming a cancelled run: ok=%v %q", resp.OK, resp.Error)
	}
	h.eventually("the gate and slot to be free", func() bool { return r.w.Status().Projects[0].InUse == 0 })
	next := h.ask(worker.Request{Op: "run", Job: job.UID})
	if !next.OK {
		t.Fatalf("after the abandon a new run is refused: %q", next.Error)
	}
	if h.settled(next.Run.RunUID).Status != "succeeded" {
		t.Fatal("the new run did not succeed")
	}
	if resp := h.ask(worker.Request{Op: "abandon", Run: next.Run.RunUID}); resp.OK || !strings.Contains(resp.Error, "not held") {
		t.Fatalf("abandoning a finished run: ok=%v %q", resp.OK, resp.Error)
	}
	if resp := h.ask(worker.Request{Op: "resume", Run: "01M5J9AAAAAAAAAAAAAAAAAAAA"}); resp.OK || !strings.Contains(resp.Error, "this worker has no run") {
		t.Fatalf("resuming an unknown run: ok=%v %q", resp.OK, resp.Error)
	}
}

// A turn that ended with its process gone and no exit status saved is a missing receipt, not an interruption; the
// detail names the agent's last words so the person can judge it.
func TestAFinishedTurnWithNoSavedExitIsHeldAsNoReceipt(t *testing.T) {
	h := newHarness(t, "--stub-linger", "--stub-text", "Everything merged.")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "review", Prompt: "Do it.", Checkout: "repo"})
	c := newCrash("session")
	r := h.start(c.hooks())
	h.admitting(r)
	_ = h.ask(worker.Request{Op: "run", Job: job.UID})
	uid := c.wait(t)
	h.eventually("the agent to finish its turn", func() bool {
		raw, _ := os.ReadFile(filepath.Join(h.dirs.Runs(), uid, "output.jsonl"))
		return strings.Contains(string(raw), `"agent_end"`)
	})
	r.stop()

	restarted := h.start(worker.Hooks{})
	h.admitting(restarted)
	run := h.settled(uid)
	if run.Status != "unknown" || !strings.HasPrefix(run.Summary, "no_receipt: ") {
		t.Fatalf("run is %q: %q", run.Status, run.Summary)
	}
	h.eventually("the hold to be resumable", func() bool { s, ok := h.runStatus(restarted, uid); return ok && s.Resumable })
	resp := h.ask(worker.Request{Op: "run-detail", Run: uid})
	if !resp.OK || resp.Detail.OutputTail != "Everything merged." || resp.Detail.Steps[0].ExitCode != nil {
		t.Fatalf("detail %+v", resp.Detail)
	}
	if !strings.Contains(restarted.w.Status().String(), "tern-kata worker resume "+uid) {
		t.Fatalf("status lacks the resume command:\n%s", restarted.w.Status())
	}
}

// Login and quota failures are holds a person lifts, not failures to retry; any other provider error is just failed.
func TestProviderFailuresHoldForLoginAndQuotaAndResumeHoldsAgainAfterTheSameFailure(t *testing.T) {
	for status, want := range map[string]string{"401": "agent_login", "403": "agent_login", "429": "agent_quota", "500": ""} {
		t.Run(status, func(t *testing.T) {
			h := newHarness(t, "--stub-error-status", status)
			job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "review", Prompt: "Do it.", Checkout: "repo"})
			r := h.start(worker.Hooks{})
			h.admitting(r)
			uid := h.ask(worker.Request{Op: "run", Job: job.UID}).Run.RunUID
			run := h.settled(uid)
			if want == "" {
				if run.Status != "failed" {
					t.Fatalf("a %s is %q: %q, want failed", status, run.Status, run.Summary)
				}
				return
			}
			if run.Status != "unknown" || !strings.HasPrefix(run.Summary, want+": the provider answered "+status) {
				t.Fatalf("run is %q: %q", run.Status, run.Summary)
			}
			h.eventually("the hold to be resumable", func() bool { s, ok := h.runStatus(r, uid); return ok && s.Resumable })
			if resp := h.ask(worker.Request{Op: "resume", Run: uid}); !resp.OK {
				t.Fatalf("resume: %q", resp.Error)
			}
			// The same failure again: held again, with its own comment (not swallowed as a replay of the first).
			h.eventually("the second hold to be reported", func() bool {
				s, ok := h.runStatus(r, uid)
				return ok && s.Attempt == 2 && s.Held && s.Resumable && count(h.issueOf(run).Comments, "The run is held") == 2
			})
			if n := len(h.launches()); n != 2 {
				t.Fatalf("agent launched %d times, want 2", n)
			}
		})
	}
}
