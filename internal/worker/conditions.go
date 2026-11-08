package worker

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

// Conditions (ADR-0016). A job's options.conditions are checked in admission, after the definition's refresh and before
// a slot is reserved, for a scheduled occurrence of an execute job that has no run journal yet. A condition that says no
// skips the occurrence: it leaves the checkpoint's pending list, a skip record keeps the reason, and no Run, slot or run
// journal exists for it. A condition that cannot be checked leaves the occurrence pending and blocks the job.

// Skip is one skipped occurrence: skip record file entries, newest last.
type Skip struct {
	JobUID     string    `json:"job_uid"`
	JobName    string    `json:"job_name"`
	Occurrence time.Time `json:"occurrence"`
	// Condition is the cronjob.Condition* name of the condition that said no; Reason is the sentence `worker status` shows.
	Condition string    `json:"condition"`
	Reason    string    `json:"reason"`
	At        time.Time `json:"at"`
	// Merged counts the older skips of this job the per-job cap dropped in favour of this record.
	Merged int `json:"merged,omitempty"`
}

// maxSkipsPerJob bounds the skip records kept for one job: a job that polls often would otherwise grow the file, and
// rewrite it whole, on every skip.
const maxSkipsPerJob = 50

// skipStore keeps a Project's skip records. Add replaces a record of the same job and occurrence, so a crash between
// the record and the checkpoint save skips the occurrence once. Recent drops what the run journals' retention would
// have removed by now.
type skipStore interface {
	Add(s Skip) error
	Recent(now time.Time) []Skip
}

type skipFile struct {
	Version int    `json:"version"`
	Skips   []Skip `json:"skips"`
}

// fileSkips is the skipStore of one Project's skip record file (Dirs.Skips). Records are read when first needed.
type fileSkips struct {
	path string
	log  func(format string, args ...any)

	mu     sync.Mutex
	loaded bool
	skips  []Skip
}

func (f *fileSkips) load() {
	if f.loaded {
		return
	}
	f.loaded = true
	raw, err := readFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	var file skipFile
	if err == nil {
		err = json.Unmarshal(raw, &file)
	}
	if err != nil || file.Version != 1 {
		f.log("the skip record file %s is unreadable and was ignored", f.path)
		return
	}
	f.skips = file.Skips
}

func (f *fileSkips) prune(now time.Time) {
	cutoff := now.Add(-retentionAge)
	f.skips = slices.DeleteFunc(f.skips, func(s Skip) bool { return s.At.Before(cutoff) })
}

func (f *fileSkips) Add(s Skip) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.load()
	f.skips = slices.DeleteFunc(f.skips, func(o Skip) bool {
		same := o.JobUID == s.JobUID && o.Occurrence.Equal(s.Occurrence)
		if same {
			s.Merged += o.Merged
		}
		return same
	})
	f.skips = append(f.skips, s)
	f.prune(s.At)
	f.trim(s.JobUID)
	raw, _ := json.Marshal(skipFile{Version: 1, Skips: f.skips})
	if err := os.MkdirAll(filepath.Dir(f.path), 0o700); err != nil {
		return err
	}
	return replaceFile(f.path, raw)
}

func (f *fileSkips) trim(jobUID string) {
	n := 0
	for _, o := range f.skips {
		if o.JobUID == jobUID {
			n++
		}
	}
	extra := n - maxSkipsPerJob
	if extra <= 0 {
		return
	}
	seen, merged := 0, 0
	for i := range f.skips {
		o := &f.skips[i]
		if o.JobUID != jobUID {
			continue
		}
		if seen++; seen <= extra {
			merged += 1 + o.Merged
		} else if seen == extra+1 {
			o.Merged += merged
		}
	}
	seen = 0
	f.skips = slices.DeleteFunc(f.skips, func(o Skip) bool {
		if o.JobUID != jobUID {
			return false
		}
		seen++
		return seen <= extra
	})
}

func (f *fileSkips) Recent(now time.Time) []Skip {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.load()
	f.prune(now)
	return slices.Clone(f.skips)
}

type JobSkips struct {
	UID  string
	Name string
	// Count is the skips still on record, which the run journals' retention bounds, with those the per-job cap folded in.
	Count int
	// Latest* describe the newest skip: the condition that said no, its reason, the occurrence it skipped and when.
	LatestCondition  string
	LatestReason     string
	LatestOccurrence time.Time
	LatestAt         time.Time
}

