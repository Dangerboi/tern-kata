package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/worker"
)

// The Tern plugin's workersummary.luau reads `worker status --json` by these key names and by the worker's zero time for a run
// that has not ended; this test fails when a rename or a changed encoding would leave the worker widget empty.
func TestWorkerStatusJSONKeepsTheKeysThePluginReads(t *testing.T) {
	ended := time.Date(2026, 11, 12, 9, 30, 0, 0, time.UTC)
	status := worker.Status{
		Executor: "build-box",
		PID:      4242,
		Projects: []worker.ProjectStatus{{Name: "a", Capacity: 3, InUse: 1}},
		Runs: []worker.RunStatus{
			{Job: "review", Issue: "kata-a#12", State: "held: agent_login", Held: true, Hold: "agent_login", HoldLine: "agent_login: not signed in", EndedAt: ended},
			{Job: "build", Issue: "kata-a#13", State: "running"},
		},
	}
	var out bytes.Buffer
	if err := writeJSON(&out, status); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Executor string
		PID      float64
		Projects []map[string]any
		Runs     []map[string]any
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("status --json = %q: %v", out.String(), err)
	}
	if got.Executor != "build-box" || got.PID != 4242 || len(got.Projects) != 1 || len(got.Runs) != 2 {
		t.Fatalf("status --json = %s; want Executor, PID, Projects and Runs by those names", out.String())
	}
	if got.Projects[0]["Capacity"] != float64(3) || got.Projects[0]["InUse"] != float64(1) {
		t.Errorf("project = %v; want Capacity 3 and InUse 1 by those names", got.Projects[0])
	}
	held, running := got.Runs[0], got.Runs[1]
	if held["Issue"] != "kata-a#12" || held["Held"] != true || held["Hold"] != "agent_login" || held["HoldLine"] != "agent_login: not signed in" {
		t.Errorf("held run = %v; want Issue, Held, Hold and HoldLine by those names", held)
	}
	if at, _ := held["EndedAt"].(string); at == "" || strings.HasPrefix(at, "0001") {
		t.Errorf("held run EndedAt = %v; want its real time", held["EndedAt"])
	}
	if at, _ := running["EndedAt"].(string); !strings.HasPrefix(at, "0001") || running["Held"] != false {
		t.Errorf("running run = %v; want the zero time (year 1) for EndedAt and Held false", running)
	}
}
