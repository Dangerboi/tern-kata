package worker_test

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/worker"
	"github.com/Dangerboi/tern-kata/internal/worker/workertest"
)

func writeConfig(h *harness) error { return os.WriteFile(h.dirs.ConfigPath(), []byte(h.config), 0o600) }

func (h *harness) settledOnDisk(r *running, uid string) {
	h.t.Helper()
	h.eventually("the journal of "+uid+" to settle", func() bool {
		j, err := r.w.Store().Load(uid)
		return err == nil && j.Phase == worker.PhaseSettled
	})
	// The run's slot is freed by the worker's goroutine just after the journal settles; a request made before that is refused as at capacity.
	h.eventually("the slot of "+uid+" to be free", func() bool { return r.w.Status().Projects[0].InUse == 0 })
}

func TestAFinalObservationNeverOverwritesANewerObservationBySomeoneElse(t *testing.T) {
	h := newHarness(t)
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "review", Prompt: "Do it.", Checkout: "repo"})
	var once sync.Once
	r := h.start(worker.Hooks{AfterSave: func(point string, j *worker.Journal) error {
		if point != "finished" {
			return nil
		}
		once.Do(func() { // a person records the run as unknown while the worker has it finished
			_, _, err := h.kata.Kata.Observe(context.Background(), j.ProjectID, worker.Observation{
				RunUID: j.RunUID, JobUID: j.JobUID, DefinitionEventUID: j.DefinitionEventUID, OccurrenceKey: j.OccurrenceKey, IssueUID: j.Issue.UID,
				Executor: j.Executor, Actor: j.Actor, Status: "unknown", Message: "carol is checking this by hand", StartedAt: j.StartedAt, ExpectedRevision: j.Revision,
			})
			if err != nil {
				t.Error(err)
			}
		})
		return nil
	}})
	h.admitting(r)
	uid := h.ask(worker.Request{Op: "run", Job: job.UID}).Run.RunUID
	h.settledOnDisk(r, uid)

	run, err := h.kata.Kata.ShowRun(context.Background(), h.proj.ID, uid)
	if err != nil || run.Status != "unknown" || run.Summary != "carol is checking this by hand" {
		t.Fatalf("kata holds %q %q (%v): the worker overwrote what carol recorded", run.Status, run.Summary, err)
	}
	status, _ := h.runStatus(r, uid)
	if len(status.Inspect) != 1 || !strings.Contains(r.w.Status().String(), "needs inspection") {
		t.Fatalf("status %+v:\n%s", status, r.w.Status())
	}
	report := worker.Doctor(context.Background(), worker.DoctorOptions{Dirs: h.dirs, Kata: h.kata.Kata, Clock: h.clock, Actor: "x"})
	if !strings.Contains(report.String(), "run "+uid+" needs inspection") {
		t.Fatalf("doctor:\n%s", report)
	}
}

func TestANotifyRunSendsTheRequestItJournaledNotOneRecomputedAfterARestart(t *testing.T) {
	h := newHarness(t)
	issue := h.kata.Issue(t, h.proj, "Weekly")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "nudge", ExistingUID: issue.UID, Notify: &workertest.Notify{Recipient: "current-owner-or-author"}})
	uid, _ := h.killedAt("notify-resolved", job)
	h.kata.EditIssue(t, h.proj, issue.UID, "Weekly, renamed meanwhile", "carol")

	restarted := h.start(worker.Hooks{})
	h.admitting(restarted)
	h.settled(uid)
	_, message, ok := h.kata.InboxRequest(t, h.proj, issue.UID, "tester")
	if !ok || !strings.HasSuffix(message, ": Weekly") {
		t.Fatalf("the request to the author reads %q (present %v), want the one worked out before the restart", message, ok)
	}
	if _, _, ok := h.kata.InboxRequest(t, h.proj, issue.UID, "carol"); ok {
		t.Fatal("the request was recomputed for the new owner")
	}
}