func jobSkips(skips []Skip) []JobSkips {
	byJob := map[string]*JobSkips{}
	var out []*JobSkips
	for _, s := range skips {
		j := byJob[s.JobUID]
		if j == nil {
			j = &JobSkips{UID: s.JobUID}
			byJob[s.JobUID] = j
			out = append(out, j)
		}
		j.Count += 1 + s.Merged
		if !s.At.Before(j.LatestAt) {
			j.Name, j.LatestCondition, j.LatestReason, j.LatestOccurrence, j.LatestAt = s.JobName, s.Condition, s.Reason, s.Occurrence, s.At
		}
	}
	res := make([]JobSkips, len(out))
	for i, j := range out {
		res[i] = *j
	}
	slices.SortFunc(res, func(a, b JobSkips) int { return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.UID, b.UID)) })
	return res
}

// readConditions is the job's conditions as admission applies them: only an execute job has any. A job whose options do
// not parse carries the reason instead.
func readConditions(job cronjob.Job) (c cronjob.Conditions, problem string) {
	if job.Definition.Action.Kind != cronjob.ActionExecute {
		return cronjob.Conditions{}, ""
	}
	c, err := job.Conditions()
	if err != nil {
		return cronjob.Conditions{}, err.Error()
	}
	return c, ""
}

func windowStart(now time.Time, loc *time.Location, week bool) time.Time {
	n := now.In(loc)
	start := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, loc)
	if week {
		start = start.AddDate(0, 0, -int((n.Weekday()+6)%7))
	}
	return start
}

func (st *jobSchedule) gated() bool { return st.conditions.Any() || st.conditionsProblem != "" }

// gate checks the conditions of a scheduled occurrence that is about to be admitted. skip is set when a condition says
// no; blocked is the reason when they cannot be checked; both empty admit the occurrence. An occurrence that already has
// a run journal is always admitted: a crash left it pending after its journal was saved, and a skip never contradicts a
// run that is journalled.
func (s *projectSchedule) gate(ctx context.Context, st *jobSchedule, jobUID string, at time.Time) (skip *Skip, blocked string) {
	journalled, err := s.journalExists(RunUID(jobUID, OccurrenceKey(at), at))
	if err != nil {
		return nil, fmt.Sprintf("cannot tell whether %s already has a run: %v", OccurrenceKey(at), err)
	}
	if journalled {
		return nil, ""
	}
	if st.conditionsProblem != "" {
		return nil, fmt.Sprintf("its conditions cannot be checked: %s", st.conditionsProblem)
	}
	now := s.now()
	limits := st.conditions.Limits()
	var starts []time.Time
	counted := false
	for _, l := range limits {
		if l.Runs == 0 {
			continue
		}
		if !counted {
			if starts, err = s.runStarts(jobUID); err != nil {
				return nil, fmt.Sprintf("its runs cannot be counted: %v", err)
			}
			counted = true
		}
		from := windowStart(now, st.zone, l.Week)
		n := 0
		for _, t := range starts {
			if !t.Before(from) {
				n++
			}
		}
		if n >= l.Runs {
			reason := fmt.Sprintf("limit of %d %s per %s reached: %d started since %s", l.Runs, plural(l.Runs, "run", "runs"), l.Per, n, from.Format("2006-01-02 15:04 MST"))
			return st.newSkip(jobUID, at, l.Condition, reason, now), ""
		}
	}
	if skip, blocked := s.gateSpend(st, jobUID, at, now); skip != nil || blocked != "" {
		return skip, blocked
	}
	if st.conditions.NewCommits {
		reason, why := s.checkCommits(ctx, jobUID, st.checkoutKey, st.conditions.Folders)
		if why != "" {
			return nil, fmt.Sprintf("its new commits cannot be checked: %s", why)
		}
		if reason != "" {
			return st.newSkip(jobUID, at, cronjob.ConditionNewCommits, reason, now), ""
		}
	}
	return nil, ""
}

func (st *jobSchedule) newSkip(jobUID string, at time.Time, condition, reason string, now time.Time) *Skip {
	return &Skip{JobUID: jobUID, JobName: st.name, Occurrence: at, Condition: condition, Reason: reason, At: now.UTC()}
}

// RunSpend is what one run journal says a run spent: when it was prepared and the cost omp reported, nil when it
// reported none (a run still going has reported none yet).
type RunSpend struct {
	Start   time.Time
	CostUSD *float64
}

