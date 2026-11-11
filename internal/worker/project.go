package worker

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

func jitter(lo, hi time.Duration) time.Duration {
	if hi <= lo {
		return lo
	}
	return lo + rand.N(hi-lo) //nolint:gosec // G404: spreads refresh times, nothing to guess
}

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
		dirty     bool
		// dirtyReads is the job list's read count when the last event dirtied it: a read past it answers that event.
		dirtyReads uint64
		// dirtyWait is how long after the last read a dirty job list is read again: the debounce, doubled after each failed
		// read up to maxDirtyWait, and back to the debounce once a read answers.
		dirtyWait = jobRefreshDebounce
		recovered bool
		// recoveredRuns outlives a pass that could not read every journal, so the next one only picks up what is left.
		recoveredRuns = map[string]bool{}
		started       bool
		takeover      = w.opt.Takeover
		nextRoute     time.Time
		// issueEvent is set when the loop woke for an Issue event: only that tick may reuse a young job listing.
		issueEvent bool
	)
	// dirtyDue is when events that dirtied the job list may refresh it: after a short settle and dirtyWait.
	dirtyDue := func() time.Time {
		return maxTime(dirtyAt.Add(250*time.Millisecond), w.jobsListedAt(ps).Add(dirtyWait))
	}
	starting, ok := w.beginStarting(ctx)
	if !ok {
		return
	}
	held := true
	defer func() { starting() }()
	// failingOr is next, or the instant a failing route is looked at again when it is failing.
	failingOr := func(next time.Time) time.Time {
		if at, failing := w.routeFailing(ps); failing {
			return at
		}
		return next
	}
	for ctx.Err() == nil {
		now := w.clock.Now()
		tried := false
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
				held = false
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
		if !now.Before(nextLease) && !recovered && !held {
			// The place given back while the Project could not resolve is taken again before it recovers.
			if starting, ok = w.beginStarting(ctx); !ok {
				return
			}
			held = true
		}
		if !now.Before(nextLease) {
			state := claimLease(ctx, w.opt.Kata, w.clock, w.opt.Config.Executor, ps.actor, ps.project.ID, takeover)
			takeover = false
			w.mu.Lock()
			ps.lease = state
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
				recovered, tried = w.recoverProject(ctx, ps, recoveredRuns), true
			}
			starting()
			held = false
		}
		if !recovered && !tried && w.leaseHeld(ps) { // a failed recovery is retried on every pass, not only at lease renewal
			recovered = w.recoverProject(ctx, ps, recoveredRuns)
		}
		if ps.resolved {
			nextRoute = failingOr(cmp.Or(nextRoute, now.Add(routeIdle)))
			if !now.Before(nextRoute) {
				_ = w.checkRoute(ctx, ps, true)
				nextRoute = failingOr(now.Add(routeIdle))
			}
		}
		if w.leaseHeld(ps) {
			if !started {
				started = true
				w.runs.Go(func() { w.follow(ctx, ps) })
			}
			select {
			case <-ps.wake:
				dirty, dirtyAt, dirtyReads = true, w.clock.Now(), w.jobsReadCount(ps)
			default:
			}
			due := nextDefs.IsZero() || !now.Before(nextDefs)
			if dirty && !now.Before(dirtyDue()) {
				due = true
			}
			if due {
				dirty = false
				if err := w.refreshJobs(ctx, ps); err != nil {
					w.opt.Log("Project %s: %v", ps.cfg.Name, err)
					dirty = true // dirty flags survive failed refreshes
					dirtyWait = min(2*dirtyWait, maxDirtyWait)
				}
				nextDefs = w.clock.Now().Add(w.refreshEvery(ps))
			}
		}
		if ps.resolved && !w.halted.Load() {
			w.retryPending(ps)
		}
		wake := minTime(now.Add(time.Minute), nextLease)
		if !recovered && w.leaseHeld(ps) {
			wake = minTime(wake, now.Add(10*time.Second))
		}
		if dirty && w.jobsReadCount(ps) > dirtyReads {
			dirty = false // a listing after the event already satisfied it
		}
		if !dirty {
			dirtyWait = jobRefreshDebounce
		}
		if w.leaseHeld(ps) {
			if next := w.tick(ctx, ps, issueEvent && !dirty); !next.IsZero() {
				wake = minTime(wake, next)
			}
		}
		issueEvent = false
		if !nextDefs.IsZero() && w.leaseHeld(ps) {
			wake = minTime(wake, nextDefs)
		}
		if dirty {
			wake = minTime(wake, dirtyDue())
		}
		if ps.resolved {
			wake = minTime(wake, nextRoute)
		}
		select {
		case <-ctx.Done():
			return
		case <-ps.wake:
			dirty, dirtyAt, dirtyReads = true, w.clock.Now(), w.jobsReadCount(ps)
		case <-ps.kick:
		case <-ps.issueKick:
			issueEvent = true
		case <-w.clock.After(max(wake.Sub(w.clock.Now()), time.Millisecond)):
		}
	}
}

// jobRefreshDebounce is the least time between two reads of a Project's job list that events caused. Only a tick that
// answers an Issue event may reuse a listing this young; due-time and freed-slot admission always lists again.
const jobRefreshDebounce = 2 * time.Second

