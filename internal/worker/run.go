package worker

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

// noteHook halts the worker's run machinery when a test's save hook "killed" it, as a crash would: nothing after the
// named journal point runs, in this process.
func (w *Worker) noteHook(err error) {
	if err != nil && errors.Is(err, ErrKilled) {
		w.halted.Store(true)
	}
}

// spawn runs the journal's state machine in the background, once per run.
func (w *Worker) spawn(ps *projectState, uid string) {
	w.mu.Lock()
	if ps.running[uid] || w.halted.Load() {
		w.mu.Unlock()
		return
	}
	if ps.running == nil {
		ps.running = map[string]bool{}
	}
	ps.running[uid] = true
	w.mu.Unlock()
	w.runs.Add(1)
	go func() {
		defer w.runs.Done()
		err := w.advance(w.ctx, ps, uid)
		owes := w.owesSettlement(uid)
		w.mu.Lock()
		delete(ps.running, uid)
		if owes {
			if ps.owing == nil {
				ps.owing = map[string]bool{}
			}
			ps.owing[uid] = true
		} else {
			delete(ps.owing, uid)
		}
		w.mu.Unlock()
		w.finishDrain()
		if err != nil && w.ctx.Err() == nil && !w.halted.Load() {
			w.opt.Log("run %s: %v", uid, err)
		}
	}()
}

// owesSettlement is whether an accepted run that is not held still has reports or its settlement to deliver: a
// transient kata failure stopped it, and retryPending will drive it on. An idle drain waits for such a run.
func (w *Worker) owesSettlement(uid string) bool {
	j, err := w.store.Load(uid)
	return err == nil && j.owes()
}

// retryPending resumes runs a transient kata failure left half-reported or half-admitted; the Project loop calls it at least every minute.
func (w *Worker) retryPending(ps *projectState) {
	journals, _, _ := w.store.Scan()
	for _, j := range journals {
		if j.ProjectUID != ps.project.UID || j.Executor != w.opt.Config.Executor {
			continue
		}
		if !j.owes() {
			w.mu.Lock()
			delete(ps.owing, j.RunUID) // settled or held by another path since: nothing left to wait for
			w.mu.Unlock()
			w.finishDrain()
		}
		switch {
		case j.driveStep() == driveAdmit && !w.holdsSlotOf(ps, j.RunUID):
			// prepare saved this journal but has not bound its slot yet; RequestRun or the scheduler spawns it next
		case j.retryable():
			w.spawn(ps, j.RunUID)
		case j.settled() && !w.life().occupiesSlot(j):
			w.release(ps, j.RunUID) // a held run whose process has since gone no longer takes a slot
		}
	}
}

// advance drives a run from the phase its journal is in to settled. Each step saves before it acts; a step that
// cannot finish returns its error and the run stays where it is for retryPending or the next start.
func (w *Worker) advance(ctx context.Context, ps *projectState, uid string) (err error) {
	defer func() {
		w.noteHook(err)
		w.dropCancelSignal(uid)
		if j, lerr := w.store.Load(uid); lerr == nil && !w.life().occupiesSlot(j) {
			w.release(ps, uid)
		}
		w.kick(ps) // a slot or an overlap gate may have opened: queued occurrences are due another look
	}()
	for ctx.Err() == nil && !w.halted.Load() {
		j, err := w.store.Load(uid)
		if err != nil {
			return err
		}
		switch j.driveStep() {
		case driveAdmit:
			err = w.admit(ctx, ps, j)
		case driveReopen:
			err = w.reopen(ctx, ps, j)
		case driveLaunch:
			err = w.launch(ctx, ps, j)
		case driveReport:
			err = w.report(ctx, ps, j)
		case driveNone:
			return nil
		default:
			return fmt.Errorf("run is %s; recovery classifies that first", j.Phase)
		}
		if errors.Is(err, errRunCancelled) {
			continue // the cancel request came first: the next pass concludes it
		}
		if err != nil {
			return err
		}
	}
	return ctx.Err()
}

// recoverRun continues a run found in the journals at start. A launch with no start record may have delivered the
// prompt, and a start record with no exit status means the worker stopped while the agent ran; neither is ever
// repeated (ADR-0005, ADR-0006). Everything else just continues.
func (w *Worker) recoverRun(ctx context.Context, ps *projectState, uid string) {
	j, err := w.store.Load(uid)
	if err != nil {
		return
	}
	switch r := w.life().recover(j); r.Step {
	case recoverHold:
		err = w.hold(j, r.Hold, r.Detail)
	case recoverConcludeCancel:
		err = w.concludeCancel(j) // asked to stop and its process is provably gone
	case recoverConclude:
		err = w.conclude(ctx, j)
	}
	w.noteHook(err)
	if err != nil {
		w.opt.Log("run %s: %v", uid, err)
		return
	}
	w.spawn(ps, uid)
}

