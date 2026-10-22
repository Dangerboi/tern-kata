package worker_test

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/worker"
	"github.com/Dangerboi/tern-kata/internal/worker/workertest"
)

type answer struct {
	resp worker.Response
	err  error
}

// askAsync sends req from its own goroutine, for tests that hold the worker mid-request.
func (h *harness) askAsync(req worker.Request) <-chan answer {
	out := make(chan answer, 1)
	go func() {
		resp, err := worker.Ask(context.Background(), h.dirs, req)
		out <- answer{resp, err}
	}()
	return out
}

func isRunRead(r *http.Request) bool {
	return r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/cron/runs/")
}

func isIssueRead(r *http.Request) bool {
	_, ref, ok := strings.Cut(r.URL.Path, "/issues/")
	return r.Method == http.MethodGet && ok && !strings.Contains(ref, "/")
}

func isJobList(r *http.Request) bool {
	return r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/cron/jobs")
}

// Run now reads the Project's jobs again before it prepares anything, so an edit the event stream has not delivered
// yet is the definition the run is frozen with.
func TestRunNowUsesTheJobsDefinitionAsKataHoldsItNow(t *testing.T) {
	h := newHarness(t)
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "review", Prompt: "Old prompt.", Checkout: "repo"})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	h.eventually("the job to be known", func() bool { return r.w.Status().Projects[0].AdapterJobs == 1 })
	h.kata.ReplaceJob(t, h.proj, job, workertest.Job{Name: "review", Prompt: "New prompt.", Checkout: "repo"})

	resp := h.ask(worker.Request{Op: "run", Job: job.UID})
	if !resp.OK {
		t.Fatalf("run: %q", resp.Error)
	}
	h.settled(resp.Run.RunUID)
	if got := h.launches(); len(got) != 1 || !strings.Contains(got[0]["prompt"].(string), "New prompt.") || strings.Contains(got[0]["prompt"].(string), "Old prompt.") {
		t.Fatalf("the run was prepared from the cached definition: %v", got)
	}
}

func TestRunNowIsRefusedWhenTheProjectsJobsCannotBeRefreshed(t *testing.T) {
	h := newHarness(t)
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "review", Prompt: "Do it.", Checkout: "repo"})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	h.eventually("the job to be known", func() bool { return r.w.Status().Projects[0].AdapterJobs == 1 })
	h.kata.Fail(isJobList)
	defer h.kata.Heal()

	resp := h.ask(worker.Request{Op: "run", Job: job.UID})
	if resp.OK || !strings.Contains(resp.Error, "refresh") {
		t.Fatalf("run with an unreadable job list: ok=%v %q", resp.OK, resp.Error)
	}
	if journals, _, _ := r.w.Store().Scan(); len(journals) != 0 || len(h.launches()) != 0 {
		t.Fatalf("a refused request left %d journals and %d launches", len(journals), len(h.launches()))
	}
}

// A notify job has no readiness requirement: its Issue being closed is no reason to refuse it.
func TestANotifyJobOnAClosedIssueStillNotifies(t *testing.T) {
	h := newHarness(t)
	issue := h.kata.Issue(t, h.proj, "Weekly")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "nudge", ExistingUID: issue.UID, Notify: &workertest.Notify{Recipient: "bob", Message: "Still due"}})
	h.kata.CloseIssue(t, h.proj, issue.UID)
	r := h.start(worker.Hooks{})
	h.admitting(r)

	resp := h.ask(worker.Request{Op: "run", Job: job.UID})
	if !resp.OK {
		t.Fatalf("run: %q", resp.Error)
	}
	if run := h.settled(resp.Run.RunUID); run.Status != "succeeded" {
		t.Fatalf("run %+v", run)
	}
	if _, message, ok := h.kata.InboxRequest(t, h.proj, issue.UID, "bob"); !ok || message != "Still due" {
		t.Fatalf("inbox request %q (present %v)", message, ok)
	}
}

