package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
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
		w.mu.Lock()
		delete(ps.running, uid)
		w.mu.Unlock()
		if err != nil && w.ctx.Err() == nil && !w.halted.Load() {
			w.opt.Log("run %s: %v", uid, err)
		}
	}()
}

// retryPending resumes runs a transient kata failure left half-reported or half-admitted; the Project loop calls it at least every minute.
func (w *Worker) retryPending(ps *projectState) {
	journals, _, _ := w.store.Scan()
	for _, j := range journals {
		if j.ProjectUID != ps.project.UID || j.Executor != w.opt.Config.Executor {
			continue
		}
		switch {
		case j.Phase == PhasePrepared || j.Phase == PhaseAdmitted || j.Phase == PhaseResuming || j.Phase == PhaseFinished:
			w.spawn(ps, j.RunUID)
		case j.Phase == PhaseSettled && !holdsSlot(j):
			w.release(ps, j.RunUID) // a held run whose process has since gone no longer takes a slot
		}
	}
}

// advance drives a run from the phase its journal is in to settled. Each step saves before it acts; a step that
// cannot finish returns its error and the run stays where it is for retryPending or the next start.
func (w *Worker) advance(ctx context.Context, ps *projectState, uid string) (err error) {
	defer func() {
		w.noteHook(err)
		if j, lerr := w.store.Load(uid); lerr == nil && !holdsSlot(j) {
			w.release(ps, uid)
		}
		w.kick(ps) // a slot or an overlap gate may have opened: queued occurrences are due another look
	}()
	for ctx.Err() == nil && !w.halted.Load() {
		j, err := w.store.Load(uid)
		if err != nil {
			return err
		}
		switch j.Phase {
		case PhasePrepared:
			err = w.admit(ctx, ps, j)
		case PhaseResuming:
			err = w.reopen(ctx, ps, j)
		case PhaseAdmitted:
			err = w.launch(ctx, ps, j)
		case PhaseFinished:
			err = w.report(ctx, ps, j)
		case PhaseSettled:
			return nil
		default:
			return fmt.Errorf("run is %s; recovery classifies that first", j.Phase)
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
	what := "agent"
	if j.activeIsCommand() {
		what = "command"
	}
	switch {
	case j.Phase == PhaseLaunching:
		err = w.hold(j, HoldLaunchUnproven, "the worker stopped while starting the "+what+", so it may have started")
	case j.Phase == PhaseStarted && j.Exit == nil:
		switch {
		case processMayBeAlive(j):
			err = w.hold(j, HoldGroupUnproven, fmt.Sprintf("the %s's process group %d is still present", what, j.Launch.PGID))
		case what == "agent" && w.finishedItsTurn(j):
			err = w.hold(j, HoldNoReceipt, "the agent finished its turn and its process is gone, but the worker saved no exit status")
		default:
			err = w.hold(j, HoldInterrupted, "the worker stopped while the "+what+" ran")
		}
	case j.Phase == PhaseStarted:
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
	if _, err := w.opt.Kata.Capabilities(ctx, j.ProjectID); err != nil {
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
	if p.Kind == "existing" {
		issue, err := w.opt.Kata.ShowIssue(ctx, j.ProjectID, p.UID)
		if err != nil {
			return nil, err
		}
		if !issue.Open() {
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
	outcome := &Outcome{Status: StatusFailed, Summary: reason, At: w.clock.Now().UTC()}
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
	cwd := cmpOr(j.Checkout, w.store.RunDir(j.RunUID))
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
	stdout := &lineSink{parser: parser, file: openCapped(filepath.Join(sp.dir, "output.jsonl"), 8<<20)}
	stderr := newTailSink(filepath.Join(sp.dir, "stderr.log"))
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
	if _, err := w.store.Update(j.RunUID, "exited", func(j *Journal) error {
		result.StderrTail = tail
		j.Exit, j.Result = exit, &result
		return nil
	}); err != nil {
		return err
	}
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
// then SIGKILL after 10 s) and the exit record's contents. A worker stop kills the group and leaves the journal
// `started`, which the next start finds interrupted. The caller saves the returned exit record.
func (w *Worker) supervise(ctx context.Context, j *Journal, cmd *exec.Cmd, sessionDir, resumeID string) (*ExitRecord, error) {
	runtime.LockOSThread() // the parent-death signal follows the thread that started the child
	defer runtime.UnlockOSThread()
	if _, err := w.store.Update(j.RunUID, "launching", func(j *Journal) error {
		j.Phase = PhaseLaunching
		j.Launch = nil // a record of an earlier attempt must not stand for this one's process
		return nil
	}); err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, startError{err}
	}
	pid := cmd.Process.Pid
	if _, err := w.store.Update(j.RunUID, "started", func(j *Journal) error {
		j.Phase = PhaseStarted
		j.Launch = &LaunchRecord{PID: pid, PGID: pid, StartTicks: startTicks(pid), SessionDir: sessionDir, SessionID: resumeID, At: w.clock.Now().UTC()}
		return nil
	}); err != nil {
		signalGroup(pid, true)
		_ = cmd.Wait()
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timedOut := false
	var waitErr error
	select {
	case waitErr = <-done:
	case <-ctx.Done():
		signalGroup(pid, true)
		<-done
		return nil, ctx.Err() // the journal stays started: the next start finds it interrupted
	case <-w.clock.After(w.timeLeft(j)):
		timedOut = true
		signalGroup(pid, false)
		select {
		case waitErr = <-done:
		case <-ctx.Done():
			signalGroup(pid, true)
			<-done
			return nil, ctx.Err()
		case <-w.clock.After(10 * time.Second):
			signalGroup(pid, true)
			waitErr = <-done
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

// conclude proves the process group gone and decides the outcome of a run that has an exit record.
func (w *Worker) conclude(ctx context.Context, j *Journal) error {
	command := j.activeIsCommand()
	if command && j.Launch != nil && groupAlive(j.Launch.PGID) {
		// A command's leftover process is not the worker's to guess about: it may still be doing what the command began.
		return w.hold(j, HoldGroupUnproven, fmt.Sprintf("process group %d is still present after the command exited", j.Launch.PGID))
	}
	if j.Launch != nil && groupAlive(j.Launch.PGID) {
		signalGroup(j.Launch.PGID, false)
		for i := 0; i < 150 && groupAlive(j.Launch.PGID); i++ {
			if i == 50 {
				signalGroup(j.Launch.PGID, true)
			}
			if !w.sleep(ctx, 200*time.Millisecond) {
				return ctx.Err()
			}
		}
		if groupAlive(j.Launch.PGID) {
			return w.hold(j, HoldGroupUnproven, fmt.Sprintf("process group %d is still present after the agent exited", j.Launch.PGID))
		}
	}
	if hold, detail := providerHold(j); hold != "" {
		return w.hold(j, hold, detail)
	}
	var outcome *Outcome
	if command {
		outcome = classifyCommand(j, commandTail1K(w.attemptDir(j)))
	} else {
		outcome = classify(j)
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
		reason = "the agent stopped with " + cmpOr(r.StopReason, "no stop reason")
	}
	if reason != "" && r.ErrorText != "" {
		reason += ": " + r.ErrorText
	}
	if reason != "" && r.StderrTail != "" && r.ErrorText == "" {
		reason += ": " + r.StderrTail
	}
	o := &Outcome{InputTokens: r.InputTokens, OutputTokens: r.OutputTokens}
	if reason == "" {
		o.Status, o.Summary = StatusSucceeded, cmpOr(final, "(the agent gave no final message)")
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
// comment and label the closure check calls for (ADR-0009). Every write is an intent saved before it is sent.
func (w *Worker) report(ctx context.Context, ps *projectState, j *Journal) error {
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
		case StatusCancelled:
			intents = append(intents, Intent{Kind: IntentComment, Key: j.commentKey("cancelled"), Text: "The run was abandoned: " + j.Outcome.Summary})
		}
		if j.Notify != nil { // a notify run's only record is the Run itself; the Issue gets no comment or label
			intents = intents[:1]
		}
		var err error
		if j, err = w.store.Update(j.RunUID, "intents-planned", func(j *Journal) error { j.Intents = intents; return nil }); err != nil {
			return err
		}
	}
	for i := range j.Intents {
		it := j.Intents[i]
		if it.Done {
			continue
		}
		err := w.send(ctx, j, it)
		var herr error
		j, herr = w.store.Update(j.RunUID, "intent-"+string(it.Kind), func(j *Journal) error {
			j.Intents[i].Done = err == nil
			j.Intents[i].Error = ""
			if err != nil {
				j.Intents[i].Error = err.Error()
			}
			return nil
		})
		if herr != nil {
			return herr
		}
	}
	for _, it := range j.Intents {
		if !it.Done {
			return fmt.Errorf("%s not delivered yet: %s", it.Kind, it.Error)
		}
	}
	_, err := w.store.Update(j.RunUID, "settled", func(j *Journal) error { j.Phase = PhaseSettled; return nil })
	return err
}

// send delivers one intent. A final observation that finds the stored run already at the intended status counts as delivered.
func (w *Worker) send(ctx context.Context, j *Journal, it Intent) error {
	switch it.Kind {
	case IntentComment:
		return w.opt.Kata.Comment(ctx, j.ProjectID, j.Actor, j.Issue.UID, it.Key, it.Text)
	case IntentLabel:
		return w.opt.Kata.AddLabel(ctx, j.ProjectID, j.Actor, j.Issue.UID, it.Text)
	}
	obs := w.observation(j, j.Outcome.Status, j.Outcome.Summary, j.Revision)
	end := j.Outcome.At
	obs.EndedAt, obs.InputTokens, obs.OutputTokens = &end, j.Outcome.InputTokens, j.Outcome.OutputTokens
	_, _, err := w.opt.Kata.Observe(ctx, j.ProjectID, obs)
	if errors.Is(err, ErrConflict) {
		current, serr := w.opt.Kata.ShowRun(ctx, j.ProjectID, j.RunUID)
		if serr != nil {
			return err
		}
		if current.Status == j.Outcome.Status {
			return nil
		}
		obs.ExpectedRevision = current.Revision
		if current.Status == StatusRunning || current.Status == StatusUnknown { // unknown is this worker's own earlier hold
			_, _, err = w.opt.Kata.Observe(ctx, j.ProjectID, obs)
		}
	}
	return err
}

// lineSink feeds omp's stdout, line by line, to the parser and keeps the first bytes in a capped file.
type lineSink struct {
	mu       sync.Mutex
	parser   *agentParser
	file     io.WriteCloser
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

// cappedFile writes the first max bytes and drops the rest, so a chatty agent cannot fill the disk.
type cappedFile struct {
	f    *os.File
	left int
}

func openCapped(path string, max int) io.WriteCloser {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nopCloser{io.Discard}
	}
	return &cappedFile{f: f, left: max}
}

func (c *cappedFile) Write(p []byte) (int, error) {
	if c.left > 0 {
		n := min(len(p), c.left)
		_, _ = c.f.Write(p[:n])
		c.left -= n
	}
	return len(p), nil
}

func (c *cappedFile) Close() error { return c.f.Close() }

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }

// tailSink keeps the first 64 KiB of stderr in a file and its last 2 KiB for failure reasons.
type tailSink struct {
	file io.WriteCloser
	mu   sync.Mutex
	tail []byte
}

func newTailSink(path string) *tailSink { return &tailSink{file: openCapped(path, 64<<10)} }

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
// outcome. The request is one metadata slot per Issue and recipient with fixed content, so repeating it after a crash
// between the send and the saved outcome changes nothing; the run's identity (ADR-0002) already keeps a restart from
// preparing the occurrence twice.
func (w *Worker) notify(ctx context.Context, j *Journal) error {
	finish := func(status, summary string) error {
		_, err := w.store.Update(j.RunUID, "finished", func(j *Journal) error {
			j.Outcome = &Outcome{Status: status, Summary: truncate(summary, maxSummaryBytes), At: w.clock.Now().UTC()}
			j.Phase = PhaseFinished
			return nil
		})
		return err
	}
	issue, err := w.opt.Kata.ShowIssue(ctx, j.ProjectID, j.Issue.UID)
	if err != nil {
		if refused(err) {
			return finish(StatusFailed, "cannot read the Issue to notify about: "+err.Error())
		}
		return err
	}
	recipient := j.Notify.Recipient
	if recipient == currentOwnerOrAuthor {
		recipient = cmpOr(issue.Owner, issue.Author)
	}
	message := j.Notify.Message
	if strings.TrimSpace(message) == "" {
		message = fmt.Sprintf("%s: %s", w.qualified(j, issue.ShortID), issue.Title)
	}
	message = truncate(message, maxNotifyBytes)
	if err := w.opt.Kata.Notify(ctx, j.ProjectID, j.Actor, j.Issue.UID, recipient, w.opt.Config.Executor, message); err != nil {
		if refused(err) {
			return finish(StatusFailed, "kata refused the notification: "+err.Error())
		}
		return err
	}
	return finish(StatusSucceeded, "Notified "+recipient+": "+message)
}

const (
	currentOwnerOrAuthor = "current-owner-or-author"
	// maxNotifyBytes is the inbox's message limit.
	maxNotifyBytes = 1024
)