// hold records an uncertain outcome: kata will get `unknown` with the reason first (ADR-0005).
func (w *Worker) hold(j *Journal, reason, detail string) error {
	_, err := w.store.Update(j.RunUID, "held", func(j *Journal) error {
		j.Outcome = &Outcome{Status: StatusUnknown, Hold: reason, Summary: reason + ": " + detail, At: w.clock.Now().UTC()}
		j.Phase = PhaseFinished
		return nil
	})
	return err
}

// preAdmissionHold ends a run before kata saw it: no observation, no Issue write, slot and gate released (ADR-0005).
func (w *Worker) preAdmissionHold(j *Journal, reason, detail string) error {
	_, err := w.store.Update(j.RunUID, "held-before-admission", func(j *Journal) error {
		j.Outcome = &Outcome{Hold: reason, Summary: reason + ": " + detail, At: w.clock.Now().UTC()}
		j.Phase = PhaseSettled
		return nil
	})
	return err
}

func (w *Worker) qualified(j *Journal, shortID string) string { return j.ProjectName + "#" + shortID }

// admit is ADR-0002's order: confirm the lease generation, resolve the per-run Issue, then send the revision-0
// running observation. Only a created or replayed reply authorizes execution.
func (w *Worker) admit(ctx context.Context, ps *projectState, j *Journal) error {
	if err := w.checkRoute(ctx, ps, false); err != nil {
		return fmt.Errorf("cannot confirm the kata route: %w", err)
	}
	state := claimLease(ctx, w.opt.Kata, w.clock, w.opt.Config.Executor, ps.actor, j.ProjectID, false)
	w.mu.Lock()
	ps.lease = state
	w.mu.Unlock()
	if state.Unknown {
		return errors.New(state.Why())
	}
	if !state.Held || state.Generation != j.Generation {
		return w.preAdmissionHold(j, HoldAdmissionRejected, fmt.Sprintf("the Project's lease is no longer held at generation %d (%s)", j.Generation, state.Why()))
	}
	if j.Issue == nil {
		issue, err := w.resolveIssue(ctx, j)
		if err != nil {
			if refused(err) {
				return w.preAdmissionHold(j, HoldCreateRejected, err.Error())
			}
			return err
		}
		if _, err := w.store.Update(j.RunUID, "issue-resolved", func(j *Journal) error { j.Issue = issue; return nil }); err != nil {
			return err
		}
		j.Issue = issue
	}
	obs := w.observation(j, StatusRunning, "admitted "+j.JournalID, 0)
	run, _, err := w.opt.Kata.Observe(ctx, j.ProjectID, obs)
	switch {
	case errors.Is(err, ErrConflict):
		return w.preAdmissionHold(j, HoldAdmissionRejected, "kata holds a different observation of this run: "+err.Error())
	case err != nil:
		return err
	}
	_, err = w.store.Update(j.RunUID, "admitted", func(j *Journal) error { j.Phase, j.Revision = PhaseAdmitted, run.Revision; return nil })
	return err
}

// StatusRunning is kata's status of a run in progress.
const StatusRunning = "running"

func (w *Worker) observation(j *Journal, status, message string, revision int64) Observation {
	o := Observation{
		RunUID: j.RunUID, JobUID: j.JobUID, DefinitionEventUID: j.DefinitionEventUID, OccurrenceKey: j.OccurrenceKey,
		Executor: j.Executor, Actor: j.Actor, Status: status, Message: message, StartedAt: j.StartedAt, ExpectedRevision: revision,
	}
	if j.Issue != nil {
		o.IssueUID = j.Issue.UID
	}
	if j.Workflow != nil {
		o.WorkflowUID, o.WorkflowEventUID = j.Workflow.UID, j.Workflow.EventUID
	}
	return o
}

// resolveIssue creates the per-run Issue with the idempotency key tern-kata:<run uid>, force_new and the run marker, or reads the job's existing one.
func (w *Worker) resolveIssue(ctx context.Context, j *Journal) (*JournalIssue, error) {
	p := j.IssuePolicy
	if p.Kind == cronjob.IssueExisting {
		issue, err := w.opt.Kata.ShowIssue(ctx, j.ProjectID, p.UID)
		if err != nil {
			return nil, err
		}
		if !issue.Open() && j.Notify == nil { // a notify run has no readiness requirement
			return nil, &APIError{Status: 409, Message: fmt.Sprintf("Issue %s is closed; reopen it first", issue.ShortID)}
		}
		return &JournalIssue{UID: issue.UID, Ref: w.qualified(j, issue.ShortID), Title: issue.Title, Body: issue.Body, Existing: true}, nil
	}
	title := p.Title + " — " + j.Occurrence.UTC().Format(time.RFC3339Nano)
	metadata := map[string]any{"tern_kata.run": j.RunUID}
	for key, offset := range map[string]*int64{"scheduled_on": p.ScheduledOffsetSeconds, "deadline_on": p.DeadlineOffsetSeconds} {
		if offset != nil {
			metadata[key] = j.Occurrence.Add(time.Duration(*offset) * time.Second).UTC().Format(time.RFC3339)
		}
	}
	issue, err := w.opt.Kata.CreateIssue(ctx, j.ProjectID, j.Actor, "tern-kata:"+j.RunUID, title, p.Body, metadata)
	if err != nil {
		return nil, err
	}
	return &JournalIssue{UID: issue.UID, Ref: w.qualified(j, issue.ShortID), Title: title, Body: p.Body}, nil
}