// With room for two runs, two requests for a job that forbids overlap arriving together admit one: the check and
// the journal that closes the gate are one step.
func TestTwoRequestsForAJobThatForbidsOverlapAdmitOne(t *testing.T) {
	h := newHarness(t, "--stub-hang")
	h.capacity("2")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "review", Prompt: "Do it.", Checkout: "repo", Overlap: "forbid"})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	h.eventually("the job to be known", func() bool { return r.w.Status().Projects[0].AdapterJobs == 1 })

	gate := h.kata.Hold(isRunRead)
	first := h.askAsync(worker.Request{Op: "run", Job: job.UID})
	<-gate.Arrived
	second := h.askAsync(worker.Request{Op: "run", Job: job.UID})
	select {
	case <-gate.Arrived: // without the fix the second request is past the gate check too
	case <-time.After(300 * time.Millisecond):
	}
	gate.Open()
	a, b := <-first, <-second
	if a.err != nil || b.err != nil {
		t.Fatal(a.err, b.err)
	}
	if a.resp.OK == b.resp.OK {
		t.Fatalf("both answered ok=%v: %q %q", a.resp.OK, a.resp.Error, b.resp.Error)
	}
	if journals, _, _ := r.w.Store().Scan(); len(journals) != 1 {
		t.Fatalf("%d runs prepared, want 1", len(journals))
	}
}

// Two resumes of one hold arriving together accept one; the one that lost never gives back the slot the winner took.
func TestTwoResumesOfOneHoldLeaveTheWinnersSlotAlone(t *testing.T) {
	h := newHarness(t, "--stub-hang")
	h.capacity("2")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "review", Prompt: "Do it.", Checkout: "repo"})
	uid, _ := h.killedAt("session", job)
	r := h.start(worker.Hooks{})
	h.admitting(r)
	h.settled(uid)
	h.eventually("the hold to be resumable", func() bool { s, ok := h.runStatus(r, uid); return ok && s.Resumable })

	gate := h.kata.Hold(isIssueRead)
	first := h.askAsync(worker.Request{Op: "resume", Run: uid})
	<-gate.Arrived
	second := h.askAsync(worker.Request{Op: "resume", Run: uid})
	select {
	case <-gate.Arrived: // without the fix the second request is past its refusal check too
	case <-time.After(300 * time.Millisecond):
	}
	gate.Release()
	var winner, loser <-chan answer
	var done answer
	select {
	case done = <-first:
		winner, loser = first, second
	case done = <-second:
		winner, loser = second, first
	}
	_ = winner
	if done.err != nil || !done.resp.OK {
		t.Fatalf("the first resume: %v %q", done.err, done.resp.Error)
	}
	gate.Release()
	if lost := <-loser; lost.err != nil || lost.resp.OK {
		t.Fatalf("the second resume was accepted too: %+v %v", lost.resp, lost.err)
	}
	gate.Open()
	if got := r.w.Status().Projects[0].InUse; got != 1 {
		t.Fatalf("the resumed run holds %d slots, want 1", got)
	}
}

// A client that connects and says nothing must not keep the worker from stopping.
func TestAnIdleControlConnectionDoesNotDelayShutdown(t *testing.T) {
	h := newHarness(t)
	r := h.start(worker.Hooks{})
	h.admitting(r)
	conn, err := net.Dial("unix", h.dirs.Socket())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	time.Sleep(50 * time.Millisecond) // the worker has accepted it
	r.cancel()
	select {
	case <-r.ended:
	case <-time.After(5 * time.Second):
		t.Fatal("the worker was still running 5s after it was asked to stop, held by an idle connection")
	}
}

// within polls cond in real time without moving the manual clock, for tests that must not let virtual time pass.
func (h *harness) within(what string, cond func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	h.t.Fatalf("timed out waiting for %s", what)
}

func (h *harness) kataRun(uid string) worker.Run {
	run, _ := h.kata.Kata.ShowRun(context.Background(), h.proj.ID, uid)
	return run
}

