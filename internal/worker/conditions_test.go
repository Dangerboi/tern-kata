package worker

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

// memSkips is a skipStore in memory.
type memSkips struct{ records []Skip }

func (m *memSkips) Add(s Skip) error {
	m.records = append(m.records, s)
	return nil
}

func (m *memSkips) Recent(time.Time) []Skip { return append([]Skip(nil), m.records...) }

// conditionRig is a Project schedule with conditions on the fake clock, checkpoints and journals of the other
// schedule tests. A run journal's start is the rig's clock when the fake admits it.
type conditionRig struct {
	store    *memCheckpoints
	journals *fakeJournals
	skips    *memSkips
	starts   map[string][]time.Time
	countErr error
	now      time.Time
	logs     []string
}

func newConditionRig(now time.Time) *conditionRig {
	return &conditionRig{
		store: &memCheckpoints{saved: map[string]checkpoint{}}, journals: &fakeJournals{journals: map[string]bool{}},
		skips: &memSkips{}, starts: map[string][]time.Time{}, now: now,
	}
}

func (r *conditionRig) schedule() *projectSchedule {
	return &projectSchedule{
		store: r.store,
		admit: func(ctx context.Context, jobUID string, at time.Time) (string, bool, error) {
			uid, created, err := r.journals.admit(ctx, jobUID, at)
			if created {
				r.starts[jobUID] = append(r.starts[jobUID], r.now)
			}
			return uid, created, err
		},
		issueDate:     func(cronjob.Job) (time.Time, bool) { return time.Time{}, false },
		stopped:       func() bool { return false },
		log:           func(format string, args ...any) { r.logs = append(r.logs, strings.TrimSpace(format)) },
		jobs:          map[string]*jobSchedule{},
		paused:        map[string]bool{},
		skips:         r.skips,
		journalExists: func(uid string) (bool, error) { return r.journals.journals[uid], nil },
		runStarts: func(jobUID string) ([]time.Time, error) {
			if r.countErr != nil {
				return nil, r.countErr
			}
			return r.starts[jobUID], nil
		},
		runsChanged: func() {},
		now:         func() time.Time { return r.now },
	}
}

// plannedAt plans the jobs at their creation and again at now, which is the rig's clock afterwards.
func (r *conditionRig) plannedAt(s *projectSchedule, jobs []cronjob.Job, created, now time.Time) {
	r.now = created
	s.planAll(jobs, nil, created)
	r.now = now
	s.planAll(jobs, nil, now)
}

func limitedJob(created time.Time, catchup string, conditions map[string]any) cronjob.Job {
	job := pausedHourlyJob(created, catchup)
	job.Definition.Options = map[string]any{"conditions": conditions}
	return job
}

func TestADueOccurrenceIsSkippedOnceTheDailyLimitIsReachedAndMakesNoRun(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	job := limitedJob(created, "all", map[string]any{"runs_per_day": 2})
	r := newConditionRig(created)
	s := r.schedule()
	r.plannedAt(s, []cronjob.Job{job}, created, created.Add(3*time.Hour))
	s.admitDue(context.Background())

	if r.journals.created != 2 {
		t.Fatalf("%d runs started, want the two the limit allows", r.journals.created)
	}
	if len(r.skips.records) != 1 {
		t.Fatalf("skip records %+v, want the third occurrence", r.skips.records)
	}
	got := r.skips.records[0]
	if !got.Occurrence.Equal(created.Add(3*time.Hour)) || got.Condition != cronjob.ConditionRunsPerDay || got.JobUID != job.UID || got.JobName != "hourly" ||
		!strings.Contains(got.Reason, "limit of 2 runs per day reached: 2 started since 2026-01-01 00:00 UTC") || !got.At.Equal(r.now) {
		t.Fatalf("skip record %+v", got)
	}
	if len(r.journals.journals) != 2 {
		t.Fatalf("%d run journals, want none for the skip", len(r.journals.journals))
	}
	if s.anyPending() || len(r.store.saved[job.UID].Pending) != 0 {
		t.Fatalf("the skipped occurrence is still pending: %v", r.store.saved[job.UID].Pending)
	}
}