// secrets reads each secret_refs source for one step. A value stays in this map and the child's environment; it
// is never written to kata or the journal (ADR-0009).
func (w *Worker) secrets(j *Journal) (map[string]string, error) {
	out := map[string]string{}
	for env, name := range j.SecretRefs {
		if strings.HasPrefix(env, "KATA_") || strings.HasPrefix(env, "TERN_KATA_") {
			return nil, fmt.Errorf("secret %q would set %s, a reserved name", name, env)
		}
		value, err := w.opt.Config.readSecret(name)
		if err != nil {
			return nil, err
		}
		out[env] = value
	}
	return out, nil
}

// readSecret resolves one secret by its worker.toml name.
func (c Config) readSecret(name string) (string, error) {
	src, ok := c.Secrets[name]
	if !ok {
		return "", fmt.Errorf("secret %q is not defined in worker.toml", name)
	}
	if src.Env != "" {
		value, ok := os.LookupEnv(src.Env)
		if !ok {
			return "", fmt.Errorf("secret %q: environment variable %s is not set", name, src.Env)
		}
		return value, nil
	}
	path := src.File
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("secret %q: %w", name, err)
		}
		path = filepath.Join(home, rest)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("secret %q: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("secret %q: %s is not a regular file (links are refused)", name, path)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("secret %q: %s must be private (mode 0600), it is %o", name, path, info.Mode().Perm())
	}
	if foreignOwned(info) {
		return "", fmt.Errorf("secret %q: %s must be owned by the user the worker runs as", name, path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("secret %q: %w", name, err)
	}
	return strings.TrimRight(string(raw), "\r\n"), nil
}

// childEnv is the worker's environment minus TERN_KATA_ internals, plus the actor and the step's secrets (ADR-0009).
func childEnv(actor string, secrets map[string]string) []string {
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "TERN_KATA_") || strings.HasPrefix(kv, "KATA_AUTHOR=") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "KATA_AUTHOR="+actor)
	for k, v := range secrets {
		env = append(env, k+"="+v)
	}
	return env
}

// launch runs the run's next attempt: a notify run's request, a workflow's next step, or a prompt job's one agent.
func (w *Worker) launch(ctx context.Context, ps *projectState, j *Journal) error {
	if j.Cancel != nil {
		return w.concludeCancel(j)
	}
	switch {
	case j.Notify != nil:
		return w.notify(ctx, j)
	case j.Workflow != nil:
		return w.workflowStep(ctx, j)
	}
	return w.runPrompt(ctx, j, attemptSpec{dir: w.store.RunDir(j.RunUID), task: j.Prompt, profile: j.Profile})
}

// failAttempt records an attempt that certainly never ran, so the failure is not uncertain: a prompt job's run fails,
// a workflow step fails (and is retried when it has retries left).
func (w *Worker) failAttempt(j *Journal, reason string) error {
	outcome := j.withCarried(&Outcome{Status: StatusFailed, Summary: reason, At: w.clock.Now().UTC()})
	if j.Workflow != nil {
		return w.concludeStep(j, outcome)
	}
	_, err := w.store.Update(j.RunUID, "finished", func(j *Journal) error {
		j.Outcome = outcome
		j.Phase = PhaseFinished
		return nil
	})
	return err
}