// A run that is running can be abandoned: its process is stopped and kata records it cancelled, with who and why.
func TestAbandoningARunningRunStopsItAndRecordsItCancelled(t *testing.T) {
	h := newHarness(t, "--stub-hang")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "review", Prompt: "Do it.", Checkout: "repo", Overlap: "forbid"})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	uid := h.ask(worker.Request{Op: "run", Job: job.UID}).Run.RunUID
	h.eventually("the run to be running and abandonable", func() bool { s, ok := h.runStatus(r, uid); return ok && s.State == "running" && s.Abandonable })

	resp := h.ask(worker.Request{Op: "abandon", Run: uid, Actor: "ann"})
	if !resp.OK || resp.Transition == nil || resp.Transition.State != "cancelling" {
		t.Fatalf("abandon of a running run: %+v", resp)
	}
	if again := h.ask(worker.Request{Op: "abandon", Run: uid, Actor: "bob"}); again.OK {
		t.Fatalf("a second abandon of a run already being cancelled was accepted: %+v", again)
	}
	run := h.settled(uid)
	if run.Status != "cancelled" || !strings.HasPrefix(run.Summary, "abandoned by ann (was running)") || run.EndedAt == nil {
		t.Fatalf("run %+v", run)
	}
	h.eventually("the abandon comment", func() bool { return count(h.issueOf(run).Comments, "The run was abandoned: abandoned by ann") == 1 })
	h.eventually("the slot and the overlap gate to be free", func() bool { return r.w.Status().Projects[0].InUse == 0 })
	if next := h.ask(worker.Request{Op: "run", Job: job.UID}); !next.OK {
		t.Fatalf("after the cancel a new run is refused: %q", next.Error)
	}
}

func TestAbandoningARunningWorkflowSkipsItsRemainingSteps(t *testing.T) {
	h := newHarness(t, "--stub-hang")
	_, job := h.workflowJob("pipeline",
		workertest.Step{Key: "agent", Prompt: "Wait."},
		workertest.Step{Key: "ship", Command: "echo shipped >> order.txt", After: []string{"agent"}},
	)
	r := h.start(worker.Hooks{})
	h.admitting(r)
	uid := h.runWorkflow(r, job)
	h.eventually("the run to be abandonable", func() bool { s, ok := h.runStatus(r, uid); return ok && s.State == "running" && s.Abandonable })
	if resp := h.ask(worker.Request{Op: "abandon", Run: uid, Actor: "ann"}); !resp.OK {
		t.Fatalf("abandon: %q", resp.Error)
	}
	run := h.settled(uid)
	if run.Status != "cancelled" {
		t.Fatalf("run %q: %q", run.Status, run.Summary)
	}
	if got := h.lines("order.txt"); len(got) != 0 {
		t.Fatalf("the step after the cancelled one ran: %v", got)
	}
}

// A workflow's expiry runs from when its run is prepared: a run that waited longer than its timeout is out of time
// before its first step starts.
func TestAWorkflowRunsExpiryStartsAtPreparationNotAtItsFirstStep(t *testing.T) {
	h := newHarness(t, "--stub-hang")
	wf := h.kata.CreateWorkflow(t, h.proj, "slow", workertest.Step{Key: "agent", Prompt: "Wait."})
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "slow job", Workflow: wf.UID, Checkout: "repo", Timeout: 3600})
	uid, _ := h.killedAt("prepared", job)

	h.clock.Advance(2 * time.Hour)
	h.start(worker.Hooks{})
	h.within("the run to time out without any further time passing", func() bool { return h.kataRun(uid).Status == "failed" })
	if run := h.kataRun(uid); !strings.Contains(run.Summary, "timed out") {
		t.Fatalf("run %+v", run)
	}
}

// An accepted resume gets one fresh expiry from its acceptance: a resume that waited longer than the timeout before
// the agent started is out of time at once.
func TestAResumesExpiryStartsWhenItIsAccepted(t *testing.T) {
	h := newHarness(t, "--stub-hang")
	wf := h.kata.CreateWorkflow(t, h.proj, "slow", workertest.Step{Key: "agent", Prompt: "Wait."})
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "slow job", Workflow: wf.UID, Checkout: "repo", Timeout: 3600})
	uid, _ := h.killedAt("session", job)

	c := newCrash("resume-accepted")
	second := h.start(c.hooks())
	h.admitting(second)
	h.settled(uid)
	h.eventually("the hold to be resumable", func() bool { s, ok := h.runStatus(second, uid); return ok && s.Resumable })
	_ = h.ask(worker.Request{Op: "resume", Run: uid})
	c.wait(t)
	second.stop()

	h.clock.Advance(2 * time.Hour)
	h.start(worker.Hooks{})
	h.within("the resumed run to time out without any further time passing", func() bool { return h.kataRun(uid).Status == "failed" })
}

