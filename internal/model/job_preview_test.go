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

func TestThePreviewOfANewJobCountsAnIntervalFromNow(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	for _, kind := range []string{"native", "prompt"} {
		card := model.JobFormState{Kind: kind, Trigger: cronjob.TriggerInterval, Interval: "900"}
		p := card.Preview(model.PaneCalendar{}, now)
		if len(p.Runs) != 3 || p.Runs[0] != "Thu 2026-10-08 12:15 UTC" {
			t.Errorf("%s: runs %v, want three from 12:15", kind, p.Runs)
		}
	}
}

func TestThePreviewOfAnExistingNativeIntervalJobCountsFromItsParentsDeliveryAnchor(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	native := cronjob.Job{Definition: cronjob.Definition{
		Action: cronjob.Action{Kind: cronjob.ActionNotify},
		Issue:  &cronjob.IssuePolicy{Kind: cronjob.IssuePerRun, ParentUID: "p"},
	}}
	opened := model.CronJob{Job: native, Parent: &model.CronJobParent{Progress: &cronjob.Progress{Anchor: time.Date(2026, 10, 8, 11, 50, 0, 0, time.UTC)}}}
	p := intervalCard(opened).Preview(model.PaneCalendar{}, now)
	if len(p.Runs) != 3 || p.Runs[0] != "Thu 2026-10-08 12:05 UTC" {
		t.Errorf("runs %v, want three from 12:05", p.Runs)
	}
}

func TestThePreviewOfACronJobListsRunsInItsZone(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	card := model.JobFormState{Trigger: cronjob.TriggerCron, Cron: "0 9 * * *", Zone: "America/New_York"}
	p := card.Preview(model.PaneCalendar{}, now)
	if len(p.Runs) != 3 || p.Runs[0] != "Thu 2026-10-08 09:00 EDT" {
		t.Errorf("runs %v, want three from Thu 09:00 EDT", p.Runs)
	}
}

func TestThePreviewOfAZoneThatDoesNotReadListsNoRunsAndDoesNotPanic(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	card := model.JobFormState{Trigger: cronjob.TriggerCron, Cron: "0 9 * * *", Zone: `"America/New_York"`}
	p := card.Preview(model.PaneCalendar{}, now)
	if len(p.Runs) != 0 || p.Err == nil {
		t.Errorf("runs %v, err %v; want no runs and an error", p.Runs, p.Err)
	}
}