// runPrompt runs one agent attempt: prompt file, `launching` mark, process, start record, wait, exit record, outcome.
// It returns once the journal says the attempt is concluded, or an error that leaves the journal where it was.
func (w *Worker) runPrompt(ctx context.Context, j *Journal, sp attemptSpec) error {
	secrets, err := w.secrets(j)
	if err != nil {
		return w.hold(j, HoldSecretUnavailable, err.Error())
	}
	attempt, resumeID := j.attempt(), ""
	in := PromptInput{
		Actor: j.Actor, IssueRef: j.Issue.Ref, IssueTitle: j.Issue.Title, IssueBody: j.Issue.Body, ExistingIssue: j.Issue.Existing,
		JobName: j.JobName, RunUID: j.RunUID, Occurrence: OccurrenceKey(j.Occurrence), Task: sp.task, Attempt: attempt,
	}
	if sp.plan != nil {
		in.Step = &StepInput{Workflow: j.Workflow.Name, About: j.Workflow.About, Key: sp.plan.Key, Number: sp.index + 1, Count: len(j.Workflow.Steps), Failure: sp.failure}
		in.Attempt = sp.tries
	}
	if attempt > 1 && j.Resume != nil {
		resumeID = j.Resume.SessionID
		in.Resumed = &ResumeNote{Hold: j.Resume.Hold, Detail: j.Resume.Detail}
	}
	prompt, err := BuildPrompt(in)
	if err != nil {
		return w.hold(j, HoldInvalidPrompt, err.Error())
	}
	promptFile := filepath.Join(sp.dir, "prompt.md")
	if err := replaceFile(promptFile, []byte(prompt)); err != nil {
		return w.hold(j, HoldInvalidPrompt, "the prompt could not be written: "+err.Error())
	}
	cwd := cmp.Or(j.Checkout, w.store.RunDir(j.RunUID))
	// A spawn error is certain, not uncertain: nothing was started.
	program, err := exec.LookPath(sp.profile.command())
	if err != nil {
		return w.failAttempt(j, "cannot start the agent: "+err.Error())
	}
	if info, err := os.Stat(cwd); err != nil || !info.IsDir() {
		return w.failAttempt(j, fmt.Sprintf("cannot start the agent: checkout %s is not a directory", cwd))
	}
	if err := os.MkdirAll(sessionFolder(sp.dir), 0o700); err != nil {
		return w.hold(j, HoldExecutionError, err.Error())
	}
	if attempt > 1 && j.Resume != nil {
		keepAttemptOutput(sp.dir, attempt-1)
	}
	cmd := exec.Command(program, agentArgs(sp.profile, sp.dir, promptFile, resumeID)...)
	cmd.Dir = cwd
	cmd.Env = childEnv(j.Actor, secrets)
	cmd.Stdin = nil // /dev/null: omp reads an open stdin to EOF as prompt text
	cmd.WaitDelay = 5 * time.Second
	isolate(cmd)
	parser := &agentParser{onSession: func(id string) {
		_, err := w.store.Update(j.RunUID, "session", func(j *Journal) error {
			if j.Launch != nil {
				j.Launch.SessionID = id
			}
			return nil
		})
		w.noteHook(err)
	}}
	stdout := &lineSink{parser: parser, file: openCapped(w.store.hooks, filepath.Join(sp.dir, "output.jsonl"), 8<<20)}
	stderr := newTailSink(w.store.hooks, filepath.Join(sp.dir, "stderr.log"))
	defer stdout.close()
	defer stderr.close()
	cmd.Stdout, cmd.Stderr = stdout, stderr

	exit, err := w.supervise(ctx, j, cmd, sessionFolder(sp.dir), resumeID)
	if se, ok := errors.AsType[startError](err); ok {
		return w.failAttempt(j, "cannot start the agent: "+se.Error())
	}
	if err != nil {
		return err
	}
	stdout.flush()
	result := parser.Result()
	tail := stderr.tailText()
	exit.Unrecorded = lossOf(stdout.file, stderr.file)
	if _, err := w.store.Update(j.RunUID, "exited", func(j *Journal) error {
		result.StderrTail = tail
		j.Exit, j.Result = exit, &result
		return nil
	}); err != nil {
		return err
	}
	w.logUnrecorded(j, exit)
	j, err = w.store.Load(j.RunUID)
	if err != nil {
		return err
	}
	return w.conclude(ctx, j)
}

// startError is a process that could not be started: nothing ran, so the failure is certain.
type startError struct{ error }

// timeLeft is how long the run's current attempt may run: the job's timeout for a prompt job, what is left of the
// run's shared expiry for a workflow step.
func (w *Worker) timeLeft(j *Journal) time.Duration {
	if j.Workflow != nil && !j.ExpiresAt.IsZero() {
		return j.ExpiresAt.Sub(w.clock.Now())
	}
	return time.Duration(j.TimeoutSeconds) * time.Second
}

