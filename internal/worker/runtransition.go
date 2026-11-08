package worker

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

// The Run lifecycle's transitions (ADR-0005, ADR-0006): the only code that writes a Journal's Phase or its Outcome.
// Each is applied to the Journal inside the store's update, so a refused transition is a failed update and saves
// nothing. The legal set is what the Worker does: a transition asked from any other Phase returns an error and leaves
// the Journal as it was.
//
//	hold, finish, concludeStep   from admitted, launching, started
//	concludeCancel               from admitted, started
//	finishWorkflow, timeOutWorkflow   from admitted
//	holdBeforeAdmission, admit   from prepared
//	reopen                       from resuming
//	markLaunching                from admitted
//	markStarted                  from launching
//	markExited                   from started
//	settle                       from finished
//	acceptResume                 from a settled hold that may resume (mayResume)
//	abandon                      from a settled hold that may be abandoned (mayAbandon)
//	requestCancel                from a Run that mayCancel

// needs refuses a transition asked from a Phase outside from.
func (j *Journal) needs(transition string, from ...Phase) error {
	if slices.Contains(from, j.Phase) {
		return nil
	}
	return fmt.Errorf("run %s cannot %s: it is %s", j.RunUID, transition, j.Phase)
}

// inFlight are the Phases in which a Run's attempt is live: its outcome may be decided from any of them.
var inFlight = []Phase{PhaseAdmitted, PhaseLaunching, PhaseStarted}

// hold records an uncertain outcome: kata will get `unknown` with the summary `reason: <code>` and the detail on the next
// line (ADR-0005).
func (j *Journal) hold(reason, detail string, at time.Time) error {
	if err := j.needs("be held", inFlight...); err != nil {
		return err
	}
	j.Outcome = &Outcome{Status: cronjob.RunUnknown, Hold: reason, Summary: cronjob.ReasonMessage(reason, detail), At: at}
	j.Phase = PhaseFinished
	return nil
}

// holdBeforeAdmission ends a Run before kata saw it: no Outcome status, nothing to report, so it is settled at once.
func (j *Journal) holdBeforeAdmission(reason, detail string, at time.Time) error {
	if err := j.needs("be held before admission", PhasePrepared); err != nil {
		return err
	}
	j.Outcome = &Outcome{Hold: reason, Summary: cronjob.ReasonMessage(reason, detail), At: at}
	j.Phase = PhaseSettled
	return nil
}

// admit records that kata accepted the revision-0 running observation.
func (j *Journal) admit(revision int64) error {
	if err := j.needs("be admitted", PhasePrepared); err != nil {
		return err
	}
	j.Phase, j.Revision = PhaseAdmitted, revision
	return nil
}

// reopen records that kata holds the running observation of an accepted resume.
func (j *Journal) reopen(revision int64) error {
	if err := j.needs("be reopened", PhaseResuming); err != nil {
		return err
	}
	j.Phase, j.Revision = PhaseAdmitted, revision
	return nil
}

// markLaunching is the mark saved before the process is started: a record of an earlier attempt must not stand for
// this one's process. A Run asked to cancel is not launched.
func (j *Journal) markLaunching() error {
	if err := j.needs("start launching", PhaseAdmitted); err != nil {
		return err
	}
	if j.Cancel != nil {
		return errRunCancelled
	}
	j.Phase = PhaseLaunching
	j.Launch = nil
	return nil
}

// markStarted saves the start record of the launched process.
func (j *Journal) markStarted(launch *LaunchRecord) error {
	if err := j.needs("be started", PhaseLaunching); err != nil {
		return err
	}
	j.Phase = PhaseStarted
	j.Launch = launch
	return nil
}

// markExited saves the exit record of the attempt's process and, for an agent, what its output said.
func (j *Journal) markExited(exit *ExitRecord, result *AgentResult) error {
	if err := j.needs("record an exit", PhaseStarted); err != nil {
		return err
	}
	j.Exit = exit
	if result != nil {
		j.Result = result
	}
	return nil
}

// finish records the Run's decided Outcome; only its reports remain.
func (j *Journal) finish(o *Outcome, at time.Time) error {
	if err := j.needs("finish", inFlight...); err != nil {
		return err
	}
	o.At = at
	j.Outcome, j.Phase = o, PhaseFinished
	return nil
}

// settle records that every Issue write and the final observation have been sent.
func (j *Journal) settle() error {
	if err := j.needs("settle", PhaseFinished); err != nil {
		return err
	}
	j.Phase = PhaseSettled
	return nil
}