func TestTheWeeklyLimitCountsFromMondayInTheTriggersZone(t *testing.T) {
	berlin, _ := time.LoadLocation("Europe/Berlin")
	// Sunday 2026-01-04 23:30 UTC is Monday 00:30 in Berlin: a new week there, still the old week in UTC.
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 1, 4, 23, 30, 0, 0, time.UTC)
	job := limitedJob(created, "latest", map[string]any{"runs_per_week": 1})
	job.Definition.Trigger = cronjob.Trigger{Kind: cronjob.TriggerCron, Cron: "0 * * * *", Timezone: "Europe/Berlin"}
	r := newConditionRig(created)
	r.starts[job.UID] = []time.Time{time.Date(2026, 1, 4, 12, 0, 0, 0, time.UTC)} // Sunday noon: the old week in Berlin
	s := r.schedule()
	r.plannedAt(s, []cronjob.Job{job}, created, now)
	s.admitDue(context.Background())

	if r.journals.created != 1 || len(r.skips.records) != 0 {
		t.Fatalf("%d runs, skips %+v; want the run admitted in the new Berlin week (starts since %s)", r.journals.created, r.skips.records, windowStart(now, berlin, true))
	}

	r.now = now.Add(time.Hour)
	s.planAll([]cronjob.Job{job}, nil, r.now)
	s.admitDue(context.Background())
	if len(r.skips.records) != 1 || !strings.Contains(r.skips.records[0].Reason, "limit of 1 run per week reached: 1 started since 2026-01-05 00:00 CET") {
		t.Fatalf("skips %+v, want the next occurrence skipped by the weekly limit", r.skips.records)
	}
}

func TestADayLimitStartsAgainWithTheNextDay(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	job := limitedJob(created, "latest", map[string]any{"runs_per_day": 1})
	r := newConditionRig(created)
	s := r.schedule()
	jobs := []cronjob.Job{job}
	for _, hour := range []int{3, 4} {
		r.plannedAt(s, jobs, created, created.Add(time.Duration(hour)*time.Hour))
		s.admitDue(context.Background())
	}
	if r.journals.created != 1 || len(r.skips.records) != 1 {
		t.Fatalf("%d runs and %d skips on the first day, want 1 and 1", r.journals.created, len(r.skips.records))
	}
	r.now = time.Date(2026, 1, 2, 1, 0, 0, 0, time.UTC)
	s.planAll(jobs, nil, r.now)
	s.admitDue(context.Background())
	if r.journals.created != 2 {
		t.Fatalf("%d runs after midnight, want the next day's first occurrence run", r.journals.created)
	}
}

func TestASkippedOccurrenceIsNeverRetriedByAnyCatchupPolicyAndLeavesNoJournal(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	due := created.Add(3 * time.Hour)
	for catchup, wantSkips := range map[string]int{"all": 3, "latest": 1, "skip": 1} {
		t.Run(catchup, func(t *testing.T) {
			job := limitedJob(created, catchup, map[string]any{"runs_per_day": 1})
			r := newConditionRig(created)
			r.starts[job.UID] = []time.Time{created.Add(30 * time.Minute)} // a run now already used the day
			s := r.schedule()
			jobs := []cronjob.Job{job}
			r.plannedAt(s, jobs, created, due)
			s.admitDue(context.Background())
			if len(r.skips.records) != wantSkips || r.journals.created != 0 || len(r.journals.journals) != 0 {
				t.Fatalf("catchup %s: %d skips, %d runs, journals %v; want %d skips and no run", catchup, len(r.skips.records), r.journals.created, r.journals.journals, wantSkips)
			}

			// Another pass, a replan and a restarted worker on the saved checkpoint find nothing to retry.
			r.now = due.Add(time.Minute)
			s.planAll(jobs, nil, r.now)
			s.admitDue(context.Background())
			restarted := r.schedule()
			restarted.planAll(jobs, nil, r.now)
			restarted.admitDue(context.Background())
			if restarted.anyPending() || len(r.skips.records) != wantSkips || r.journals.created != 0 {
				t.Fatalf("catchup %s: pending %v, %d skips, %d runs after replanning; the skip must settle its occurrence", catchup, restarted.anyPending(), len(r.skips.records), r.journals.created)
			}
		})
	}
}

func TestAJournalledOccurrenceLeftPendingByACrashIsAdmittedNotSkippedEvenAtTheLimit(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	due := created.Add(time.Hour)
	job := limitedJob(created, "all", map[string]any{"runs_per_day": 1})
	r := newConditionRig(created)
	jobs := []cronjob.Job{job}

	first := r.schedule()
	r.plannedAt(first, jobs, created, due)
	r.journals.afterNew = func() { r.store.crashed = true } // the process dies once the journal is saved
	first.admitDue(context.Background())
	if r.journals.created != 1 {
		t.Fatalf("%d journals created by the first admission, want 1", r.journals.created)
	}
	if got := r.store.saved[job.UID].Pending; len(got) != 1 || !got[0].Equal(due) {
		t.Fatalf("saved pending %v, want the occurrence still waiting after the crash", got)
	}

	r.store.crashed = false
	second := r.schedule()
	second.planAll(jobs, nil, due)
	second.admitDue(context.Background())
	if len(r.skips.records) != 0 {
		t.Fatalf("skips %+v: a run that is already journalled was skipped", r.skips.records)
	}
	if second.anyPending() || len(r.store.saved[job.UID].Pending) != 0 || r.journals.created != 1 {
		t.Fatalf("pending %v, %d journals; want the journalled occurrence admitted again and settled", r.store.saved[job.UID].Pending, r.journals.created)
	}
}

