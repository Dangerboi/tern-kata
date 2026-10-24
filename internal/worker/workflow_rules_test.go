package worker

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

func TestTheWorkerAppliesKatasFullRuleSetToTheDefinitionItReads(t *testing.T) {
	step := func(key string) cronjob.WorkflowStep {
		return cronjob.WorkflowStep{Key: key, Kind: cronjob.StepCommand, Command: "true"}
	}
	for name, tc := range map[string]struct {
		def  cronjob.WorkflowDefinition
		want string
	}{
		"version":      {cronjob.WorkflowDefinition{Version: 2, Steps: []cronjob.WorkflowStep{step("a")}}, `"version" must be 1.`},
		"no steps":     {cronjob.WorkflowDefinition{Version: 1}, `"steps" needs at least one step.`},
		"portable key": {cronjob.WorkflowDefinition{Version: 1, Steps: []cronjob.WorkflowStep{step("../a")}}, `Step "../a": "key" must be a name of letters, digits, "-", "_" or "." (not "." or ".."), at most 256 bytes.`},
		"retries":      {cronjob.WorkflowDefinition{Version: 1, Steps: []cronjob.WorkflowStep{{Key: "a", Kind: cronjob.StepCommand, Command: "true", Retries: new(int64(101))}}}, `Step "a": "retries" must be between 0 and 100.`},
		"options":      {cronjob.WorkflowDefinition{Version: 1, Steps: []cronjob.WorkflowStep{step("a")}, Options: map[string]any{"x": map[string]any{"password": "p"}}}, `"options" cannot contain credentials or local paths.`},
		"kind":         {cronjob.WorkflowDefinition{Version: 1, Steps: []cronjob.WorkflowStep{{Key: "a", Kind: "shell", Command: "true"}}}, `Step "a": "kind" must be "command" or "prompt".`},
	} {
		t.Run(name, func(t *testing.T) {
			if got := workflowRefusal(Workflow{WorkflowDefinition: tc.def}, 12); got != tc.want {
				t.Errorf("refusal %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAWorkflowKataTakesIsRefusedOnlyByTheWorkersOwnPolicy(t *testing.T) {
	prompt := func(key string, retries int64) cronjob.WorkflowStep {
		return cronjob.WorkflowStep{Key: key, Kind: cronjob.StepPrompt, Prompt: "p", Retries: &retries}
	}
	def := cronjob.WorkflowDefinition{Version: 1, Steps: []cronjob.WorkflowStep{prompt("a", 0), prompt("b", 1)}, Options: map[string]any{"mystery": true}}
	if got := workflowRefusal(Workflow{WorkflowDefinition: def}, 3); got != "" {
		t.Errorf("refused within the budget: %q", got)
	}
	if got := workflowRefusal(Workflow{WorkflowDefinition: def}, 2); got == "" {
		t.Error("a budget of 2 took three prompts")
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
	if first := workflowRefusal(Workflow{WorkflowDefinition: def}, limit); first != got.Refusals[0] {
		t.Errorf("planning refuses with %q, the exported check starts with %q", first, got.Refusals[0])
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
