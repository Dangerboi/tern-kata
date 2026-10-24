package data

import (
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
	"github.com/Dangerboi/tern-kata/internal/model"
)

const onFailFlow = `{
  "name": "Maker and checker",
  "definition": {
    "version": 1,
    "steps": [
      {"key": "make", "kind": "prompt", "prompt": "Make it.", "options": {"on_fail": {"step": "make"}}}
    ]
  }
}`

func TestTheWorkersReadingOfAWorkflowArrivesWithALoadAndWithASaveAndTheSaveIsStillWritten(t *testing.T) {
	kata := startKata(t, "0.20.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, bound := project(t, gateway, "demo")

	saved := saveWorkflow(t, gateway, workspace, bound, "", "", onFailFlow)
	if saved.Err != nil || saved.Workflow == nil {
		t.Fatalf("saved %+v", saved)
	}
	if saved.Reading == nil || len(saved.Reading.Refusals) != 1 || !strings.Contains(saved.Reading.Refusals[0], "on_fail") {
		t.Fatalf("the save's reading %+v, want the on_fail refusal", saved.Reading)
	}
	if !saved.Reading.BudgetUnchecked || !saved.Reading.ConfigMissing {
		t.Errorf("with no worker.toml the reading %+v should say the budget was not checked because the file is missing", saved.Reading)
	}
	if listed := loadWorkflows(t, gateway, workspace, bound); len(listed) != 1 || listed[0].UID != saved.Workflow.UID {
		t.Fatalf("the refused Workflow is not in kata: %+v", listed)
	}

	loaded := as[model.WorkflowsLoaded](t, run(t, gateway, model.LoadWorkflows{Workspace: workspace, ProjectID: bound.ID, ProjectUID: bound.UID}))
	reading, ok := loaded.Readings[saved.Workflow.UID]
	if !ok || len(reading.Refusals) != 1 || !strings.Contains(reading.Refusals[0], "on_fail") {
		t.Errorf("the load's readings %+v, want the on_fail refusal for %s", loaded.Readings, saved.Workflow.UID)
	}
}

func TestTheWorkersReadingOfATextTakesTheBudgetFromWorkerTomlAndEchoesTheRequest(t *testing.T) {
	prompts := func(n int) cronjob.WorkflowDefinition {
		steps := make([]cronjob.WorkflowStep, n)
		for i := range steps {
			steps[i] = cronjob.WorkflowStep{Key: string(rune('a' + i)), Kind: cronjob.StepPrompt, Prompt: "p"}
		}
		return cronjob.WorkflowDefinition{Version: 1, Steps: steps}
	}
	for _, tt := range []struct {
		name, file string
		def        cronjob.WorkflowDefinition
		refusals   int
		unchecked  bool
		missing    bool
		why        string
	}{
		{"within the budget", "executor = \"laptop\"\n[limits]\nmax_run_prompts = 2\n[profiles.default]\n", prompts(2), 0, false, false, ""},
		{"over the budget", "executor = \"laptop\"\n[limits]\nmax_run_prompts = 2\n[profiles.default]\n", prompts(3), 1, false, false, ""},
		{"no worker.toml", "", prompts(3), 0, true, true, "no worker.toml"},
		{"worker.toml that does not load", "executor = \n", prompts(3), 0, true, false, "could not be read"},
		{"no worker.toml, a config-free refusal", "", cronjob.WorkflowDefinition{Version: 1, Steps: []cronjob.WorkflowStep{{Key: "a", Kind: cronjob.StepCommand, Command: "true", Options: map[string]any{"profile": "x"}}}}, 1, true, true, "no worker.toml"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g, dirs := setupGateway(t)
			if tt.file != "" {
				writeWorkerFile(t, dirs, "worker.toml", tt.file)
			}
			got := as[model.WorkflowTextRead](t, run(t, g, model.ReadWorkflowText{Request: 41, Definition: tt.def}))
			r := got.Reading
			if got.Request != 41 || len(r.Refusals) != tt.refusals || r.BudgetUnchecked != tt.unchecked || r.ConfigMissing != tt.missing || !strings.Contains(r.BudgetWhy, tt.why) {
				t.Errorf("answer %+v", got)
			}
		})
	}
}