func TestOptionsThatDoNotParseBlockTheOccurrenceAndAreLoggedOnce(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	due := created.Add(time.Hour)
	job := pausedHourlyJob(created, "skip")
	job.Definition.Options = map[string]any{"conditions": map[string]any{"runs_per_day": "two"}}
	r := newConditionRig(created)
	s := r.schedule()
	r.plannedAt(s, []cronjob.Job{job}, created, due)
	for range 3 {
		s.admitDue(context.Background())
	}
	if r.journals.created != 0 || len(r.skips.records) != 0 || !s.anyPending() {
		t.Fatalf("%d runs, skips %+v, pending %v; want the occurrence kept waiting and unrun", r.journals.created, r.skips.records, s.anyPending())
	}
	if len(r.logs) != 1 || !strings.Contains(r.logs[0], "job %q") {
		t.Fatalf("log %q, want the problem once", r.logs)
	}
	sum := s.summary(due)
	if len(sum.blocked) != 1 || sum.blocked[0].UID != job.UID || !strings.Contains(sum.blocked[0].Text, "runs_per_day must be a whole number") {
		t.Fatalf("blocked jobs %+v, want the job with the reason", sum.blocked)
	}
}

func TestARunCountThatCannotBeReadBlocksUntilItCanAndLogsOnlyWhenTheCauseChanges(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	due := created.Add(time.Hour)
	job := limitedJob(created, "skip", map[string]any{"runs_per_day": 5})
	r := newConditionRig(created)
	s := r.schedule()
	r.plannedAt(s, []cronjob.Job{job}, created, due)

	r.countErr = errors.New("disk on fire")
	s.admitDue(context.Background())
	s.admitDue(context.Background())
	if len(r.logs) != 1 || r.journals.created != 0 || !s.anyPending() {
		t.Fatalf("logs %q, %d runs, pending %v; want one log and the job not started", r.logs, r.journals.created, s.anyPending())
	}
	if b := s.summary(due).blocked; len(b) != 1 || !strings.Contains(b[0].Text, "its runs cannot be counted: disk on fire") {
		t.Fatalf("blocked %+v", b)
	}

	r.countErr = errors.New("disk full")
	s.admitDue(context.Background())
	if len(r.logs) != 2 {
		t.Fatalf("logs %q, want a second line for the new cause", r.logs)
	}

	r.countErr = nil
	s.admitDue(context.Background())
	if r.journals.created != 1 || s.anyPending() || len(s.summary(due).blocked) != 0 {
		t.Fatalf("%d runs, pending %v; want the occurrence run once the count reads again", r.journals.created, s.anyPending())
	}
}

func TestAJobWithoutConditionsIsAdmittedAsBefore(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	due := created.Add(time.Hour)
	job := pausedHourlyJob(created, "skip")
	job.Definition.Options = map[string]any{"adapter": "tern-kata"}
	journals := &fakeJournals{journals: map[string]bool{}}
	s := newMemSchedule(&memCheckpoints{saved: map[string]checkpoint{}})
	s.admit = journals.admit // no condition seams at all: a job without the key never reaches them
	s.planAll([]cronjob.Job{job}, nil, created)
	s.planAll([]cronjob.Job{job}, nil, due)
	s.admitDue(context.Background())
	if journals.created != 1 || s.anyPending() {
		t.Fatalf("%d runs, pending %v; want the plain job run", journals.created, s.anyPending())
	}
}

func TestOnlyExecuteJobsAreChecked(t *testing.T) {
	job := limitedJob(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "skip", map[string]any{"runs_per_day": 1})
	job.Definition.Action = cronjob.Action{Kind: cronjob.ActionNotify, Recipient: "someone"}
	if c, problem := readConditions(job); c.Any() || problem != "" {
		t.Fatalf("a notify job reads conditions %+v, %q", c, problem)
	}
}