func TestAHeldRunTellsTheIssuesOwnerAndTheRequestSurvivesACrash(t *testing.T) {
	h := newHarness(t)
	h.config += "[secrets]\ntoken = { env = \"TK_NEVER_SET\" }\n"
	if err := writeConfig(h); err != nil {
		t.Fatal(err)
	}
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "needs secret", Prompt: "Do it.", Secrets: map[string]string{"STUB_SECRET": "token"}})
	uid, _ := h.killedAt("intent-final-observation", job)
	run, err := h.kata.Kata.ShowRun(context.Background(), h.proj.ID, uid)
	if err != nil {
		t.Fatal(err)
	}
	h.kata.EditIssue(t, h.proj, run.IssueUID, "", "carol")

	restarted := h.start(worker.Hooks{})
	h.admitting(restarted)
	h.eventually("the owner to be told", func() bool {
		_, _, ok := h.kata.InboxRequest(t, h.proj, run.IssueUID, "carol")
		return ok
	})
	from, message, _ := h.kata.InboxRequest(t, h.proj, run.IssueUID, "carol")
	if from != "robot" || !strings.Contains(message, "secret_unavailable") || !strings.Contains(message, "worker resume "+uid) {
		t.Fatalf("carol's request from %q reads %q", from, message)
	}
}

func TestACommandOnlyWorkflowOnAnExistingIssueSucceedsWithoutAComment(t *testing.T) {
	h := newHarness(t)
	issue := h.kata.Issue(t, h.proj, "Nightly")
	quiet := h.kata.CreateWorkflow(t, h.proj, "quiet", workertest.Step{Key: "build", Command: "echo built"}, workertest.Step{Key: "test", Command: "echo tested", After: []string{"build"}})
	loud := h.kata.CreateWorkflow(t, h.proj, "loud", workertest.Step{Key: "build", Command: "echo built"}, workertest.Step{Key: "review", Prompt: "Review.", After: []string{"build"}})
	quietJob := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "quiet job", Workflow: quiet.UID, Checkout: "repo", ExistingUID: issue.UID})
	loudJob := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "loud job", Workflow: loud.UID, Checkout: "repo", ExistingUID: issue.UID})
	r := h.start(worker.Hooks{})
	h.admitting(r)

	quietUID := h.runWorkflow(r, quietJob)
	if run := h.settled(quietUID); run.Status != "succeeded" {
		t.Fatalf("run %q: %s", run.Status, run.Summary)
	}
	h.settledOnDisk(r, quietUID)
	if i, _ := h.kata.Kata.ShowIssue(context.Background(), h.proj.ID, issue.UID); len(i.Comments) != 0 {
		t.Fatalf("an all-command run on an existing Issue commented: %q", i.Comments)
	}

	loudUID := h.runWorkflow(r, loudJob)
	h.settled(loudUID)
	h.settledOnDisk(r, loudUID)
	if i, _ := h.kata.Kata.ShowIssue(context.Background(), h.proj.ID, issue.UID); len(i.Comments) != 1 {
		t.Fatalf("a run with a prompt step left %d comments, want one", len(i.Comments))
	}
}

func TestAProfilesOwnApprovalModeReachesTheAgent(t *testing.T) {
	own := newHarness(t, "--approval-mode", "write")
	job := own.kata.CreateJob(t, own.proj, workertest.Job{Name: "review", Prompt: "Do it.", Checkout: "repo"})
	r := own.start(worker.Hooks{})
	own.admitting(r)
	own.settled(own.ask(worker.Request{Op: "run", Job: job.UID}).Run.RunUID)
	args := strings.Join(argsOf(own.launches()[0]), " ")
	if !strings.Contains(args, "--approval-mode write") || strings.Contains(args, "yolo") {
		t.Fatalf("agent started with %s, want the profile's own approval mode", args)
	}

	def := newHarness(t, "--stub-text", "ok")
	job = def.kata.CreateJob(t, def.proj, workertest.Job{Name: "review", Prompt: "Do it.", Checkout: "repo"})
	r = def.start(worker.Hooks{})
	def.admitting(r)
	def.settled(def.ask(worker.Request{Op: "run", Job: job.UID}).Run.RunUID)
	if args := strings.Join(argsOf(def.launches()[0]), " "); !strings.Contains(args, "--approval-mode yolo") {
		t.Fatalf("agent started with %s, want headless approval by default", args)
	}
}

func argsOf(launch map[string]any) []string {
	var out []string
	list, _ := launch["args"].([]any)
	for _, a := range list {
		s, _ := a.(string)
		out = append(out, s)
	}
	return out
}

