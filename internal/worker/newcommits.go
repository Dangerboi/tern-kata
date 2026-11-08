package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

// The New commits condition (ADR-0016). The worker runs git in the job's checkout. Per job it keeps the commit the job's
// last succeeded run started at, in commits/<job uid>.json beside the schedule and failure-count folders. A run notes
// its start commit in its journal when it is prepared; the commit becomes the job's saved commit when the run is
// reported succeeded, whoever asked for it. A failed, cancelled or held run leaves it, and so does a skip.

const (
	// gitTimeout bounds one git command and, as the deadline of a whole gate check, all the commands of that check, so a
	// stuck checkout holds up its Project's admission for at most this long.
	gitTimeout = 30 * time.Second
	// maxPromptCommits, maxCommitSubject and maxCommitListBytes bound the commit list handed to the agent.
	maxPromptCommits   = 30
	maxCommitSubject   = 200
	maxCommitListBytes = 8 << 10
)

// commitName is a full commit id; the saved commit is read back only in this shape, since it reaches git's arguments.
var commitName = regexp.MustCompile(`^[0-9a-f]{40,64}$`)

// savedCommit is a job's commits/<job uid>.json. RunUID is empty for the tip a first occurrence recorded. StartedAt orders
// the writers: a run that started before the saved commit's run never moves it back.
type savedCommit struct {
	Version   int       `json:"version"`
	Commit    string    `json:"commit"`
	RunUID    string    `json:"run_uid,omitempty"`
	StartedAt time.Time `json:"started_at"`
}

func (w *Worker) commitPath(projectUID, jobUID string) string {
	return filepath.Join(w.opt.Dirs.ProjectDir(projectUID), "commits", jobUID+".json")
}

// loadCommit reads the job's saved commit; ok is false when it has none yet.
func (w *Worker) loadCommit(projectUID, jobUID string) (c savedCommit, ok bool, err error) {
	raw, err := readFile(w.commitPath(projectUID, jobUID))
	if errors.Is(err, fs.ErrNotExist) {
		return savedCommit{}, false, nil
	}
	if err != nil {
		return savedCommit{}, false, err
	}
	if json.Unmarshal(raw, &c) != nil || c.Version != 1 || !commitName.MatchString(c.Commit) {
		return savedCommit{}, false, fmt.Errorf("the saved commit of job %s is unreadable (remove %s to start over)", jobUID, w.commitPath(projectUID, jobUID))
	}
	return c, true, nil
}

func (w *Worker) saveCommit(projectUID, jobUID string, c savedCommit) error {
	c.Version = 1
	raw, _ := json.Marshal(c)
	path := w.commitPath(projectUID, jobUID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return replaceFile(path, raw)
}

// noteCommit moves the job's saved commit to the commit a succeeded run started at. It is idempotent per run and keeps
// the later run's commit when runs finish out of order, so the report that replays after a crash changes nothing.
func (w *Worker) noteCommit(j *Journal) {
	if j.StartCommit == "" || j.Outcome == nil || j.Outcome.Status != cronjob.RunSucceeded || j.held() {
		return
	}
	lock := w.commits.get(j.ProjectUID + "/" + j.JobUID)
	lock.Lock()
	defer lock.Unlock()
	saved, ok, err := w.loadCommit(j.ProjectUID, j.JobUID)
	if err != nil {
		w.opt.Log("%v; it is replaced by the commit of run %s", err, j.RunUID)
	} else if ok && saved.RunUID != "" && saved.StartedAt.After(j.StartedAt) {
		return
	}
	if err := w.saveCommit(j.ProjectUID, j.JobUID, savedCommit{Commit: j.StartCommit, RunUID: j.RunUID, StartedAt: j.StartedAt}); err != nil {
		w.opt.Log("saving the commit of job %s: %v", j.JobUID, err)
	}
}

// checkoutFor is the folder of a checkout key the Project may use, as the worker resolves a job's checkout.
func (w *Worker) checkoutFor(ps *projectState, key string) (string, bool) {
	dir, ok := w.opt.Config.Checkouts[key]
	return dir, ok && ps.cfg.allowsCheckout(key)
}

// gitError is a failed git command; Code is its exit status, -1 when it did not run to an exit.
type gitError struct {
	Code int
	Msg  string
}

func (e *gitError) Error() string { return e.Msg }

// git runs git in dir and returns its standard output.
func git(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		e := &gitError{Code: -1, Msg: fmt.Sprintf("git %s: %v", args[0], err)}
		if exit, ok := errors.AsType[*exec.ExitError](err); ok {
			e.Code = exit.ExitCode()
		}
		if msg, _, _ := strings.Cut(strings.TrimSpace(stderr.String()), "\n"); msg != "" {
			e.Msg = fmt.Sprintf("git %s: %s", args[0], msg)
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			e.Msg = fmt.Sprintf("git %s did not finish within %s", args[0], gitTimeout)
		} else if ctx.Err() != nil {
			e.Msg = fmt.Sprintf("git %s was cancelled", args[0])
		}
		return "", e
	}
	return strings.TrimSpace(stdout.String()), nil
}

