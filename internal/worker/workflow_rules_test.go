package worker

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

func TestAWorkflowKataTakesIsRefusedOnlyByTheWorkersOwnPolicy(t *testing.T) {
	prompt := func(key string, retries int64) cronjob.WorkflowStep {
		return cronjob.WorkflowStep{Key: key, Kind: cronjob.StepPrompt, Prompt: "p", Retries: &retries}
	}
	def := cronjob.WorkflowDefinition{Version: 1, Steps: []cronjob.WorkflowStep{prompt("a", 0), prompt("b", 1)}, Options: map[string]any{"mystery": true}}
	for limit, wantRefused := range map[int]bool{3: false, 2: true} {
		if got := CheckWorkflow(def, &limit); (len(got.Refusals) != 0) != wantRefused {
			t.Errorf("a budget of %d: refusals %q, want refused = %v", limit, got.Refusals, wantRefused)
		}
	}
}

func TestCheckWorkflowNamesEveryRefusalAndTheUncheckedBudget(t *testing.T) {
	retries := int64(2)
	def := cronjob.WorkflowDefinition{Version: 1,
		Options: map[string]any{"on_failure": "park", "mystery": true},
		Steps: []cronjob.WorkflowStep{
			{Key: "a", Kind: cronjob.StepPrompt, Prompt: "p", Retries: &retries, Options: map[string]any{"on_fail": map[string]any{}}},
			{Key: "b", Kind: cronjob.StepCommand, Command: "true", Options: map[string]any{"profile": "x"}},
		}}
	limit := 2
	got := CheckWorkflow(def, &limit)
	if len(got.Refusals) != 4 || got.BudgetUnchecked {
		t.Fatalf("refusals %q, unchecked %v; want park, on_fail, profile and the budget", got.Refusals, got.BudgetUnchecked)
	}
	unknown := CheckWorkflow(def, nil)
	if len(unknown.Refusals) != 3 || !unknown.BudgetUnchecked {
		t.Errorf("without a limit: refusals %q, unchecked %v; want the three that need no limit", unknown.Refusals, unknown.BudgetUnchecked)
	}
	plain := cronjob.WorkflowDefinition{Version: 1, Options: map[string]any{"mystery": true}, Steps: []cronjob.WorkflowStep{{Key: "a", Kind: cronjob.StepCommand, Command: "true", Options: map[string]any{"other": 1}}}}
	if got := CheckWorkflow(plain, &limit); len(got.Refusals) != 0 {
		t.Errorf("unknown option keys refused: %q", got.Refusals)
	}
}