func TestSkipRecordsSurviveARestartReplaceTheSameOccurrenceAndArePrunedByRetention(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projects", "P1", "skips.json")
	now := time.Date(2026, 1, 20, 12, 0, 0, 0, time.UTC)
	skip := func(job string, occurrence, at time.Time) Skip {
		return Skip{JobUID: job, JobName: job, Occurrence: occurrence, Condition: cronjob.ConditionRunsPerDay, Reason: "limit of 1 run per day reached: 1 started since x", At: at}
	}
	store := &fileSkips{path: path, log: func(string, ...any) {}}
	old := now.Add(-retentionAge - time.Hour)
	for _, s := range []Skip{skip("A", old, old), skip("A", now.Add(-time.Hour), now.Add(-time.Hour)), skip("B", now, now)} {
		if err := store.Add(s); err != nil {
			t.Fatal(err)
		}
	}
	// the same occurrence again replaces its record
	if err := store.Add(skip("B", now, now.Add(time.Minute))); err != nil {
		t.Fatal(err)
	}

	restarted := &fileSkips{path: path, log: func(string, ...any) {}}
	got := restarted.Recent(now.Add(time.Minute))
	if len(got) != 2 || got[0].JobUID != "A" || got[1].JobUID != "B" || !got[1].At.Equal(now.Add(time.Minute)) {
		t.Fatalf("records after a restart %+v, want A's recent skip and B's once; the old one pruned", got)
	}
	if later := restarted.Recent(now.Add(retentionAge)); len(later) != 1 || later[0].JobUID != "B" {
		t.Fatalf("records two weeks on %+v, want only B's", later)
	}
}

func TestSkipRecordsOfOneJobStayBoundedAndTheStatusCountStaysRight(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projects", "P1", "skips.json")
	start := time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC)
	store := &fileSkips{path: path, log: func(string, ...any) {}}
	const total = 4*maxSkipsPerJob + 7
	for i := range total {
		at := start.Add(time.Duration(i) * time.Minute)
		s := Skip{JobUID: "A", JobName: "A", Occurrence: at, Condition: cronjob.ConditionRunsPerDay, Reason: "limit", At: at}
		if err := store.Add(s); err != nil {
			t.Fatal(err)
		}
	}
	other := start.Add(time.Hour)
	if err := store.Add(Skip{JobUID: "B", JobName: "B", Occurrence: other, Condition: cronjob.ConditionRunsPerDay, Reason: "limit", At: other}); err != nil {
		t.Fatal(err)
	}

	restarted := &fileSkips{path: path, log: func(string, ...any) {}}
	got := restarted.Recent(other)
	if len(got) != maxSkipsPerJob+1 {
		t.Fatalf("%d records kept, want %d for A and 1 for B", len(got), maxSkipsPerJob)
	}
	jobs := jobSkips(got)
	if len(jobs) != 2 || jobs[0].UID != "A" || jobs[0].Count != total || jobs[1].Count != 1 {
		t.Fatalf("skips per job %+v, want A counted %d and B once", jobs, total)
	}
	if want := start.Add((total - 1) * time.Minute); !jobs[0].LatestOccurrence.Equal(want) {
		t.Fatalf("latest occurrence %v, want %v", jobs[0].LatestOccurrence, want)
	}
}

func TestSkipsAreListedPerJobWithACountAndTheLatestReason(t *testing.T) {
	at := time.Date(2026, 1, 1, 3, 0, 0, 0, time.UTC)
	got := jobSkips([]Skip{
		{JobUID: "B", JobName: "beta", Occurrence: at, Condition: cronjob.ConditionRunsPerWeek, Reason: "weekly", At: at},
		{JobUID: "A", JobName: "alpha", Occurrence: at, Condition: cronjob.ConditionRunsPerDay, Reason: "first", At: at},
		{JobUID: "A", JobName: "alpha", Occurrence: at.Add(time.Hour), Condition: cronjob.ConditionRunsPerDay, Reason: "second", At: at.Add(time.Hour)},
	})
	if len(got) != 2 || got[0].UID != "A" || got[0].Count != 2 || got[0].LatestReason != "second" || !got[0].LatestOccurrence.Equal(at.Add(time.Hour)) || got[1].UID != "B" || got[1].Count != 1 {
		t.Fatalf("skips per job %+v", got)
	}
	s := Status{Projects: []ProjectStatus{{Name: "demo", Admitting: true, Capacity: 1, Skipped: got, Blocked: []JobNote{{UID: "C", Name: "gamma", Text: "its conditions cannot be checked: nope"}}}}}
	text := s.String()
	for _, want := range []string{`Job "alpha" skipped 2 occurrences, latest`, "(runs_per_day): second", `Job "beta" skipped 1 occurrence, latest`, `Job "gamma" is blocked: its conditions cannot be checked: nope`} {
		if !strings.Contains(text, want) {
			t.Fatalf("worker status lacks %q:\n%s", want, text)
		}
	}
}
