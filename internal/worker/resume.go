package worker

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RunTransition is what a resume or an abandon answers once the run's journal records it.
type RunTransition struct {
	RunUID  string
	Project string
	Job     string
	Issue   string
	// State is the run's state as saved: "resuming" after a resume, "cancelled" after an abandon.
	State string
}

// Resume continues a held run's omp session as a new headless attempt (ADR-0005 "Resume"). It refuses unless the run
// is an uncertainty hold of this worker with its whole process group proven gone, its Project's lease is held, its
// Issue is open and a slot is free. Acceptance saves the new attempt, the generation and the session id before
// anything starts; the held attempt's prompt is never sent again. by is the actor recorded for the resume.
func (w *Worker) Resume(ctx context.Context, uid, by string) (RunTransition, error) {
	if w.halted.Load() {
		return RunTransition{}, errors.New("the worker is stopping")
	}
	j, ps, err := w.heldRun(uid)
	if err != nil {
		return RunTransition{}, err
	}
	if reason := w.resumeRefusal(j); reason != "" {
		return RunTransition{}, fmt.Errorf("run %s cannot be resumed: %s", uid, reason)
	}
	if why := w.admitsNothing(ps); why != "" {
		return RunTransition{}, fmt.Errorf("run %s cannot be resumed: Project %s admits nothing: %s", uid, ps.cfg.Name, why)
	}
	issue, err := w.opt.Kata.ShowIssue(ctx, j.ProjectID, j.Issue.UID)
	if err != nil {
		return RunTransition{}, fmt.Errorf("run %s cannot be resumed: its Issue could not be read: %w", uid, err)
	}
	if !issue.Open() {
		return RunTransition{}, fmt.Errorf("run %s cannot be resumed: its Issue %s is closed; reopen it first, or abandon the run", uid, j.Issue.Ref)
	}
	w.release(ps, uid) // its process is proven gone, so a slot kept from before the hold is stale
	if !w.reserve(ps) {
		return RunTransition{}, fmt.Errorf("run %s cannot be resumed: Project %s is at capacity (%d); try again when a run ends", uid, ps.cfg.Name, ps.cfg.Capacity)
	}
	bound := false
	defer func() {
		if !bound {
			w.unreserve(ps)
		}
	}()
	w.mu.Lock()
	generation := ps.lease.Generation
	w.mu.Unlock()
	by = cmpOr(by, ps.actor)
	updated, err := w.store.Update(uid, "resume-accepted", func(j *Journal) error {
		// Checked again under the journal's lock: two requests for one hold accept one.
		if reason := w.resumeRefusal(j); reason != "" {
			return fmt.Errorf("run %s cannot be resumed: %s", uid, reason)
		}
		j.Attempt = j.attempt() + 1
		j.Resume = &ResumeRecord{Attempt: j.Attempt, SessionID: w.sessionID(j), By: by, At: w.clock.Now().UTC(), Hold: j.Outcome.Hold, Detail: j.Outcome.Summary}
		j.Generation = generation
		j.Outcome, j.Exit, j.Result, j.Intents = nil, nil, nil, nil
		j.Phase = PhaseResuming
		return nil
	})
	if err != nil {
		w.noteHook(err)
		return RunTransition{}, err
	}
	w.bind(ps, uid, j.JobUID)
	bound = true
	w.spawn(ps, uid)
	return transition(updated), nil
}

