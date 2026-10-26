package driver_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

// A double click on a card of the Jobs, Runs, Workflows and Attention Pages does what selecting it and pressing Enter does.

func TestDoubleClickOnAJobCardSelectsItAndFocusesTheInspector(t *testing.T) {
	s := jobsPane(t, 160, promptJob("J1", "A"), promptJob("J2", "B"))
	m := step(t, s, click(jobRow(t, s.Model, "J2"), view.JobOpenClick("J2"))).State.Model
	if m.Jobs.Selected != "J2" || m.Focus != model.ColumnDetail || m.CurrentMode() != model.ModeJobInspector {
		t.Errorf("selected %q focus %v mode %q, want J2 with the inspector focused", m.Jobs.Selected, m.Focus, m.CurrentMode())
	}
	entered := step(t, feed(t, s, key("j")).State, key("enter")).State.Model
	if entered.Jobs.Selected != m.Jobs.Selected || entered.Focus != m.Focus {
		t.Errorf("enter gave selected %q focus %v", entered.Jobs.Selected, entered.Focus)
	}

	unknown := step(t, s, click(jobRow(t, s.Model, "J1"), view.JobOpenClick("nope"))).State.Model
	if unknown.Jobs.Selected != "J1" || unknown.Focus != model.ColumnList {
		t.Errorf("a uid off the list moved selection to %q focus %v", unknown.Jobs.Selected, unknown.Focus)
	}
}

func TestDoubleClickOnAJobCardUnderAPopupCardDoesNothing(t *testing.T) {
	s := jobsPane(t, 160, promptJob("J1", "A"), promptJob("J2", "B"))
	row := jobRow(t, s.Model, "J2")
	s = step(t, s, key("n")).State
	if s.Model.TopCard() == 0 {
		t.Fatal("n opened no card")
	}
	m := step(t, s, click(row, view.JobOpenClick("J2"))).State.Model
	if m.Jobs.Selected != "J1" || m.Focus != s.Model.Focus || m.TopCard() != s.Model.TopCard() {
		t.Errorf("selected %q focus %v card %v, want nothing changed", m.Jobs.Selected, m.Focus, m.TopCard())
	}
}

func TestDoubleClickOnARunCardSelectsItAndReadsTheRunAgain(t *testing.T) {
	s := runsPane(t, 160, finishedRun("R2"), finishedRun("R1"))
	out := step(t, s, click(runRow(t, s.Model, "R1"), view.RunOpenClick("R1")))
	m := out.State.Model
	if m.Runs.Selected != "R1" || m.CurrentMode() != model.ModeRunInspector {
		t.Fatalf("selected %q mode %q, want R1 in the run inspector", m.Runs.Selected, m.CurrentMode())
	}
	shown := false
	for _, cmd := range out.Cmds {
		if show, ok := cmd.(model.ShowRun); ok && show.UID == "R1" && show.ProjectID == 7 {
			shown = true
		}
	}
	if !shown {
		t.Errorf("cmds %#v, want the run read again", out.Cmds)
	}

	unknown := step(t, s, click(runRow(t, s.Model, "R1"), view.RunOpenClick("nope")))
	if unknown.State.Model.CurrentMode() != model.ModeRuns || len(unknown.Cmds) != 0 {
		t.Errorf("a uid off the list gave mode %q cmds %#v", unknown.State.Model.CurrentMode(), unknown.Cmds)
	}
}

func TestDoubleClickOnARunCardUnderAnotherLayerDoesNothing(t *testing.T) {
	s := runsPane(t, 160, finishedRun("R2"), finishedRun("R1"))
	row := runRow(t, s.Model, "R1")
	s = step(t, s, key("?")).State
	out := step(t, s, click(row, view.RunOpenClick("R1")))
	m := out.State.Model
	if m.Runs.Selected != "R2" || m.CurrentMode() != model.ModeHelp || len(out.Cmds) != 0 {
		t.Errorf("selected %q mode %q cmds %#v, want nothing changed", m.Runs.Selected, m.CurrentMode(), out.Cmds)
	}
}

func TestDoubleClickOnAWorkflowRowSelectsItAndShowsTheSteps(t *testing.T) {
	s := workflowsPane(t, 160, reviewFlow(), nightlyFlow())
	m := step(t, s, click(workflowRow(t, s.Model, "W2"), view.WorkflowOpenClick("W2"))).State.Model
	if m.Workflows.Selected != "W2" || !m.Workflows.Inspecting {
		t.Errorf("selected %q inspecting %v, want W2 with its steps shown", m.Workflows.Selected, m.Workflows.Inspecting)
	}

	unknown := step(t, s, click(workflowRow(t, s.Model, "W1"), view.WorkflowOpenClick("nope"))).State.Model
	if unknown.Workflows.Inspecting {
		t.Error("a uid off the list opened the steps")
	}
}

func TestDoubleClickOnAWorkflowRowUnderAPopupCardDoesNothing(t *testing.T) {
	s := workflowsPane(t, 160, reviewFlow(), nightlyFlow())
	row := workflowRow(t, s.Model, "W2")
	s = step(t, s, key("n")).State
	if s.Model.TopCard() != model.CardWorkflow {
		t.Fatalf("card %v, want the Workflow card", s.Model.TopCard())
	}
	m := step(t, s, click(row, view.WorkflowOpenClick("W2"))).State.Model
	if m.Workflows.Inspecting || m.Workflows.Selected != s.Model.Workflows.Selected {
		t.Errorf("selected %q inspecting %v, want nothing changed", m.Workflows.Selected, m.Workflows.Inspecting)
	}
}

func TestDoubleClickOnAnAttentionCardOpensItsIssueWithTheDetailFocused(t *testing.T) {
	s := attentionPageWith(t, attentionRequests(t), 0)
	m := step(t, s, click(view.AttentionID("work-aa11"), view.AttentionOpenClick("work-aa11"))).State.Model
	if m.Page != model.PageIssues || m.SelectedUID != "work-aa11" || m.Focus != model.ColumnDetail || m.DetailHidden() {
		t.Errorf("page %v selected %q focus %v, want the Issues Page on the request's Issue with its detail focused", m.Page, m.SelectedUID, m.Focus)
	}

	unknown := step(t, s, click(view.AttentionID("work-aa11"), view.AttentionOpenClick("nope"))).State.Model
	if unknown.Page != model.PageAttention {
		t.Errorf("a uid off the list left for page %v", unknown.Page)
	}
}

func TestDoubleClickOnAnAttentionCardUnderAnotherLayerDoesNothing(t *testing.T) {
	s := attentionPageWith(t, attentionRequests(t), 0)
	s = step(t, s, key("?")).State
	m := step(t, s, click(view.AttentionID("work-aa11"), view.AttentionOpenClick("work-aa11"))).State.Model
	if m.Page != model.PageAttention || m.CurrentMode() != model.ModeHelp {
		t.Errorf("page %v mode %q, want the Attention Page under help", m.Page, m.CurrentMode())
	}
}
