package worker

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

// Scheduling (ADR-0004, ADR-0007, ADR-0011). Each Project loop keeps, for every enabled adapter `execute` job with a
// schedulable trigger, a checkpoint (what it has decided through, and the occurrences still waiting to run) and the
// next future occurrence. tick is the whole evaluation: plan what became due, apply the job's catchup policy, then
// admit waiting occurrences oldest first through the one admission path (Worker.RunOccurrence).

const (
	// onTimeSlack is how late an occurrence may be and still count as on time: the loop wakes at the occurrence, so
	// only a sleeping machine, a long pause or a busy loop makes one later. Catchup applies to later ones.
	onTimeSlack = 10 * time.Second
	// catchupWindow and catchupMax bound the occurrences one job keeps pending (ADR-0011).
	catchupWindow = 7 * 24 * time.Hour
	catchupMax    = 100
	// enumerateMax bounds one evaluation's work however small an interval is.
	enumerateMax = 1_000_000
)

// trigger yields the occurrences of one job definition. It is the extension point for a new trigger kind: build it in
// newTrigger and the rest of the schedule (checkpoints, catchup, overlap, capacity, admission) applies unchanged.
type trigger interface {
	next(t time.Time) time.Time
}

type scheduled struct{ cronjob.Schedule }

func (s scheduled) next(t time.Time) time.Time { return s.Next(t) }

type onceTrigger struct{ at time.Time }

func (o onceTrigger) next(t time.Time) time.Time {
	if o.at.After(t) {
		return o.at
	}
	return time.Time{}
}

func isIssueDate(t cronjob.Trigger) bool {
	return t.Kind == cronjob.TriggerIssueScheduled || t.Kind == cronjob.TriggerIssueDeadline
}

// newTrigger builds the job's trigger. date is the instant an Issue-date trigger fires at (see Worker.issueDate). A nil
// trigger with no error means the job schedules nothing here: a manual job runs only on request, and an Issue-date job
// waits while its Issue has no such date. A cron or interval trigger is the module's schedule; the worker never builds one
// for a Native job, so it never passes a delivery anchor.
func newTrigger(job cronjob.Job, date time.Time) (trigger, error) {
	t := job.Definition.Trigger
	switch t.Kind {
	case cronjob.TriggerCron, cronjob.TriggerInterval:
		schedule, ok := job.Schedule(time.Time{})
		if !ok {
			return nil, job.TriggerError()
		}
		return scheduled{schedule}, nil
	case cronjob.TriggerOnce:
		at, err := cronjob.OnceAt(t)
		if err != nil {
			return nil, err
		}
		return onceTrigger{at}, nil
	case cronjob.TriggerIssueScheduled, cronjob.TriggerIssueDeadline:
		if date.IsZero() {
			return nil, nil
		}
		return onceTrigger{date}, nil // an Issue date is one instant; a changed date is a new trigger
	}
	return nil, nil
}

// adapterJob reports whether the job is one the worker may schedule at all: kata's daemon owns native jobs and archived
// jobs are gone. disabledAdapterJob is such a job that is switched off.
func adapterJob(job cronjob.Job) bool { return !job.Archived() && !job.Native() }

func disabledAdapterJob(job cronjob.Job) bool { return adapterJob(job) && !job.Definition.Enabled }

// schedulable reports whether the worker plans occurrences for the job. Native jobs belong to kata's daemon, disabled
// and archived jobs schedule nothing, and so does the one notify job kata's own date sweeper already delivers: a
// zero-lead `current-owner-or-author` notification of the trigger's own Issue on its date with no message of its own
// (ADR-0011): the sweeper sends only its generic "Scheduled date reached" text, so a custom message is the worker's. A job that notifies
// another Issue is the worker's: the sweeper notifies only the Issue whose date was reached. A job whose options.adapter
// names another adapter is that adapter's.
func schedulable(job cronjob.Job) bool {
	d := job.Definition
	if !adapterJob(job) || !d.Enabled {
		return false
	}
	if job.OtherAdapter() != "" {
		return false
	}
	switch d.Action.Kind {
	case cronjob.ActionExecute:
		return true
	case cronjob.ActionNotify:
		sameIssue := d.Issue == nil || d.Issue.UID == d.Trigger.IssueUID
		return !(isIssueDate(d.Trigger) && sameIssue && d.Trigger.LeadSeconds == 0 && d.Action.Recipient == currentOwnerOrAuthor && strings.TrimSpace(d.Action.Message) == "")
	}
	return false
}

