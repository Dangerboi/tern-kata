package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
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
// newTrigger and the rest of the scheduler (checkpoints, catchup, overlap, capacity, admission) applies unchanged.
type trigger interface {
	// next is the first occurrence strictly after t, the zero time when there is none.
	next(t time.Time) time.Time
}

// cronTrigger is a cron expression in the trigger's zone, evaluated by kata's own parser.
type cronTrigger struct{ schedule cron.Schedule }

func (c cronTrigger) next(t time.Time) time.Time {
	n := c.schedule.Next(t)
	if n.IsZero() {
		return n
	}
	return n.UTC()
}

// intervalTrigger fires at anchor + n*step for n >= 1, anchored at the job's creation as kata anchors its own.
type intervalTrigger struct {
	anchor time.Time
	step   time.Duration
}

func (i intervalTrigger) next(t time.Time) time.Time {
	if t.Before(i.anchor) {
		return i.anchor.Add(i.step)
	}
	n := t.Sub(i.anchor)/i.step + 1
	return i.anchor.Add(n * i.step)
}

// onceTrigger fires at one instant.
type onceTrigger struct{ at time.Time }

func (o onceTrigger) next(t time.Time) time.Time {
	if o.at.After(t) {
		return o.at
	}
	return time.Time{}
}

// parseCron reads a trigger's cron expression with kata's parser (the five fields and descriptors) in the trigger's
// zone (UTC when it has none). An embedded TZ= must agree with the trigger's zone.
func parseCron(t Trigger) (cron.Schedule, error) {
	expression, zone := strings.TrimSpace(t.Cron), t.Timezone
	if strings.HasPrefix(expression, "TZ=") || strings.HasPrefix(expression, "CRON_TZ=") {
		directive, rest, ok := strings.Cut(expression, " ")
		if !ok {
			return nil, errors.New("invalid timezone directive")
		}
		_, embedded, _ := strings.Cut(directive, "=")
		if zone != "" && embedded != zone {
			return nil, errors.New("conflicting cron timezone")
		}
		zone, expression = embedded, strings.TrimSpace(rest)
	}
	if zone == "" {
		zone = "UTC"
	}
	if _, err := time.LoadLocation(zone); err != nil {
		return nil, errors.New("invalid timezone")
	}
	schedule, err := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor).Parse("CRON_TZ=" + zone + " " + expression)
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}
	return schedule, nil
}

// newTrigger builds the job's trigger. A nil trigger with no error means the job schedules nothing here: a manual job
// runs only on request, and trigger kinds this evaluator does not own yet (issue dates) are left to their own source.
func newTrigger(job Job) (trigger, error) {
	t := job.Definition.Trigger
	switch t.Kind {
	case "cron":
		schedule, err := parseCron(t)
		if err != nil {
			return nil, err
		}
		if every, ok := schedule.(cron.ConstantDelaySchedule); ok { // @every is an interval, anchored the same way
			return intervalTrigger{anchor: job.CreatedAt.UTC(), step: every.Delay}, nil
		}
		return cronTrigger{schedule}, nil
	case "interval":
		if t.IntervalSeconds <= 0 {
			return nil, errors.New("interval must be positive")
		}
		return intervalTrigger{anchor: job.CreatedAt.UTC(), step: time.Duration(t.IntervalSeconds) * time.Second}, nil
	case "once":
		at, err := time.Parse(time.RFC3339, t.At)
		if err != nil {
			return nil, errors.New("once requires an RFC3339 instant")
		}
		return onceTrigger{at.UTC()}, nil
	}
	return nil, nil
}

// schedulable reports whether the worker plans occurrences for the job. Native jobs belong to kata's daemon, disabled
// and deleted jobs schedule nothing, and notify jobs are not run yet.
func schedulable(job Job) bool {
	d := job.Definition
	return !job.Deleted && !job.IsNative() && d.Enabled && d.ActionKind == "execute"
}

// busyError is a refusal that waiting cures: the Project is full or the job's overlap gate is closed.
type busyError struct {
	msg      string
	capacity bool
}

func (e *busyError) Error() string { return e.msg }

// jobSchedule is one job's schedule state, owned by its Project's loop.
type jobSchedule struct {
	defEvent string
	trig     trigger
	why      string // the definition's trigger can't be scheduled
	through  time.Time
	pending  []time.Time // occurrences to run, oldest first
	next     time.Time
	lastErr  string
}

