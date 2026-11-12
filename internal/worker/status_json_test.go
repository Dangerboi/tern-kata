package worker

import (
	"encoding/json"
	"strings"
	"testing"
)

// workersummary.luau reads the worker's status JSON by these exact Go field names, so renaming a field here would blank the
// plugin's worker widget without any Go test failing; this pins them, and that a run still going encodes a zero EndedAt as year 1.
func TestStatusJSONKeepsTheKeysTheWorkerSummaryReads(t *testing.T) {
	st := Status{
		Executor: "omp",
		PID:      42,
		Projects: []ProjectStatus{{Name: "p", Capacity: 3, InUse: 1}},
		Runs: []RunStatus{
			{Issue: "p#1", Job: "j", Held: true, Hold: "approval", Detail: "d", HoldLine: "waits for approval"},
			{Issue: "p#2", Job: "j", State: "running"},
		},
	}
	raw, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	has := func(obj any, keys ...string) map[string]any {
		t.Helper()
		m, ok := obj.(map[string]any)
		if !ok {
			t.Fatalf("%v is not an object", obj)
		}
		for _, k := range keys {
			if _, ok := m[k]; !ok {
				t.Errorf("status JSON lacks %q: %s", k, raw)
			}
		}
		return m
	}
	has(got, "PID", "Executor", "Projects", "Runs")
	has(got["Projects"].([]any)[0], "Capacity", "InUse")
	runs := got["Runs"].([]any)
	has(runs[0], "Held", "Hold", "HoldLine", "Issue", "Job", "EndedAt")
	running := has(runs[1], "Held", "Issue", "Job", "EndedAt")
	if at, _ := running["EndedAt"].(string); !strings.HasPrefix(at, "0001") {
		t.Errorf("a running run's EndedAt is %q, want the zero time starting 0001", at)
	}
}
