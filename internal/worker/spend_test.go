package worker

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

func usd(x float64) *float64 { return &x }

// spendSchedule is a rig schedule whose run journals are the given spends.
func spendSchedule(r *conditionRig, runs *[]RunSpend) *projectSchedule {
	s := r.schedule()
	s.runSpend = func(string) ([]RunSpend, error) { return *runs, nil }
	return s
}

func TestOmpUsageCostIsSummedAndAbsentWhenOmpReportedNone(t *testing.T) {
	turn := func(usage string) string {
		return `{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"ok"}],"stopReason":"stop","usage":` + usage + `}}`
	}
	cases := []struct {
		name  string
		lines []string
		want  *float64
	}{
		{"cost in every turn adds up", []string{turn(`{"input":1,"output":1,"cost":{"input":0.1,"total":0.25}}`), turn(`{"input":1,"output":1,"cost":{"total":0.5}}`)}, usd(0.75)},
		{"usage without cost", []string{turn(`{"input":1,"output":1}`)}, nil},
		{"a cost without a total", []string{turn(`{"input":1,"output":1,"cost":{"input":0.1}}`)}, nil},
		{"only the turns that report a cost add", []string{turn(`{"input":1,"output":1}`), turn(`{"input":1,"output":1,"cost":{"total":0.5}}`)}, usd(0.5)},
		{"a reported zero is a reported cost", []string{turn(`{"input":1,"output":1,"cost":{"total":0}}`)}, usd(0)},
		{"no usage at all", []string{`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"ok"}],"stopReason":"stop"}}`}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parse(c.lines...).CostUSD
			if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
				t.Fatalf("cost %v, want %v", got, c.want)
			}
		})
	}
}

func TestAnOutcomeCarriesTheCostOfTheAgentAndOfEveryResumedAttempt(t *testing.T) {
	j := &Journal{Result: &AgentResult{FinalText: "done", Terminal: true, StopReason: "stop", CostUSD: usd(0.5)}, Exit: &ExitRecord{}}
	if o := classify(j); o.CostUSD == nil || *o.CostUSD != 0.5 {
		t.Fatalf("outcome cost %v, want what omp reported", o.CostUSD)
	}
	j.CarriedCost = usd(1.25)
	if o := j.withCarried(classify(j)); o.CostUSD == nil || *o.CostUSD != 1.75 {
		t.Fatalf("outcome cost %v, want the held attempts' cost added", o.CostUSD)
	}
	j.Result.CostUSD = nil
	if o := j.withCarried(classify(j)); o.CostUSD == nil || *o.CostUSD != 1.25 {
		t.Fatalf("outcome cost %v, want the carried cost alone", o.CostUSD)
	}
	j.CarriedCost = nil
	if o := j.withCarried(classify(j)); o.CostUSD != nil {
		t.Fatalf("outcome cost %v, want none when no attempt reported one", *o.CostUSD)
	}
}

func TestAWorkflowRunsCostIsTheCostOfEveryAttemptOfEveryStep(t *testing.T) {
	if got := sumStepCost([]StepRecord{{}, {}}); got != nil {
		t.Fatalf("cost %v, want none", *got)
	}
	got := sumStepCost([]StepRecord{{CostUSD: usd(0.25)}, {}, {CostUSD: usd(0.5)}})
	if got == nil || *got != 0.75 {
		t.Fatalf("cost %v, want 0.75", got)
	}
}