// requestCancel saves the request to stop a Run that is admitted or running; it is concluded later.
func (j *Journal) requestCancel(by string, at time.Time) error {
	if reason := j.mayCancel(); reason != "" {
		return fmt.Errorf("run %s cannot be abandoned: %s", j.RunUID, reason)
	}
	j.Cancel = &CancelRecord{By: by, At: at}
	return nil
}

// concludeCancel ends a cancelled Run: its outcome is `cancelled` and the steps that never ran are skipped.
func (j *Journal) concludeCancel(at time.Time) error {
	if err := j.needs("conclude a cancel", PhaseAdmitted, PhaseStarted); err != nil {
		return err
	}
	if j.Cancel == nil {
		return errors.New("no cancel request is saved")
	}
	o := &Outcome{Status: cronjob.RunCancelled, Summary: fmt.Sprintf("abandoned by %s (was running)", j.Cancel.By), At: at}
	if j.Result != nil {
		o.InputTokens, o.OutputTokens, o.CostUSD = j.Result.InputTokens, j.Result.OutputTokens, j.Result.CostUSD
	}
	j.withCarried(o)
	if j.Workflow != nil {
		for i := range j.Steps {
			switch r := &j.Steps[i]; {
			case r.State == StepRunning:
				r.State, r.Summary, r.EndedAt = StepFailed, "cancelled by "+j.Cancel.By, at
				r.Launch, r.Exit = j.Launch, j.Exit
				r.addUsage(o)
			case !terminalStep(r.State):
				r.State, r.Summary = StepSkipped, "skipped: the run was cancelled"
			}
		}
		o.sumSteps(j.Steps)
	}
	j.CarriedInput, j.CarriedOutput, j.CarriedCost = nil, nil, nil
	j.Outcome, j.Phase = o, PhaseFinished
	return nil
}

// concludeStep records how the current step's attempt ended and what comes next: the next step, a retry after a
// certain failure, or the end of the run. The attempt's start record and receipt move into the step's record.
func (j *Journal) concludeStep(outcome *Outcome, at time.Time) error {
	if err := j.needs("conclude a step", inFlight...); err != nil {
		return err
	}
	i := j.activeStep()
	if i < 0 {
		return errors.New("no step is running")
	}
	r, plan := &j.Steps[i], j.Workflow.Steps[i]
	r.Launch, r.Exit, r.Summary, r.EndedAt = j.Launch, j.Exit, outcome.Summary, at
	if j.Result != nil {
		r.StderrTail = j.Result.StderrTail
	}
	// A retried step keeps what its earlier attempts used; outcome already carries what a resume continued.
	r.addUsage(outcome)
	timedOut := j.Exit != nil && j.Exit.TimedOut
	switch {
	case outcome.Status == cronjob.RunSucceeded:
		r.State, r.LastFailure = StepSucceeded, ""
	case timedOut:
		// The expiry is the run's, shared by all steps and retries: nothing else would get time to run.
		r.State = StepFailed
		j.skipUnfinished(i + 1)
	case r.Tries <= plan.Retries:
		r.State, r.LastFailure = StepPending, outcome.Summary
	default:
		r.State = StepFailed
	}
	j.Launch, j.Exit, j.Result, j.Resume = nil, nil, nil, nil
	j.CarriedInput, j.CarriedOutput, j.CarriedCost = nil, nil, nil
	j.Phase = PhaseAdmitted
	return nil
}

// skipUnfinished skips every step from index from on that never reached a final state, because the run timed out.
func (j *Journal) skipUnfinished(from int) {
	for i := from; i < len(j.Steps); i++ {
		if !terminalStep(j.Steps[i].State) {
			j.Steps[i].State, j.Steps[i].Summary = StepSkipped, "skipped: the run timed out"
		}
	}
}

// addUsage adds the tokens and cost of an attempt's outcome to the step's own.
func (r *StepRecord) addUsage(o *Outcome) {
	r.InputTokens, r.OutputTokens = addTokens(r.InputTokens, o.InputTokens), addTokens(r.OutputTokens, o.OutputTokens)
	r.CostUSD = addCost(r.CostUSD, o.CostUSD)
}

// sumSteps sets the outcome's tokens and cost to what every attempt of every step used.
func (o *Outcome) sumSteps(steps []StepRecord) {
	o.InputTokens, o.OutputTokens = sumStepTokens(steps)
	o.CostUSD = sumStepCost(steps)
}

