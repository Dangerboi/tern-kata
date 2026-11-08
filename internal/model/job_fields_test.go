package model_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestJobCardTabSkipsTheFieldsThatDoNotApplyAndStepsItsChoices(t *testing.T) {
	var m model.Model
	m.JobForm = model.JobFormState{Kind: "native", Trigger: cronjob.TriggerCron, Enabled: true, Catchup: model.CatchupLatest}
	m.JobForm.Open, m.JobForm.Field = true, model.JobFieldKind
	want := []model.Field{model.JobFieldKind, model.JobFieldName, model.JobFieldTrigger, model.JobFieldCron, model.JobFieldZone, model.JobFieldParent,
		model.JobFieldTitle, model.JobFieldBody, model.JobFieldRecipient, model.JobFieldMessage, model.JobFieldCatchup, model.JobFieldEnabled}
	var visited []model.Field
	for range want {
		visited = append(visited, m.JobForm.Field)
		m, _ = clickPress(m, "tab")
	}
	if !slices.Equal(visited, want) || m.JobForm.Field != want[0] {
		t.Fatalf("tab visits %v and wraps to %q, want %v", visited, m.JobForm.Field, want)
	}

	m, _ = model.Update(m, model.FocusFieldMsg{Field: model.JobFieldEnabled})
	m, _ = clickPress(m, "space")
	if m.JobForm.Enabled {
		t.Fatalf("space on Enabled left it on")
	}
	m, _ = model.Update(m, model.ChooseMsg{Field: model.JobFieldEnabled, Index: 0})
	if !m.JobForm.Enabled {
		t.Fatalf("choosing Yes left Enabled off")
	}
}

func TestSteppingAJobChoiceWithNoChoicesKeepsTheError(t *testing.T) {
	var m model.Model
	m.JobForm = model.JobFormState{Kind: "workflow", Trigger: cronjob.TriggerCron, Enabled: true, Catchup: model.CatchupLatest, WorkflowsRead: true}
	m.JobForm.Open, m.JobForm.Field, m.JobForm.Err = true, model.JobFieldWorkflow, errors.New("pick the workflow to run.")
	for _, chord := range []string{"space", "left", "right"} {
		if next, _ := clickPress(m, chord); next.JobForm.Err == nil {
			t.Errorf("%s on a workflow picker with no workflows dismissed the error", chord)
		}
	}
}