// dirStat is one stat of a checkout folder; isDir is valid once done is closed.
type dirStat struct {
	done  chan struct{}
	isDir bool
}

// statsInFlight holds the checkouts with a stat that has not returned, as a hung mount never lets it. headCommit
// starts no second stat on one: later callers wait on the running stat, so a stuck checkout costs one goroutine, not
// one per check. statDir is a seam for tests.
var (
	statsMu       sync.Mutex
	statsInFlight = map[string]*dirStat{}
	statDir       = os.Stat
)

// startStat returns the running stat of dir, starting one when there is none.
func startStat(dir string) *dirStat {
	statsMu.Lock()
	defer statsMu.Unlock()
	if s, ok := statsInFlight[dir]; ok {
		return s
	}
	s := &dirStat{done: make(chan struct{})}
	statsInFlight[dir] = s
	go func() {
		info, err := statDir(dir)
		s.isDir = err == nil && info.IsDir()
		statsMu.Lock()
		delete(statsInFlight, dir)
		statsMu.Unlock()
		close(s.done)
	}()
	return s
}

// headCommit is the commit the checkout's branch is at.
func headCommit(ctx context.Context, dir string) (string, error) {
	stat := startStat(dir)
	select {
	case <-stat.done:
		if !stat.isDir {
			return "", fmt.Errorf("the checkout %s is not a directory", dir)
		}
	case <-ctx.Done():
		return "", fmt.Errorf("the checkout %s could not be read: %w", dir, ctx.Err())
	}
	tip, err := git(ctx, dir, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	if err != nil {
		return "", fmt.Errorf("cannot read the branch tip of checkout %s: %w", dir, err)
	}
	if !commitName.MatchString(tip) {
		return "", fmt.Errorf("cannot read the branch tip of checkout %s: git answered %q", dir, tip)
	}
	return tip, nil
}

// inBranch reports whether commit is the tip or one of its ancestors. A commit git does not know is not in the branch.
func inBranch(ctx context.Context, dir, commit string) (bool, error) {
	if _, err := git(ctx, dir, "cat-file", "-e", commit+"^{commit}"); err != nil {
		if g, ok := errors.AsType[*gitError](err); ok && g.Code > 0 {
			return false, nil
		}
		return false, err
	}
	if _, err := git(ctx, dir, "merge-base", "--is-ancestor", commit, "HEAD"); err != nil {
		if g, ok := errors.AsType[*gitError](err); ok && g.Code == 1 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// folderSpecs are the pathspecs of the folders, taken literally, after a `--`; none for no folders.
func folderSpecs(folders []string) []string {
	if len(folders) == 0 {
		return nil
	}
	specs := []string{"--"}
	for _, f := range folders {
		specs = append(specs, ":(literal)"+f)
	}
	return specs
}

// newCommits counts the commits in since..HEAD (under the folders, when given) and lists the newest as bounded lines of
// abbreviated id and subject.
func newCommits(ctx context.Context, dir, since string, folders []string) (count int, list string, err error) {
	specs := folderSpecs(folders)
	out, err := git(ctx, dir, append([]string{"rev-list", "--count", since + "..HEAD"}, specs...)...)
	if err != nil {
		return 0, "", err
	}
	if _, err := fmt.Sscanf(out, "%d", &count); err != nil {
		return 0, "", fmt.Errorf("git rev-list --count answered %q", out)
	}
	if count == 0 {
		return 0, "", nil
	}
	out, err = git(ctx, dir, append([]string{"log", "--format=%h %s", fmt.Sprintf("--max-count=%d", maxPromptCommits), since + "..HEAD"}, specs...)...)
	if err != nil {
		return 0, "", err
	}
	var b strings.Builder
	shown := 0
	for _, line := range strings.Split(out, "\n") {
		line = "- " + truncate(strings.Join(strings.FieldsFunc(line, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }), " "), maxCommitSubject) + "\n"
		if b.Len()+len(line) > maxCommitListBytes {
			break
		}
		b.WriteString(line)
		shown++
	}
	if shown < count {
		fmt.Fprintf(&b, "- and %d more\n", count-shown)
	}
	return count, b.String(), nil
}

// folderWords names the folders a reason is about: "" for the whole checkout, else " under docs, src".
func folderWords(folders []string) string {
	if len(folders) == 0 {
		return ""
	}
	return " under " + strings.Join(folders, ", ")
}

// commitGate is the New commits check admission makes for a scheduled occurrence: skip is why nothing new is there to
// work on, blocked why the commits cannot be counted. The first occurrence records the branch tip and skips.
func (w *Worker) commitGate(ctx context.Context, ps *projectState, jobUID, checkoutKey string, folders []string) (skip, blocked string) {
	if checkoutKey == "" {
		return "", "it asks for new commits but names no checkout (checkout_key)"
	}
	dir, ok := w.checkoutFor(ps, checkoutKey)
	if !ok {
		return "", fmt.Sprintf("it asks for new commits in checkout %q, which is not available to Project %s in worker.toml", checkoutKey, ps.cfg.Name)
	}
	ctx, cancel := context.WithTimeout(ctx, gitTimeout) // one deadline for the whole check
	defer cancel()
	tip, err := headCommit(ctx, dir)
	if err != nil {
		return "", err.Error()
	}
	lock := w.commits.get(ps.project.UID + "/" + jobUID)
	lock.Lock()
	defer lock.Unlock()
	saved, ok, err := w.loadCommit(ps.project.UID, jobUID)
	if err != nil {
		return "", err.Error()
	}
	if !ok {
		if err := w.saveCommit(ps.project.UID, jobUID, savedCommit{Commit: tip, StartedAt: w.clock.Now().UTC()}); err != nil {
			return "", fmt.Sprintf("cannot save the branch tip: %v", err)
		}
		return fmt.Sprintf("first run: waiting for new commits (the branch is at %s)", truncate(tip, 8)), ""
	}
	in, err := inBranch(ctx, dir, saved.Commit)
	if err != nil {
		return "", fmt.Sprintf("cannot tell whether %s is still in the branch: %v", truncate(saved.Commit, 8), err)
	}
	if !in {
		return "", fmt.Sprintf("commit %s, where its last succeeded run started, is no longer in the branch of checkout %s (history was rewritten, or another branch is checked out); run the job once with Run now to start again from the branch tip", truncate(saved.Commit, 8), dir)
	}
	count, _, err := newCommits(ctx, dir, saved.Commit, folders)
	if err != nil {
		return "", fmt.Sprintf("cannot count the new commits: %v", err)
	}
	if count == 0 {
		return fmt.Sprintf("no new commits%s since %s", folderWords(folders), truncate(saved.Commit, 8)), ""
	}
	return "", ""
}

// commitContext is what a run of a job with the New commits condition notes when it is prepared: the commit it starts at
// and, for a scheduled run, the new commits since the job's saved commit for the agent's prompt. A manual run is not
// checked, so a commit that cannot be read only leaves it without a start commit, and it moves nothing when it succeeds.
func (w *Worker) commitContext(ctx context.Context, ps *projectState, job cronjob.Job, dir string, folders []string, manual bool) (start, list string, err error) {
	if start, err = headCommit(ctx, dir); err != nil {
		if manual {
			w.opt.Log("job %q: the start commit of a manual run cannot be noted: %v", job.Name, err)
			return "", "", nil
		}
		return "", "", fmt.Errorf("job %q: %w", job.Name, err)
	}
	if manual {
		return start, "", nil
	}
	saved, ok, err := w.loadCommit(ps.project.UID, job.UID)
	if err != nil || !ok {
		return start, "", err
	}
	if _, list, err = newCommits(ctx, dir, saved.Commit, folders); err != nil {
		return "", "", fmt.Errorf("job %q: cannot list the new commits: %w", job.Name, err)
	}
	return start, list, nil
}
