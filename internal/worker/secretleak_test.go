package worker_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/worker"
	"github.com/Dangerboi/tern-kata/internal/worker/workertest"
)

// allowToken lists [secrets] token (from the environment variable TK_TEST_SECRET) for Project demo.
func (h *harness) allowToken() {
	h.t.Helper()
	h.config = strings.Replace(h.config, "name = \"demo\"\n", "name = \"demo\"\nsecrets = [\"token\"]\n", 1) + "[secrets]\ntoken = { env = \"TK_TEST_SECRET\" }\n"
	if err := os.WriteFile(h.dirs.ConfigPath(), []byte(h.config), 0o600); err != nil {
		h.t.Fatal(err)
	}
}

// savedByTheWorker is every text of the run that leaves the worker for kata or stays in its journal.
func (h *harness) savedByTheWorker(uid string) map[string]string {
	h.t.Helper()
	journal, err := os.ReadFile(filepath.Join(h.dirs.Runs(), uid, "run.json"))
	if err != nil {
		h.t.Fatal(err)
	}
	run := h.kataRun(uid)
	issue, _ := h.kata.Kata.ShowIssue(context.Background(), h.proj.ID, run.IssueUID)
	return map[string]string{"journal": string(journal), "run summary": run.Summary, "Issue comments": strings.Join(issue.Comments, "\n")}
}

func TestACommandStepThatPrintsASecretDoesNotSendItToKataOrTheJournal(t *testing.T) {
	h := buildHarness(t)
	t.Setenv("TK_TEST_SECRET", "s3cret-value")
	h.allowToken()
	wf := h.kata.CreateWorkflow(t, h.proj, "leaky", workertest.Step{Key: "oops", Command: `echo "auth: $MY_TOKEN"; exit 3`})
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "leaky job", Workflow: wf.UID, Checkout: "repo", Secrets: map[string]string{"MY_TOKEN": "token"}})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	uid := h.runWorkflow(r, job)
	run := h.settled(uid)

	if run.Status != "failed" || !strings.Contains(run.Summary, "auth: ***") {
		t.Fatalf("run is %q: %q, want a failure showing the output with the secret masked", run.Status, run.Summary)
	}
	for what, text := range h.savedByTheWorker(uid) {
		if strings.Contains(text, "s3cret-value") {
			t.Errorf("the secret leaked into the %s", what)
		}
	}
}

func TestAnAgentThatPrintsASecretDoesNotSendItToKataOrTheJournal(t *testing.T) {
	h := buildHarness(t, "--stub-text", "the token is s3cret-value", "--stub-exit", "3")
	t.Setenv("TK_TEST_SECRET", "s3cret-value")
	h.allowToken()
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "leaky", Prompt: "Do it.", Secrets: map[string]string{"STUB_SECRET": "token"}})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	uid := h.ask(worker.Request{Op: "run", Job: job.UID}).Run.RunUID
	run := h.settled(uid)

	if run.Status != "failed" || !strings.Contains(run.Summary, "the token is ***") {
		t.Fatalf("run is %q: %q, want a failure showing the message with the secret masked", run.Status, run.Summary)
	}
	for what, text := range h.savedByTheWorker(uid) {
		// The journal also holds the profile's arguments, which are where this test told the stub what to say.
		text = strings.ReplaceAll(text, `"the token is s3cret-value"`, "")
		if strings.Contains(text, "s3cret-value") {
			t.Errorf("the secret leaked into the %s", what)
		}
	}
}

func TestAShortSecretIsNotMasked(t *testing.T) {
	h := buildHarness(t)
	t.Setenv("TK_TEST_SECRET", "abc")
	h.allowToken()
	wf := h.kata.CreateWorkflow(t, h.proj, "short", workertest.Step{Key: "say", Command: `echo "abcdef"; exit 3`})
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "short job", Workflow: wf.UID, Checkout: "repo", Secrets: map[string]string{"MY_TOKEN": "token"}})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	run := h.settled(h.runWorkflow(r, job))
	if !strings.Contains(run.Summary, "abcdef") {
		t.Fatalf("summary %q: a value under 8 bytes should not be masked", run.Summary)
	}
}

func TestAnEnvSecretReachesOnlyTheJobsThatListIt(t *testing.T) {
	h := buildHarness(t)
	t.Setenv("TK_TEST_SECRET", "s3cret-value")
	// The Project does not list the secret, so no job of it may see the variable the source reads.
	h.config += "[secrets]\ntoken = { env = \"TK_TEST_SECRET\" }\n"
	if err := os.WriteFile(h.dirs.ConfigPath(), []byte(h.config), 0o600); err != nil {
		t.Fatal(err)
	}
	wf := h.kata.CreateWorkflow(t, h.proj, "peek", workertest.Step{Key: "peek", Command: `printf '%s' "$TK_TEST_SECRET" > seen.txt`})
	job := h.kata.CreateJob(t, h.proj, workertest.Job{Name: "peek job", Workflow: wf.UID, Checkout: "repo"})
	r := h.start(worker.Hooks{})
	h.admitting(r)
	if run := h.settled(h.runWorkflow(r, job)); run.Status != "succeeded" {
		t.Fatalf("run is %q: %q", run.Status, run.Summary)
	}
	if got := h.lines("seen.txt"); len(got) != 0 {
		t.Fatalf("the command step saw %q in TK_TEST_SECRET, want nothing", got)
	}
}
