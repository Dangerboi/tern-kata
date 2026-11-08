package worker

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
	"github.com/Dangerboi/tern-kata/internal/data/ompext"
)

// noteHook halts the worker's run machinery when a test's save hook "killed" it, as a crash would: nothing after the
// named journal point runs, in this process.
func (w *Worker) noteHook(err error) {
	if err != nil && errors.Is(err, ErrKilled) {
		w.halted.Store(true)
	}
}

func (w *Worker) spawn(ps *projectState, uid string) {
	w.mu.Lock()
	if w.flight[uid].driven || w.halted.Load() {
		w.mu.Unlock()
		return
	}
	f := w.flight[uid]
	f.driven = true
	w.flight[uid] = f
	w.mu.Unlock()
	w.runs.Go(func() {
		err := w.advance(w.ctx, ps, uid)
		j, lerr := w.store.Load(uid)
		owes := lerr == nil && j.owes() // a transient kata failure stopped it: retryPending will drive it on
		w.updateFlight(uid, func(f *flight) { f.driven, f.owes = false, owes })
		if err != nil && w.ctx.Err() == nil && !w.halted.Load() {
			w.opt.Log("run %s: %v", uid, err)
		}
	})
}

// retryPending resumes runs a transient kata failure left half-reported or half-admitted; the Project loop calls it at least every minute.
func (w *Worker) retryPending(ps *projectState) {
	journals, _, err := w.store.Scan()
	if err != nil { // the readable journals are still driven; the unreadable runs are retried by the next scan
		if msg := err.Error(); w.scanErr.Load() == nil || *w.scanErr.Load() != msg {
			w.scanErr.Store(&msg)
			w.opt.Log("journals: %v", err)
		}
	} else if w.scanErr.Load() != nil {
		w.scanErr.Store(nil)
	}
	for _, j := range journals {
		if j.ProjectUID != ps.project.UID || j.Executor != w.opt.Config.Executor {
			continue
		}
		w.settleFlight(j.RunUID, j.owes()) // settled or held by another path since: nothing left to wait for
		switch {
		case j.driveStep() == driveAdmit && !w.holdsSlotOf(ps, j.RunUID):
			// prepare saved this journal but has not bound its slot yet; RequestRun or the Project schedule spawns it next
		case j.retryable():
			w.spawn(ps, j.RunUID)
		case j.settled():
			w.syncSlot(ps, j) // a held run whose process has since gone no longer takes a slot
		}
	}
}