func TestADueOccurrenceIsSkippedOnceTheDaysSpendReachedTheLimitAndTheReasonNamesBoth(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	job := limitedJob(created, "all", map[string]any{"spend_per_day": 5})
	runs := []RunSpend{
		{Start: created.Add(-time.Hour), CostUSD: usd(9)}, // the day before: not this window
		{Start: created.Add(time.Hour), CostUSD: usd(3)},
		{Start: created.Add(2 * time.Hour), CostUSD: usd(2.5)},
	}
	r := newConditionRig(created)
	s := spendSchedule(r, &runs)
	r.plannedAt(s, []cronjob.Job{job}, created, created.Add(3*time.Hour))
	s.admitDue(context.Background())

	if r.journals.created != 0 || len(r.skips.records) != 3 {
		t.Fatalf("%d runs and %d skips, want the three due occurrences skipped", r.journals.created, len(r.skips.records))
	}
	got := r.skips.records[0]
	if got.Condition != cronjob.ConditionSpendPerDay || !strings.Contains(got.Reason, "limit of $5.00 per day reached: $5.50 spent since 2026-01-01 00:00 UTC") || strings.Contains(got.Reason, "partial") {
		t.Fatalf("skip %+v", got)
	}
}

func TestTheSpendLimitIsReachedAtTheLimitAndOpenBelowIt(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		spent float64
		skips int
	}{{4.99, 0}, {5, 1}, {0.1 + 0.2 + 4.7, 1}} {
		job := limitedJob(created, "latest", map[string]any{"spend_per_day": 5})
		runs := []RunSpend{{Start: created.Add(time.Hour), CostUSD: usd(tc.spent)}}
		r := newConditionRig(created)
		s := spendSchedule(r, &runs)
		r.plannedAt(s, []cronjob.Job{job}, created, created.Add(2*time.Hour))
		s.admitDue(context.Background())
		if len(r.skips.records) != tc.skips || r.journals.created != 1-tc.skips {
			t.Fatalf("spent %v: %d skips and %d runs", tc.spent, len(r.skips.records), r.journals.created)
		}
	}
}

func TestASpendWindowStartsAgainAtMidnightAndTheWeekOnMondayInTheTriggersZone(t *testing.T) {
	// Friday 2026-01-02; the week began Monday 2025-12-29 and the day that follows starts a fresh day window.
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	runs := []RunSpend{{Start: time.Date(2026, 1, 1, 22, 0, 0, 0, time.UTC), CostUSD: usd(9)}}
	now := time.Date(2026, 1, 2, 1, 0, 0, 0, time.UTC)

	day := limitedJob(created, "latest", map[string]any{"spend_per_day": 5})
	r := newConditionRig(created)
	s := spendSchedule(r, &runs)
	r.plannedAt(s, []cronjob.Job{day}, created, now)
	s.admitDue(context.Background())
	if r.journals.created != 1 || len(r.skips.records) != 0 {
		t.Fatalf("%d runs, skips %+v; want yesterday's spend not counted today", r.journals.created, r.skips.records)
	}

	week := limitedJob(created, "latest", map[string]any{"spend_per_day": 5, "spend_per_week": 8})
	r = newConditionRig(created)
	s = spendSchedule(r, &runs)
	r.plannedAt(s, []cronjob.Job{week}, created, now)
	s.admitDue(context.Background())
	if len(r.skips.records) != 1 || r.skips.records[0].Condition != cronjob.ConditionSpendPerWeek ||
		!strings.Contains(r.skips.records[0].Reason, "limit of $8.00 per week reached: $9.00 spent since 2025-12-29 00:00 UTC") {
		t.Fatalf("skips %+v, want the weekly limit to say no", r.skips.records)
	}

	// Sunday 23:30 UTC is Monday 00:30 in Berlin: the old week's spend no longer counts there.
	berlin := limitedJob(created, "latest", map[string]any{"spend_per_week": 5})
	berlin.Definition.Trigger = cronjob.Trigger{Kind: cronjob.TriggerCron, Cron: "0 * * * *", Timezone: "Europe/Berlin"}
	runs = []RunSpend{{Start: time.Date(2026, 1, 4, 12, 0, 0, 0, time.UTC), CostUSD: usd(9)}}
	r = newConditionRig(created)
	s = spendSchedule(r, &runs)
	r.plannedAt(s, []cronjob.Job{berlin}, created, time.Date(2026, 1, 4, 23, 30, 0, 0, time.UTC))
	s.admitDue(context.Background())
	if r.journals.created != 1 || len(r.skips.records) != 0 {
		t.Fatalf("%d runs, skips %+v; want the new Berlin week open", r.journals.created, r.skips.records)
	}
}