// Abandon records a held run as cancelled (ADR-0005 "Abandon"): "abandoned by <actor>" with the held reason, an
// Issue comment, and the slot and overlap gate freed. A hold from before kata saw the run is closed locally without
// writing to kata. It refuses a run that is not held, still being reported, or whose recorded process group is still present.
func (w *Worker) Abandon(ctx context.Context, uid, by string) (RunTransition, error) {
	if w.halted.Load() {
		return RunTransition{}, errors.New("the worker is stopping")
	}
	j, ps, err := w.heldRun(uid)
	if err != nil {
		return RunTransition{}, err
	}
	if reason := abandonRefusal(j); reason != "" {
		return RunTransition{}, fmt.Errorf("run %s cannot be abandoned: %s", uid, reason)
	}
	by = cmpOr(by, ps.actor)
	updated, err := w.store.Update(uid, "abandoned", func(j *Journal) error {
		if reason := abandonRefusal(j); reason != "" {
			return fmt.Errorf("run %s cannot be abandoned: %s", uid, reason)
		}
		held := j.Outcome
		j.Outcome = &Outcome{Status: StatusCancelled, Summary: fmt.Sprintf("abandoned by %s (was held: %s)", by, held.Summary), At: w.clock.Now().UTC()}
		j.Intents = nil
		j.Phase = PhaseFinished
		if held.Status == "" { // held before admission: kata never saw it, so nothing is reported
			j.Phase = PhaseSettled
		}
		return nil
	})
	if err != nil {
		w.noteHook(err)
		return RunTransition{}, err
	}
	if updated.Phase == PhaseFinished {
		w.spawn(ps, uid)
	}
	return transition(updated), nil
}

func transition(j *Journal) RunTransition {
	t := RunTransition{RunUID: j.RunUID, Project: j.ProjectName, Job: j.JobName, State: runStatus(j).State}
	if j.Issue != nil {
		t.Issue = j.Issue.Ref
	}
	return t
}

// heldRun loads a run this worker owns and the Project state serving it.
func (w *Worker) heldRun(uid string) (*Journal, *projectState, error) {
	j, err := w.store.Load(uid)
	switch {
	case errors.Is(err, ErrNoJournal) && w.store.IsQuarantined(uid):
		return nil, nil, fmt.Errorf("run %s has a damaged journal, set aside in quarantine; run `tern-kata worker doctor`", uid)
	case errors.Is(err, ErrNoJournal):
		return nil, nil, fmt.Errorf("this worker has no run %s", uid)
	case err != nil:
		return nil, nil, err
	case j.Executor != w.opt.Config.Executor:
		return nil, nil, fmt.Errorf("run %s belongs to worker %q, not %q", uid, j.Executor, w.opt.Config.Executor)
	}
	for _, ps := range w.projects {
		w.mu.Lock()
		match := ps.cfg.Name == j.ProjectName && (!ps.resolved || ps.project.UID == j.ProjectUID)
		w.mu.Unlock()
		if match {
			return j, ps, nil
		}
	}
	return nil, nil, fmt.Errorf("run %s belongs to Project %s, which this worker does not serve right now", uid, j.ProjectName)
}

// resumeRefusal is why a run cannot be resumed, empty when it can (the lease, Issue and capacity are checked by Resume).
func (w *Worker) resumeRefusal(j *Journal) string {
	switch {
	case !j.Held():
		return "it is " + runStatus(j).State + ", not held"
	case j.Outcome.Status != StatusUnknown:
		return fmt.Sprintf("it was held (%s) before kata saw it and nothing ran, so there is nothing to continue; abandon it", j.Outcome.Hold)
	case j.Phase != PhaseSettled:
		return "its hold is still being reported to kata; try again in a moment"
	case processMayBeAlive(j) && j.Launch == nil:
		return "the worker cannot prove the agent was never started, so it will not start another on this run; abandon it"
	case processMayBeAlive(j):
		return fmt.Sprintf("the agent's process group %d is still present; the worker never stops a process to make a resume possible", j.Launch.PGID)
	case w.sessionID(j) == "" && !j.activeIsCommand():
		return fmt.Sprintf("there is no omp session to continue (%s); only abandon remains", j.Outcome.Hold)
	}
	return ""
}

// abandonRefusal is why a run cannot be abandoned, empty when it can.
func abandonRefusal(j *Journal) string {
	switch {
	case !j.Held():
		return "it is " + runStatus(j).State + ", not held"
	case j.Phase != PhaseSettled:
		return "its hold is still being reported to kata; try again in a moment"
	case j.Launch != nil && processMayBeAlive(j):
		return fmt.Sprintf("the agent's process group %d is still present; stop it first", j.Launch.PGID)
	}
	return ""
}