func TestStatusAndDoctorListEachJobTheWorkerSetsAsideAndWarnOfManyIssuesADay(t *testing.T) {
	h := newHarness(t)
	h.kata.CreateJob(t, h.proj, workertest.Job{Name: "fine", Prompt: "Do it."})
	h.kata.CreateJob(t, h.proj, workertest.Job{Name: "no profile", Prompt: "Do it.", Profile: "nope"})
	h.kata.CreateJob(t, h.proj, workertest.Job{Name: "no checkout", Prompt: "Do it.", Checkout: "elsewhere"})
	h.kata.CreateJob(t, h.proj, workertest.Job{Name: "too long", Prompt: "Do it.", Timeout: 24 * 3600})
	loop := h.kata.CreateWorkflow(t, h.proj, "loop", workertest.Step{Key: "make", Prompt: "Make."}, workertest.Step{Key: "check", Prompt: "Check.", After: []string{"make"}, Options: map[string]any{"on_fail": "make"}})
	h.kata.CreateJob(t, h.proj, workertest.Job{Name: "looping", Workflow: loop.UID})
	h.kata.CreateJob(t, h.proj, workertest.Job{Name: "frequent", Prompt: "Do it.", Trigger: worker.Trigger{Kind: "interval", IntervalSeconds: 3600 * 24 / 100}})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	h.eventually("the definitions to be read", func() bool { return len(r.w.Status().Projects[0].Aside) == 4 })

	want := []string{
		`Job "looping" is set aside: it runs workflow "loop", which the worker does not run: step "check": options.on_fail`,
		`Job "no checkout" is set aside: it names checkout "elsewhere"`,
		`Job "no profile" is set aside: it names profile "nope"`,
		`Job "too long" is set aside: it asks for a timeout of 24h0m0s, over max_timeout 8h0m0s`,
		`Job "frequent" creates about 100 Issues a day; consider an existing Issue`,
	}
	text := r.w.Status().String()
	for _, line := range want {
		if !strings.Contains(text, line) {
			t.Errorf("worker status lacks %q:\n%s", line, text)
		}
	}
	if strings.Contains(text, `"fine"`) {
		t.Errorf("worker status mentions the job it runs:\n%s", text)
	}
	over := h.ask(worker.Request{Op: "status"}).Status.Projects[0]
	if len(over.Aside) != 4 || len(over.Warnings) != 1 || over.Aside[2].Name != "no profile" {
		t.Errorf("the control socket answered %+v", over)
	}
	report := worker.Doctor(context.Background(), worker.DoctorOptions{Dirs: h.dirs, Kata: h.kata.Kata, Clock: h.clock, Actor: "x"}).String()
	for _, line := range want {
		line = strings.Replace(strings.Replace(line, "Job ", "job ", 1), "is set aside", "is set aside", 1)
		if !strings.Contains(report, line) {
			t.Errorf("worker doctor lacks %q:\n%s", line, report)
		}
	}
}

// A notify run is not abandoned while it is being sent: the refusal says so, and the run ends as it was sent.
func TestAbandoningANotifyRunBeforeItSendsIsRefusedAndTheRunEndsSent(t *testing.T) {
	h := newHarness(t)
	issue := h.kata.Issue(t, h.proj, "Weekly")
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "nudge", ExistingUID: issue.UID, Notify: &workertest.Notify{Recipient: "current-owner-or-author"}})
	paused, release := make(chan string, 1), make(chan struct{})
	r := h.start(worker.Hooks{AfterSave: func(point string, j *worker.Journal) error {
		if point == "notify-resolved" {
			paused <- j.RunUID
			<-release
		}
		return nil
	}})
	h.admitting(r)
	_ = h.ask(worker.Request{Op: "run", Job: job.UID})
	var uid string
	select {
	case uid = <-paused:
	case <-time.After(15 * time.Second):
		t.Fatal("the notify run never reached the journal point before its send")
	}
	resp := h.ask(worker.Request{Op: "abandon", Run: uid, Actor: "ann"})
	close(release)
	if resp.OK || !strings.Contains(resp.Error, "being sent") || !strings.Contains(resp.Error, "in a moment") {
		t.Fatalf("abandon of a notify run before its send: %+v", resp)
	}
	h.settled(uid)
	run, err := h.kata.Kata.ShowRun(context.Background(), h.proj.ID, uid)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "succeeded" {
		t.Fatalf("the run ended %q (%s), want succeeded as sent", run.Status, run.Summary)
	}
	if _, _, ok := h.kata.InboxRequest(t, h.proj, issue.UID, "tester"); !ok {
		t.Fatal("the inbox request was not delivered")
	}
}