// An attempt a resume continued and the one that finished the run both count: kata's tokens are their sum.
func TestTokenUsageSumsTheAttemptsOfAResumedRun(t *testing.T) {
	h := newHarness(t, "--stub-linger-first")
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
	h.eventually("the hold to be resumable", func() bool { s, ok := h.runStatus(restarted, uid); return ok && s.Resumable })
	if resp := h.ask(worker.Request{Op: "resume", Run: uid}); !resp.OK {
		t.Fatalf("resume: %q", resp.Error)
	}
	h.eventually("the resumed run to succeed", func() bool { return h.kataRun(uid).Status == "succeeded" })
	run := h.kataRun(uid)
	if run.InputTokens == nil || *run.InputTokens != 20 || run.OutputTokens == nil || *run.OutputTokens != 4 {
		t.Fatalf("tokens %v in, %v out; the stub's two attempts used 10 and 2 each", deref(run.InputTokens), deref(run.OutputTokens))
	}
}

func deref(n *int64) any {
	if n == nil {
		return nil
	}
	return *n
}

// A step that failed and was retried used tokens in both attempts.
func TestTokenUsageSumsTheRetriesOfAWorkflowStep(t *testing.T) {
	h := newHarness(t, "--stub-fail-once-when", "FLAKY")
	_, job := h.workflowJob("recovering", workertest.Step{Key: "agent", Prompt: "FLAKY task.", Retries: 1})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	run := h.settled(h.runWorkflow(r, job))
	if run.Status != "succeeded" || run.InputTokens == nil || *run.InputTokens != 20 || run.OutputTokens == nil || *run.OutputTokens != 4 {
		t.Fatalf("run %q tokens %v in, %v out; the failed attempt and its retry used 10 and 2 each", run.Status, deref(run.InputTokens), deref(run.OutputTokens))
	}
}

// Old settled, fully reported runs are removed daily; held runs and recent ones stay.
func TestSettledRunsOlderThanTwoWeeksAreRemovedAndHeldOnesStay(t *testing.T) {
	h := newHarness(t)
	h.config += "[secrets]\ntoken = { env = \"TK_NEVER_SET\" }\n"
	_ = os.WriteFile(h.dirs.ConfigPath(), []byte(h.config), 0o600)
	h.capacity("3")
	ok := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "fine", Prompt: "Do it.", Checkout: "repo"})
	needs := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "needs secret", Prompt: "Do it.", Secrets: map[string]string{"STUB_SECRET": "token"}})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	reported := func(uid string) bool {
		s, found := h.runStatus(r, uid)
		return found && s.Pending == "" && (s.State == "succeeded" || s.Held)
	}
	old := h.ask(worker.Request{Op: "run", Job: ok.UID}).Run.RunUID
	held := h.ask(worker.Request{Op: "run", Job: needs.UID}).Run.RunUID
	h.eventually("both runs to be reported", func() bool { return reported(old) && reported(held) })

	h.clock.Advance(10 * 24 * time.Hour)
	recent := h.ask(worker.Request{Op: "run", Job: ok.UID}).Run.RunUID
	h.eventually("the recent run to be reported", func() bool { return reported(recent) })
	h.clock.Advance(5 * 24 * time.Hour) // the first run is now 15 days old, the recent one 5

	exists := func(uid string) bool { _, err := os.Stat(filepath.Join(h.dirs.Runs(), uid)); return err == nil }
	h.within("the 15-day-old run to be removed", func() bool { return !exists(old) })
	if !exists(recent) || !exists(held) {
		t.Fatalf("the recent run (%v) or the held run (%v) was removed", exists(recent), exists(held))
	}
}

