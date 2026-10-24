package worker

import (
	"context"
	"sync"
	"time"
)

// GroupState is the answer of a Group proof.
type GroupState int

const (
	// GroupGone: no process of the run's group is left.
	GroupGone GroupState = iota
	// GroupPresent: at least one process of the group is still there.
	GroupPresent
	// GroupUnchecked: the platform could not tell; GroupProof.Reason says why.
	GroupUnchecked
)

// GroupProof is whether a run's process group is gone: gone, present, or cannot be checked (with the reason). The
// platform adapters answer it (checkGroup); the Unix adapter never answers GroupUnchecked, the Windows adapter does
// whenever opening or querying the Job Object fails for a reason other than "no such job".
type GroupProof struct {
	State GroupState
	// Reason is why the group could not be checked; set only for GroupUnchecked.
	Reason string
}

// Gone is the settlement test every caller applies: a group that cannot be checked is not proven gone.
func (p GroupProof) Gone() bool { return p.State == GroupGone }

// The one process group termination policy (ADR-0006): SIGTERM, SIGKILL termGrace later on the Worker's clock, then a
// bounded wait of killWait for the group proof. On Windows both signals end the Job Object.
const (
	termGrace = 10 * time.Second
	killWait  = 20 * time.Second
	groupPoll = 200 * time.Millisecond
)

// proveGroup is the group proof of a launch: the platform adapter's answer, unless a test hook says the group holds out.
func (w *Worker) proveGroup(l *LaunchRecord) GroupProof {
	if hold := w.opt.Hooks.GroupHoldsOut; hold != nil && hold(l) {
		return GroupProof{State: GroupPresent}
	}
	return checkGroup(l)
}

// uncheckedLog remembers, per run, the reason its group last could not be checked.
type uncheckedLog struct {
	mu     sync.Mutex
	reason map[string]string
}

// changed records proof as the latest answer for the run and reports whether it is a cannot-be-checked answer whose
// reason differs from the run's previous one. Any other answer forgets the reason, so a check that fails again later
// is said again.
func (u *uncheckedLog) changed(runUID string, proof GroupProof) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	if proof.State != GroupUnchecked {
		delete(u.reason, runUID)
		return false
	}
	if prev, ok := u.reason[runUID]; ok && prev == proof.Reason {
		return false
	}
	if u.reason == nil {
		u.reason = map[string]string{}
	}
	u.reason[runUID] = proof.Reason
	return true
}

// noteGroup logs a group that cannot be checked, once per run per change of reason.
func (w *Worker) noteGroup(runUID string, l *LaunchRecord, proof GroupProof) {
	if w.unchecked.changed(runUID, proof) {
		w.opt.Log("run %s: process group %d cannot be checked: %s; the run keeps its slot and overlap gate until the check succeeds", runUID, l.PGID, proof.Reason)
	}
}

// terminateGroup stops the run's process group by the one policy and returns the proof it ended with, not gone only
// when the group outlived the bounded wait. A ctx that ends first kills the group at once (a stop now) and returns
// ctx's error.
func (w *Worker) terminateGroup(ctx context.Context, l *LaunchRecord) (GroupProof, error) {
	grace := w.clock.After(termGrace) // taken before the signal, so a test clock never misses it
	if ctx.Err() == nil {
		signalGroup(l, false)
	}
	var deadline <-chan time.Time
	for {
		if err := ctx.Err(); err != nil {
			signalGroup(l, true)
			return w.proveGroup(l), err
		}
		proof := w.proveGroup(l)
		if proof.Gone() {
			return proof, nil
		}
		select {
		case <-ctx.Done():
		case <-grace:
			signalGroup(l, true)
			grace, deadline = nil, w.clock.After(killWait)
		case <-deadline:
			return proof, nil
		case <-w.clock.After(groupPoll):
		}
	}
}
