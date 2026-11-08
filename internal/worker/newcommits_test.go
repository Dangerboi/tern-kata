package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

// gitIn runs git in dir for a test repository and returns its output.
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// commitFileIn commits a change to path (made if it is new) with the subject and returns the commit.
func commitFileIn(t *testing.T, dir, path, subject string) string {
	t.Helper()
	full := filepath.Join(dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(full, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(f, subject)
	f.Close()
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-m", subject)
	return gitIn(t, dir, "rev-parse", "HEAD")
}

func newRepo(t *testing.T) (dir, first string) {
	t.Helper()
	dir = t.TempDir()
	gitIn(t, dir, "init", "-q")
	return dir, commitFileIn(t, dir, "README.md", "First commit")
}

func TestHeadCommitIsTheBranchTipAndAMissingCheckoutOrANonRepositoryIsAnError(t *testing.T) {
	dir, first := newRepo(t)
	got, err := headCommit(context.Background(), dir)
	if err != nil || got != first {
		t.Fatalf("tip %q, %v; want %s", got, err, first)
	}
	if _, err := headCommit(context.Background(), filepath.Join(dir, "gone")); err == nil || !strings.Contains(err.Error(), "is not a directory") {
		t.Fatalf("missing checkout: %v", err)
	}
	if _, err := headCommit(context.Background(), t.TempDir()); err == nil || !strings.Contains(err.Error(), "cannot read the branch tip") {
		t.Fatalf("not a repository: %v", err)
	}
}

func TestNewCommitsListsTheCommitsSinceAsBoundedLines(t *testing.T) {
	dir, first := newRepo(t)
	if count, list, err := newCommits(context.Background(), dir, first, nil); err != nil || count != 0 || list != "" {
		t.Fatalf("none: %d %q %v", count, list, err)
	}
	commitFileIn(t, dir, "a.txt", "Add a")
	commitFileIn(t, dir, "b.txt", "Add b\twith   odd spacing")
	count, list, err := newCommits(context.Background(), dir, first, nil)
	if err != nil || count != 2 {
		t.Fatalf("count %d, %v", count, err)
	}
	lines := strings.Split(strings.TrimSpace(list), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "- ") || !strings.HasSuffix(lines[0], " Add b with odd spacing") || !strings.HasSuffix(lines[1], " Add a") {
		t.Fatalf("list %q, want the newest first as `- <id> <subject>`", list)
	}
}

func TestNewCommitsBoundsTheListInCountAndSize(t *testing.T) {
	dir, first := newRepo(t)
	for i := range maxPromptCommits + 5 {
		commitFileIn(t, dir, "a.txt", fmt.Sprintf("Change %d", i))
	}
	count, list, err := newCommits(context.Background(), dir, first, nil)
	if err != nil || count != maxPromptCommits+5 {
		t.Fatalf("count %d, %v", count, err)
	}
	if lines := strings.Split(strings.TrimSpace(list), "\n"); len(lines) != maxPromptCommits+1 || lines[len(lines)-1] != "- and 5 more" {
		t.Fatalf("%d lines, last %q; want %d commits and `and 5 more`", len(lines), lines[len(lines)-1], maxPromptCommits)
	}

	dir, first = newRepo(t)
	long := strings.Repeat("x", 5000)
	commitFileIn(t, dir, "a.txt", long)
	_, list, err = newCommits(context.Background(), dir, first, nil)
	if err != nil || len(list) > maxCommitSubject+16 || !strings.HasPrefix(list, "- ") {
		t.Fatalf("a %d byte subject gave a %d byte line", len(long), len(list))
	}
}

func TestNewCommitsUnderFoldersCountsOnlyCommitsThatTouchThem(t *testing.T) {
	dir, first := newRepo(t)
	commitFileIn(t, dir, "src/main.go", "Change src")
	commitFileIn(t, dir, "docs/a/guide.md", "Change docs")
	commitFileIn(t, dir, "docsx/other.md", "Change a folder that only starts like docs")
	for folders, want := range map[string][]string{
		"docs":     {"Change docs"},
		"src,docs": {"Change docs", "Change src"},
		"nothing":  nil,
	} {
		count, list, err := newCommits(context.Background(), dir, first, strings.Split(folders, ","))
		if err != nil || count != len(want) {
			t.Fatalf("folders %s: count %d, %v; want %v", folders, count, err, want)
		}
		for _, subject := range want {
			if !strings.Contains(list, subject) {
				t.Fatalf("folders %s: list %q lacks %q", folders, list, subject)
			}
		}
		if strings.Contains(list, "only starts like docs") || (folders == "docs" && strings.Contains(list, "Change src")) {
			t.Fatalf("folders %s: list %q holds a commit outside them", folders, list)
		}
	}
}

func TestInBranchTellsAnAncestorFromRewrittenHistoryAndAnUnknownCommit(t *testing.T) {
	dir, first := newRepo(t)
	second := commitFileIn(t, dir, "a.txt", "Second")
	ctx := context.Background()
	for commit, want := range map[string]bool{first: true, second: true, strings.Repeat("a", 40): false} {
		if got, err := inBranch(ctx, dir, commit); err != nil || got != want {
			t.Fatalf("%s in branch: %v, %v; want %v", commit, got, err, want)
		}
	}
	gitIn(t, dir, "reset", "-q", "--hard", first)
	commitFileIn(t, dir, "b.txt", "Rewritten")
	if got, err := inBranch(ctx, dir, second); err != nil || got {
		t.Fatalf("a commit left by a reset is in the branch: %v, %v", got, err)
	}
}

func TestNoteCommitMovesTheSavedCommitOnlyForASucceededRunAndNeverBackwards(t *testing.T) {
	w := &Worker{opt: Options{Dirs: Dirs{State: t.TempDir()}, Log: func(string, ...any) {}}}
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	commit := func(c string) string { return strings.Repeat(c, 40) }
	run := func(uid, commit string, started time.Time, status cronjob.RunStatus) *Journal {
		return &Journal{RunUID: uid, ProjectUID: "P", JobUID: "J", StartCommit: commit, StartedAt: started, Outcome: &Outcome{Status: status}}
	}
	saved := func() string {
		c, ok, err := w.loadCommit("P", "J")
		if err != nil || !ok {
			return ""
		}
		return c.Commit
	}

	w.noteCommit(run("failed", commit("a"), at, cronjob.RunFailed))
	w.noteCommit(run("cancelled", commit("a"), at, cronjob.RunCancelled))
	held := run("held", commit("a"), at, cronjob.RunUnknown)
	held.Outcome.Hold = HoldExecutionError
	w.noteCommit(held)
	w.noteCommit(run("none", "", at, cronjob.RunSucceeded))
	if got := saved(); got != "" {
		t.Fatalf("saved commit %q after runs that did not succeed", got)
	}

	w.noteCommit(run("one", commit("a"), at, cronjob.RunSucceeded))
	if got := saved(); got != commit("a") {
		t.Fatalf("saved %q, want the first succeeded run's start", got)
	}
	w.noteCommit(run("two", commit("b"), at.Add(time.Hour), cronjob.RunSucceeded))
	w.noteCommit(run("one", commit("a"), at, cronjob.RunSucceeded)) // a replayed report, or a slower older run
	if got := saved(); got != commit("b") {
		t.Fatalf("saved %q, want the later run's commit kept", got)
	}
}

func TestACommitFileThatIsNotACommitIsRefusedNotPassedToGit(t *testing.T) {
	w := &Worker{opt: Options{Dirs: Dirs{State: t.TempDir()}, Log: func(string, ...any) {}}}
	if err := w.saveCommit("P", "J", savedCommit{Commit: "--output=/tmp/x", StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := w.loadCommit("P", "J"); err == nil || !strings.Contains(err.Error(), "unreadable") {
		t.Fatalf("load: %v", err)
	}
}

func TestTheGateSkipsOrBlocksOnTheNewCommitsCheckAndAdmitsWhenThereAreCommits(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	job := limitedJob(created, "skip", map[string]any{"new_commits": true})
	job.Definition.CheckoutKey = "repo"
	var gotKey string
	var gotFolders []string
	verdict := func() (string, string) { return "", "" }
	r := newConditionRig(created)
	s := r.schedule()
	s.checkCommits = func(_ context.Context, jobUID, key string, folders []string) (string, string) {
		gotKey, gotFolders = key, folders
		return verdict()
	}
	r.plannedAt(s, []cronjob.Job{job}, created, created.Add(time.Hour))

	verdict = func() (string, string) { return "no new commits since abc", "" }
	s.admitDue(context.Background())
	if r.journals.created != 0 || len(r.skips.records) != 1 || s.anyPending() {
		t.Fatalf("%d runs, skips %+v, pending %v; want a settled skip", r.journals.created, r.skips.records, s.anyPending())
	}
	if rec := r.skips.records[0]; rec.Condition != cronjob.ConditionNewCommits || rec.Reason != "no new commits since abc" || gotKey != "repo" || gotFolders != nil {
		t.Fatalf("record %+v, checkout %q, folders %v", rec, gotKey, gotFolders)
	}

	r.now = created.Add(2 * time.Hour)
	s.planAll([]cronjob.Job{job}, nil, r.now)
	logged := len(r.logs)
	verdict = func() (string, string) { return "", "git is broken" }
	for range 3 {
		s.admitDue(context.Background())
	}
	if r.journals.created != 0 || len(r.skips.records) != 1 || !s.anyPending() {
		t.Fatalf("%d runs, skips %d, pending %v; want the occurrence kept waiting", r.journals.created, len(r.skips.records), s.anyPending())
	}
	if len(r.logs) != logged+1 {
		t.Fatalf("log %q, want the problem once", r.logs[logged:])
	}

	verdict = func() (string, string) { return "", "" }
	s.admitDue(context.Background())
	if r.journals.created != 1 || s.anyPending() {
		t.Fatalf("%d runs, pending %v; want the run to start once git answers", r.journals.created, s.anyPending())
	}
}

func TestThePromptListsTheNewCommitsBeforeTheTaskAndOnlyWhenThereAreSome(t *testing.T) {
	in := PromptInput{Actor: "robot", IssueRef: "demo#1", JobName: "review", RunUID: "R", Occurrence: "2026-01-01T00:00:00Z", Task: "Review it.", Attempt: 1}
	plain, err := BuildPrompt(in)
	if err != nil || strings.Contains(plain, "New commits") {
		t.Fatalf("a prompt without commits mentions them: %v\n%s", err, plain)
	}
	in.NewCommits = "- abc1234 Fix the parser\n- def5678 Add docs\n"
	got, err := BuildPrompt(in)
	if err != nil {
		t.Fatal(err)
	}
	section := strings.Index(got, "# New commits")
	if section < 0 || section > strings.Index(got, "# Task") || !strings.Contains(got, "- abc1234 Fix the parser\n- def5678 Add docs\n") {
		t.Fatalf("prompt:\n%s", got)
	}
}

func TestASucceededRunReplacesTheTipAFirstOccurrenceSavedEvenWhenItStartedBeforeIt(t *testing.T) {
	w := &Worker{opt: Options{Dirs: Dirs{State: t.TempDir()}, Log: func(string, ...any) {}}}
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	tip, runStart := strings.Repeat("a", 40), strings.Repeat("b", 40)
	if err := w.saveCommit("P", "J", savedCommit{Commit: tip, StartedAt: at}); err != nil { // the first occurrence's note: no run
		t.Fatal(err)
	}
	w.noteCommit(&Journal{RunUID: "manual", ProjectUID: "P", JobUID: "J", StartCommit: runStart, StartedAt: at.Add(-time.Second), Outcome: &Outcome{Status: cronjob.RunSucceeded}})
	if c, ok, err := w.loadCommit("P", "J"); err != nil || !ok || c.Commit != runStart || c.RunUID != "manual" {
		t.Fatalf("saved %+v, %v; want the run's start commit", c, err)
	}
}

func TestCatchupAllGivesTheNewCommitsToOneOccurrenceAndSkipsTheRest(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	job := limitedJob(created, "all", map[string]any{"new_commits": true})
	job.Definition.CheckoutKey = "repo"
	r := newConditionRig(created)
	s := r.schedule()
	s.checkCommits = func(context.Context, string, string, []string) (string, string) { return "", "" }
	r.plannedAt(s, []cronjob.Job{job}, created, created.Add(3*time.Hour))
	s.admitDue(context.Background())
	if r.journals.created != 1 || len(r.skips.records) != 2 || s.anyPending() {
		t.Fatalf("%d runs, %d skips, pending %v; want one run and two skips", r.journals.created, len(r.skips.records), s.anyPending())
	}
	skipLogs := 0
	for _, l := range r.logs {
		if strings.Contains(l, "skipped") {
			skipLogs++
		}
	}
	if skipLogs != 1 {
		t.Fatalf("%d skip log lines, want the repeated claimed-commits skip said once: %q", skipLogs, r.logs)
	}
	want := "the new commits went to the run of " + OccurrenceKey(created.Add(time.Hour))
	for _, rec := range r.skips.records {
		if rec.Condition != cronjob.ConditionNewCommits || rec.Reason != want {
			t.Fatalf("skip %+v, want reason %q", rec, want)
		}
	}
}

func TestAGateReasonIsLoggedAgainWhenItRecursAfterTheGatePassed(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	job := limitedJob(created, "skip", map[string]any{"new_commits": true})
	job.Definition.CheckoutKey = "repo"
	r := newConditionRig(created)
	s := r.schedule()
	why := "git is broken"
	s.checkCommits = func(context.Context, string, string, []string) (string, string) { return "", why }
	r.plannedAt(s, []cronjob.Job{job}, created, created.Add(time.Hour))
	logged := len(r.logs)
	s.admitDue(context.Background())
	s.admitDue(context.Background())
	if len(r.logs) != logged+1 {
		t.Fatalf("logs %q, want the block once", r.logs[logged:])
	}
	why = ""
	s.admit = func(context.Context, string, time.Time) (string, bool, error) {
		return "", false, &busyError{msg: "full"}
	}
	s.admitDue(context.Background())
	why = "git is broken"
	s.admitDue(context.Background())
	if len(r.logs) != logged+2 {
		t.Fatalf("logs %q, want the same block logged again after the gate passed", r.logs[logged:])
	}
}

func TestACancelledContextMakesTheCommitCheckFailAtOnce(t *testing.T) {
	dir, _ := newRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if _, err := headCommit(ctx, dir); err == nil || time.Since(start) > 5*time.Second {
		t.Fatalf("err %v after %s; want a prompt failure", err, time.Since(start))
	}
}

func TestAHungCheckoutIsNotStatedAgainWhileTheFirstStatIsStuck(t *testing.T) {
	release := make(chan struct{})
	var calls atomic.Int32
	old := statDir
	statDir = func(string) (os.FileInfo, error) { calls.Add(1); <-release; return nil, errors.New("gone") }
	defer func() { statDir = old }()
	dir := "/hung/checkout"
	for range 3 {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		if _, err := headCommit(ctx, dir); err == nil || !strings.Contains(err.Error(), "could not be read") {
			t.Fatalf("check: %v", err)
		}
		cancel()
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("%d stats started, want 1", n)
	}
	close(release)
	deadline := time.Now().Add(5 * time.Second)
	for {
		statsMu.Lock()
		n := len(statsInFlight)
		statsMu.Unlock()
		if n == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the stat never cleared its mark")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestConcurrentChecksOfOneCheckoutShareASlowStat(t *testing.T) {
	dir, _ := newRepo(t)
	var calls atomic.Int32
	old := statDir
	statDir = func(d string) (os.FileInfo, error) {
		calls.Add(1)
		time.Sleep(200 * time.Millisecond)
		return os.Stat(d)
	}
	defer func() { statDir = old }()
	var wg sync.WaitGroup
	errs := make([]error, 4)
	tips := make([]string, 4)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tips[i], errs[i] = headCommit(context.Background(), dir)
		}()
	}
	wg.Wait()
	for i := range errs {
		if errs[i] != nil || tips[i] == "" || tips[i] != tips[0] {
			t.Fatalf("caller %d: %q, %v", i, tips[i], errs[i])
		}
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("%d stats started, want 1", n)
	}
}

func TestAGateCancelledByShutdownLeavesTheJobUnblockedAndLogsNothing(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	job := limitedJob(created, "skip", map[string]any{"new_commits": true})
	job.Definition.CheckoutKey = "repo"
	r := newConditionRig(created)
	s := r.schedule()
	ctx, cancel := context.WithCancel(context.Background())
	s.checkCommits = func(context.Context, string, string, []string) (string, string) {
		cancel()
		return "", "git was cancelled"
	}
	r.plannedAt(s, []cronjob.Job{job}, created, created.Add(time.Hour))
	logged := len(r.logs)
	s.admitDue(ctx)
	st := s.jobs[job.UID]
	if st.blockedBy != "" || st.gateErr != "" || len(r.logs) != logged || !s.anyPending() {
		t.Fatalf("blockedBy %q, gateErr %q, logs %q, pending %v; want nothing blocked or logged", st.blockedBy, st.gateErr, r.logs[logged:], s.anyPending())
	}
}