// maxDirtyWait caps the backoff of a job list that keeps failing to read after an event, as the loop's other retries are capped.
const maxDirtyWait = 5 * time.Minute

func (w *Worker) jobsListedAt(ps *projectState) time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	return ps.jobsAt
}

func (w *Worker) jobsReadCount(ps *projectState) uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return ps.jobsReads
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
		if !j.Native() {
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
	ps.jobsAt = w.clock.Now()
	if err != nil {
		ps.jobsErr = err.Error()
		return fmt.Errorf("listing jobs: %w", err)
	}
	// Only a listing that answered counts: a failed one leaves an event's dirty job list dirty, so it is tried again.
	ps.jobsReads++
	ps.jobs = make(map[string]cronjob.Job, len(jobs))
	for _, j := range jobs {
		ps.jobs[j.UID] = j
	}
	ps.aside, ps.warnings = aside, warnings
	ps.jobsErr = ""
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
				poke(ps.issueKick)
			}
			if strings.HasPrefix(e.Type, "cron.job.") || strings.HasPrefix(e.Type, "cron.workflow.") {
				w.writeCursor(ps.project.UID, cursor)
				poke(ps.wake)
			}
		})
		w.mu.Lock()
		ps.streamOpen = false
		w.mu.Unlock()
		if ctx.Err() != nil {
			return
		}
		var reset *ResetRequired
		if errors.As(err, &reset) {
			// The events up to the position were skipped, so read the definitions again; the reconnect still backs off.
			if reset.After > 0 {
				cursor = reset.After
				w.writeCursor(ps.project.UID, cursor)
			}
			poke(ps.wake)
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

// reserve takes a slot for ps, false when the Project or the worker is full or a stop was asked. The caller binds it to
// a uid or gives it back.
func (w *Worker) reserve(ps *projectState) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopping != "" || len(ps.held)+ps.pending >= ps.cfg.Capacity {
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

func (w *Worker) bind(ps *projectState, j *Journal) {
	occupies := w.life(j).occupiesSlot(j)
	w.mu.Lock()
	defer w.mu.Unlock()
	ps.pending = max(ps.pending-1, 0)
	ps.setSlot(j, occupies)
}

func (w *Worker) unreserve(ps *projectState) {
	w.mu.Lock()
	ps.pending = max(ps.pending-1, 0)
	w.mu.Unlock()
	w.finishDrain()
}

func (w *Worker) syncSlot(ps *projectState, j *Journal) {
	occupies := w.life(j).occupiesSlot(j)
	w.mu.Lock()
	defer w.mu.Unlock()
	ps.setSlot(j, occupies)
}

// setSlot is the slot map's only writer; the caller holds Worker.mu and passes lifecycle.occupiesSlot's answer.
func (ps *projectState) setSlot(j *Journal, occupies bool) {
	if occupies {
		ps.held[j.RunUID] = j.JobUID
	} else {
		delete(ps.held, j.RunUID)
	}
}

func (w *Worker) holdsSlotOf(ps *projectState, uid string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, ok := ps.held[uid]
	return ok
}

// gateClosed reports whether a job's overlap gate is closed, and by which run (ADR-0007), from the store's index of
// runs that close their gate; no journal is read. When the runs folder cannot be read an unsettled run of this job may
// be hidden in it, so the gate reads closed and err says why.
func (w *Worker) gateClosed(jobUID, projectUID string) (runUID string, err error) {
	if err := w.store.RunsReadable(); err != nil {
		return "", err
	}
	return w.store.ClosingRun(projectUID, jobUID), nil
}

// recoverProject continues the Project's journals after the lease decision (ADR-0006 "Recovery order"): rebuild the
// slots first, then settle or continue each run. done holds the runs already continued, so a later pass after an
// incomplete one never recovers a run twice. False means some journal could not be read and the caller tries again.
func (w *Worker) recoverProject(ctx context.Context, ps *projectState, done map[string]bool) bool {
	journals, quarantined, err := w.store.Scan()
	if err != nil {
		if msg := err.Error(); msg != ps.loggedScanErr { // a scan that keeps failing is said once
			ps.loggedScanErr = msg
			w.opt.Log("journals: %v", err)
		}
	} else {
		ps.loggedScanErr = ""
	}
	for _, uid := range quarantined {
		w.opt.Log("run %s: its journal could not be read and was moved to quarantine", uid)
	}
	w.store.SeedGates(journals)
	var mine []*Journal
	for _, j := range journals {
		if j.ProjectUID == ps.project.UID && j.Executor == w.opt.Config.Executor && !done[j.RunUID] {
			mine = append(mine, j)
			done[j.RunUID] = true
			w.syncSlot(ps, j)
		}
	}
	w.mu.Lock()
	ps.recovered, ps.recoverErr = err == nil, ""
	if err != nil {
		ps.recoverErr = err.Error()
	}
	w.mu.Unlock()
	for _, j := range mine {
		if j.settled() {
			w.handedOn(j.RunUID)
			continue
		}
		w.runs.Go(func() {
			defer w.handedOn(j.RunUID) // after recoverRun spawned it, so the run is always counted in one place
			w.recoverRun(ctx, ps, j.RunUID)
		})
	}
	return err == nil
}

func (w *Worker) leaseHeld(ps *projectState) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return ps.lease.Held
}