type busyError struct {
	msg      string
	capacity bool
}

func (e *busyError) Error() string { return e.msg }

// transientError is a refusal that says nothing about the Issue's readiness: a read that failed, a stop, or a Project
// that admits nothing for now. It leaves a parked occurrence parked.
type transientError struct{ err error }

func (e *transientError) Error() string { return e.err.Error() }
func (e *transientError) Unwrap() error { return e.err }

// parkedError is a refusal that waiting cures: the job's existing Issue is someday or scheduled for later (ADR-0002).
// reason is what `worker status` shows; until is the instant the Issue becomes ready by itself, zero for someday.
type parkedError struct {
	msg    string
	reason string
	until  time.Time
}

func (e *parkedError) Error() string { return e.msg }

type parkedIssue struct {
	reason string
	until  time.Time
}

// parked reports whether the Issue is someday or scheduled for an instant still ahead, nil when it is ready. The
// instant is kata's (planning dates), never parsed here; a past or absent date and any deadline_on never defer.
func (w *Worker) parked(ctx context.Context, issue Issue) (*parkedIssue, error) {
	if issue.Someday() {
		return &parkedIssue{reason: "Issue is someday"}, nil
	}
	d, err := w.opt.Kata.PlanningDates(ctx, issue.ProjectID, issue.UID)
	if err != nil {
		return nil, err
	}
	if d.Scheduled.After(w.clock.Now()) {
		return &parkedIssue{reason: "Issue is scheduled for " + d.Scheduled.Local().Format("2006-01-02 15:04"), until: d.Scheduled}, nil
	}
	return nil, nil
}

type jobSchedule struct {
	defEvent string
	trig     trigger
	why      string // the definition's trigger can't be scheduled
	through  time.Time
	date     time.Time // the instant an Issue-date trigger last fired at, the zero time when its Issue has none
	// dateSeen is true once a planning-date read succeeded: date is then an observed baseline (possibly known-absent),
	// not just the zero value of a read that never happened.
	dateSeen bool
	pending  []time.Time // occurrences to run, oldest first
	next     time.Time
	lastErr  string
	// waiting is why the occurrences wait for the job's Issue, empty when they do not; wakeAt is the instant the Issue
	// becomes ready by itself, the zero time when only an Issue event can change it.
	waiting string
	wakeAt  time.Time
	// parked is the pending occurrences that last waited for the job's Issue; unlike waiting it survives a restart,
	// so they are not graced out before admission looks at the Issue again.
	parked []time.Time
	// catchup is the policy the last plan applied, which decides how many pending occurrences a park records.
	catchup string
	// name is the job's name as of the last plan. conditions and conditionsProblem are its options.conditions as the
	// last plan read them (conditionsProblem is why they cannot be read); zone is where its days and weeks are counted.
	// blockedBy is why admission could not check the conditions in the last pass.
	name              string
	conditions        cronjob.Conditions
	conditionsProblem string
	zone              *time.Location
	checkoutKey       string
	blockedBy         string
	// lastSkip is the condition and reason of the job's previous skip, so a repeated skip is logged once.
	lastSkip string
	// gateErr is the reason the gate last blocked the job and has not passed since; lastErr holds it while it is the
	// last thing logged.
	gateErr string
}

// checkpoint is projects/<uid>/schedule/<job uid>.json (ADR-0006): everything decided through a time and the
// occurrences still waiting, for the definition event that produced them.
type checkpoint struct {
	Version            int         `json:"version"`
	DefinitionEventUID string      `json:"definition_event_uid"`
	Through            time.Time   `json:"through"`
	Date               time.Time   `json:"date,omitzero"` // the Issue-date trigger's instant the pending work belongs to
	DateSeen           bool        `json:"date_seen,omitempty"`
	Pending            []time.Time `json:"pending,omitempty"`
	Parked             []time.Time `json:"parked,omitempty"` // the pending occurrences that waited for the job's Issue
	Paused             bool        `json:"paused,omitempty"` // the job was disabled: its time off is never caught up
}

// checkpointStore keeps one checkpoint per job. A save that fails is the store's to report: the schedule goes on with
// its in-memory state and the next change saves again.
type checkpointStore interface {
	Load(jobUID string) (checkpoint, bool)
	Save(jobUID string, c checkpoint)
}