// supervise starts cmd and waits for it: the `launching` mark, the start record, the wait with the run's expiry (SIGTERM,
// then SIGKILL after 10 s, ADR-0006) and the exit record's contents. A worker stop kills the group and leaves the journal
// `started`, which the next start finds interrupted. The caller saves the returned exit record.
func (w *Worker) supervise(ctx context.Context, j *Journal, cmd *exec.Cmd, sessionDir, resumeID string) (*ExitRecord, error) {
	runtime.LockOSThread() // the parent-death signal follows the thread that started the child
	defer runtime.UnlockOSThread()
	if _, err := w.store.Update(j.RunUID, "launching", func(j *Journal) error {
		if j.Cancel != nil {
			return errRunCancelled
		}
		j.Phase = PhaseLaunching
		j.Launch = nil // a record of an earlier attempt must not stand for this one's process
		return nil
	}); err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, startError{err}
	}
	launch, err := adopt(cmd)
	if err != nil {
		_ = cmd.Wait()
		return nil, startError{err}
	}
	launch.SessionDir, launch.SessionID, launch.At = sessionDir, resumeID, w.clock.Now().UTC()
	if _, err := w.store.Update(j.RunUID, "started", func(j *Journal) error {
		j.Phase = PhaseStarted
		j.Launch = launch
		return nil
	}); err != nil {
		stopped, stop := context.WithCancel(ctx)
		stop() // a context that has ended kills the group at once
		_, _ = w.terminateGroup(stopped, launch)
		_ = cmd.Wait()
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timedOut := false
	var waitErr error
	cancel := w.cancelSignalOf(j.RunUID)
	select {
	case waitErr = <-done:
	case <-ctx.Done():
		_, _ = w.terminateGroup(ctx, launch) // kills at once: ctx has ended
		<-done
		return nil, ctx.Err() // the journal stays started: the next start finds it interrupted
	case <-cancel.ch:
		if waitErr, err = w.stopAndWait(ctx, launch, done); err != nil {
			return nil, err
		}
	case <-w.clock.After(w.timeLeft(j)):
		timedOut = true
		if waitErr, err = w.stopAndWait(ctx, launch, done); err != nil {
			return nil, err
		}
	}
	exit := &ExitRecord{At: w.clock.Now().UTC(), TimedOut: timedOut}
	if ee, ok := errors.AsType[*exec.ExitError](waitErr); ok {
		exit.Code, exit.Signal = ee.ExitCode(), exitSignal(ee.ProcessState)
	} else if waitErr != nil {
		exit.Code = -1
	}
	return exit, nil
}

// stopAndWait stops the run's group by the termination policy and returns what the process answered. An error means
// the worker stopped first (the journal stays `started`).
func (w *Worker) stopAndWait(ctx context.Context, launch *LaunchRecord, done <-chan error) (waitErr error, err error) {
	_, err = w.terminateGroup(ctx, launch)
	waitErr = <-done
	if err != nil {
		return nil, err
	}
	return waitErr, nil
}

// conclude proves the process group gone and decides the outcome of a run that has an exit record.
func (w *Worker) conclude(ctx context.Context, j *Journal) error {
	command := j.activeIsCommand()
	if command && j.Launch != nil && j.Cancel == nil && !j.Exit.TimedOut && !w.proveGroup(j.Launch).Gone() {
		// A command that ended by itself leaves a process that is not the worker's to guess about: it may still be doing
		// what the command began. One the worker stopped (timeout, abandon) is stopped completely below.
		return w.hold(j, HoldGroupUnproven, fmt.Sprintf("process group %d is still present after the command exited", j.Launch.PGID))
	}
	if j.Launch != nil && !w.proveGroup(j.Launch).Gone() {
		proof, err := w.terminateGroup(ctx, j.Launch)
		if err != nil {
			return err
		}
		if !proof.Gone() {
			return w.hold(j, HoldGroupUnproven, fmt.Sprintf("process group %d is still present after the agent exited", j.Launch.PGID))
		}
	}
	if j.Cancel != nil {
		return w.concludeCancel(j)
	}
	if hold, detail := providerHold(j); hold != "" {
		return w.hold(j, hold, detail)
	}
	var outcome *Outcome
	if command {
		outcome = classifyCommand(j, commandTail1K(w.attemptDir(j)))
	} else {
		outcome = j.withCarried(classify(j))
	}
	if j.Workflow != nil {
		return w.concludeStep(j, outcome)
	}
	_, err := w.store.Update(j.RunUID, "finished", func(j *Journal) error {
		outcome.At = w.clock.Now().UTC()
		j.Outcome, j.Phase = outcome, PhaseFinished
		return nil
	})
	return err
}

// commentKey is the idempotency key of the run's comment of the given kind. A resumed attempt gets its own, so its
// comment is not swallowed as a replay of the first attempt's.
func (j *Journal) commentKey(kind string) string {
	key := "tern-kata:" + j.RunUID + ":comment:" + kind
	if a := j.attempt(); a > 1 {
		key += fmt.Sprintf(":a%d", a)
	}
	return key
}

