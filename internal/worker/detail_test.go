package worker

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

func TestADamagedJournalWithMoreStepsThanItsWorkflowHasNoKindForTheExtraOnes(t *testing.T) {
	j := journalAt(PhaseStarted, "", func(j *Journal) {
		j.Workflow = &WorkflowPlan{Steps: []PlanStep{{Kind: cronjob.StepCommand}}}
		j.Steps = []StepRecord{{State: StepSucceeded}, {State: StepRunning, Dir: "step-2"}}
	})
	steps := stepsOf(j)
	if len(steps) != 2 || steps[0].Kind != cronjob.StepCommand || steps[1].Kind != "" {
		t.Fatalf("steps = %+v", steps)
	}
	w := &Worker{store: NewStore(Dirs{Config: t.TempDir(), State: t.TempDir()}, Hooks{})}
	if folder, id := w.sessionOf(j); folder != "" || id != "" {
		t.Fatalf("sessionOf = %q, %q", folder, id)
	}
}
