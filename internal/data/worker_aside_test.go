package data

import (
	"context"
	"testing"

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