// classify is the rule of ADR-0009: exit 0 and a terminal agent_end whose last assistant message stopped normally is
// succeeded; anything else is failed with the cause first. The summary is the final assistant message, cut to kata's bound.
func classify(j *Journal) *Outcome {
	r, e := j.Result, j.Exit
	final := strings.TrimSpace(r.FinalText)
	var reason string
	switch {
	case e.TimedOut:
		reason = fmt.Sprintf("timed out after %s", time.Duration(j.TimeoutSeconds)*time.Second)
	case e.Signal != "":
		reason = "the agent was ended by " + e.Signal
	case e.Code != 0:
		reason = fmt.Sprintf("the agent exited with status %d", e.Code)
	case !r.Terminal:
		reason = "the agent exited without finishing a turn (no agent_end)"
	case r.StopReason != "stop":
		reason = "the agent stopped with " + cmp.Or(r.StopReason, "no stop reason")
	}
	if reason != "" && r.ErrorText != "" {
		reason += ": " + r.ErrorText
	}
	if reason != "" && r.StderrTail != "" && r.ErrorText == "" {
		reason += ": " + r.StderrTail
	}
	o := &Outcome{InputTokens: r.InputTokens, OutputTokens: r.OutputTokens}
	if reason == "" {
		o.Status, o.Summary = StatusSucceeded, cmp.Or(final, "(the agent gave no final message)")
	} else {
		o.Status, o.Summary = StatusFailed, reason
		if final != "" {
			o.Summary += "\n\n" + final
		}
	}
	o.Summary = truncate(o.Summary, maxSummaryBytes)
	return o
}

// report sends what the run owes kata: the final observation first (it never waits for an Issue write), then the
// comment and label the closure check calls for (ADR-0009), and for a held run the inbox request to the Issue's owner or
// author (ADR-0005). Every write is an intent saved before it is sent, and nothing is sent while the Project's route is
// unconfirmed (ADR-0004).
func (w *Worker) report(ctx context.Context, ps *projectState, j *Journal) error {
	if err := w.checkRoute(ctx, ps, false); err != nil {
		return fmt.Errorf("cannot confirm the kata route: %w", err)
	}
	if len(j.Intents) == 0 {
		intents := []Intent{{Kind: IntentFinal}}
		issueOpen := true
		if j.Outcome.Status == StatusSucceeded && !j.Issue.Existing {
			if issue, err := w.opt.Kata.ShowIssue(ctx, j.ProjectID, j.Issue.UID); err == nil {
				issueOpen = issue.Open()
			}
		} else if j.Outcome.Status == StatusSucceeded {
			issueOpen = false
		}
		switch j.Outcome.Status {
		case StatusSucceeded:
			if quietSuccess(j) {
				break // every step was a command that exited 0 on an existing Issue: the Run says it all
			}
			if j.Issue.Existing || issueOpen {
				intents = append(intents, Intent{Kind: IntentComment, Key: j.commentKey("succeeded"), Text: j.Outcome.Summary})
				if !j.Issue.Existing {
					intents = append(intents, Intent{Kind: IntentLabel, Text: "needs-review"})
				}
			}
		case StatusFailed:
			intents = append(intents, Intent{Kind: IntentComment, Key: j.commentKey("failed"), Text: "The run failed: " + j.Outcome.Summary})
		case StatusUnknown:
			intents = append(intents, Intent{Kind: IntentComment, Key: j.commentKey("held"), Text: "The run is held: " + j.Outcome.Summary + "\n\nResume it with `tern-kata worker resume " + j.RunUID + "` or abandon it with `tern-kata worker abandon " + j.RunUID + "`."})
			intents = append(intents, Intent{Kind: IntentNotify, Text: heldNotice(j)})
		case StatusCancelled:
			intents = append(intents, Intent{Kind: IntentComment, Key: j.commentKey("cancelled"), Text: "The run was abandoned: " + j.Outcome.Summary})
		}
		if j.Notify != nil { // a notify run's only record is the Run itself; the Issue gets no comment, label or request
			intents = intents[:1]
		}
		var err error
		if j, err = w.store.Update(j.RunUID, "intents-planned", func(j *Journal) error { j.Intents = intents; return nil }); err != nil {
			return err
		}
	}
	for i := range j.Intents {
		if j.Intents[i].Done || j.Intents[i].Inspect != "" {
			continue
		}
		var err error
		if it := j.Intents[i]; it.Kind == IntentNotify && it.Recipient == "" {
			if j, err = w.resolveRecipient(ctx, j, i); err == nil && j.Intents[i].Done {
				continue // the Issue names nobody to tell
			}
		}
		var revision int64
		if err == nil {
			revision, err = w.send(ctx, j, j.Intents[i])
		}
		var herr error
		kind := j.Intents[i].Kind
		j, herr = w.store.Update(j.RunUID, "intent-"+string(kind), func(j *Journal) error {
			it := &j.Intents[i]
			it.Done, it.Error, it.Inspect = err == nil, "", ""
			if inspect, ok := errors.AsType[*inspectError](err); ok {
				it.Inspect = inspect.reason
			} else if err != nil {
				it.Error = err.Error()
			}
			if err == nil && revision > 0 {
				j.Revision = revision
			}
			return nil
		})
		if herr != nil {
			return herr
		}
	}
	for _, it := range j.Intents {
		if !it.Done && it.Inspect == "" {
			return fmt.Errorf("%s not delivered yet: %s", it.Kind, it.Error)
		}
	}
	for _, why := range j.Inspections() {
		w.opt.Log("run %s: needs inspection: %s", j.RunUID, why)
	}
	_, err := w.store.Update(j.RunUID, "settled", func(j *Journal) error { j.Phase = PhaseSettled; return nil })
	return err
}

