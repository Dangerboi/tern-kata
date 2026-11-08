package worker

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
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
}

// skipStore keeps a Project's skip records. Add replaces a record of the same job and occurrence, so a crash between
// the record and the checkpoint save skips the occurrence once. Recent drops what the run journals' retention would
// have removed by now.
type skipStore interface {
	Add(s Skip) error
	Recent(now time.Time) []Skip
}

// skipFile is the content of a Project's skip record file.
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
	f.skips = slices.DeleteFunc(f.skips, func(o Skip) bool { return o.JobUID == s.JobUID && o.Occurrence.Equal(s.Occurrence) })
	f.skips = append(f.skips, s)
	f.prune(s.At)
	raw, _ := json.Marshal(skipFile{Version: 1, Skips: f.skips})
	if err := os.MkdirAll(filepath.Dir(f.path), 0o700); err != nil {
		return err
	}
	return replaceFile(f.path, raw)
}

func (f *fileSkips) Recent(now time.Time) []Skip {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.load()
	f.prune(now)
	return slices.Clone(f.skips)
}

// JobSkips is one job's recent skips as `worker status` shows them: how many, and the latest.
type JobSkips struct {
	UID  string
	Name string
	// Count is the skips still on record, which the run journals' retention bounds.
	Count int
	// Latest* describe the newest skip: the condition that said no, its reason, the occurrence it skipped and when.
	LatestCondition  string
	LatestReason     string
	LatestOccurrence time.Time
	LatestAt         time.Time
}

// jobSkips groups skip records by job, jobs by name.
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
		j.Count++
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

// windowStart is the start of the calendar day, or of the Monday-first week, that contains now in loc.
func windowStart(now time.Time, loc *time.Location, week bool) time.Time {
	n := now.In(loc)
	start := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, loc)
	if week {
		start = start.AddDate(0, 0, -int((n.Weekday()+6)%7))
	}
	return start
}

// gated reports whether admission looks at the job's conditions at all.
func (st *jobSchedule) gated() bool { return st.conditions.Any() || st.conditionsProblem != "" }

// gate checks the conditions of a scheduled occurrence that is about to be admitted. skip is set when a condition says
// no; blocked is the reason when they cannot be checked; both empty admit the occurrence. An occurrence that already has
// a run journal is always admitted: a crash left it pending after its journal was saved, and a skip never contradicts a
// run that is journalled.
func (s *projectSchedule) gate(st *jobSchedule, jobUID string, at time.Time) (skip *Skip, blocked string) {
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
	limits := []struct {
		condition string
		max       int
		week      bool
		per       string
	}{
		{cronjob.ConditionRunsPerDay, st.conditions.RunsPerDay, false, "day"},
		{cronjob.ConditionRunsPerWeek, st.conditions.RunsPerWeek, true, "week"},
	}
	var starts []time.Time
	counted := false
	for _, l := range limits {
		if l.max == 0 {
			continue
		}
		if !counted {
			if starts, err = s.runStarts(jobUID); err != nil {
				return nil, fmt.Sprintf("its runs cannot be counted: %v", err)
			}
			counted = true
		}
		from := windowStart(now, st.zone, l.week)
		n := 0
		for _, t := range starts {
			if !t.Before(from) {
				n++
			}
		}
		if n >= l.max {
			reason := fmt.Sprintf("limit of %d %s per %s reached: %d started since %s", l.max, plural(l.max, "run", "runs"), l.per, n, from.Format("2006-01-02 15:04 MST"))
			return &Skip{JobUID: jobUID, JobName: st.name, Occurrence: at, Condition: l.condition, Reason: reason, At: now.UTC()}, ""
		}
	}
	return nil, ""
}

// runStarts is when each run journal of the Project's job was prepared, whoever asked for the run.
func (w *Worker) runStarts(projectUID, jobUID string) ([]time.Time, error) {
	journals, _, err := w.store.Scan()
	if err != nil {
		return nil, err
	}
	var starts []time.Time
	for _, j := range journals {
		if j.ProjectUID == projectUID && j.JobUID == jobUID {
			starts = append(starts, j.StartedAt)
		}
	}
	return starts, nil
}

// journalExists says whether the run has a journal here.
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
func (s *projectSchedule) applyGate(jobUID string, st *jobSchedule, at time.Time) bool {
	skip, why := s.gate(st, jobUID, at)
	if skip != nil {
		if err := s.skips.Add(*skip); err != nil {
			why = fmt.Sprintf("the skip of %s cannot be recorded: %v", OccurrenceKey(at), err)
		}
	}
	if why != "" {
		st.blockedBy = why
		if why != st.lastErr {
			st.lastErr = why
			s.log("job %q: %s", st.name, why)
		}
		return false
	}
	if skip == nil {
		return true
	}
	st.pending = slices.DeleteFunc(st.pending, func(t time.Time) bool { return t.Equal(at) })
	st.lastErr, st.waiting, st.wakeAt, st.parked = "", "", time.Time{}, nil
	s.save(jobUID, st)
	s.log("job %q: occurrence %s skipped: %s", st.name, OccurrenceKey(at), skip.Reason)
	return false
}