func TestRunsThatReportedNoCostCountAsZeroAndMakeTheTotalPartial(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	job := limitedJob(created, "latest", map[string]any{"spend_per_day": 5})
	runs := []RunSpend{
		{Start: created.Add(time.Hour), CostUSD: usd(5)},
		{Start: created.Add(90 * time.Minute)},
		{Start: created.Add(100 * time.Minute)},
	}
	r := newConditionRig(created)
	s := spendSchedule(r, &runs)
	r.plannedAt(s, []cronjob.Job{job}, created, created.Add(2*time.Hour))
	s.admitDue(context.Background())
	if len(r.skips.records) != 1 || !strings.Contains(r.skips.records[0].Reason, "$5.00 spent since 2026-01-01 00:00 UTC (partial: 2 runs reported no cost and count as $0)") {
		t.Fatalf("skips %+v", r.skips.records)
	}

	runs = []RunSpend{{Start: created.Add(time.Hour)}, {Start: created.Add(90 * time.Minute)}}
	r = newConditionRig(created)
	s = spendSchedule(r, &runs)
	r.plannedAt(s, []cronjob.Job{job}, created, created.Add(2*time.Hour))
	s.admitDue(context.Background())
	if len(r.skips.records) != 0 || r.journals.created != 1 {
		t.Fatalf("%d runs and skips %+v; runs without a cost spend nothing", r.journals.created, r.skips.records)
	}
	st := Status{Projects: []ProjectStatus{{Name: "demo", Admitting: true, Capacity: 1, Spent: s.jobSpends(r.now)}}}
	text := st.String()
	for _, want := range []string{`Job "hourly" spent $0.00 of $5.00 this day (since 2026-01-01 00:00 UTC), partial: 2 runs reported no cost and count as $0`} {
		if !strings.Contains(text, want) {
			t.Fatalf("worker status lacks %q:\n%s", want, text)
		}
	}
}