// checkpoint is projects/<uid>/schedule/<job uid>.json (ADR-0006): everything decided through a time and the
// occurrences still waiting, for the definition event that produced them.
type checkpoint struct {
	Version            int         `json:"version"`
	DefinitionEventUID string      `json:"definition_event_uid"`
	Through            time.Time   `json:"through"`
	Pending            []time.Time `json:"pending,omitempty"`
}

func (w *Worker) checkpointPath(ps *projectState, jobUID string) string {
	return filepath.Join(w.opt.Dirs.ProjectDir(ps.project.UID), "schedule", jobUID+".json")
}

func (w *Worker) loadCheckpoint(ps *projectState, jobUID string) (checkpoint, bool) {
	raw, err := os.ReadFile(w.checkpointPath(ps, jobUID))
	if err != nil {
		return checkpoint{}, false
	}
	var c checkpoint
	if json.Unmarshal(raw, &c) != nil || c.Version != 1 || c.Through.IsZero() {
		w.opt.Log("Project %s: the schedule checkpoint of job %s is unreadable and was ignored", ps.cfg.Name, jobUID)
		return checkpoint{}, false
	}
	return c, true
}

func (w *Worker) saveCheckpoint(ps *projectState, jobUID string, st *jobSchedule) {
	raw, _ := json.Marshal(checkpoint{Version: 1, DefinitionEventUID: st.defEvent, Through: st.through, Pending: st.pending})
	path := w.checkpointPath(ps, jobUID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err == nil {
		err = replaceFile(path, raw)
		if err != nil {
			w.opt.Log("Project %s: saving the schedule of job %s: %v", ps.cfg.Name, jobUID, err)
		}
	}
}

// kick wakes the Project loop to look at its queued occurrences.
func (w *Worker) kick(ps *projectState) {
	select {
	case ps.kick <- struct{}{}:
	default:
	}
}

// selectOccurrences applies a job's catchup policy to every occurrence not yet run (ADR-0011). Occurrences within
// onTimeSlack are never missed. `all` keeps the rest, `latest` only the newest of everything, and `skip` the newest
// missed one still within grace. Nothing older than 7 days is kept, and at most 100 (the oldest overflow is dropped).
func selectOccurrences(cands []time.Time, now time.Time, catchup string, grace time.Duration) (keep []time.Time, dropped int) {
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
	switch catchup {
	case "all":
		keep = append(missed, onTime...)
	case "latest":
		keep = append(missed, onTime...)
		if len(keep) > 1 {
			keep = keep[len(keep)-1:]
		}
	default:
		if n := len(missed); n > 0 && now.Sub(missed[n-1]) <= grace {
			keep = append(keep, missed[n-1])
		}
		keep = append(keep, onTime...)
	}
	if over := len(keep) - catchupMax; over > 0 {
		keep = keep[over:]
	}
	return keep, len(cands) - len(keep)
}

// plan brings one job's state up to now: rebuild it when the definition changed, collect the occurrences that became
// due, and apply the catchup policy to them and to those still waiting.
func (w *Worker) plan(ps *projectState, job Job, now time.Time) {
	st := ps.sched[job.UID]
	if st == nil {
		st = &jobSchedule{}
		ps.sched[job.UID] = st
		if c, ok := w.loadCheckpoint(ps, job.UID); ok {
			st.defEvent, st.through, st.pending = c.DefinitionEventUID, c.Through, c.Pending
		}
	}
	changed := false
	if st.through.IsZero() { // first sight: work starts at the later of now and the job's creation
		st.through, changed = maxTime(now, job.CreatedAt), true
	}
	if st.defEvent != job.DefinitionEventUID || st.trig == nil && st.why == "" {
		if st.defEvent != job.DefinitionEventUID && len(st.pending) > 0 { // a changed definition supersedes unadmitted work
			w.opt.Log("Project %s: job %s changed; %d waiting occurrence(s) dropped", ps.cfg.Name, job.Name, len(st.pending))
		}
		if st.defEvent != job.DefinitionEventUID {
			st.pending, st.defEvent, changed = nil, job.DefinitionEventUID, true
		}
		var err error
		st.trig, err = newTrigger(job)
		st.why = ""
		if err != nil {
			st.why = err.Error()
			w.opt.Log("Project %s: job %s is not scheduled: %s", ps.cfg.Name, job.Name, st.why)
		}
	}
	st.next = time.Time{}
	if st.trig == nil {
		if changed {
			w.saveCheckpoint(ps, job.UID, st)
		}
		return
	}
	cands := slices.Clone(st.pending)
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
	keep, dropped := selectOccurrences(cands, now, d.Catchup, time.Duration(d.GraceSeconds)*time.Second)
	if dropped > 0 {
		w.opt.Log("Project %s: job %s (catchup %s): %d missed occurrence(s) not run", ps.cfg.Name, job.Name, cmpOr(d.Catchup, "skip"), dropped)
	}
	if !slices.Equal(keep, st.pending) {
		st.pending, changed = keep, true
	}
	st.next = st.trig.next(now)
	if changed {
		w.saveCheckpoint(ps, job.UID, st)
	}
}

// tick evaluates one Project's schedule and returns when the next occurrence of any of its jobs is due, the zero time
// when none is. It runs on the Project loop while the Project's lease is held.
func (w *Worker) tick(ctx context.Context, ps *projectState) time.Time {
	if w.halted.Load() || ctx.Err() != nil || w.admitsNothing(ps) != "" {
		return time.Time{}
	}
	if ps.sched == nil {
		ps.sched = map[string]*jobSchedule{}
	}
	now := w.clock.Now()
	if !w.planAll(ps, now) {
		return time.Time{}
	}
	if w.anyPending(ps) {
		// Scheduled work never starts from a cached definition (ADR-0011): read the jobs again, replan, then admit.
		if err := w.refreshJobs(ctx, ps); err != nil {
			w.opt.Log("Project %s: %v", ps.cfg.Name, err)
			return time.Time{}
		}
		w.planAll(ps, now)
		w.admitDue(ctx, ps)
	}
	var next time.Time
	queued, scheduled := 0, 0
	for _, st := range ps.sched {
		queued += len(st.pending)
		if st.trig != nil {
			scheduled++
		}
		if !st.next.IsZero() && (next.IsZero() || st.next.Before(next)) {
			next = st.next
		}
	}
	w.mu.Lock()
	ps.scheduled, ps.queued, ps.nextRun = scheduled, queued, next
	w.mu.Unlock()
	return next
}

// planAll plans every schedulable job in the cache and forgets the state of jobs that went away. False means the
// Project has no definitions yet.
func (w *Worker) planAll(ps *projectState, now time.Time) bool {
	w.mu.Lock()
	loaded := !ps.jobsAt.IsZero()
	var jobs []Job
	for _, j := range ps.jobs {
		if schedulable(j) {
			jobs = append(jobs, j)
		}
	}
	w.mu.Unlock()
	if !loaded {
		return false
	}
	live := map[string]bool{}
	for _, j := range jobs {
		live[j.UID] = true
		w.plan(ps, j, now)
	}
	for uid := range ps.sched {
		if !live[uid] { // disabled or gone: nothing runs, and the checkpoint keeps the pause for its return
			delete(ps.sched, uid)
		}
	}
	return true
}

func (w *Worker) anyPending(ps *projectState) bool {
	for _, st := range ps.sched {
		if len(st.pending) > 0 {
			return true
		}
	}
	return false
}

// admitDue admits the Project's waiting occurrences, oldest first. A full Project stops admission; a closed overlap
// gate holds only its own job. Whatever stays waiting is looked at again when a run ends and at least every minute.
func (w *Worker) admitDue(ctx context.Context, ps *projectState) {
	type due struct {
		job string
		at  time.Time
	}
	var queue []due
	for uid, st := range ps.sched {
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
	for _, d := range queue {
		st := ps.sched[d.job]
		if blocked[d.job] || ctx.Err() != nil || w.halted.Load() {
			continue
		}
		_, _, err := w.RunOccurrence(ctx, ps.cfg.Name, d.job, d.at)
		if err == nil {
			st.pending = slices.DeleteFunc(st.pending, func(t time.Time) bool { return t.Equal(d.at) })
			st.lastErr = ""
			w.saveCheckpoint(ps, d.job, st)
			continue
		}
		var busy *busyError
		if errors.As(err, &busy) {
			if busy.capacity {
				return
			}
			blocked[d.job] = true
			continue
		}
		blocked[d.job] = true // the occurrences of a job that cannot start keep their order
		if msg := err.Error(); msg != st.lastErr {
			st.lastErr = msg
			w.opt.Log("Project %s: %s", ps.cfg.Name, msg)
		}
	}
}