// finishWorkflow decides the Run's outcome once every step is succeeded, failed or skipped: failed when any step failed.
func (j *Journal) finishWorkflow(at time.Time) error {
	if err := j.needs("finish a workflow", PhaseAdmitted); err != nil {
		return err
	}
	status := cronjob.RunSucceeded
	for _, r := range j.Steps {
		if r.State == StepFailed {
			status = cronjob.RunFailed
		}
	}
	o := &Outcome{Status: status, Summary: truncate(workflowSummary(j, status), maxSummaryBytes), At: at}
	o.sumSteps(j.Steps)
	j.Outcome, j.Phase = o, PhaseFinished
	return nil
}

// timeOutWorkflow ends a Run whose expiry passed before its next step started: every step that never ran is skipped
// and the Run fails as timed out, with no process started.
func (j *Journal) timeOutWorkflow(at time.Time) error {
	if err := j.needs("time out a workflow", PhaseAdmitted); err != nil {
		return err
	}
	j.skipUnfinished(0)
	reason := fmt.Sprintf("timed out after %s", time.Duration(j.TimeoutSeconds)*time.Second)
	o := &Outcome{Status: cronjob.RunFailed, At: at}
	o.Summary = truncate(reason+"\n\n"+workflowSummary(j, cronjob.RunFailed), maxSummaryBytes)
	o.sumSteps(j.Steps)
	j.Outcome, j.Phase = o, PhaseFinished
	return nil
}

// resumeTerms is what accepting a resume needs from outside the Journal: who asked, the lease generation it is
// accepted under, the fresh expiry of a workflow, and the probes for the held attempt's tokens and omp session.
type resumeTerms struct {
	By         string
	Generation int64
	At         time.Time
	Tokens     func(*Journal) (in, out *int64)
	Cost       func(*Journal) *float64
	SessionID  func(*Journal) string
}

// acceptResume moves a held Run whose hold is reported and whose process is gone back to work: the held attempt's
// tokens are carried, the attempt counted, and the Run waits for kata to confirm its running observation.
func (l lifecycle) acceptResume(j *Journal, t resumeTerms) error {
	if reason := l.mayResume(j); reason != "" {
		return fmt.Errorf("run %s cannot be resumed: %s", j.RunUID, reason)
	}
	in, out := t.Tokens(j)
	j.CarriedInput, j.CarriedOutput = addTokens(j.CarriedInput, in), addTokens(j.CarriedOutput, out)
	j.CarriedCost = addCost(j.CarriedCost, t.Cost(j))
	j.Attempt = j.attempt() + 1
	j.Resume = &ResumeRecord{Attempt: j.Attempt, SessionID: t.SessionID(j), By: t.By, At: t.At, Hold: j.Outcome.Hold, Detail: j.Outcome.holdDetail()}
	j.Generation = t.Generation
	if j.Workflow != nil { // one fresh expiry from the frozen timeout, from the moment the resume is accepted
		j.ExpiresAt = t.At.Add(time.Duration(j.TimeoutSeconds) * time.Second)
	}
	if i := j.activeStep(); i >= 0 {
		j.Steps[i].StderrTail = "" // the resumed attempt writes its own
	}
	j.Outcome, j.Exit, j.Result, j.Intents = nil, nil, nil, nil
	j.Phase = PhaseResuming
	return nil
}

// abandon ends a held Run the user no longer wants as `cancelled`. A hold from before admission was never seen by
// kata, so nothing is reported and it is settled at once.
func (l lifecycle) abandon(j *Journal, by string, at time.Time) error {
	if reason := l.mayAbandon(j); reason != "" {
		return fmt.Errorf("run %s cannot be abandoned: %s", j.RunUID, reason)
	}
	if !j.held() {
		return fmt.Errorf("run %s cannot be abandoned: it is %s, not held", j.RunUID, j.stateName())
	}
	beforeAdmission := j.holdClass() == holdBeforeAdmission
	held := j.Outcome
	j.Outcome = &Outcome{Status: cronjob.RunCancelled, Summary: fmt.Sprintf("abandoned by %s (was held: %s)", by, held.holdLine()), At: at, CostUSD: held.CostUSD}
	j.Intents = nil
	j.Phase = PhaseFinished
	if beforeAdmission {
		j.Phase = PhaseSettled
	}
	return nil
}
