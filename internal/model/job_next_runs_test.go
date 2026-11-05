package model_test

import (
	"slices"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
	"github.com/Dangerboi/tern-kata/internal/model"
)

func nextRunsFacts(job model.CronJob) []model.JobFact {
	job.UID = "J"
	m := model.Model{Jobs: model.JobsState{Jobs: []model.CronJob{job}, Selected: "J"}}
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	var facts []model.JobFact
	for _, f := range m.JobFacts(now) {
		if f.Label == "Next runs" || (len(facts) > 0 && f.Label == "") && len(facts) < 3 {
			facts = append(facts, f)
		}
	}
	return facts
}

func cronJob(enabled bool, trigger cronjob.Trigger, parent *model.CronJobParent) model.CronJob {
	return model.CronJob{
		Job:    cronjob.Job{Definition: cronjob.Definition{Enabled: enabled, Trigger: trigger}},
		Parent: parent,
	}
}

func TestTheJobInspectorListsTheNextThreeRunsOfACronJob(t *testing.T) {
	facts := nextRunsFacts(cronJob(true, cronjob.Trigger{Kind: cronjob.TriggerCron, Cron: "0 9 * * *", Timezone: "UTC"}, nil))
	var got []string
	for _, f := range facts {
		got = append(got, f.Value)
	}
	want := []string{"Fri 2026-10-09 09:00 UTC", "Sat 2026-10-10 09:00 UTC", "Sun 2026-10-11 09:00 UTC"}
	if !slices.Equal(got, want) {
		t.Errorf("runs %v, want %v", got, want)
	}
	if len(facts) == 0 || facts[0].Label != "Next runs" {
		t.Errorf("first fact %+v, want the Next runs label", facts)
	}
}

func TestTheJobInspectorSaysWhyAJobHasNoNextRuns(t *testing.T) {
	cron := cronjob.Trigger{Kind: cronjob.TriggerCron, Cron: "0 9 * * *", Timezone: "UTC"}
	for name, c := range map[string]struct {
		job  model.CronJob
		want string
	}{
		"paused":        {cronJob(false, cron, nil), "none while paused"},
		"closed parent": {cronJob(true, cron, &model.CronJobParent{Status: "closed"}), "none while the parent is closed"},
		"manual":        {cronJob(true, cronjob.Trigger{Kind: cronjob.TriggerManual}, nil), "none: runs only when started"},
	} {
		facts := nextRunsFacts(c.job)
		if len(facts) != 1 || facts[0].Label != "Next runs" || facts[0].Value != c.want {
			t.Errorf("%s: facts %+v, want one Next runs fact %q", name, facts, c.want)
		}
	}
}