type fileCheckpoints struct {
	dir string
	log func(format string, args ...any)
}

func (f fileCheckpoints) path(jobUID string) string { return filepath.Join(f.dir, jobUID+".json") }

func (f fileCheckpoints) Load(jobUID string) (checkpoint, bool) {
	raw, err := readFile(f.path(jobUID))
	if err != nil {
		return checkpoint{}, false
	}
	var c checkpoint
	if json.Unmarshal(raw, &c) != nil || c.Version != 1 || c.Through.IsZero() {
		f.log("the schedule checkpoint of job %s is unreadable and was ignored", jobUID)
		return checkpoint{}, false
	}
	return c, true
}

func (f fileCheckpoints) Save(jobUID string, c checkpoint) {
	c.Version = 1
	if err := saveJSON(f.path(jobUID), c); err != nil {
		f.log("saving the schedule of job %s: %v", jobUID, err)
	}
}

// projectSchedule is one Project's schedule: it owns the checkpoints, plans what became due and admits waiting
// occurrences. It reaches the rest of the worker through function fields and stores: the checkpoint store, admit (run preparation: one
// occurrence is always one run, and a journal that already covers it is returned with created false), issueDate and
// stopped. log is the Project's own logger, prefixed with its name. It belongs to the Project loop.
type projectSchedule struct {
	store     checkpointStore
	admit     func(ctx context.Context, jobUID string, occurrence time.Time) (runUID string, created bool, err error)
	issueDate func(job cronjob.Job) (at time.Time, known bool)
	stopped   func() bool
	log       func(format string, args ...any)
	jobs      map[string]*jobSchedule
	// paused holds the disabled jobs whose checkpoint already carries the pause mark.
	paused map[string]bool
	// resumeChecked holds the enabled jobs whose checkpoint markResumed already looked at, so a refusal that lasts reads
	// (and reports) it once; a job that is disabled is taken out again.
	resumeChecked map[string]bool
	// skips keeps the skip records; journalExists says whether a run journal covers a run uid, runStarts when the
	// Project's journals of a job were prepared, and now is the clock. They serve conditions only, so a schedule built
	// without them admits a job that has none exactly as before.
	skips         skipStore
	journalExists func(runUID string) (bool, error)
	runStarts     func(jobUID string) ([]time.Time, error)
	// runSpend is what the job's run journals spent, for the spend conditions.
	runSpend func(jobUID string) ([]RunSpend, error)
	// runsChanged drops what runStarts and runSpend have read, so the next question scans the journals again; the
	// schedule calls it at the start of each admission pass, after each admit and per summary. A schedule built without ledger seams sets a no-op.
	runsChanged func()
	now         func() time.Time
	// checkCommits is the New commits check (skip: why nothing is new, blocked: why it cannot be told); nil for a
	// schedule built without it.
	checkCommits func(ctx context.Context, jobUID, checkoutKey string, folders []string) (skip, blocked string)
}

// save writes the job's state as its checkpoint, which never carries the pause mark: a saved job is scheduled.
func (s *projectSchedule) save(jobUID string, st *jobSchedule) {
	s.store.Save(jobUID, checkpoint{DefinitionEventUID: st.defEvent, Through: st.through, Date: st.date, DateSeen: st.dateSeen, Pending: st.pending, Parked: st.parked})
}

func (w *Worker) newProjectSchedule(ps *projectState) *projectSchedule {
	name := ps.cfg.Name
	log := func(format string, args ...any) {
		w.opt.Log("Project %s: "+format, append([]any{name}, args...)...)
	}
	ledger := &runLedger{scan: w.store.Scan, projectUID: ps.project.UID}
	return &projectSchedule{
		store: fileCheckpoints{dir: filepath.Join(w.opt.Dirs.ProjectDir(ps.project.UID), "schedule"), log: log},
		admit: func(ctx context.Context, jobUID string, at time.Time) (string, bool, error) {
			return w.RunOccurrence(ctx, name, jobUID, at)
		},
		issueDate:     func(job cronjob.Job) (time.Time, bool) { return w.issueDate(ps, job) },
		stopped:       w.halted.Load,
		log:           log,
		jobs:          map[string]*jobSchedule{},
		paused:        map[string]bool{},
		skips:         &fileSkips{path: w.opt.Dirs.Skips(ps.project.UID), log: log},
		journalExists: w.journalExists,
		runStarts:     ledger.runStarts,
		runSpend:      ledger.runSpend,
		runsChanged:   ledger.reset,
		now:           w.clock.Now,
		checkCommits: func(ctx context.Context, jobUID, checkoutKey string, folders []string) (string, string) {
			return w.commitGate(ctx, ps, jobUID, checkoutKey, folders)
		},
	}
}

