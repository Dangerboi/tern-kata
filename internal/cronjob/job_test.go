package cronjob_test

import (
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

func perRun() *cronjob.IssuePolicy {
	return &cronjob.IssuePolicy{Kind: cronjob.IssuePerRun, ParentUID: "P"}
}
func existingIssue() *cronjob.IssuePolicy {
	return &cronjob.IssuePolicy{Kind: cronjob.IssueExisting, UID: "I"}
}

func job(action cronjob.ActionKind, issue *cronjob.IssuePolicy, trigger cronjob.Trigger) cronjob.Job {
	return cronjob.Job{
		UID:        "J",
		CreatedAt:  time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC),
		Definition: cronjob.Definition{Enabled: true, Trigger: trigger, Action: cronjob.Action{Kind: action}, Issue: issue},
	}
}

func TestNativeAndAdapterJobs(t *testing.T) {
	disabledNative := job(cronjob.ActionNotify, perRun(), cronjob.Trigger{Kind: cronjob.TriggerCron, Cron: "0 9 * * *"})
	disabledNative.Definition.Enabled = false
	cases := []struct {
		name         string
		job          cronjob.Job
		native       bool
		notifies     bool
		runsWorkflow bool
	}{
		{"notify with a fresh Issue per run", job(cronjob.ActionNotify, perRun(), cronjob.Trigger{}), true, true, false},
		{"a disabled Native job is still Native", disabledNative, true, true, false},
		{"notify with an existing Issue", job(cronjob.ActionNotify, existingIssue(), cronjob.Trigger{}), false, true, false},
		{"notify with an Issue-date trigger and no Issue", job(cronjob.ActionNotify, nil, cronjob.Trigger{Kind: cronjob.TriggerIssueDeadline}), false, true, false},
		{"execute with a fresh Issue per run", job(cronjob.ActionExecute, perRun(), cronjob.Trigger{}), false, false, false},
		{"execute a Workflow", withWorkflow(job(cronjob.ActionExecute, perRun(), cronjob.Trigger{})), false, false, true},
		{"a workflow uid on a notify job is no Workflow job", withWorkflow(job(cronjob.ActionNotify, existingIssue(), cronjob.Trigger{})), false, true, false},
		{"an action kata adds later is neither", job("page", perRun(), cronjob.Trigger{}), false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.job.Native(); got != tc.native {
				t.Errorf("Native() = %v; want %v", got, tc.native)
			}
			if got := tc.job.Notifies(); got != tc.notifies {
				t.Errorf("Notifies() = %v; want %v", got, tc.notifies)
			}
			if got := tc.job.RunsWorkflow(); got != tc.runsWorkflow {
				t.Errorf("RunsWorkflow() = %v; want %v", got, tc.runsWorkflow)
			}
			wantParent := ""
			if tc.native {
				wantParent = "P"
			}
			if got := tc.job.ParentUID(); got != wantParent {
				t.Errorf("ParentUID() = %q; want %q", got, wantParent)
			}
		})
	}
}

func withWorkflow(j cronjob.Job) cronjob.Job {
	j.Definition.Action.WorkflowUID = "W"
	return j
}