// advance drives a run from the phase its journal is in to settled. Each step saves before it acts; a step that
// cannot finish returns its error and the run stays where it is for retryPending or the next start.
func (w *Worker) advance(ctx context.Context, ps *projectState, uid string) (err error) {
	defer func() {
		w.noteHook(err)
		w.dropCancelSignal(uid)
		if j, lerr := w.store.Load(uid); lerr == nil {
			w.syncSlot(ps, j)
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
	switch r := w.life(j).recover(j); r.Step {
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

func (w *Worker) record(uid, point string, transition func(j *Journal, at time.Time) error) error {
	_, err := w.store.Update(uid, point, func(j *Journal) error { return transition(j, w.clock.Now().UTC()) })
	return err
}

func (w *Worker) finish(uid string, o *Outcome) error {
	return w.record(uid, "finished", func(j *Journal, at time.Time) error { return j.finish(o, at) })
}

func (w *Worker) endAttempt(j *Journal, o *Outcome) error {
	if j.Workflow == nil {
		return w.finish(j.RunUID, o)
	}
	return w.record(j.RunUID, "step-concluded", func(j *Journal, at time.Time) error { return j.concludeStep(o, at) })
}

// hold records an uncertain outcome: kata will get `unknown` with the reason first (ADR-0005).
func (w *Worker) hold(j *Journal, reason, detail string) error {
	return w.record(j.RunUID, "held", func(j *Journal, at time.Time) error {
		if err := j.hold(reason, detail, at); err != nil {
			return err
		}
		// What the run has cost so far, so a spend limit counts a held run: resumed attempts, concluded steps and the
		// held attempt. A resume moves the held attempt into CarriedCost, so the sum stays the same.
		j.Outcome.CostUSD = addCost(sumStepCost(j.Steps), addCost(j.CarriedCost, w.heldCost(j)))
		return nil
	})
}

// preAdmissionHold ends a run before kata saw it: no observation, no Issue write, slot and gate released (ADR-0005).
func (w *Worker) preAdmissionHold(j *Journal, reason, detail string) error {
	return w.record(j.RunUID, "held-before-admission", func(j *Journal, at time.Time) error {
		return j.holdBeforeAdmission(reason, detail, at)
	})
}

func (w *Worker) qualified(j *Journal, shortID string) string { return j.ProjectName + "#" + shortID }

// admit is ADR-0002's order: confirm the lease generation, resolve the per-run Issue, then send the revision-0
// running observation. Only a created or replayed reply authorizes execution.
func (w *Worker) admit(ctx context.Context, ps *projectState, j *Journal) error {
	if err := w.checkRoute(ctx, ps, false); err != nil {
		return fmt.Errorf("cannot confirm the kata route: %w", err)
	}
	state := w.renewLease(ctx, ps, j.ProjectID)
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
	obs := w.observation(j, cronjob.RunRunning, "admitted "+j.JournalID, 0)
	run, _, err := w.opt.Kata.Observe(ctx, j.ProjectID, obs)
	switch {
	case errors.Is(err, ErrConflict):
		return w.preAdmissionHold(j, HoldAdmissionRejected, "kata holds a different observation of this run: "+err.Error())
	case err != nil:
		return err
	}
	_, err = w.store.Update(j.RunUID, "admitted", func(j *Journal) error { return j.admit(run.Revision) })
	return err
}

func (w *Worker) observation(j *Journal, status cronjob.RunStatus, message string, revision int64) Observation {
	o := Observation{
		RunUID: j.RunUID, JobUID: j.JobUID, DefinitionEventUID: j.DefinitionEventUID, OccurrenceKey: j.OccurrenceKey,
		Executor: j.Executor, Actor: j.Actor, Status: status, Message: message, StartedAt: j.StartedAt, ExpectedRevision: revision,
	}
	if j.Issue != nil {
		o.IssueUID = j.Issue.UID
	}
	if j.Workflow != nil {
		o.WorkflowUID, o.WorkflowDefinitionEventUID = j.Workflow.UID, j.Workflow.EventUID
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
		if j.Notify == nil {
			p, err := w.parked(ctx, issue)
			if err != nil {
				return nil, err
			}
			if p != nil {
				return nil, &APIError{Status: 409, Message: fmt.Sprintf("Issue %s is parked (%s)", issue.ShortID, p.reason)}
			}
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
	pc, served := w.opt.Config.project(j.ProjectName)
	for env, name := range j.SecretRefs {
		if reservedEnvName(env, envCaseInsensitive) {
			return nil, fmt.Errorf("secret %q would set %s, a reserved name", name, env)
		}
		if !served || !pc.allowsSecret(name) {
			return nil, fmt.Errorf("secret %q is not available to Project %s in worker.toml", name, j.ProjectName)
		}
		value, err := w.opt.Config.readSecret(name)
		if err != nil {
			return nil, err
		}
		out[env] = value
	}
	return out, nil
}

// promptKeptKata are the kata variables a prompt step keeps: the routing and credential names kata's CLI needs to reach
// the worker's daemon. Every other KATA_* name (kata-plugin's steering variables among them) is dropped.
var promptKeptKata = map[string]bool{
	"KATA_HOME": true, "KATA_SERVER": true, "KATA_DAEMON": true, "KATA_AUTH_TOKEN": true, "KATA_AUTH_TOKEN_FILE": true,
	"KATA_TRUST_PRIVATE_NETWORK": true, "KATA_ALLOW_INSECURE": true,
}

// childEnv is the worker's environment for a prompt step (ADR-0009): without TERN_KATA_ internals and every KATA_*
// except the routing and credential names, plus the actor and the step's secrets.
func childEnv(actor string, secrets map[string]string) []string {
	return promptEnv(os.Environ(), actor, secrets, envCaseInsensitive)
}

const envCaseInsensitive = runtime.GOOS == "windows"

// reservedEnvName reports whether name is one the worker keeps for itself: TERN_KATA_* and KATA_*. Names compare
// case-insensitively when caseInsensitive (Windows).
func reservedEnvName(name string, caseInsensitive bool) bool {
	if caseInsensitive {
		name = strings.ToUpper(name)
	}
	return strings.HasPrefix(name, "TERN_KATA_") || strings.HasPrefix(name, "KATA_")
}

func promptEnv(environ []string, actor string, secrets map[string]string, caseInsensitive bool) []string {
	var env []string
	for _, kv := range environ {
		name, _, _ := strings.Cut(kv, "=")
		if caseInsensitive {
			name = strings.ToUpper(name)
		}
		if reservedEnvName(name, caseInsensitive) && !promptKeptKata[name] {
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
	return w.endAttempt(j, j.withCarried(&Outcome{Status: cronjob.RunFailed, Summary: reason}))
}

func (w *Worker) runPrompt(ctx context.Context, j *Journal, sp attemptSpec) error {
	secrets, err := w.secrets(j)
	if err != nil {
		return w.hold(j, HoldSecretUnavailable, err.Error())
	}
	attempt, resumeID := j.attempt(), ""
	in := PromptInput{
		Actor: j.Actor, IssueRef: j.Issue.Ref, IssueTitle: j.Issue.Title, IssueBody: j.Issue.Body, ExistingIssue: j.Issue.Existing,
		JobName: j.JobName, RunUID: j.RunUID, Occurrence: OccurrenceKey(j.Occurrence), Task: sp.task, Attempt: attempt, NewCommits: j.NewCommits,
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
	// tern-kata's omp extension names the run's session to the agent's `kata close`.
	var extension string
	if ompext.IsOmp(program) {
		if extension, err = ompext.Install(sp.dir); err != nil {
			return w.hold(j, HoldExecutionError, "the omp extension could not be written: "+err.Error())
		}
	}
	cmd := exec.Command(program, agentArgs(sp.profile, sp.dir, promptFile, resumeID, extension)...)
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
		return j.markExited(exit, &result)
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
	// The expiry counts from before the agent is launched, not from when its start record is saved: starting a process
	// and saving that record can take long, and the run shows as running from the `launching` mark on.
	deadline := w.clock.Now().Add(w.timeLeft(j))
	if _, err := w.store.Update(j.RunUID, "launching", func(j *Journal) error {
		return j.markLaunching()
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
		return j.markStarted(launch)
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
		//nolint:errcheck // waits for the exit; the run's error is ctx's
		<-done
		return nil, ctx.Err() // the journal stays started: the next start finds it interrupted
	case <-cancel.ch:
		if waitErr, err = w.stopAndWait(ctx, launch, done); err != nil {
			return nil, err
		}
	case <-w.clock.After(deadline.Sub(w.clock.Now())):
		timedOut = true
		if waitErr, err = w.stopAndWait(ctx, launch, done); err != nil {
			return nil, err
		}
	}
	exit := &ExitRecord{At: w.clock.Now().UTC(), TimedOut: timedOut}
	exit.Code, exit.Signal = exitStatus(cmd, waitErr)
	return exit, nil
}

// exitStatus is the code and signal the process ended with. Wait returns ErrWaitDelay when the process exited but a
// descendant kept its output pipes open: that is the process's own exit, not a failure to wait.
func exitStatus(cmd *exec.Cmd, waitErr error) (code int, signal string) {
	if ee, ok := errors.AsType[*exec.ExitError](waitErr); ok {
		return ee.ExitCode(), exitSignal(ee.ProcessState)
	}
	if errors.Is(waitErr, exec.ErrWaitDelay) && cmd.ProcessState != nil {
		return cmd.ProcessState.ExitCode(), exitSignal(cmd.ProcessState)
	}
	if waitErr != nil {
		return -1, ""
	}
	return 0, ""
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

func (w *Worker) conclude(ctx context.Context, j *Journal) error {
	command := j.activeIsCommand()
	var proof GroupProof
	if j.Launch != nil {
		proof = w.life(j).Group(j.Launch)
	}
	if command && j.Launch != nil && j.Cancel == nil && !j.Exit.TimedOut && !proof.Gone() {
		// A command that ended by itself leaves a process that is not the worker's to guess about: it may still be doing
		// what the command began. One the worker stopped (timeout, abandon) is stopped completely below.
		return w.hold(j, HoldGroupUnproven, exitedHold("command", j.Launch.PGID, proof))
	}
	if j.Launch != nil && !proof.Gone() {
		proof, err := w.terminateGroup(ctx, j.Launch)
		if err != nil {
			return err
		}
		w.noteGroup(j.RunUID, j.Launch, proof)
		if !proof.Gone() {
			return w.hold(j, HoldGroupUnproven, exitedHold("agent", j.Launch.PGID, proof))
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
		var tail string // a log that lost output has no tail to put in a summary (ADR-0008)
		if j.Exit.Unrecorded == "" {
			tail = commandTail1K(w.attemptDir(j))
		}
		outcome = classifyCommand(j, tail)
	} else {
		outcome = j.withCarried(classify(j))
	}
	return w.endAttempt(j, outcome)
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
	o := &Outcome{InputTokens: r.InputTokens, OutputTokens: r.OutputTokens, CostUSD: r.CostUSD}
	if reason == "" {
		o.Status, o.Summary = cronjob.RunSucceeded, cmp.Or(final, "(the agent gave no final message)")
	} else {
		o.Status, o.Summary = cronjob.RunFailed, reason
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
	w.noteOutcome(j)
	w.noteCommit(j)
	if err := w.checkRoute(ctx, ps, false); err != nil {
		return fmt.Errorf("cannot confirm the kata route: %w", err)
	}
	var planErr error // why the comment and label could not be planned yet
	if len(j.Intents) == 0 || j.IntentsPartial {
		var intents []Intent
		intents, planErr = w.planIntents(ctx, j)
		var err error
		if j, err = w.store.Update(j.RunUID, "intents-planned", func(j *Journal) error {
			j.Intents, j.IntentsPartial = intents, planErr != nil
			return nil
		}); err != nil {
			return err
		}
	}
	for i := range j.Intents {
		if j.Intents[i].Done || j.Intents[i].Inspect != "" {
			continue
		}
		var err error
		if j, err = w.deliver(ctx, j, i); err != nil {
			return err
		}
	}
	if j.IntentsPartial {
		return fmt.Errorf("cannot tell whether the run's Issue is still open: %w", planErr)
	}
	for _, it := range j.Intents {
		if !it.Done && it.Inspect == "" {
			return fmt.Errorf("%s not delivered yet: %s", it.Kind, it.Error)
		}
	}
	for _, why := range j.Inspections() {
		w.opt.Log("run %s: needs inspection: %s", j.RunUID, why)
	}
	_, err := w.store.Update(j.RunUID, "settled", func(j *Journal) error { return j.settle() })
	return err
}

// planIntents is what the run owes kata: the final observation, planned first and never waiting for an Issue read, then
// the comment and label the closure check calls for, which do. A failed Issue read returns the intents so far and the
// error; the caller saves them as partial and plans the rest again.
func (w *Worker) planIntents(ctx context.Context, j *Journal) ([]Intent, error) {
	intents := []Intent{{Kind: IntentFinal}}
	if j.IntentsPartial {
		intents = slices.Clone(j.Intents)
	}
	if j.Notify != nil { // a notify run's only record is the Run itself; the Issue gets no comment, label or request
		return intents[:1], nil
	}
	issueOpen := true
	if j.Outcome.Status == cronjob.RunSucceeded && !j.Issue.Existing {
		issue, err := w.opt.Kata.ShowIssue(ctx, j.ProjectID, j.Issue.UID)
		if err != nil {
			return intents, err
		}
		issueOpen = issue.Open()
	}
	switch j.Outcome.Status {
	case cronjob.RunSucceeded:
		if quietSuccess(j) {
			break // every step was a command that exited 0 on an existing Issue: the Run says it all
		}
		if j.Issue.Existing || issueOpen {
			intents = append(intents, Intent{Kind: IntentComment, Key: j.commentKey("succeeded"), Text: j.Outcome.Summary})
			if !j.Issue.Existing {
				intents = append(intents,
					Intent{Kind: IntentLabel, Text: "needs-review"},
					Intent{Kind: IntentNotify, Text: reviewNotice(j)})
			}
		}
	case cronjob.RunFailed:
		intents = append(intents, Intent{Kind: IntentComment, Key: j.commentKey("failed"), Text: "The run failed: " + j.Outcome.Summary})
	case cronjob.RunUnknown:
		intents = append(intents,
			Intent{Kind: IntentComment, Key: j.commentKey("held"), Text: "The run is held: " + j.Outcome.holdLine() + "\n\n" + resumeOrAbandon(j.RunUID)},
			Intent{Kind: IntentNotify, Text: heldNotice(j)})
	case cronjob.RunCancelled:
		intents = append(intents, Intent{Kind: IntentComment, Key: j.commentKey("cancelled"), Text: "The run was abandoned: " + j.Outcome.Summary})
	}
	return intents, nil
}

func (w *Worker) deliver(ctx context.Context, j *Journal, i int) (*Journal, error) {
	var err error
	if it := j.Intents[i]; it.Kind == IntentNotify && it.Recipient == "" {
		if j, err = w.resolveRecipient(ctx, j, i); err == nil && j.Intents[i].Done {
			return j, nil // the Issue names nobody to tell
		}
	}
	var revision int64
	if err == nil {
		revision, err = w.send(ctx, j, j.Intents[i])
	}
	return w.store.Update(j.RunUID, "intent-"+string(j.Intents[i].Kind), func(j *Journal) error {
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
	return truncate(fmt.Sprintf("Run %s of %q on %s is held: %s %s", j.RunUID, j.JobName, j.Issue.Ref, j.Outcome.holdLine(), resumeOrAbandon(j.RunUID)), maxNotifyBytes)
}

// reviewNotice is the Attention request for a succeeded run that left its Issue open, saved in its intent so a retry sends
// the same words.
func reviewNotice(j *Journal) string {
	return truncate(fmt.Sprintf("Run %s of %q succeeded but left %s open: it needs review.", j.RunUID, j.JobName, j.Issue.Ref), maxNotifyBytes)
}

// resolveRecipient reads the Issue and saves who its owner or author is in the notify intent, once: from then on a
// retry sends to that address whatever the Issue says.
func (w *Worker) resolveRecipient(ctx context.Context, j *Journal, i int) (*Journal, error) {
	issue, err := w.opt.Kata.ShowIssue(ctx, j.ProjectID, j.Issue.UID)
	if err != nil {
		return j, err
	}
	recipient := cmp.Or(issue.Owner, issue.Author)
	updated, err := w.store.Update(j.RunUID, "intent-notify-resolved", func(j *Journal) error {
		j.Intents[i].Recipient, j.Intents[i].Done = recipient, recipient == ""
		return nil
	})
	if err != nil {
		return j, err // the caller keeps the journal it had
	}
	return updated, nil
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
	if current.Status == j.Outcome.Status && current.Summary == j.Outcome.Summary {
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

// ansiSeq matches the escape sequences a colored stderr carries: CSI, OSC and any other ESC, intermediates, final
// escape. An OSC body stops at newline, so an unterminated one cannot swallow the text after it.
var ansiSeq = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07\x1b\n]*(?:\x07|\x1b\\)?|\x1b[ -/]*[0-~]`)

// plainText strips escape sequences and control bytes: they would cost up to six bytes each as JSON escapes in the
// journal, which is bounded.
func plainText(s string) string {
	s = ansiSeq.ReplaceAllString(strings.ToValidUTF8(s, ""), "")
	s = strings.Map(func(r rune) rune {
		if r != '\n' && r != '\t' && unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}

// notify is a notify run's whole work: send the inbox request to the recipient on the run's Issue and record the
// outcome. The request is worked out from the Issue once and saved in the journal, so a retry or a restart sends the
// same words to the same address; before sending, the Issue is read, and a request already there counts as sent
// (ADR-0006 "read-back"). The run's identity (ADR-0002) already keeps a restart from preparing the occurrence twice.
func (w *Worker) notify(ctx context.Context, j *Journal) error {
	finish := func(status cronjob.RunStatus, summary string) error {
		return w.finish(j.RunUID, &Outcome{Status: status, Summary: truncate(summary, maxSummaryBytes)})
	}
	notice := j.Notify.Resolved
	if notice == nil {
		issue, err := w.opt.Kata.ShowIssue(ctx, j.ProjectID, j.Issue.UID)
		if err != nil {
			if refused(err) {
				return finish(cronjob.RunFailed, "cannot read the Issue to notify about: "+err.Error())
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
		return finish(cronjob.RunFailed, "the Issue has no owner or author to notify")
	}
	if err := w.sendNotice(ctx, j, notice.Recipient, notice.Message); err != nil {
		if refused(err) {
			return finish(cronjob.RunFailed, "kata refused the notification: "+err.Error())
		}
		return err
	}
	return finish(cronjob.RunSucceeded, "Notified "+notice.Recipient+": "+notice.Message)
}

const (
	currentOwnerOrAuthor = "current-owner-or-author"
	maxNotifyBytes       = 1024
)
