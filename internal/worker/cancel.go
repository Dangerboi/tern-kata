package worker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// errRunCancelled stops an attempt that was about to start: a cancel request was saved first, so nothing is started.
var errRunCancelled = errors.New("the run was cancelled before its process started")

// cancelSignal wakes the goroutine supervising a run when a cancel request for it is saved.
type cancelSignal struct {
	ch   chan struct{}
	once sync.Once
}

func (c *cancelSignal) fire() { c.once.Do(func() { close(c.ch) }) }

// cancelSignalOf is the run's signal, created on first use.
func (w *Worker) cancelSignalOf(uid string) *cancelSignal {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancels == nil {
		w.cancels = map[string]*cancelSignal{}
	}
	if w.cancels[uid] == nil {
		w.cancels[uid] = &cancelSignal{ch: make(chan struct{})}
	}
	return w.cancels[uid]
}

func (w *Worker) dropCancelSignal(uid string) {
	w.mu.Lock()
	delete(w.cancels, uid)
	w.mu.Unlock()
}

// cancelRefusal is why a run that is not held cannot be cancelled now, empty when it can: it is admitted or running
// with its process not yet reported over, and nobody asked yet.
func cancelRefusal(j *Journal) string {
	switch {
	case j.Cancel != nil:
		return fmt.Sprintf("%s already asked to cancel it", j.Cancel.By)
	case j.Outcome != nil || j.Phase == PhaseFinished || j.Phase == PhaseSettled:
		return "it is " + runStatus(j).State + ", not held"
	case j.Notify != nil:
		return "its notification is being sent and it ends in a moment"
	case j.Phase == PhasePrepared:
		return "it is still being admitted; try again in a moment"
	case j.Phase == PhaseResuming:
		return "its resume is still being confirmed; try again in a moment"
	case j.Exit != nil:
		return "its process has already ended and its outcome is being decided"
	}
	return ""
}

// cancelRunning asks a running run to stop (ADR-0008 "Cancellation"): the request is saved first, then the process
// group gets SIGTERM and SIGKILL after 10 s, and the run ends `cancelled` once the group is proven gone.
func (w *Worker) cancelRunning(ps *projectState, uid, by string) (*Journal, error) {
	updated, err := w.store.Update(uid, "cancel-requested", func(j *Journal) error {
		if reason := cancelRefusal(j); reason != "" {
			return fmt.Errorf("run %s cannot be abandoned: %s", uid, reason)
		}
		j.Cancel = &CancelRecord{By: by, At: w.clock.Now().UTC()}
		return nil
	})
	if err != nil {
		w.noteHook(err)
		return nil, err
	}
	w.cancelSignalOf(uid).fire()
	w.spawn(ps, uid) // a run waiting to start has a goroutine only when something drives it
	return updated, nil
}

// concludeCancel ends a cancelled run: its outcome is `cancelled` and the steps that never ran are skipped. The
// tokens the stopped attempt and its resumed predecessors used are kept.
func (w *Worker) concludeCancel(j *Journal) error {
	_, err := w.store.Update(j.RunUID, "cancelled", func(j *Journal) error {
		if j.Cancel == nil {
			return errors.New("no cancel request is saved")
		}
		now := w.clock.Now().UTC()
		o := &Outcome{Status: StatusCancelled, Summary: fmt.Sprintf("abandoned by %s (was running)", j.Cancel.By), At: now}
		if j.Result != nil {
			o.InputTokens, o.OutputTokens = j.Result.InputTokens, j.Result.OutputTokens
		}
		j.withCarried(o)
		if j.Workflow != nil {
			for i := range j.Steps {
				switch r := &j.Steps[i]; {
				case r.State == StepRunning:
					r.State, r.Summary, r.EndedAt = StepFailed, "cancelled by "+j.Cancel.By, now
					r.Launch, r.Exit = j.Launch, j.Exit
					r.InputTokens, r.OutputTokens = addTokens(r.InputTokens, o.InputTokens), addTokens(r.OutputTokens, o.OutputTokens)
				case !terminalStep(r.State):
					r.State, r.Summary = StepSkipped, "skipped: the run was cancelled"
				}
			}
			o.InputTokens, o.OutputTokens = sumStepTokens(j.Steps)
		}
		j.CarriedInput, j.CarriedOutput = nil, nil
		j.Outcome, j.Phase = o, PhaseFinished
		return nil
	})
	return err
}

// terminateGroup signals the process group SIGTERM, then SIGKILL after 10 s, and waits for the process to end.
// waitErr is what the process answered; an error means the worker stopped first (the journal stays `started`).
func (w *Worker) terminateGroup(ctx context.Context, launch *LaunchRecord, done <-chan error) (waitErr error, err error) {
	signalGroup(launch, false)
	select {
	case waitErr = <-done:
	case <-ctx.Done():
		signalGroup(launch, true)
		<-done
		return nil, ctx.Err()
	case <-w.clock.After(10 * time.Second):
		signalGroup(launch, true)
		waitErr = <-done
	}
	return waitErr, nil
}