// A run kata shows as running for this executor but whose folder is gone is never rerun; abandon records it unknown
// "local state lost" and comments on its Issue, and the doctor points at that.
func TestAbandoningARunWhoseLocalStateIsLostRecordsItUnknown(t *testing.T) {
	h := newHarness(t, "--stub-hang")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "review", Prompt: "Do it.", Checkout: "repo"})
	uid, _ := h.killedAt("started", job)
	if err := os.RemoveAll(filepath.Join(h.dirs.Runs(), uid)); err != nil {
		t.Fatal(err)
	}
	r := h.start(worker.Hooks{})
	h.admitting(r)

	report := worker.Doctor(context.Background(), worker.DoctorOptions{Dirs: h.dirs, Kata: h.kata.Kata, Clock: h.clock})
	if !strings.Contains(report.String(), "tern-kata worker abandon "+uid) {
		t.Fatalf("the doctor does not name the way out:\n%s", report)
	}
	resp := h.ask(worker.Request{Op: "abandon", Run: uid, Actor: "ann"})
	if !resp.OK || resp.Transition == nil || resp.Transition.State != "unknown" {
		t.Fatalf("abandon of a lost run: %+v", resp)
	}
	run := h.kataRun(uid)
	if run.Status != "unknown" || run.Summary != "local state lost" {
		t.Fatalf("kata holds %q %q", run.Status, run.Summary)
	}
	if c := h.issueOf(run).Comments; len(c) != 1 || !strings.Contains(c[0], "local state lost") {
		t.Fatalf("comments %q", c)
	}
	if again := h.ask(worker.Request{Op: "abandon", Run: uid, Actor: "ann"}); !again.OK || len(h.issueOf(run).Comments) != 1 {
		t.Fatalf("abandoning again must change nothing: %+v", again)
	}
	if after := worker.Doctor(context.Background(), worker.DoctorOptions{Dirs: h.dirs, Kata: h.kata.Kata, Clock: h.clock}); strings.Contains(after.String(), uid) {
		t.Fatalf("the doctor still lists the settled run:\n%s", after)
	}
	if n := len(h.launches()); n != 1 {
		t.Fatalf("the agent was started %d times, want only the original launch", n)
	}
}

// A damaged journal sits in quarantine, vetoing its uid. Abandon acknowledges it (and settles what kata shows); the
// folder stays, and the doctor stops calling it a problem.
func TestAbandoningAQuarantinedRunAcknowledgesItAndKeepsItsFolder(t *testing.T) {
	h := newHarness(t, "--stub-hang")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "review", Prompt: "Do it.", Checkout: "repo"})
	uid, _ := h.killedAt("started", job)
	if err := os.WriteFile(filepath.Join(h.dirs.Runs(), uid, "run.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := h.start(worker.Hooks{})
	h.admitting(r)
	h.eventually("the damaged journal to be quarantined", func() bool { return len(r.w.Status().Quarantined) == 1 })

	opts := worker.DoctorOptions{Dirs: h.dirs, Kata: h.kata.Kata, Clock: h.clock}
	if before := worker.Doctor(context.Background(), opts); before.Problems() == 0 || !strings.Contains(before.String(), "tern-kata worker abandon "+uid) {
		t.Fatalf("the doctor must flag the quarantine and name abandon:\n%s", before)
	}
	if resp := h.ask(worker.Request{Op: "abandon", Run: uid, Actor: "ann"}); !resp.OK {
		t.Fatalf("abandon of a quarantined run: %q", resp.Error)
	}
	if run := h.kataRun(uid); run.Status != "unknown" || run.Summary != "local state lost" {
		t.Fatalf("kata holds %q %q", run.Status, run.Summary)
	}
	if _, err := os.Stat(filepath.Join(h.dirs.State, "quarantine", "runs", uid)); err != nil {
		t.Fatalf("the quarantined folder must stay: %v", err)
	}
	after := worker.Doctor(context.Background(), opts)
	if after.Problems() != 0 || !strings.Contains(after.String(), "acknowledged by ann") {
		t.Fatalf("after the acknowledgement:\n%s", after)
	}
	if got := r.w.Status().Quarantined; len(got) != 0 {
		t.Fatalf("status still nags about %v", got)
	}
}