// spentSince sums the runs that started at or after from. A run with no reported cost adds nothing and is counted in
// Unreported.
func spentSince(runs []RunSpend, from time.Time) (spent float64, unreported int) {
	for _, r := range runs {
		if r.Start.Before(from) {
			continue
		}
		if r.CostUSD == nil {
			unreported++
			continue
		}
		spent += *r.CostUSD
	}
	return math.Round(spent*1e6) / 1e6, unreported
}

func spendWindows(c cronjob.Conditions, loc *time.Location, now time.Time, runs []RunSpend) []SpendWindow {
	var out []SpendWindow
	for _, l := range c.Limits() {
		if l.Spend == 0 {
			continue
		}
		from := windowStart(now, loc, l.Week)
		spent, unreported := spentSince(runs, from)
		out = append(out, SpendWindow{Condition: l.Condition, Per: l.Per, Limit: l.Spend, Spent: spent, Since: from, Unreported: unreported})
	}
	return out
}

func formatDollars(x float64) string {
	if x != 0 && x < 0.01 {
		return fmt.Sprintf("$%.4f", x)
	}
	return fmt.Sprintf("$%.2f", x)
}

func partialNote(unreported int) string {
	if unreported == 0 {
		return ""
	}
	return fmt.Sprintf("partial: %d %s reported no cost and count as $0", unreported, plural(unreported, "run", "runs"))
}

func (s *projectSchedule) gateSpend(st *jobSchedule, jobUID string, at, now time.Time) (skip *Skip, blocked string) {
	if st.conditions.SpendPerDay == 0 && st.conditions.SpendPerWeek == 0 {
		return nil, ""
	}
	runs, err := s.runSpend(jobUID)
	if err != nil {
		return nil, fmt.Sprintf("its spend cannot be counted: %v", err)
	}
	for _, w := range spendWindows(st.conditions, st.zone, now, runs) {
		if w.Spent < w.Limit {
			continue
		}
		reason := fmt.Sprintf("limit of %s per %s reached: %s spent since %s", formatDollars(w.Limit), w.Per, formatDollars(w.Spent), w.Since.Format("2006-01-02 15:04 MST"))
		if note := partialNote(w.Unreported); note != "" {
			reason += " (" + note + ")"
		}
		return st.newSkip(jobUID, at, w.Condition, reason, now), ""
	}
	return nil, ""
}

type JobSpend struct {
	UID     string
	Name    string
	Windows []SpendWindow
}

// SpendWindow is one window of a JobSpend: the limit, what omp reported spent since the window began, and how many
// runs of the window reported no cost, so that the total is partial.
type SpendWindow struct {
	Condition  string // the spend condition that limits the window
	Per        string // "day" or "week"
	Limit      float64
	Spent      float64
	Since      time.Time
	Unreported int
}

// jobSpends is the spend of every job that has a spend limit, jobs by name. A job whose runs cannot be read is left
// out: the gate blocks it with the reason.
func (s *projectSchedule) jobSpends(now time.Time) []JobSpend {
	s.runsChanged()
	if s.runSpend == nil {
		return nil
	}
	var out []JobSpend
	for uid, st := range s.jobs {
		if st.conditions.SpendPerDay == 0 && st.conditions.SpendPerWeek == 0 {
			continue
		}
		runs, err := s.runSpend(uid)
		if err != nil {
			continue
		}
		j := JobSpend{UID: uid, Name: st.name}
		j.Windows = spendWindows(st.conditions, st.zone, now, runs)
		out = append(out, j)
	}
	slices.SortFunc(out, func(a, b JobSpend) int { return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.UID, b.UID)) })
	return out
}

// runLedger reads the Project's run journals once and serves every run-count and spend question from that read until
// reset, so a tick with many limited jobs scans the journals once per admission pass and after each admit rather than once per job and per question.
type runLedger struct {
	scan       func() ([]*Journal, []string, error)
	projectUID string
	mu         sync.Mutex
	loaded     bool
	journals   []*Journal
	err        error
}

func (l *runLedger) reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.loaded, l.journals, l.err = false, nil, nil
}

func (l *runLedger) read() ([]*Journal, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.loaded {
		l.journals, _, l.err = l.scan()
		l.loaded = true
	}
	return l.journals, l.err
}