// The anchor rule: what an interval or @every counts from depends on whose job it is.
func TestScheduleAnchorRule(t *testing.T) {
	created := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	delivery := time.Date(2026, 10, 1, 8, 0, 30, 0, time.UTC)
	now := time.Date(2026, 10, 1, 8, 10, 0, 0, time.UTC)
	interval := cronjob.Trigger{Kind: cronjob.TriggerInterval, IntervalSeconds: 3600}
	every := cronjob.Trigger{Kind: cronjob.TriggerCron, Cron: "@every 1h"}
	cases := []struct {
		name     string
		job      cronjob.Job
		anchor   time.Time
		wantNext time.Time
		wantOK   bool
	}{
		{"an Adapter job's interval counts from its creation", job(cronjob.ActionExecute, perRun(), interval), time.Time{}, created.Add(time.Hour), true},
		{"an Adapter job's interval ignores a delivery anchor", job(cronjob.ActionExecute, perRun(), interval), delivery, created.Add(time.Hour), true},
		{"an Adapter job's @every counts from its creation", job(cronjob.ActionExecute, perRun(), every), time.Time{}, created.Add(time.Hour), true},
		{"a Native job's interval counts from the delivery anchor it is given", job(cronjob.ActionNotify, perRun(), interval), delivery, delivery.Add(time.Hour), true},
		{"a Native job's interval with no delivery anchor is unknown", job(cronjob.ActionNotify, perRun(), interval), time.Time{}, time.Time{}, false},
		// kata's own trigger documentation leaves a Native job's @every unanchored, and the Pane's missed count depends on it:
		// the parser's own schedule counts from the instant asked about, never from the creation or the delivery anchor.
		{"a Native job's @every stays unanchored", job(cronjob.ActionNotify, perRun(), every), delivery, now.Add(time.Hour), true},
		{"a Native job's @every ignores the creation", job(cronjob.ActionNotify, perRun(), every), time.Time{}, now.Add(time.Hour), true},
		{"an expression is read in its zone", job(cronjob.ActionExecute, perRun(), cronjob.Trigger{Kind: cronjob.TriggerCron, Cron: "0 9 * * *", Timezone: "Europe/Berlin"}), time.Time{}, time.Date(2026, 10, 2, 7, 0, 0, 0, time.UTC), true},
		{"an expression that does not parse is unknown", job(cronjob.ActionExecute, perRun(), cronjob.Trigger{Kind: cronjob.TriggerCron, Cron: "nope"}), time.Time{}, time.Time{}, false},
		{"an interval out of bounds is unknown", job(cronjob.ActionExecute, perRun(), cronjob.Trigger{Kind: cronjob.TriggerInterval}), time.Time{}, time.Time{}, false},
		{"an Adapter job with no known creation has no anchored interval", withoutCreation(job(cronjob.ActionExecute, perRun(), interval)), time.Time{}, time.Time{}, false},
		{"a once trigger has no schedule", job(cronjob.ActionExecute, perRun(), cronjob.Trigger{Kind: cronjob.TriggerOnce, At: "2026-11-01T09:00:00Z"}), time.Time{}, time.Time{}, false},
		{"a manual trigger has no schedule", job(cronjob.ActionExecute, perRun(), cronjob.Trigger{Kind: cronjob.TriggerManual}), time.Time{}, time.Time{}, false},
		{"an Issue-date trigger has no schedule", job(cronjob.ActionNotify, nil, cronjob.Trigger{Kind: cronjob.TriggerIssueDeadline}), time.Time{}, time.Time{}, false},
		{"a trigger kata adds later has no schedule", job(cronjob.ActionExecute, perRun(), cronjob.Trigger{Kind: "webhook"}), time.Time{}, time.Time{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			schedule, ok := tc.job.Schedule(tc.anchor)
			if ok != tc.wantOK {
				t.Fatalf("Schedule ok = %v; want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if got := schedule.Next(now); !got.Equal(tc.wantNext) {
				t.Errorf("Next(%v) = %v; want %v", now, got, tc.wantNext)
			}
		})
	}
}

func withoutCreation(j cronjob.Job) cronjob.Job {
	j.CreatedAt = time.Time{}
	return j
}

func TestIntervalAnchor(t *testing.T) {
	delivery := time.Date(2026, 10, 1, 8, 0, 30, 0, time.UTC)
	adapter := job(cronjob.ActionExecute, perRun(), cronjob.Trigger{})
	native := job(cronjob.ActionNotify, perRun(), cronjob.Trigger{})
	if got, ok := adapter.IntervalAnchor(delivery); !ok || !got.Equal(adapter.CreatedAt) {
		t.Errorf("Adapter job anchor = %v, %v; want its creation", got, ok)
	}
	if got, ok := native.IntervalAnchor(delivery); !ok || !got.Equal(delivery) {
		t.Errorf("Native job anchor = %v, %v; want the delivery anchor", got, ok)
	}
	if _, ok := native.IntervalAnchor(time.Time{}); ok {
		t.Error("a Native job with no delivery anchor has an interval anchor")
	}
}

func TestIssuesPerDay(t *testing.T) {
	now := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	hourly := cronjob.Trigger{Kind: cronjob.TriggerCron, Cron: "0 * * * *"}
	every := func(seconds int64) cronjob.Trigger {
		return cronjob.Trigger{Kind: cronjob.TriggerInterval, IntervalSeconds: seconds}
	}
	disabled := job(cronjob.ActionExecute, perRun(), hourly)
	disabled.Definition.Enabled = false
	cases := []struct {
		name string
		job  cronjob.Job
		want int
	}{
		{"an hourly cron expression", job(cronjob.ActionExecute, perRun(), hourly), 24},
		{"a weekday cron expression averages over the week", job(cronjob.ActionExecute, perRun(), cronjob.Trigger{Kind: cronjob.TriggerCron, Cron: "0 9 * * 1-5"}), 1},
		{"an interval of 15 minutes", job(cronjob.ActionExecute, perRun(), every(900)), 96},
		{"an interval counted from its creation", job(cronjob.ActionExecute, perRun(), every(3600)), 24},
		{"@every as a cron expression", job(cronjob.ActionExecute, perRun(), cronjob.Trigger{Kind: cronjob.TriggerCron, Cron: "@every 30m"}), 48},
		{"a Workflow job counts like a prompt job", withWorkflow(job(cronjob.ActionExecute, perRun(), hourly)), 24},
		{"a disabled job makes none", disabled, 0},
		{"a Native job makes children, not Issues of its own", job(cronjob.ActionNotify, perRun(), every(60)), 0},
		{"a notify job makes none", job(cronjob.ActionNotify, existingIssue(), every(60)), 0},
		{"an execute job on an existing Issue makes none", job(cronjob.ActionExecute, existingIssue(), every(60)), 0},
		{"an execute job with no Issue policy makes none", job(cronjob.ActionExecute, nil, every(60)), 0},
		{"a once trigger has no rate", job(cronjob.ActionExecute, perRun(), cronjob.Trigger{Kind: cronjob.TriggerOnce, At: "2026-11-01T09:00:00Z"}), 0},
		{"a manual trigger has no rate", job(cronjob.ActionExecute, perRun(), cronjob.Trigger{Kind: cronjob.TriggerManual}), 0},
		{"an expression that does not parse has no rate", job(cronjob.ActionExecute, perRun(), cronjob.Trigger{Kind: cronjob.TriggerCron, Cron: "nope"}), 0},
		{"an action kata adds later makes none", job("page", perRun(), every(60)), 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.job.IssuesPerDay(now); got != tc.want {
				t.Errorf("IssuesPerDay = %d; want %d", got, tc.want)
			}
		})
	}
}

func TestArchived(t *testing.T) {
	j := job(cronjob.ActionExecute, perRun(), cronjob.Trigger{})
	if j.Archived() {
		t.Error("a job with no deletion time is archived")
	}
	when := time.Now()
	j.DeletedAt = &when
	if !j.Archived() {
		t.Error("a job with a deletion time is not archived")
	}
}
