package model_test

import (
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestScheduleWordsCoverEveryTriggerKind(t *testing.T) {
	for _, tc := range []struct {
		name    string
		trigger model.CronTrigger
		want    string
	}{
		{"cron in a zone", model.CronTrigger{Kind: model.TriggerCron, Cron: "0 9 * * *", Timezone: "Europe/Berlin"}, "every day at 09:00 Europe/Berlin"},
		{"cron with no zone reads in UTC", model.CronTrigger{Kind: model.TriggerCron, Cron: "30 6 * * 1-5"}, "every weekday at 06:30 UTC"},
		{"cron on named days", model.CronTrigger{Kind: model.TriggerCron, Cron: "0 18 * * Mon,Thu", Timezone: "UTC"}, "every Mon, Thu at 18:00 UTC"},
		{"cron every few minutes", model.CronTrigger{Kind: model.TriggerCron, Cron: "*/15 * * * *", Timezone: "UTC"}, "every 15 minutes UTC"},
		{"cron on a day of the month", model.CronTrigger{Kind: model.TriggerCron, Cron: "0 8 1 * *", Timezone: "UTC"}, "on day 1 of every month at 08:00 UTC"},
		{"cron descriptor", model.CronTrigger{Kind: model.TriggerCron, Cron: "@daily", Timezone: "Europe/Berlin"}, "every day at 00:00 Europe/Berlin"},
		{"cron with a zone in the expression", model.CronTrigger{Kind: model.TriggerCron, Cron: "CRON_TZ=Asia/Tokyo 0 7 * * *"}, "every day at 07:00 Asia/Tokyo"},
		{"cron too odd for a phrase reads as itself", model.CronTrigger{Kind: model.TriggerCron, Cron: "5 4 1-15 1,6 *", Timezone: "UTC"}, "cron 5 4 1-15 1,6 * UTC"},
		{"unparseable cron", model.CronTrigger{Kind: model.TriggerCron, Cron: "every tuesday"}, ""},
		{"cron with an unknown zone", model.CronTrigger{Kind: model.TriggerCron, Cron: "0 9 * * *", Timezone: "Mars/Olympus"}, ""},
		{"interval of minutes", model.CronTrigger{Kind: model.TriggerInterval, IntervalSeconds: 900}, "every 15 minutes"},
		{"interval of one hour", model.CronTrigger{Kind: model.TriggerInterval, IntervalSeconds: 3600}, "every hour"},
		{"interval of two days", model.CronTrigger{Kind: model.TriggerInterval, IntervalSeconds: 172800}, "every 2 days"},
		{"interval that is no whole minute", model.CronTrigger{Kind: model.TriggerInterval, IntervalSeconds: 90}, "every 90 seconds"},
		{"once in a zone", model.CronTrigger{Kind: model.TriggerOnce, At: "2026-11-01T08:00:00Z", Timezone: "Europe/Berlin"}, "once at 2026-11-01 09:00 Europe/Berlin"},
		{"once in UTC", model.CronTrigger{Kind: model.TriggerOnce, At: "2026-11-01T08:00:00Z"}, "once at 2026-11-01 08:00 UTC"},
		{"once with an offset", model.CronTrigger{Kind: model.TriggerOnce, At: "2026-11-01T09:30:00+01:00"}, "once at 2026-11-01 09:30 +01:00"},
		{"once without an instant", model.CronTrigger{Kind: model.TriggerOnce, At: "tomorrow"}, ""},
		{"manual", model.CronTrigger{Kind: model.TriggerManual}, "manual"},
		{"issue scheduled", model.CronTrigger{Kind: model.TriggerIssueScheduled, IssueUID: "I"}, "at the Issue's scheduled date"},
		{"issue deadline", model.CronTrigger{Kind: model.TriggerIssueDeadline, IssueUID: "I"}, "at the Issue's deadline"},
		{"issue deadline with a lead", model.CronTrigger{Kind: model.TriggerIssueDeadline, IssueUID: "I", LeadSeconds: 2 * 86400}, "2 days before the Issue's deadline"},
		{"issue deadline with a one hour lead", model.CronTrigger{Kind: model.TriggerIssueDeadline, IssueUID: "I", LeadSeconds: 3600}, "1 hour before the Issue's deadline"},
	} {
		if got := tc.trigger.Words(); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

func nativeJob(trigger model.CronTrigger, parent *model.CronJobParent) model.CronJob {
	return model.CronJob{
		UID: "J", Name: "Report",
		Definition: model.CronDefinition{
			Enabled: true, Trigger: trigger, Overlap: "allow", Catchup: "all",
			Action: model.CronAction{Kind: model.ActionNotify},
			Issue:  &model.CronIssuePolicy{Kind: model.IssuePerRun, ParentUID: "P", Title: "Tick"},
		},
		Parent: parent,
	}
}

func TestPauseReasonsFollowTheJobsAndItsParentsState(t *testing.T) {
	open := &model.CronJobParent{UID: "P", Status: "open"}
	archivedAt := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	disabled := nativeJob(model.CronTrigger{}, open)
	disabled.Definition.Enabled = false
	archived := nativeJob(model.CronTrigger{}, &model.CronJobParent{UID: "P", Status: "closed"})
	archived.DeletedAt = &archivedAt
	adapter := model.CronJob{Definition: model.CronDefinition{Enabled: true, Action: model.CronAction{Kind: model.ActionExecute, Prompt: "p"}}}
	for _, tc := range []struct {
		name            string
		job             model.CronJob
		projectArchived bool
		want            model.PauseReason
		state           string
	}{
		{"running", nativeJob(model.CronTrigger{}, open), false, model.NotPaused, "active"},
		{"closed parent", nativeJob(model.CronTrigger{}, &model.CronJobParent{UID: "P", Status: "closed"}), false, model.PauseParentClosed, "paused: parent closed"},
		{"deleted parent", nativeJob(model.CronTrigger{}, &model.CronJobParent{UID: "P", Status: "open", Deleted: true}), false, model.PauseParentDeleted, "paused: parent deleted"},
		{"archived Project", nativeJob(model.CronTrigger{}, open), true, model.PauseProjectArchived, "paused: Project archived"},
		{"disabled job beats a closed parent", func() model.CronJob {
			j := nativeJob(model.CronTrigger{}, &model.CronJobParent{UID: "P", Status: "closed"})
			j.Definition.Enabled = false
			return j
		}(), false, model.PauseDisabled, "disabled"},
		{"disabled", disabled, false, model.PauseDisabled, "disabled"},
		{"archived job beats everything", archived, true, model.PauseArchived, "archived"},
		{"adapter job runs", adapter, false, model.NotPaused, "active"},
		{"adapter job of an archived Project", adapter, true, model.PauseProjectArchived, "paused: Project archived"},
	} {
		got := tc.job.Pause(tc.projectArchived)
		if got != tc.want || got.StateText() != tc.state {
			t.Errorf("%s: %q reading %q, want %q reading %q", tc.name, got, got.StateText(), tc.want, tc.state)
		}
	}
}

func TestNextRunIsLeftOutWhereItCannotBeWorkedOut(t *testing.T) {
	now := time.Date(2026, 10, 22, 10, 7, 0, 0, time.UTC)
	anchor := time.Date(2026, 10, 22, 8, 0, 0, 0, time.UTC)
	open := &model.CronJobParent{UID: "P", Status: "open", Progress: &model.CronProgress{Anchor: anchor, Through: now}}
	adapter := func(trigger model.CronTrigger) model.CronJob {
		return model.CronJob{Definition: model.CronDefinition{Enabled: true, Trigger: trigger, Action: model.CronAction{Kind: model.ActionExecute, Prompt: "p"}}}
	}
	for _, tc := range []struct {
		name string
		job  model.CronJob
		want string
	}{
		{"cron in its zone", adapter(model.CronTrigger{Kind: model.TriggerCron, Cron: "0 9 * * *", Timezone: "Europe/Berlin"}), "2026-10-23T07:00:00Z"},
		{"native interval counts from the anchor", nativeJob(model.CronTrigger{Kind: model.TriggerInterval, IntervalSeconds: 3600}, open), "2026-10-22T11:00:00Z"},
		{"once in the future", adapter(model.CronTrigger{Kind: model.TriggerOnce, At: "2026-11-01T08:00:00Z"}), "2026-11-01T08:00:00Z"},
		{"once in the past", adapter(model.CronTrigger{Kind: model.TriggerOnce, At: "2026-10-01T08:00:00Z"}), ""},
		{"adapter interval has no known start", adapter(model.CronTrigger{Kind: model.TriggerInterval, IntervalSeconds: 3600}), ""},
		{"native interval without progress", nativeJob(model.CronTrigger{Kind: model.TriggerInterval, IntervalSeconds: 3600}, &model.CronJobParent{Status: "open"}), ""},
		{"unparseable cron", adapter(model.CronTrigger{Kind: model.TriggerCron, Cron: "nope"}), ""},
		{"manual", adapter(model.CronTrigger{Kind: model.TriggerManual}), ""},
		{"issue date", adapter(model.CronTrigger{Kind: model.TriggerIssueScheduled, IssueUID: "I"}), ""},
		{"paused job", nativeJob(model.CronTrigger{Kind: model.TriggerInterval, IntervalSeconds: 3600}, &model.CronJobParent{Status: "closed", Progress: open.Progress}), ""},
	} {
		next, ok := tc.job.NextRun(now, false)
		got := ""
		if ok {
			got = next.UTC().Format(time.RFC3339)
		}
		if got != tc.want {
			t.Errorf("%s: next run %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestMissedCountsTheOccurrencesDeliveryOwes(t *testing.T) {
	anchor := time.Date(2026, 10, 22, 8, 0, 0, 0, time.UTC)
	through := time.Date(2026, 10, 22, 9, 30, 0, 0, time.UTC)
	now := time.Date(2026, 10, 22, 12, 15, 0, 0, time.UTC)
	parent := &model.CronJobParent{UID: "P", Status: "closed", Progress: &model.CronProgress{Anchor: anchor, Through: through}}
	if got := nativeJob(model.CronTrigger{Kind: model.TriggerInterval, IntervalSeconds: 3600}, parent).Missed(now); got != 3 {
		t.Errorf("interval: %d missed, want 3 (10:00, 11:00, 12:00)", got)
	}
	if got := nativeJob(model.CronTrigger{Kind: model.TriggerCron, Cron: "0 * * * *", Timezone: "UTC"}, parent).Missed(now); got != 3 {
		t.Errorf("cron: %d missed, want 3", got)
	}
	if got := nativeJob(model.CronTrigger{Kind: model.TriggerInterval, IntervalSeconds: 3600}, &model.CronJobParent{Status: "closed"}).Missed(now); got != 0 {
		t.Errorf("without progress: %d missed, want 0", got)
	}
}