// runSpend is what each run journal of the Project's job spent, whoever asked for the run. A notify run never calls
// omp and is left out; any other run that reported no cost counts as zero.
func (l *runLedger) runSpend(jobUID string) ([]RunSpend, error) {
	journals, err := l.read()
	if err != nil {
		return nil, err
	}
	var runs []RunSpend
	for _, j := range journals {
		if j.ProjectUID != l.projectUID || j.JobUID != jobUID || j.Notify != nil {
			continue
		}
		r := RunSpend{Start: j.StartedAt}
		if j.Outcome != nil { // an outcome already holds the carried cost (withCarried)
			r.CostUSD = j.Outcome.CostUSD
		} else {
			r.CostUSD = addCost(j.CarriedCost, sumStepCost(j.Steps)) // a run still going: its resumed attempts' cost and its finished steps'
		}
		runs = append(runs, r)
	}
	return runs, nil
}

func (l *runLedger) runStarts(jobUID string) ([]time.Time, error) {
	journals, err := l.read()
	if err != nil {
		return nil, err
	}
	var starts []time.Time
	for _, j := range journals {
		if j.ProjectUID == l.projectUID && j.JobUID == jobUID {
			starts = append(starts, j.StartedAt)
		}
	}
	return starts, nil
}

func (w *Worker) journalExists(runUID string) (bool, error) {
	_, err := w.store.Load(runUID)
	if errors.Is(err, ErrNoJournal) {
		return false, nil
	}
	return err == nil, err
}

// applyGate is the gate step of admitDue for one pending occurrence: true admits it. A skip is recorded first and then
// settles the occurrence like an admitted one, so a crash between the two skips it once. A condition that cannot be
// checked, or a skip that cannot be recorded, leaves the occurrence pending and marks the job blocked; the message is
// logged once until it changes, as a refusal is.
func (s *projectSchedule) applyGate(ctx context.Context, jobUID string, st *jobSchedule, at time.Time) bool {
	skip, why := s.gate(ctx, st, jobUID, at)
	if ctx.Err() != nil { // shutting down: whatever the gate could not check is no reason to block the job
		return false
	}
	if skip != nil {
		if err := s.skips.Add(*skip); err != nil {
			why = fmt.Sprintf("the skip of %s cannot be recorded: %v", OccurrenceKey(at), err)
		}
	}
	if why != "" {
		st.blockedBy = why
		st.gateErr = why
		if why != st.lastErr {
			st.lastErr = why
			s.log("job %q: %s", st.name, why)
		}
		return false
	}
	if st.gateErr != "" { // the gate passes again: its reason is logged afresh if the cause comes back
		if st.lastErr == st.gateErr {
			st.lastErr = ""
		}
		st.gateErr = ""
	}
	if skip == nil {
		st.lastSkip = ""
		return true
	}
	s.settleSkip(jobUID, st, at, skip, skip.Condition+"\x00"+skip.Reason)
	return false
}

// settleSkip settles an occurrence whose skip record is saved: it leaves the pending list like an admitted one, and the
// skip is logged unless its key repeats the job's previous one.
func (s *projectSchedule) settleSkip(jobUID string, st *jobSchedule, at time.Time, skip *Skip, key string) {
	st.pending = slices.DeleteFunc(st.pending, func(t time.Time) bool { return t.Equal(at) })
	st.lastErr, st.waiting, st.wakeAt, st.parked = "", "", time.Time{}, nil
	s.save(jobUID, st)
	if key != st.lastSkip {
		st.lastSkip = key
		s.log("job %q: occurrence %s skipped: %s", st.name, OccurrenceKey(at), skip.Reason)
	}
}

// skipForClaimedCommits skips an occurrence of a New commits job whose commits an earlier occurrence of the same pass
// took, so catch-up does not send several runs over one range. false means the skip could not be recorded and the
// occurrence stays pending.
func (s *projectSchedule) skipForClaimedCommits(jobUID string, st *jobSchedule, at, claimedBy time.Time) bool {
	skip := st.newSkip(jobUID, at, cronjob.ConditionNewCommits, fmt.Sprintf("the new commits went to the run of %s", OccurrenceKey(claimedBy)), s.now())
	if err := s.skips.Add(*skip); err != nil {
		st.blockedBy = fmt.Sprintf("the skip of %s cannot be recorded: %v", OccurrenceKey(at), err)
		return false
	}
	s.settleSkip(jobUID, st, at, skip, cronjob.ConditionNewCommits+"\x00claimed") // the reason names the claimant, which differs per pass
	return true
}