// inspectError is a write that cannot be delivered without a person looking: kata holds something a retry would
// overwrite (ADR-0006).
type inspectError struct{ reason string }

func (e *inspectError) Error() string { return e.reason }

// quietSuccess is ADR-0009's quiet success: a run of command steps only, each of which exited 0, on an existing Issue
// says nothing there; its final observation carries the receipt.
func quietSuccess(j *Journal) bool {
	if !j.Issue.Existing || j.Workflow == nil || len(j.Workflow.Steps) == 0 || len(j.Steps) != len(j.Workflow.Steps) {
		return false
	}
	for i, s := range j.Workflow.Steps {
		step := j.Steps[i]
		if s.Kind != cronjob.StepCommand || step.State != StepSucceeded || step.Exit == nil || step.Exit.Code != 0 {
			return false
		}
	}
	return true
}

// heldNotice is the inbox request for a held run, saved in its intent so a retry sends the same words.
func heldNotice(j *Journal) string {
	return truncate(fmt.Sprintf("Run %s of %q on %s is held: %s Resume it with `tern-kata worker resume %s` or abandon it with `tern-kata worker abandon %s`.",
		j.RunUID, j.JobName, j.Issue.Ref, j.Outcome.Summary, j.RunUID, j.RunUID), maxNotifyBytes)
}

// resolveRecipient reads the Issue and saves who its owner or author is in the notify intent, once: from then on a
// retry sends to that address whatever the Issue says.
func (w *Worker) resolveRecipient(ctx context.Context, j *Journal, i int) (*Journal, error) {
	issue, err := w.opt.Kata.ShowIssue(ctx, j.ProjectID, j.Issue.UID)
	if err != nil {
		return j, err
	}
	recipient := cmp.Or(issue.Owner, issue.Author)
	return w.store.Update(j.RunUID, "intent-notify-resolved", func(j *Journal) error {
		j.Intents[i].Recipient, j.Intents[i].Done = recipient, recipient == ""
		return nil
	})
}

// send delivers one intent and returns the Run revision kata stored when it was a final observation. A final
// observation that finds kata holding a different revision is never sent over it: the journal's revision is the one
// the worker last wrote, and anything newer is somebody else's observation (ADR-0006). It counts as delivered only
// when the stored run already says what the worker means to.
func (w *Worker) send(ctx context.Context, j *Journal, it Intent) (int64, error) {
	switch it.Kind {
	case IntentComment:
		return 0, w.opt.Kata.Comment(ctx, j.ProjectID, j.Actor, j.Issue.UID, it.Key, it.Text)
	case IntentLabel:
		return 0, w.opt.Kata.AddLabel(ctx, j.ProjectID, j.Actor, j.Issue.UID, it.Text)
	case IntentNotify:
		return 0, w.sendNotice(ctx, j, it.Recipient, it.Text)
	}
	obs := w.observation(j, j.Outcome.Status, j.Outcome.Summary, j.Revision)
	end := j.Outcome.At
	obs.EndedAt, obs.InputTokens, obs.OutputTokens = &end, j.Outcome.InputTokens, j.Outcome.OutputTokens
	stored, _, err := w.opt.Kata.Observe(ctx, j.ProjectID, obs)
	if !errors.Is(err, ErrConflict) {
		return stored.Revision, err
	}
	current, serr := w.opt.Kata.ShowRun(ctx, j.ProjectID, j.RunUID)
	if serr != nil {
		return 0, err
	}
	if current.Status == j.Outcome.Status {
		return current.Revision, nil
	}
	return 0, &inspectError{fmt.Sprintf("kata holds this run as %s at revision %d, not at the revision %d this worker last wrote; the final %s observation was not sent over it", current.Status, current.Revision, j.Revision, j.Outcome.Status)}
}

// sendNotice puts an inbox request on the run's Issue. The write is a read-back class (ADR-0006): the Issue is read
// first and a request already there with these words from this actor counts as sent.
func (w *Worker) sendNotice(ctx context.Context, j *Journal, recipient, message string) error {
	issue, err := w.opt.Kata.ShowIssue(ctx, j.ProjectID, j.Issue.UID)
	if err != nil {
		return err
	}
	if from, text, ok := issue.InboxRequest(recipient); ok && from == j.Actor && text == message {
		return nil
	}
	return w.opt.Kata.Notify(ctx, j.ProjectID, j.Actor, j.Issue.UID, recipient, w.opt.Config.Executor, message)
}

// lineSink feeds omp's stdout, line by line, to the parser and keeps the first bytes in a capped file.
type lineSink struct {
	mu       sync.Mutex
	parser   *agentParser
	file     *cappedFile
	buf      []byte
	skipping bool
}