func TestWorkerStatusShowsEachLimitedJobsSpendInItsWindows(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	limited := limitedJob(created, "latest", map[string]any{"spend_per_day": 5, "spend_per_week": 20})
	plain := limitedJob(created, "latest", map[string]any{"runs_per_day": 9})
	plain.UID, plain.Name = "01PLAIN", "plain"
	runs := []RunSpend{{Start: time.Date(2025, 12, 31, 12, 0, 0, 0, time.UTC), CostUSD: usd(6)}, {Start: created.Add(time.Hour), CostUSD: usd(1.5)}}
	r := newConditionRig(created)
	s := spendSchedule(r, &runs)
	r.plannedAt(s, []cronjob.Job{limited, plain}, created, created.Add(2*time.Hour))
	spend := s.summary(r.now).spend
	if len(spend) != 1 || spend[0].UID != limited.UID || len(spend[0].Windows) != 2 {
		t.Fatalf("spend %+v, want the limited job only, with a day and a week", spend)
	}
	text := Status{Projects: []ProjectStatus{{Name: "demo", Admitting: true, Capacity: 1, Spent: spend}}}.String()
	for _, want := range []string{`spent $1.50 of $5.00 this day (since 2026-01-01 00:00 UTC)`, `spent $7.50 of $20.00 this week (since 2025-12-29 00:00 UTC)`} {
		if !strings.Contains(text, want) {
			t.Fatalf("worker status lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "partial") {
		t.Fatalf("a total of reported costs is not partial:\n%s", text)
	}
}

func TestASpendThatCannotBeCountedBlocksTheOccurrence(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	job := limitedJob(created, "latest", map[string]any{"spend_per_day": 5})
	r := newConditionRig(created)
	s := r.schedule()
	s.runSpend = func(string) ([]RunSpend, error) { return nil, errors.New("disk gone") }
	r.plannedAt(s, []cronjob.Job{job}, created, created.Add(2*time.Hour))
	s.admitDue(context.Background())
	if r.journals.created != 0 || len(r.skips.records) != 0 || !s.anyPending() || !strings.Contains(s.jobs[job.UID].blockedBy, "its spend cannot be counted: disk gone") {
		t.Fatalf("%d runs, skips %+v, blocked %q", r.journals.created, r.skips.records, s.jobs[job.UID].blockedBy)
	}
}

func TestSpendIsCountedFromRunJournalsSurvivesARestartAndDropsWithTheirRetention(t *testing.T) {
	dirs := Dirs{Config: t.TempDir(), State: t.TempDir()}
	store := NewStore(dirs, Hooks{})
	day := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	add := func(uid, job string, start time.Time, outcome *Outcome, mod func(*Journal)) {
		j := &Journal{Version: 1, RunUID: uid, ProjectUID: "P", JobUID: job, StartedAt: start, Outcome: outcome, Phase: PhaseSettled}
		if mod != nil {
			mod(j)
		}
		if err := store.Create(j, "prepared"); err != nil {
			t.Fatal(err)
		}
	}
	done := func(at time.Time, cost *float64) *Outcome {
		return &Outcome{Status: cronjob.RunSucceeded, At: at.Add(time.Minute), CostUSD: cost}
	}
	add("01M5JESXAK1NBYS6N2ZD9RTM0A", "J", day.Add(-time.Hour), done(day.Add(-time.Hour), usd(2)), nil)
	add("01M5JESXAK1NBYS6N2ZD9RTM0B", "J", day.Add(time.Hour), done(day.Add(time.Hour), usd(1.5)), nil)
	add("01M5JESXAK1NBYS6N2ZD9RTM0C", "J", day.Add(2*time.Hour), done(day.Add(2*time.Hour), nil), nil)
	add("01M5JESXAK1NBYS6N2ZD9RTM0D", "J", day.Add(3*time.Hour), nil, func(j *Journal) { j.Phase = PhaseStarted })
	add("01M5JESXAK1NBYS6N2ZD9RTM0E", "OTHER", day.Add(time.Hour), done(day.Add(time.Hour), usd(100)), nil)
	add("01M5JESXAK1NBYS6N2ZD9RTM0F", "J", day.Add(time.Hour), done(day.Add(time.Hour), nil), func(j *Journal) { j.Notify = &NotifySpec{Recipient: "x"} })

	w := &Worker{store: NewStore(dirs, Hooks{})} // a restarted worker reads the same files
	led := &runLedger{w: w, projectUID: "P"}
	runs, err := led.runSpend("J")
	if err != nil || len(runs) != 4 {
		t.Fatalf("runs %+v, %v; want the job's four runs, not the other job's or the notify run", runs, err)
	}
	now := day.Add(4 * time.Hour)
	got := spendWindows(cronjob.Conditions{SpendPerDay: 5, SpendPerWeek: 20}, time.UTC, now, runs)
	if len(got) != 2 || got[0].Spent != 1.5 || got[0].Unreported != 2 || got[1].Spent != 3.5 || got[1].Unreported != 2 || !got[1].Since.Equal(time.Date(2025, 12, 29, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("windows %+v", got)
	}

	removed, err := store.Retain(day.Add(90 * time.Minute)) // what ended before 01:30 goes: the first two runs, the other job's and the notify run
	if err != nil || len(removed) != 4 {
		t.Fatalf("removed %v, %v", removed, err)
	}
	led.reset()
	runs, err = led.runSpend("J")
	if err != nil || len(runs) != 2 {
		t.Fatalf("runs %+v, %v; want the two that remain", runs, err)
	}
	if spent, unreported := spentSince(runs, windowStart(now, time.UTC, true)); spent != 0 || unreported != 2 {
		t.Fatalf("spent %v with %d unreported after the retained journals went, want $0 from the runs left", spent, unreported)
	}
}
