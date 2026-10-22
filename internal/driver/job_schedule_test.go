package driver_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// adapterJobOn is an agent prompt job with the given trigger, made at jobAnchor.
func adapterJobOn(uid string, trigger model.CronTrigger) model.CronJob {
	job := promptJob(uid, "Job "+uid)
	job.Definition.Trigger = trigger
	return job
}

func TestJobsPageRowsPutTheScheduleInWordsForEveryTriggerKind(t *testing.T) {
	cases := []struct {
		trigger model.CronTrigger
		want    string // "" is a trigger that can't be read: the row names the kind and no schedule
	}{
		{model.CronTrigger{Kind: model.TriggerCron, Cron: "0 9 * * *", Timezone: "Europe/Berlin"}, "every day at 09:00 Europe/Berlin"},
		{model.CronTrigger{Kind: model.TriggerCron, Cron: "30 6 * * 1-5"}, "every weekday at 06:30 UTC"},
		{model.CronTrigger{Kind: model.TriggerCron, Cron: "0 18 * * Mon,Thu", Timezone: "UTC"}, "every Mon, Thu at 18:00 UTC"},
		{model.CronTrigger{Kind: model.TriggerCron, Cron: "*/15 * * * *", Timezone: "UTC"}, "every 15 minutes UTC"},
		{model.CronTrigger{Kind: model.TriggerCron, Cron: "0 8 1 * *", Timezone: "UTC"}, "on day 1 of every month at 08:00 UTC"},
		{model.CronTrigger{Kind: model.TriggerCron, Cron: "@daily", Timezone: "Europe/Berlin"}, "every day at 00:00 Europe/Berlin"},
		{model.CronTrigger{Kind: model.TriggerCron, Cron: "CRON_TZ=Asia/Tokyo 0 7 * * *"}, "every day at 07:00 Asia/Tokyo"},
		{model.CronTrigger{Kind: model.TriggerCron, Cron: "5 4 1-15 1,6 *", Timezone: "UTC"}, "cron 5 4 1-15 1,6 * UTC"},
		{model.CronTrigger{Kind: model.TriggerCron, Cron: "every tuesday"}, ""},
		{model.CronTrigger{Kind: model.TriggerCron, Cron: "0 9 * * *", Timezone: "Mars/Olympus"}, ""},
		{model.CronTrigger{Kind: model.TriggerInterval, IntervalSeconds: 900}, "every 15 minutes"},
		{model.CronTrigger{Kind: model.TriggerInterval, IntervalSeconds: 3600}, "every hour"},
		{model.CronTrigger{Kind: model.TriggerInterval, IntervalSeconds: 172800}, "every 2 days"},
		{model.CronTrigger{Kind: model.TriggerInterval, IntervalSeconds: 90}, "every 90 seconds"},
		{model.CronTrigger{Kind: model.TriggerOnce, At: "2026-11-01T08:00:00Z", Timezone: "Europe/Berlin"}, "once at 2026-11-01 09:00 Europe/Berlin"},
		{model.CronTrigger{Kind: model.TriggerOnce, At: "2026-11-01T08:00:00Z"}, "once at 2026-11-01 08:00 UTC"},
		{model.CronTrigger{Kind: model.TriggerOnce, At: "2026-11-01T09:30:00+01:00"}, "once at 2026-11-01 09:30 +01:00"},
		{model.CronTrigger{Kind: model.TriggerOnce, At: "tomorrow"}, ""},
		{model.CronTrigger{Kind: model.TriggerManual}, "manual"},
		{model.CronTrigger{Kind: model.TriggerIssueScheduled, IssueUID: "I"}, "at the Issue's scheduled date"},
		{model.CronTrigger{Kind: model.TriggerIssueDeadline, IssueUID: "I"}, "at the Issue's deadline"},
		{model.CronTrigger{Kind: model.TriggerIssueDeadline, IssueUID: "I", LeadSeconds: 2 * 86400}, "2 days before the Issue's deadline"},
		{model.CronTrigger{Kind: model.TriggerIssueDeadline, IssueUID: "I", LeadSeconds: 3600}, "1 hour before the Issue's deadline"},
	}
	jobs := make([]model.CronJob, len(cases))
	for i, tc := range cases {
		jobs[i] = adapterJobOn(fmt.Sprintf("J%02d", i), tc.trigger)
	}
	s := jobsPane(t, 160, jobs...)
	for i, tc := range cases {
		text := jobCardText(t, s, jobs[i].UID)
		if tc.want == "" {
			if strings.Contains(text, "agent prompt ·") {
				t.Errorf("%+v: the row words a schedule it can't read: %s", tc.trigger, text)
			}
			continue
		}
		wantText(t, fmt.Sprintf("%+v", tc.trigger), text, "agent prompt · "+tc.want)
	}
}

func TestJobsPageRowsLeaveOutTheNextRunWhereItCannotBeWorkedOut(t *testing.T) {
	// The Pane's clock is Thursday 2026-10-08 12:00 UTC; a native job counts from its parent's anchor at 08:00.
	cases := []struct {
		name string
		job  model.CronJob
		want string // "" is no next run
	}{
		{"cron in its zone", adapterJobOn("J1", model.CronTrigger{Kind: model.TriggerCron, Cron: "0 9 * * *", Timezone: "Europe/Berlin"}), "next 2026-10-09 07:00"},
		{"native interval counts from the anchor", nativeJob("J2", "Hourly", jobParent("open", false)), "next 13:00"},
		{"once in the future", adapterJobOn("J3", model.CronTrigger{Kind: model.TriggerOnce, At: "2026-11-01T08:00:00Z"}), "next 2026-11-01 08:00"},
		{"once in the past", adapterJobOn("J4", model.CronTrigger{Kind: model.TriggerOnce, At: "2026-10-01T08:00:00Z"}), ""},
		{"unparseable cron", adapterJobOn("J5", model.CronTrigger{Kind: model.TriggerCron, Cron: "nope"}), ""},
		{"manual", adapterJobOn("J6", model.CronTrigger{Kind: model.TriggerManual}), ""},
		{"issue date", adapterJobOn("J7", model.CronTrigger{Kind: model.TriggerIssueScheduled, IssueUID: "I"}), ""},
		{"native interval without progress", nativeJob("J8", "No progress", &model.CronJobParent{UID: "parent-a", Status: "open"}), ""},
	}
	jobs := make([]model.CronJob, len(cases))
	for i, tc := range cases {
		jobs[i] = tc.job
	}
	s := jobsPane(t, 160, jobs...)
	for _, tc := range cases {
		text := jobCardText(t, s, tc.job.UID)
		if tc.want == "" && strings.Contains(text, "next ") {
			t.Errorf("%s: draws a next run: %s", tc.name, text)
		} else if tc.want != "" {
			wantText(t, tc.name, text, tc.want)
		}
	}
}

func TestResumingACronJobCountsTheOccurrencesItMissedInItsZone(t *testing.T) {
	// The parent's progress stopped at 08:30 and the Pane's clock is 12:00: an hourly cron owes 09:00, 10:00, 11:00 and 12:00.
	job := disabled(owing("J1", "open", "all"))
	job.Definition.Trigger = model.CronTrigger{Kind: model.TriggerCron, Cron: "0 * * * *", Timezone: "UTC"}
	out := step(t, jobsPane(t, 160, job), key("p"))
	wantText(t, "card", backfillText(t, out.State), "4 child Issues", "4 missed runs")
}