const maxLine = 8 << 20

func (s *lineSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(p)
	_, _ = s.file.Write(p)
	for len(p) > 0 {
		i := indexByte(p, '\n')
		if i < 0 {
			if !s.skipping {
				s.buf = append(s.buf, p...)
				if len(s.buf) > maxLine {
					s.buf, s.skipping = nil, true
				}
			}
			break
		}
		if !s.skipping {
			s.buf = append(s.buf, p[:i]...)
			s.parser.Line(s.buf)
		}
		s.buf, s.skipping = s.buf[:0], false
		p = p[i+1:]
	}
	return n, nil
}

func indexByte(p []byte, c byte) int {
	for i, b := range p {
		if b == c {
			return i
		}
	}
	return -1
}

// flush parses a last line that had no newline.
func (s *lineSink) flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.buf) > 0 && !s.skipping {
		s.parser.Line(s.buf)
	}
	s.buf = nil
}

func (s *lineSink) close() { _ = s.file.Close() }

// cappedFile records the first max bytes of a capture and drops the rest, so a chatty agent cannot fill the disk.
type cappedFile struct {
	*capture
	left int
}

func openCapped(hooks Hooks, path string, max int) *cappedFile {
	return &cappedFile{capture: openCapture(hooks, path), left: max}
}

func (c *cappedFile) Write(p []byte) (int, error) {
	if c.left > 0 {
		n := min(len(p), c.left)
		_, _ = c.capture.Write(p[:n])
		c.left -= n
	}
	return len(p), nil
}

// tailSink keeps the first 64 KiB of stderr in a file and its last 2 KiB for failure reasons.
type tailSink struct {
	file *cappedFile
	mu   sync.Mutex
	tail []byte
}

func newTailSink(hooks Hooks, path string) *tailSink {
	return &tailSink{file: openCapped(hooks, path, 64<<10)}
}

func (t *tailSink) Write(p []byte) (int, error) {
	_, _ = t.file.Write(p)
	t.mu.Lock()
	t.tail = append(t.tail, p...)
	if len(t.tail) > 2048 {
		t.tail = t.tail[len(t.tail)-2048:]
	}
	t.mu.Unlock()
	return len(p), nil
}

func (t *tailSink) tailText() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.TrimSpace(strings.ToValidUTF8(string(t.tail), ""))
}

func (t *tailSink) close() { _ = t.file.Close() }

// notify is a notify run's whole work: send the inbox request to the recipient on the run's Issue and record the
// outcome. The request is worked out from the Issue once and saved in the journal, so a retry or a restart sends the
// same words to the same address; before sending, the Issue is read, and a request already there counts as sent
// (ADR-0006 "read-back"). The run's identity (ADR-0002) already keeps a restart from preparing the occurrence twice.
func (w *Worker) notify(ctx context.Context, j *Journal) error {
	finish := func(status, summary string) error {
		_, err := w.store.Update(j.RunUID, "finished", func(j *Journal) error {
			j.Outcome = &Outcome{Status: status, Summary: truncate(summary, maxSummaryBytes), At: w.clock.Now().UTC()}
			j.Phase = PhaseFinished
			return nil
		})
		return err
	}
	notice := j.Notify.Resolved
	if notice == nil {
		issue, err := w.opt.Kata.ShowIssue(ctx, j.ProjectID, j.Issue.UID)
		if err != nil {
			if refused(err) {
				return finish(StatusFailed, "cannot read the Issue to notify about: "+err.Error())
			}
			return err
		}
		recipient := j.Notify.Recipient
		if recipient == currentOwnerOrAuthor {
			recipient = cmp.Or(issue.Owner, issue.Author)
		}
		message := j.Notify.Message
		if strings.TrimSpace(message) == "" {
			message = fmt.Sprintf("%s: %s", w.qualified(j, issue.ShortID), issue.Title)
		}
		saved, err := w.store.Update(j.RunUID, "notify-resolved", func(j *Journal) error {
			j.Notify.Resolved = &ResolvedNotice{Recipient: recipient, Message: truncate(message, maxNotifyBytes)}
			return nil
		})
		if err != nil {
			return err
		}
		j, notice = saved, saved.Notify.Resolved
	}
	if notice.Recipient == "" {
		return finish(StatusFailed, "the Issue has no owner or author to notify")
	}
	if err := w.sendNotice(ctx, j, notice.Recipient, notice.Message); err != nil {
		if refused(err) {
			return finish(StatusFailed, "kata refused the notification: "+err.Error())
		}
		return err
	}
	return finish(StatusSucceeded, "Notified "+notice.Recipient+": "+notice.Message)
}

const (
	currentOwnerOrAuthor = "current-owner-or-author"
	// maxNotifyBytes is the inbox's message limit.
	maxNotifyBytes = 1024
)
