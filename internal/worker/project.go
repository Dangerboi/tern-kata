package worker

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"
)

// jitter is a random duration between lo and hi.
func jitter(lo, hi time.Duration) time.Duration {
	if hi <= lo {
		return lo
	}
	return lo + rand.N(hi-lo)
}

// sleep waits d on the worker's clock; false means ctx ended first.
func (w *Worker) sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-w.clock.After(d):
		return true
	}
}

// serveProject is one Project's loop: resolve it, take or renew its lease, follow its events, refresh its jobs and
// recover its journals (ADR-0003, ADR-0004). It wakes at least every minute and recomputes what is due, so a sleeping
// laptop or a changed clock is noticed.
func (w *Worker) serveProject(ctx context.Context, ps *projectState) {
	var (
		retry     = 10 * time.Second
		nextLease time.Time
		nextDefs  time.Time
		dirtyAt   time.Time
		refreshed time.Time
		dirty     bool
		recovered bool
		started   bool
		takeover  = w.opt.Takeover
		nextRoute time.Time
	)
	starting, ok := w.beginStarting(ctx)
	if !ok {
		return
	}
	defer starting()
	for ctx.Err() == nil {
		now := w.clock.Now()
		if !ps.resolved {
			blocked, err := w.resolve(ctx, ps)
			w.mu.Lock()
			switch {
			case err != nil:
				ps.blocked = "kata could not be asked: " + err.Error()
			default:
				ps.blocked = blocked
			}
			ps.resolved = err == nil && blocked == ""
			w.mu.Unlock()
			if !ps.resolved {
				starting() // a Project kata cannot answer for yet waits without holding a start-up place
				wait := jitter(retry/2, retry)
				if err == nil {
					wait = jitter(2*time.Minute+30*time.Second, 5*time.Minute)
				}
				retry = min(retry*2, 5*time.Minute)
				if !w.sleep(ctx, wait) {
					return
				}
				continue
			}
			retry = 10 * time.Second
		}
		if !now.Before(nextLease) {
			state := claimLease(ctx, w.opt.Kata, w.clock, w.opt.Config.Executor, ps.actor, ps.project.ID, takeover)
			takeover = false
			w.mu.Lock()
			ps.lease, ps.leaseAt = state, now
			w.mu.Unlock()
			switch {
			case state.Held:
				nextLease, retry = now.Add(LeaseRenew), 10*time.Second
			case state.Unknown:
				nextLease = now.Add(jitter(retry/2, retry))
				retry = min(retry*2, 5*time.Minute)
				w.opt.Log("Project %s: %s", ps.cfg.Name, state.Why())
			default:
				nextLease = now.Add(jitter(3*time.Minute, 5*time.Minute))
				w.opt.Log("Project %s: %s", ps.cfg.Name, state.Why())
			}
			if !recovered && !state.Unknown {
				recovered = true
				w.recoverProject(ctx, ps)
			}
			starting()
		}
		if ps.resolved {
			if nextRoute.IsZero() {
				nextRoute = now.Add(routeIdle)
			}
			if at, failing := w.routeFailing(ps); failing {
				nextRoute = at
			}
			if !now.Before(nextRoute) {
				_ = w.checkRoute(ctx, ps, true)
				nextRoute = now.Add(routeIdle)
				if at, failing := w.routeFailing(ps); failing {
					nextRoute = at
				}
			}
		}
		if w.leaseHeld(ps) {
			if !started {
				started = true
				w.runs.Add(1)
				go func() { defer w.runs.Done(); w.follow(ctx, ps) }()
			}
			select {
			case <-ps.wake:
				dirty, dirtyAt = true, w.clock.Now()
			default:
			}
			due := nextDefs.IsZero() || !now.Before(nextDefs)
			if dirty && !now.Before(maxTime(dirtyAt.Add(250*time.Millisecond), refreshed.Add(2*time.Second))) {
				due = true
			}
			if due {
				dirty = false
				refreshed = now
				if err := w.refreshJobs(ctx, ps); err != nil {
					w.opt.Log("Project %s: %v", ps.cfg.Name, err)
					dirty = true // dirty flags survive failed refreshes
				}
				nextDefs = w.clock.Now().Add(w.refreshEvery(ps))
			}
		}
		if ps.resolved && !w.halted.Load() {
			w.retryPending(ps)
		}
		wake := minTime(now.Add(time.Minute), nextLease)
		if w.leaseHeld(ps) {
			if next := w.tick(ctx, ps); !next.IsZero() {
				wake = minTime(wake, next)
			}
		}
		if !nextDefs.IsZero() && w.leaseHeld(ps) {
			wake = minTime(wake, nextDefs)
		}
		if dirty {
			wake = minTime(wake, maxTime(dirtyAt.Add(250*time.Millisecond), refreshed.Add(2*time.Second)))
		}
		if ps.resolved {
			wake = minTime(wake, nextRoute)
		}
		select {
		case <-ctx.Done():
			return
		case <-ps.wake:
			dirty, dirtyAt = true, w.clock.Now()
		case <-ps.kick:
		case <-w.clock.After(max(wake.Sub(w.clock.Now()), time.Millisecond)):
		}
	}
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// refreshEvery is the fallback definition refresh: 15 to 18 minutes, or 2.5 to 5 for a Project with no adapter jobs.
func (w *Worker) refreshEvery(ps *projectState) time.Duration {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, j := range ps.jobs {
		if !j.IsNative() {
			return jitter(15*time.Minute, 18*time.Minute)
		}
	}
	return jitter(150*time.Second, 5*time.Minute)
}

// refreshJobs re-reads the Project's live jobs. Native jobs stay in the cache (so a run request can name them in its
// refusal) but are never run.
func (w *Worker) refreshJobs(ctx context.Context, ps *projectState) error {
	jobs, err := w.opt.Kata.Jobs(ctx, ps.project.ID)
	var aside, warnings []JobNote
	if err == nil {
		aside, warnings = definitionNotes(ctx, w.opt.Kata, w.opt.Config, ps.cfg, ps.project.ID, jobs, w.clock.Now())
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err != nil {
		ps.jobsErr = err.Error()
		return fmt.Errorf("listing jobs: %w", err)
	}
	ps.jobs = make(map[string]Job, len(jobs))
	for _, j := range jobs {
		ps.jobs[j.UID] = j
	}
	ps.aside, ps.warnings = aside, warnings
	ps.jobsErr, ps.jobsAt = "", w.clock.Now()
	return nil
}

// follow keeps one event stream open for the Project and wakes the loop when a job or workflow changed (ADR-0004).
// The saved cursor is a hint: nothing is admitted or settled because of it.
func (w *Worker) follow(ctx context.Context, ps *projectState) {
	backoff := 500 * time.Millisecond
	cursor := w.readCursor(ps.project.UID)
	for ctx.Err() == nil {
		if err := w.checkRoute(ctx, ps, true); err != nil {
			at, _ := w.routeFailing(ps)
			if !w.sleep(ctx, max(at.Sub(w.clock.Now()), time.Second)) {
				return
			}
			continue
		}
		var opened time.Time
		delivered := false
		err := w.opt.Kata.Stream(ctx, ps.project.ID, cursor, func() {
			opened = w.clock.Now()
			w.mu.Lock()
			ps.streamOpen = true
			w.mu.Unlock()
		}, func(e Event) {
			delivered = true
			cursor = max(cursor, e.ID)
			if strings.HasPrefix(e.Type, "issue.") {
				ps.datesDirty.Store(true)
				w.kick(ps)
			}
			if strings.HasPrefix(e.Type, "cron.job.") || strings.HasPrefix(e.Type, "cron.workflow.") {
				w.writeCursor(ps.project.UID, cursor)
				select {
				case ps.wake <- struct{}{}:
				default:
				}
			}
		})
		w.mu.Lock()
		ps.streamOpen = false
		w.mu.Unlock()
		if ctx.Err() != nil {
			return
		}
		if errors.Is(err, ErrResetRequired) {
			cursor = 0
			select {
			case ps.wake <- struct{}{}:
			default:
			}
			continue
		}
		if delivered || (!opened.IsZero() && w.clock.Now().Sub(opened) >= 5*time.Minute) {
			backoff = 500 * time.Millisecond
		}
		if !w.sleep(ctx, jitter(backoff, 2*backoff)) {
			return
		}
		backoff = min(backoff*2, 5*time.Minute)
	}
}

// Slots (ADR-0007). A slot is reserved before a run is prepared and held while its process may be alive.

// reserve takes a slot for ps, false when the Project or the worker is full. The caller binds it to a uid or gives it back.
func (w *Worker) reserve(ps *projectState) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(ps.held)+ps.pending >= ps.cfg.Capacity {
		return false
	}
	if max := w.opt.Config.Limits.MaxActiveRuns; max > 0 && w.slotsInUse() >= max {
		return false
	}
	ps.pending++
	return true
}

func (w *Worker) slotsInUse() int {
	n := 0
	for _, ps := range w.projects {
		n += len(ps.held) + ps.pending
	}
	return n
}

// bind turns a pending reservation into the run's slot.
func (w *Worker) bind(ps *projectState, uid, jobUID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	ps.pending = max(ps.pending-1, 0)
	ps.held[uid] = jobUID
}

func (w *Worker) unreserve(ps *projectState) {
	w.mu.Lock()
	defer w.mu.Unlock()
	ps.pending = max(ps.pending-1, 0)
}

// release frees the run's slot.
func (w *Worker) release(ps *projectState, uid string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(ps.held, uid)
}

// holdsSlot reports whether the journal's run still occupies a slot: it is in flight, or its process may be alive.
func holdsSlot(j *Journal) bool {
	switch j.Phase {
	case PhasePrepared, PhaseAdmitted, PhaseResuming, PhaseLaunching, PhaseStarted:
		return true
	}
	if j.Outcome != nil {
		switch j.Outcome.Hold {
		case HoldLaunchUnproven, HoldInterrupted, HoldNoReceipt, HoldGroupUnproven:
			return processMayBeAlive(j)
		}
	}
	return j.Phase != PhaseSettled
}

// processMayBeAlive is the settlement proof for a run the worker spawned: its recorded process group is gone. With no
// start record the process cannot be ruled out.
func processMayBeAlive(j *Journal) bool {
	if j.Launch == nil {
		return j.Phase == PhaseLaunching || (j.Outcome != nil && j.Outcome.Hold == HoldLaunchUnproven)
	}
	if !groupAlive(j.Launch.PGID) {
		return false
	}
	if j.Launch.StartTicks != 0 {
		if now := startTicks(j.Launch.PID); now != 0 && now != j.Launch.StartTicks {
			return false // the pid was reused
		}
	}
	return true
}

// gateClosed reports whether a job's overlap gate is closed: an earlier run is unsettled, or held for the user (ADR-0007).
func (w *Worker) gateClosed(jobUID, projectUID string) (runUID string) {
	journals, _, _ := w.store.Scan()
	for _, j := range journals {
		if j.JobUID != jobUID || j.ProjectUID != projectUID {
			continue
		}
		if j.Phase != PhaseSettled || (j.Outcome != nil && j.Outcome.Hold != "" && j.Outcome.Status == StatusUnknown) {
			return j.RunUID
		}
	}
	return ""
}

// recoverProject continues the Project's journals after the lease decision (ADR-0006 "Recovery order"): rebuild the
// slots first, then settle or continue each run.
func (w *Worker) recoverProject(ctx context.Context, ps *projectState) {
	journals, quarantined, err := w.store.Scan()
	if err != nil {
		w.opt.Log("journals: %v", err)
		return
	}
	for _, uid := range quarantined {
		w.opt.Log("run %s: its journal could not be read and was moved to quarantine", uid)
	}
	var mine []*Journal
	for _, j := range journals {
		if j.ProjectUID == ps.project.UID && j.Executor == w.opt.Config.Executor {
			mine = append(mine, j)
			if holdsSlot(j) {
				w.mu.Lock()
				ps.held[j.RunUID] = j.JobUID
				w.mu.Unlock()
			}
		}
	}
	for _, j := range mine {
		if j.Phase == PhaseSettled {
			continue
		}
		w.runs.Add(1)
		go func() { defer w.runs.Done(); w.recoverRun(ctx, ps, j.RunUID) }()
	}
}

// leaseHeld reports whether this worker holds the Project's lease.
func (w *Worker) leaseHeld(ps *projectState) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return ps.lease.Held
}
