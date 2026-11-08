package data

import (
	"context"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/worker"
)

func TestTheWorkersReasonForSettingAJobAsideReachesThePane(t *testing.T) {
	status := worker.Status{Executor: "laptop", Projects: []worker.ProjectStatus{{Name: "work", Admitting: true, Capacity: 1,
		Aside: []worker.JobNote{{UID: "J1", Name: "Triage", Text: `it names profile "nope", which is not available to Project work in worker.toml`}}}}}
	dirs, _ := fakeWorker(t, map[string]worker.Response{"status": {OK: true, Status: &status}})
	got := workerGateway(dirs).loadWorker(context.Background(), model.LoadWorker{Request: 1})
	if len(got.Status.Projects) != 1 || got.Status.Projects[0].Unsupported["J1"] != status.Projects[0].Aside[0].Text {
		t.Fatalf("status %+v", got.Status)
	}
}

func TestTheWorkersSkipsOfAJobReachThePane(t *testing.T) {
	at := time.Date(2026, 10, 8, 11, 30, 0, 0, time.UTC)
	status := worker.Status{Executor: "laptop", Projects: []worker.ProjectStatus{{Name: "work", Admitting: true, Capacity: 1,
		Skipped: []worker.JobSkips{{UID: "J1", Name: "Triage", Count: 3, LatestCondition: "runs_per_day", LatestReason: "2 of 2 runs today", LatestAt: at}}}}}
	dirs, _ := fakeWorker(t, map[string]worker.Response{"status": {OK: true, Status: &status}})
	got := workerGateway(dirs).loadWorker(context.Background(), model.LoadWorker{Request: 1})
	want := model.JobSkips{UID: "J1", Name: "Triage", Count: 3, LatestCondition: "runs_per_day", LatestReason: "2 of 2 runs today", LatestAt: at}
	if len(got.Status.Projects) != 1 || len(got.Status.Projects[0].Skipped) != 1 || got.Status.Projects[0].Skipped[0] != want {
		t.Fatalf("status %+v, want the skips %+v", got.Status, want)
	}
}