// poke wakes the loop behind ch without blocking when it is already woken.
func poke(ch chan<- struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

func (w *Worker) kick(ps *projectState) { poke(ps.kick) }

// selectOccurrences applies a job's catchup policy to every occurrence not yet run (ADR-0011). Occurrences within
// onTimeSlack are never missed. `all` keeps the rest, `latest` only the newest of everything, and `skip` at most one
// missed occurrence: the newest one that is still within grace or that waits for its Issue (a parked one is not graced
// out), so a long park never turns `skip` into `all`. When an occurrence is on time while a parked one waits, the
// parked one is dropped and the newest on time inherits the mark; a missed one that takes over from it inherits the
// mark too, which is the returned stillParked. Nothing older than 7 days is kept, and at most 100 (the oldest
// overflow is dropped).
func selectOccurrences(cands []time.Time, now time.Time, catchup string, grace time.Duration, parked []time.Time) (keep, stillParked []time.Time, dropped int) {
	var onTime, missed []time.Time
	for _, c := range cands {
		switch lateness := now.Sub(c); {
		case lateness > catchupWindow:
		case lateness <= onTimeSlack:
			onTime = append(onTime, c)
		default:
			missed = append(missed, c)
		}
	}
	inParked := func(t time.Time) bool { return slices.ContainsFunc(parked, t.Equal) }
	switch catchup {
	case "all":
		keep = append(missed, onTime...)
	case "latest":
		keep = append(missed, onTime...)
		if len(keep) > 1 {
			keep = keep[len(keep)-1:]
		}
	default:
		wasParked := slices.ContainsFunc(missed, inParked)
		if wasParked && len(onTime) > 0 {
			// The wait ended as an occurrence came due: it replaces the missed ones, so the wait runs it once.
			stillParked = append(stillParked, onTime[len(onTime)-1])
		} else {
			for i := len(missed) - 1; i >= 0; i-- {
				if m := missed[i]; inParked(m) || i == len(missed)-1 && now.Sub(m) <= grace {
					keep = append(keep, m)
					if wasParked {
						stillParked = append(stillParked, m)
					}
					break
				}
			}
		}
		stillParked = append(stillParked, slices.DeleteFunc(slices.Clone(parked), func(t time.Time) bool { return !slices.ContainsFunc(onTime, t.Equal) })...)
		keep = append(keep, onTime...)
	}
	if over := len(keep) - catchupMax; over > 0 {
		keep = keep[over:]
	}
	if catchup == "all" || catchup == "latest" {
		stillParked = slices.DeleteFunc(slices.Clone(parked), func(t time.Time) bool { return !slices.ContainsFunc(keep, t.Equal) })
	} else {
		stillParked = slices.DeleteFunc(stillParked, func(t time.Time) bool { return !slices.ContainsFunc(keep, t.Equal) })
	}
	return keep, stillParked, len(cands) - len(keep)
}

func (s *projectSchedule) plan(job cronjob.Job, now time.Time) {
	st := s.jobs[job.UID]
	resumed := false
	if st == nil {
		st = &jobSchedule{}
		s.jobs[job.UID] = st
		if c, ok := s.store.Load(job.UID); ok {
			st.defEvent, st.through, st.date, st.pending = c.DefinitionEventUID, c.Through, c.Date, c.Pending
			st.parked = c.Parked
			st.dateSeen = c.DateSeen || !c.Date.IsZero()
			if c.Paused { // it was disabled: scheduling starts again now, and the first date read after it is the baseline
				st.through = maxTime(now, job.CreatedAt)
				st.date, st.dateSeen = time.Time{}, false
				resumed = true
			}
		}
	}
	st.name = job.Name
	if st.conditions, st.conditionsProblem = readConditions(job); st.gated() {
		st.zone = job.Definition.Trigger.Location()
		st.checkoutKey = job.Definition.CheckoutKey
	}
	changed := resumed
	firstSight := st.through.IsZero()
	if firstSight { // first sight: work starts at the later of now and the job's creation
		st.through, changed = maxTime(now, job.CreatedAt), true
	}
	dateChanged := false
	if isIssueDate(job.Definition.Trigger) {
		at, known := s.issueDate(job)
		if known && !st.dateSeen {
			// The first successful read is the baseline, never a change: a read that failed before it must not make a
			// date that was already there look like one that just moved.
			st.dateSeen, changed = true, true
			if !at.Equal(st.date) {
				st.pending, st.date, dateChanged = nil, at, true
			}
		} else if known && !at.Equal(st.date) {
			// The Issue's date moved: work waiting for the old value is stale, and an earlier date that is already
			// behind the checkpoint is evaluated again under the catchup policy.
			if len(st.pending) > 0 {
				s.log("the date of job %s changed; %d waiting occurrence(s) dropped", job.Name, len(st.pending))
			}
			if !at.IsZero() && at.Before(st.through) {
				st.through = at.Add(-time.Nanosecond)
			}
			st.pending, st.date, changed, dateChanged = nil, at, true, true
		}
	}
	if st.defEvent != job.DefinitionEventUID || st.trig == nil && st.why == "" || dateChanged {
		if st.defEvent != job.DefinitionEventUID && len(st.pending) > 0 { // a changed definition supersedes unadmitted work
			s.log("job %s changed; %d waiting occurrence(s) dropped", job.Name, len(st.pending))
		}
		if st.defEvent != job.DefinitionEventUID {
			st.pending, st.defEvent, changed = nil, job.DefinitionEventUID, true
		}
		var err error
		st.trig, err = newTrigger(job, st.date)
		st.why = ""
		if err != nil {
			st.why = err.Error()
			s.log("job %s is not scheduled: %s", job.Name, st.why)
		}
	}
	st.next = time.Time{}
	if st.trig == nil {
		if changed {
			s.save(job.UID, st)
		}
		return
	}
	cands := slices.Clone(st.pending)
	if st.through.After(now) { // the clock ran ahead and was corrected back: occurrences in the wrong-clock span are not invented
		st.through, changed = now, true
	}
	if now.After(st.through) {
		from := maxTime(st.through, now.Add(-catchupWindow))
		var fresh []time.Time
		for range enumerateMax {
			at := st.trig.next(from)
			if at.IsZero() || at.After(now) {
				break
			}
			if len(fresh) == catchupMax*2 {
				fresh = fresh[catchupMax:] // only the newest can survive the bound
			}
			fresh = append(fresh, at)
			from = at
		}
		cands = append(cands, fresh...)
		st.through, changed = now, true
	}
	d := job.Definition
	st.catchup = d.Catchup
	keep, parked, dropped := selectOccurrences(cands, now, d.Catchup, time.Duration(d.GraceSeconds)*time.Second, st.parked)
	if !slices.EqualFunc(parked, st.parked, time.Time.Equal) {
		st.parked, changed = parked, true
	}
	if dropped > 0 {
		s.log("job %s (catchup %s): %d missed occurrence(s) not run", job.Name, cmp.Or(d.Catchup, "skip"), dropped)
	}
	if !slices.Equal(keep, st.pending) {
		st.pending, changed = keep, true
	}
	st.next = st.trig.next(now)
	if changed {
		s.save(job.UID, st)
	}
}

// tick evaluates one Project's schedule and returns when the next occurrence of any of its jobs is due, the zero time
// when none is. issueEvent says an Issue event woke the loop. It runs on the Project loop while the Project's lease is
// held.
func (w *Worker) tick(ctx context.Context, ps *projectState, issueEvent bool) time.Time {
	if w.halted.Load() || ctx.Err() != nil {
		return time.Time{}
	}
	if ps.sched == nil {
		ps.sched = w.newProjectSchedule(ps)
	}
	if w.admitsNothing(ps) != "" {
		// Nothing is planned or admitted, but a job disabled meanwhile is still noted, so that enabling it again never
		// runs the time it was off.
		w.markDisabled(ps)
		return time.Time{}
	}
	now := w.clock.Now()
	w.refreshDates(ctx, ps)
	if !w.planAll(ps, now) {
		return time.Time{}
	}
	if ps.sched.anyPending() {
		// Scheduled work never starts from a cached definition (ADR-0011): read the jobs again, replan, then admit.
		// Only a tick caused by an Issue event reuses a listing younger than the refresh debounce, so a burst of
		// Issue events lists the jobs once.
		if w.jobsStale(ps, issueEvent) {
			if err := w.refreshJobs(ctx, ps); err != nil {
				w.opt.Log("Project %s: %v", ps.cfg.Name, err)
				return time.Time{}
			}
			w.planAll(ps, now)
		}
		ps.sched.admitDue(ctx)
	}
	sum := ps.sched.summary(now)
	w.mu.Lock()
	for i := range sum.waiting {
		sum.waiting[i].Name = ps.jobs[sum.waiting[i].UID].Name
	}
	for i := range sum.blocked {
		sum.blocked[i].Name = ps.jobs[sum.blocked[i].UID].Name
	}
	slices.SortFunc(sum.blocked, func(a, b JobNote) int { return strings.Compare(a.Name, b.Name) })
	ps.skipped, ps.blockedJobs, ps.spent = sum.skips, sum.blocked, sum.spend
	slices.SortFunc(sum.waiting, func(a, b JobNote) int { return strings.Compare(a.Name, b.Name) })
	ps.scheduled, ps.queued, ps.nextRun, ps.waiting = sum.scheduled, sum.queued, sum.next, sum.waiting
	w.mu.Unlock()
	if !sum.wake.IsZero() && (sum.next.IsZero() || sum.wake.Before(sum.next)) {
		return sum.wake
	}
	return sum.next
}

// jobsStale reports whether the Project's job list must be read again before admission. A listing that failed or was
// never made always is. Otherwise only a tick that answers an Issue event may reuse a listing younger than the
// refresh debounce: due-time and freed-slot admission never starts from a cached copy.
func (w *Worker) jobsStale(ps *projectState, issueEvent bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if ps.jobsAt.IsZero() || ps.jobsErr != "" || !issueEvent {
		return true
	}
	return w.clock.Now().Sub(ps.jobsAt) >= jobRefreshDebounce
}

type summary struct {
	queued, scheduled int
	next, wake        time.Time
	waiting           []JobNote
	// blocked lists the jobs whose conditions could not be checked, with the reason; skips the recent skips per job.
	blocked []JobNote
	skips   []JobSkips
	// spend lists the spend in the window of each job with a spend limit.
	spend []JobSpend
}

func (s *projectSchedule) summary(now time.Time) summary {
	var sum summary
	for uid, st := range s.jobs {
		sum.queued += len(st.pending)
		if len(st.pending) == 0 {
			st.blockedBy = ""
		}
		if st.blockedBy != "" {
			sum.blocked = append(sum.blocked, JobNote{UID: uid, Text: st.blockedBy})
		}
		if len(st.pending) == 0 {
			st.waiting, st.wakeAt = "", time.Time{}
		}
		if st.waiting != "" {
			sum.waiting = append(sum.waiting, JobNote{UID: uid, Text: st.waiting})
			sum.wake = sooner(sum.wake, st.wakeAt, now)
		}
		if st.trig != nil {
			sum.scheduled++
		}
		if !st.next.IsZero() && (sum.next.IsZero() || st.next.Before(sum.next)) {
			sum.next = st.next
		}
	}
	if s.skips != nil {
		sum.skips = jobSkips(s.skips.Recent(now))
	}
	sum.spend = s.jobSpends(now)
	return sum
}

// sooner is the earlier of wake and the Issue's scheduled instant at, look again then rather than at the next fallback
// check. An instant already behind now is skipped: a full Project left that job waiting, and a wake in the past would
// spin the loop; a run ending or the regular checks look at it again.
func sooner(wake, at, now time.Time) time.Time {
	if at.After(now) && (wake.IsZero() || at.Before(wake)) {
		return at
	}
	return wake
}

// markDisabled records the Project's disabled jobs in its schedule without planning anything, and the start of the jobs
// that were enabled again meanwhile: their time off is not caught up, but the time since they were enabled is.
func (w *Worker) markDisabled(ps *projectState) {
	w.mu.Lock()
	disabled := map[string]bool{}
	var enabled []cronjob.Job
	for _, j := range ps.jobs {
		if disabledAdapterJob(j) {
			disabled[j.UID] = true
		} else if schedulable(j) {
			enabled = append(enabled, j)
		}
	}
	w.mu.Unlock()
	ps.sched.markPaused(disabled)
	now := w.clock.Now()
	for _, j := range enabled {
		ps.sched.markResumed(j, now)
	}
}

// planAll is the worker's side of planning: it hands the Project's schedulable jobs to the Project schedule. False means the
// Project has no definitions yet.
func (w *Worker) planAll(ps *projectState, now time.Time) bool {
	w.mu.Lock()
	loaded := ps.jobs != nil
	var jobs []cronjob.Job
	disabled := map[string]bool{}
	for _, j := range ps.jobs {
		if schedulable(j) {
			jobs = append(jobs, j)
		} else if disabledAdapterJob(j) {
			disabled[j.UID] = true
		}
	}
	w.mu.Unlock()
	if !loaded {
		return false
	}
	ps.sched.planAll(jobs, disabled, now)
	return true
}

func (s *projectSchedule) planAll(jobs []cronjob.Job, disabled map[string]bool, now time.Time) {
	live := map[string]bool{}
	for _, j := range jobs {
		live[j.UID] = true
		s.plan(j, now)
	}
	for uid := range s.jobs {
		if !live[uid] { // disabled or gone: nothing runs
			delete(s.jobs, uid)
		}
	}
	s.markPaused(disabled)
}

// markPaused forgets the state of the disabled jobs and marks their checkpoints, so that their return starts from the
// moment they are enabled, with or without a restart in between.
func (s *projectSchedule) markPaused(disabled map[string]bool) {
	// Work waiting from before is superseded by the enabling write's new definition event.
	for uid := range disabled {
		delete(s.jobs, uid)
		delete(s.resumeChecked, uid)
	}
	for uid := range s.paused {
		if !disabled[uid] {
			delete(s.paused, uid)
		}
	}
	for uid := range disabled {
		if s.paused[uid] {
			continue
		}
		s.paused[uid] = true
		if c, ok := s.store.Load(uid); ok && !c.Paused {
			c.Paused = true
			s.store.Save(uid, c)
		}
	}
}

// markResumed saves the checkpoint of a job that is enabled again but not planned yet as resumed: scheduling starts at
// now and the first date read is the baseline, exactly as plan would do.
func (s *projectSchedule) markResumed(job cronjob.Job, now time.Time) {
	if s.jobs[job.UID] != nil || s.resumeChecked[job.UID] {
		return
	}
	if s.resumeChecked == nil {
		s.resumeChecked = map[string]bool{}
	}
	s.resumeChecked[job.UID] = true
	if c, ok := s.store.Load(job.UID); ok && c.Paused {
		c.Paused, c.Through = false, maxTime(now, job.CreatedAt)
		c.Date, c.DateSeen = time.Time{}, false
		s.store.Save(job.UID, c)
	}
}

func (s *projectSchedule) anyPending() bool {
	for _, st := range s.jobs {
		if len(st.pending) > 0 {
			return true
		}
	}
	return false
}

// admitDue admits the waiting occurrences, oldest first. A full Project stops admission; a closed overlap gate holds
// only its own job. Whatever stays waiting is looked at again when a run ends and at least every minute. An occurrence
// leaves the checkpoint only after the journal is admitted, so a crash in between admits it again and the journal that
// already covers it answers (the same occurrence is always the same run).
func (s *projectSchedule) admitDue(ctx context.Context) {
	type due struct {
		job string
		at  time.Time
	}
	var queue []due
	for uid, st := range s.jobs {
		for _, at := range st.pending {
			queue = append(queue, due{uid, at})
		}
	}
	slices.SortFunc(queue, func(a, b due) int {
		if c := a.at.Compare(b.at); c != 0 {
			return c
		}
		return strings.Compare(a.job, b.job)
	})
	blocked := map[string]bool{}
	claimed := map[string]time.Time{} // New commits jobs: the occurrence admitted this pass, which took the commits
	for _, st := range s.jobs {
		st.blockedBy = ""
	}
	s.runsChanged() // journals may have changed since the last pass; the ledger rescans after each admit below
	for _, d := range queue {
		st := s.jobs[d.job]
		if blocked[d.job] || ctx.Err() != nil || s.stopped() {
			continue
		}
		if at, ok := claimed[d.job]; ok {
			if !s.skipForClaimedCommits(d.job, st, d.at, at) {
				blocked[d.job] = true
			}
			continue
		}
		if st.gated() {
			if !s.applyGate(ctx, d.job, st, d.at) {
				blocked[d.job] = st.blockedBy != ""
				continue
			}
		}
		_, _, err := s.admit(ctx, d.job, d.at)
		s.runsChanged() // the admit may have created a run journal
		if err == nil {
			st.pending = slices.DeleteFunc(st.pending, func(t time.Time) bool { return t.Equal(d.at) })
			st.lastErr, st.waiting, st.wakeAt, st.parked = "", "", time.Time{}, nil
			s.save(d.job, st)
			if st.conditions.NewCommits {
				claimed[d.job] = d.at
			}
			continue
		}
		var parked *parkedError
		if errors.As(err, &parked) {
			blocked[d.job] = true
			st.waiting, st.wakeAt = parked.reason, parked.until
			mark := st.pending
			if st.catchup != "all" && st.catchup != "latest" && len(mark) > 1 { // skip keeps one missed occurrence: the newest
				mark = mark[len(mark)-1:]
			}
			if !slices.EqualFunc(st.parked, mark, time.Time.Equal) {
				st.parked = slices.Clone(mark)
				s.save(d.job, st)
			}
			continue
		}
		st.waiting, st.wakeAt = "", time.Time{}
		var busy *busyError
		if errors.As(err, &busy) && busy.capacity {
			return // the Issue is ready and a slot is all it lacks: the parked mark stays so the grace cannot drop it
		}
		var transient *transientError
		if len(st.parked) > 0 && !errors.As(err, &transient) { // a refusal that ends the wait is subject to the grace again
			st.parked = nil
			s.save(d.job, st)
		}
		if busy != nil {
			blocked[d.job] = true
			continue
		}
		blocked[d.job] = true // the occurrences of a job that cannot start keep their order
		if msg := err.Error(); msg != st.lastErr {
			st.lastErr = msg
			s.log("%s", msg)
		}
	}
}

type issueDates struct {
	at        time.Time // when the read succeeded
	scheduled time.Time
	deadline  time.Time
}

// datesStale is how long a read stays good without an Issue event, a net for a dropped event stream.
const datesStale = 5 * time.Minute

// issueDate is the instant an Issue-date job fires at: the Issue's scheduled date, or its deadline less the trigger's
// lead. known is false until the worker has read the Issue's dates; a date the Issue lacks is known and zero.
func (w *Worker) issueDate(ps *projectState, job cronjob.Job) (at time.Time, known bool) {
	t := job.Definition.Trigger
	d, ok := ps.dates[t.IssueUID]
	if !ok {
		return time.Time{}, false
	}
	if t.Kind == cronjob.TriggerIssueScheduled {
		return d.scheduled, true
	}
	if d.deadline.IsZero() {
		return time.Time{}, true
	}
	return d.deadline.Add(-time.Duration(t.LeadSeconds) * time.Second), true
}

// refreshDates reads the planning dates of every Issue an Issue-date job watches: once, again when the Project's event
// stream reports an Issue change, and at least every datesStale. A failed read keeps the last good dates (ADR-0011).
func (w *Worker) refreshDates(ctx context.Context, ps *projectState) {
	dirty := ps.datesDirty.Swap(false)
	w.mu.Lock()
	var issues []string
	for _, j := range ps.jobs {
		if schedulable(j) && isIssueDate(j.Definition.Trigger) && !slices.Contains(issues, j.Definition.Trigger.IssueUID) {
			issues = append(issues, j.Definition.Trigger.IssueUID)
		}
	}
	w.mu.Unlock()
	now := w.clock.Now()
	if ps.dates == nil {
		ps.dates = map[string]issueDates{}
	}
	for uid := range ps.dates {
		if !slices.Contains(issues, uid) {
			delete(ps.dates, uid)
		}
	}
	for _, uid := range issues {
		if have, ok := ps.dates[uid]; ok && !dirty && now.Sub(have.at) < datesStale {
			continue
		}
		read, err := w.opt.Kata.PlanningDates(ctx, ps.project.ID, uid)
		if err != nil {
			if ctx.Err() == nil {
				w.opt.Log("Project %s: reading the planning dates of Issue %s: %v", ps.cfg.Name, uid, err)
			}
			ps.datesDirty.Store(true) // try again at the next look
			continue
		}
		ps.dates[uid] = issueDates{at: now, scheduled: read.Scheduled, deadline: read.Deadline}
	}
}
