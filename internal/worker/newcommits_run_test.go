package worker_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
	"github.com/Dangerboi/tern-kata/internal/worker"
	"github.com/Dangerboi/tern-kata/internal/worker/workertest"
)

// The New commits condition on a real temporary git repository, through the whole worker: the clock fires the job's
// occurrences, kata holds the runs, and the stub agent logs the prompt it was given.

func (h *harness) git(args ...string) string {
	h.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", h.repo}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		h.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// commit commits a change to path in the repository with the subject.
func (h *harness) commit(path, subject string) {
	h.t.Helper()
	full := filepath.Join(h.repo, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		h.t.Fatal(err)
	}
	f, err := os.OpenFile(full, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		h.t.Fatal(err)
	}
	fmt.Fprintln(f, subject)
	f.Close()
	h.git("add", "-A")
	h.git("commit", "-q", "-m", subject)
}

// commitsJob is a job on the repository that fires every hour and runs only when it has new commits.
func (h *harness) commitsJob(conditions map[string]any) cronjob.Job {
	h.t.Helper()
	h.git("init", "-q")
	h.commit("README.md", "First commit")
	return h.kata.CreateJob(h.t, h.proj, workertest.Job{Name: "review", Prompt: "Review it.", Checkout: "repo", Trigger: interval(3600), Conditions: conditions})
}

// skipsOf is how many skips the worker holds for its jobs; the reason is the latest one's.
func (h *harness) skipsOf(r *running) (count int, reason string) {
	for _, k := range h.status(r).Skipped {
		count += k.Count
		reason = k.LatestReason
	}
	return count, reason
}

func (h *harness) waitSkips(r *running, n int) string {
	h.t.Helper()
	var reason string
	h.eventually(fmt.Sprintf("%d skips", n), func() bool {
		var got int
		got, reason = h.skipsOf(r)
		return got == n
	})
	return reason
}

func (h *harness) prompts() []string {
	var out []string
	for _, l := range h.launches() {
		out = append(out, fmt.Sprint(l["prompt"]))
	}
	return out
}

var newCommitsOn = map[string]any{"new_commits": true}

func TestNewCommitsWaitsOnTheFirstOccurrenceSkipsWithoutCommitsAndStartsWithTheCommitsInThePrompt(t *testing.T) {
	h := newHarness(t)
	job := h.commitsJob(newCommitsOn)
	r := h.start(worker.Hooks{})
	h.scheduled(r, 1)

	first := h.fire(r, time.Time{})
	if reason := h.waitSkips(r, 1); !strings.Contains(reason, "first run: waiting for new commits") {
		t.Fatalf("reason %q", reason)
	}
	h.quiet(job, 0)

	second := h.fire(r, first)
	if reason := h.waitSkips(r, 2); !strings.Contains(reason, "no new commits since") {
		t.Fatalf("reason %q", reason)
	}
	h.quiet(job, 0)

	h.commit("src/parser.go", "Fix the parser")
	h.commit("docs/guide.md", "Document the parser")
	third := h.fire(r, second)
	runs := h.waitRuns(job, 1)
	h.settledInKata(r, runs[0].UID)
	prompts := h.prompts()
	if len(prompts) != 1 {
		t.Fatalf("%d agent launches, want 1", len(prompts))
	}
	for _, want := range []string{"# New commits", "Fix the parser", "Document the parser"} {
		if !strings.Contains(prompts[0], want) {
			t.Fatalf("the prompt lacks %q:\n%s", want, prompts[0])
		}
	}
	if strings.Index(prompts[0], "# New commits") > strings.Index(prompts[0], "# Task") || strings.Contains(prompts[0], "First commit") {
		t.Fatalf("the commits belong before the task, and only the new ones:\n%s", prompts[0])
	}

	h.fire(r, third) // the succeeded run moved the saved commit: the same commits do not run again
	h.waitSkips(r, 3)
	h.quiet(job, 1)
}

func TestNewCommitsUnderFoldersIgnoresCommitsThatTouchOnlyOtherFolders(t *testing.T) {
	h := newHarness(t)
	job := h.commitsJob(map[string]any{"new_commits": map[string]any{"folders": []any{"docs"}}})
	r := h.start(worker.Hooks{})
	h.scheduled(r, 1)

	first := h.fire(r, time.Time{})
	h.waitSkips(r, 1)
	h.commit("src/main.go", "Change only src")
	second := h.fire(r, first)
	if reason := h.waitSkips(r, 2); !strings.Contains(reason, "no new commits under docs since") {
		t.Fatalf("reason %q", reason)
	}
	h.quiet(job, 0)

	h.commit("docs/guide.md", "Change the guide")
	h.fire(r, second)
	runs := h.waitRuns(job, 1)
	h.settledInKata(r, runs[0].UID)
	prompt := h.prompts()[0]
	if !strings.Contains(prompt, "Change the guide") || strings.Contains(prompt, "Change only src") {
		t.Fatalf("the prompt lists the commits under docs only:\n%s", prompt)
	}
}

func TestAFailedRunLeavesTheSavedCommitAndASucceededManualRunMovesIt(t *testing.T) {
	h := newHarness(t, "--stub-exit", "3")
	job := h.commitsJob(newCommitsOn)
	r := h.start(worker.Hooks{})
	h.scheduled(r, 1)

	first := h.fire(r, time.Time{})
	h.waitSkips(r, 1)
	h.commit("a.txt", "Break the build")
	second := h.fire(r, first)
	runs := h.waitRuns(job, 1)
	if run := h.settledInKata(r, runs[0].UID); run.Status != cronjob.RunFailed {
		t.Fatalf("run %s, want failed", run.Status)
	}
	third := h.fire(r, second) // the failed run did not use the commit up
	runs = h.waitRuns(job, 2)
	if run := h.settledInKata(r, runs[1].UID); run.Status != cronjob.RunFailed {
		t.Fatalf("run %s, want failed", run.Status)
	}
	for i, prompt := range h.prompts() {
		if !strings.Contains(prompt, "Break the build") {
			t.Fatalf("prompt %d lacks the commit the failed run was meant to review:\n%s", i, prompt)
		}
	}
	r.stop()

	h.agentArgs() // the agent works again
	r = h.start(worker.Hooks{})
	h.admitting(r)
	h.commit("a.txt", "Fix the build")
	manual := h.runOnce(r, job.UID) // Run now is not checked against the condition
	if got := h.kataRun(manual); got.Status != cronjob.RunSucceeded {
		t.Fatalf("manual run %s, want succeeded", got.Status)
	}
	h.scheduled(r, 1)
	h.fire(r, third) // the succeeded manual run started at the new tip: nothing new since
	h.waitSkips(r, 2)
	h.quiet(job, 3)
}

func TestAMissingCheckoutOrRewrittenHistoryBlocksTheRunUntilARunNowSucceeds(t *testing.T) {
	h := newHarness(t)
	job := h.commitsJob(newCommitsOn)
	r := h.start(worker.Hooks{})
	h.scheduled(r, 1)

	first := h.fire(r, time.Time{})
	h.waitSkips(r, 1)
	original := h.git("rev-parse", "HEAD")
	h.commit("a.txt", "Second")
	h.runOnce(r, job.UID) // Run now: the saved commit becomes the Second commit
	h.git("reset", "-q", "--hard", original)
	h.commit("b.txt", "Rewritten")

	second := h.fire(r, first)
	blocked := func() string {
		for _, n := range h.status(r).Blocked {
			return n.Text
		}
		return ""
	}
	h.eventually("the job to be blocked", func() bool { return blocked() != "" })
	if got := blocked(); !strings.Contains(got, "no longer in the branch") || !strings.Contains(got, "Run now") {
		t.Fatalf("blocked: %q", got)
	}
	h.quiet(job, 1)

	h.runOnce(r, job.UID) // started at the rewritten branch's tip
	h.eventually("the occurrence to be settled by a skip", func() bool { n, _ := h.skipsOf(r); return n == 2 })
	if got := blocked(); got != "" {
		t.Fatalf("still blocked: %q", got)
	}

	if err := os.RemoveAll(h.repo); err != nil {
		t.Fatal(err)
	}
	h.fire(r, second)
	h.eventually("the job to be blocked by a missing checkout", func() bool { return strings.Contains(blocked(), "is not a directory") })
	h.quiet(job, 2)
}
