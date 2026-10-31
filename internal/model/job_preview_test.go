package model_test

import (
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
	"github.com/Dangerboi/tern-kata/internal/model"
)

func intervalCard(opened model.CronJob) model.JobFormState {
	return model.JobFormState{Change: true, Opened: opened, Trigger: cronjob.TriggerInterval, Interval: "900"}
}

func TestThePreviewOfAnExistingIntervalJobWithoutAStartPointListsNoRunsAndSaysWhy(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	native := cronjob.Job{Definition: cronjob.Definition{
		Action: cronjob.Action{Kind: cronjob.ActionNotify},
		Issue:  &cronjob.IssuePolicy{Kind: cronjob.IssuePerRun, ParentUID: "p"},
	}}
	adapter := cronjob.Job{}
	for name, opened := range map[string]model.CronJob{
		"native job with no readable progress": {Job: native},
		"adapter job with no creation time":    {Job: adapter},
	} {
		p := intervalCard(opened).Preview(model.PaneCalendar{}, now)
		if len(p.Runs) != 0 || p.Note == "" {
			t.Errorf("%s: runs %v, note %q; want no runs and a note", name, p.Runs, p.Note)
		}
	}
}

func TestThePreviewOfAnExistingIntervalJobWithAStartPointCountsFromIt(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	created := time.Date(2026, 10, 8, 11, 55, 0, 0, time.UTC)
	p := intervalCard(model.CronJob{Job: cronjob.Job{CreatedAt: created}}).Preview(model.PaneCalendar{}, now)
	if len(p.Runs) != 3 || p.Note != "" {
		t.Fatalf("runs %v, note %q", p.Runs, p.Note)
	}
	if want := "Thu 2026-10-08 12:10 UTC"; p.Runs[0] != want {
		t.Errorf("first run %q, want %q", p.Runs[0], want)
	}
}