// reopen confirms the resumed Run's running observation in kata, keeping the original start time and clearing the end,
// then lets the attempt launch. A crash before the journal is saved replays it: an identical stored observation counts as sent.
func (w *Worker) reopen(ctx context.Context, ps *projectState, j *Journal) error {
	r := j.Resume
	if r == nil {
		return errors.New("a resuming run has no resume record")
	}
	msg := fmt.Sprintf("resumed after %s (attempt %d)", r.Hold, r.Attempt)
	stored, err := w.opt.Kata.ShowRun(ctx, j.ProjectID, j.RunUID)
	if err != nil {
		return err
	}
	if !(stored.Status == StatusRunning && stored.Summary == msg) {
		stored, _, err = w.opt.Kata.Observe(ctx, j.ProjectID, w.observation(j, StatusRunning, msg, stored.Revision))
		if errors.Is(err, ErrConflict) {
			current, serr := w.opt.Kata.ShowRun(ctx, j.ProjectID, j.RunUID)
			if serr != nil || current.Status != StatusRunning || current.Summary != msg {
				return err
			}
			stored, err = current, nil
		}
		if err != nil {
			return err
		}
	}
	_, err = w.store.Update(j.RunUID, "resumed", func(j *Journal) error {
		j.Phase, j.Revision = PhaseAdmitted, stored.Revision
		return nil
	})
	return err
}

// providerHold classifies an attempt that ended with a provider failure a retry cannot fix (ADR-0005 "Quota and
// login holds"): 401 and 403 hold agent_login, 429 holds agent_quota. Any other status is an ordinary failure.
func providerHold(j *Journal) (reason, detail string) {
	r := j.Result
	if r == nil || j.Exit == nil || j.Exit.TimedOut || r.StopReason != "error" {
		return "", ""
	}
	switch r.ErrorStatus {
	case 401, 403:
		reason = HoldAgentLogin
	case 429:
		reason = HoldAgentQuota
	default:
		return "", ""
	}
	return reason, fmt.Sprintf("the provider answered %d: %s", r.ErrorStatus, cmpOr(strings.TrimSpace(r.ErrorText), "no message"))
}

// finishedItsTurn reports whether the agent's saved output shows a terminal agent_end: the turn ended, so a hold
// with no exit status is a missing receipt rather than an interruption.
func (w *Worker) finishedItsTurn(j *Journal) bool {
	return readOutput(filepath.Join(w.attemptDir(j), "output.jsonl")).Terminal
}

// readOutput folds a saved output.jsonl through the same parser the live run used.
func readOutput(path string) AgentResult {
	f, err := os.Open(path)
	if err != nil {
		return AgentResult{}
	}
	defer f.Close()
	p := &agentParser{}
	r := bufio.NewReaderSize(f, 64<<10)
	for {
		line, err := r.ReadBytes('\n')
		p.Line(line)
		if err != nil {
			break
		}
	}
	return p.Result()
}

// sessionID is the omp session a resume would continue: the one the run recorded or, when the worker stopped before
// saving it, the only session file in the run's session folder. Empty when no session file exists.
func (w *Worker) sessionID(j *Journal) string {
	dir := sessionFolder(w.attemptDir(j))
	if j.Launch != nil && j.Launch.SessionID != "" {
		if m, _ := filepath.Glob(filepath.Join(dir, "*_"+j.Launch.SessionID+".jsonl")); len(m) > 0 {
			return j.Launch.SessionID
		}
		return ""
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*_*.jsonl"))
	if len(files) != 1 {
		return ""
	}
	name := strings.TrimSuffix(filepath.Base(files[0]), ".jsonl")
	return name[strings.LastIndexByte(name, '_')+1:]
}

// keepAttemptOutput moves the finished attempt's output and stderr aside so the next attempt writes fresh files and
// the held attempt's remain readable. An existing aside copy is never replaced.
func keepAttemptOutput(runDir string, attempt int) {
	for _, name := range []string{"output.jsonl", "stderr.log", "output.log"} {
		ext := filepath.Ext(name)
		aside := filepath.Join(runDir, fmt.Sprintf("%s.attempt%d%s", strings.TrimSuffix(name, ext), attempt, ext))
		if _, err := os.Stat(aside); err == nil {
			continue
		}
		_ = os.Rename(filepath.Join(runDir, name), aside)
	}
}
