package worker

import (
	"bufio"
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

// RunTransition is what a resume or an abandon answers once the run's journal records it.
type RunTransition struct {
	RunUID  string
	Project string
	Job     string
	Issue   string
	// State is the run's state as saved: "resuming" after a resume, "cancelled" after an abandon of a held run,
	// "cancelling" after an abandon of a running one, "unknown" after one of a run whose local state was lost and
	// "acknowledged" after one of a quarantined run kata holds nothing running of.
	State string
}

// Resume continues a held run's omp session as a new headless attempt (ADR-0005 "Resume"). It refuses unless the run
// is an uncertainty hold of this worker with its whole process group proven gone, its Project's lease is held, its
// Issue is open and a slot is free. Acceptance saves the new attempt, the generation, the session id and a fresh
// expiry before anything starts; the held attempt's prompt is never sent again. by is the actor recorded for the
// resume. One request owns a run at a time, so two resumes of one hold accept one and the other never touches its slot.
func (w *Worker) Resume(ctx context.Context, uid, by string) (RunTransition, error) {
	if w.halted.Load() || w.isStopping() {
		return RunTransition{}, errStopping
	}
	op := w.runOps.get(uid)
	op.Lock()
	defer op.Unlock()
	j, ps, err := w.heldRun(uid)
	if err != nil {
		return RunTransition{}, err
	}
	if reason := w.life(j).mayResume(j); reason != "" {
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
	w.syncSlot(ps, j) // its process is proven gone, so a slot kept from before the hold is stale
	if !w.reserve(ps) {
		if w.isStopping() {
			return RunTransition{}, errStopping
		}
		return RunTransition{}, fmt.Errorf("run %s cannot be resumed: Project %s is at capacity (%d); try again when a run ends", uid, ps.cfg.Name, ps.cfg.Capacity)
	}
	bound := false
	defer func() {
		if !bound {
			w.unreserve(ps)
		}
	}()
	// The lease is renewed right before the agent may start (ADR-0003); the cached one can predate another machine's takeover.
	state := claimLease(ctx, w.opt.Kata, w.clock, w.opt.Config.Executor, ps.actor, j.ProjectID, false)
	if !state.Unknown { // a transient kata failure says nothing about the lease; keep what the loop knew
		w.mu.Lock()
		ps.lease = state
		w.mu.Unlock()
	}
	if !state.Held {
		return RunTransition{}, fmt.Errorf("run %s cannot be resumed: Project %s's lease is not held: %s", uid, ps.cfg.Name, state.Why())
	}
	generation := state.Generation
	by = cmp.Or(by, ps.actor)
	updated, err := w.store.Update(uid, "resume-accepted", func(j *Journal) error {
		// Checked again under the journal's lock: a request that is not this worker's own cannot slip in between.
		return w.life(j).acceptResume(j, resumeTerms{By: by, Generation: generation, At: w.clock.Now().UTC(), Tokens: w.heldTokens, SessionID: w.sessionID})
	})
	if err != nil {
		w.noteHook(err)
		return RunTransition{}, err
	}
	w.bind(ps, updated)
	bound = true
	w.spawn(ps, uid)
	return transition(updated), nil
}

// heldTokens is what the held attempt's agent reported: from its saved result, else from the output it left behind
// (a worker that stopped before saving the exit status still has its agent's last usage on disk).
func (w *Worker) heldTokens(j *Journal) (in, out *int64) {
	if r := j.Result; r != nil && (r.InputTokens != nil || r.OutputTokens != nil) {
		return r.InputTokens, r.OutputTokens
	}
	r := readOutput(filepath.Join(w.attemptDir(j), "output.jsonl"))
	return r.InputTokens, r.OutputTokens
}

// Abandon ends a run the user no longer wants (ADR-0005 "Abandon"). A held run is recorded as cancelled, "abandoned
// by <actor>" with the held reason, with an Issue comment, and its slot and overlap gate freed; a hold from before
// kata saw the run is closed locally without writing to kata. A running run is asked to stop: its process group gets
// SIGTERM, then SIGKILL after 10 s, and the run is recorded cancelled once the group is gone (ADR-0008). A run this
// machine lost (no journal, or one in quarantine) is recorded `unknown` "local state lost" in kata. It refuses a run
// that is settled, still being reported, or whose recorded process group is still present, and every Project that
// admits nothing, as run and resume do.
func (w *Worker) Abandon(ctx context.Context, uid, by string) (RunTransition, error) {
	if w.halted.Load() {
		return RunTransition{}, errors.New("the worker is stopping")
	}
	op := w.runOps.get(uid)
	op.Lock()
	defer op.Unlock()
	j, ps, err := w.heldRun(uid)
	if lost, ok := errors.AsType[*noRunError](err); ok {
		return w.abandonLost(ctx, lost, by)
	}
	if err != nil {
		return RunTransition{}, err
	}
	if why := w.admitsNothing(ps); why != "" {
		return RunTransition{}, fmt.Errorf("run %s cannot be abandoned: Project %s admits nothing: %s", uid, ps.cfg.Name, why)
	}
	by = cmp.Or(by, ps.actor)
	if reason := w.life(j).mayAbandon(j); reason != "" {
		return RunTransition{}, fmt.Errorf("run %s cannot be abandoned: %s", uid, reason)
	}
	if !j.held() {
		updated, err := w.cancelRunning(ps, uid, by)
		if err != nil {
			return RunTransition{}, err
		}
		return transition(updated), nil
	}
	updated, err := w.store.Update(uid, "abandoned", func(j *Journal) error {
		return w.life(j).abandon(j, by, w.clock.Now().UTC())
	})
	if err != nil {
		w.noteHook(err)
		return RunTransition{}, err
	}
	if updated.reportDue() {
		w.spawn(ps, uid)
	}
	return transition(updated), nil
}

func transition(j *Journal) RunTransition {
	t := RunTransition{RunUID: j.RunUID, Project: j.ProjectName, Job: j.JobName, State: j.stateName()}
	if j.Issue != nil {
		t.Issue = j.Issue.Ref
	}
	return t
}

// noRunError means this worker holds no readable journal of the run: it has none, or it was set aside in quarantine.
type noRunError struct {
	uid         string
	quarantined bool
}

func (e *noRunError) Error() string {
	if e.quarantined {
		return fmt.Sprintf("run %s has a damaged journal, set aside in quarantine; run `tern-kata worker doctor`", e.uid)
	}
	return fmt.Sprintf("this worker has no run %s", e.uid)
}

// heldRun loads a run this worker owns and the Project state serving it.
func (w *Worker) heldRun(uid string) (*Journal, *projectState, error) {
	j, err := w.store.Load(uid)
	switch {
	case errors.Is(err, ErrNoJournal):
		return nil, nil, &noRunError{uid: uid, quarantined: w.store.IsQuarantined(uid)}
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
	if !(stored.Status == cronjob.RunRunning && stored.Summary == msg) {
		stored, _, err = w.opt.Kata.Observe(ctx, j.ProjectID, w.observation(j, cronjob.RunRunning, msg, stored.Revision))
		if errors.Is(err, ErrConflict) {
			current, serr := w.opt.Kata.ShowRun(ctx, j.ProjectID, j.RunUID)
			if serr != nil || current.Status != cronjob.RunRunning || current.Summary != msg {
				return err
			}
			stored, err = current, nil
		}
		if err != nil {
			return err
		}
	}
	_, err = w.store.Update(j.RunUID, "resumed", func(j *Journal) error {
		return j.reopen(stored.Revision)
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
	return reason, fmt.Sprintf("the provider answered %d: %s", r.ErrorStatus, cmp.Or(strings.TrimSpace(r.ErrorText), "no message"))
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
	var launched string
	if j.Launch != nil {
		launched = j.Launch.SessionID
	}
	return sessionIn(sessionFolder(w.attemptDir(j)), launched)
}

// sessionIn is the session in an omp session folder: launched when its file is there, else (launched unknown) the only
// session file. Empty when none.
func sessionIn(dir, launched string) string {
	if launched != "" {
		if m, _ := filepath.Glob(filepath.Join(dir, "*_"+launched+".jsonl")); len(m) > 0 {
			return launched
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
