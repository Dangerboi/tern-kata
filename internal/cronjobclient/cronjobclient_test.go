package cronjobclient_test

import (
	"testing"
	"time"

	g "go.kenn.io/kata/pkg/client/generated"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
	"github.com/Dangerboi/tern-kata/internal/cronjobclient"
)

func TestJobReadsEveryFieldAndMissingValuesAsEmpty(t *testing.T) {
	created := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	interval := int64(900)
	text := "ship it"
	wire := g.CronJob{
		UID: "J", Name: "nightly", Author: "me", Revision: 3, DefinitionEventUID: "E", ProjectID: 7, CreatedAt: created, UpdatedAt: created,
		Definition: g.CronJobDefinitionResponse{
			Enabled: true,
			Trigger: g.TriggerResponse{Kind: "interval", IntervalSeconds: &interval},
			Action:  g.ActionResponse{Kind: "execute", Prompt: &text},
			Issue:   &g.IssuePolicyResponse{Kind: "per-run"},
			Options: map[string]any{"profile": "fast"},
		},
	}
	got := cronjobclient.Job(wire)
	if got.UID != "J" || got.Name != "nightly" || got.Revision != 3 || got.DefinitionEventUID != "E" || got.ProjectID != 7 || !got.CreatedAt.Equal(created) {
		t.Errorf("job identity %+v", got)
	}
	d := got.Definition
	if d.Trigger.Kind != cronjob.TriggerInterval || d.Trigger.IntervalSeconds != 900 || d.Trigger.Cron != "" || d.Trigger.LeadSeconds != 0 {
		t.Errorf("trigger %+v", d.Trigger)
	}
	if d.Action.Kind != cronjob.ActionExecute || d.Action.Prompt != text || d.Action.WorkflowUID != "" {
		t.Errorf("action %+v", d.Action)
	}
	if d.Issue == nil || d.Issue.Kind != cronjob.IssuePerRun || d.Issue.UID != "" {
		t.Errorf("issue policy %+v", d.Issue)
	}
	if d.Profile != "fast" || d.Options["profile"] != "fast" {
		t.Errorf("profile %q, options %v", d.Profile, d.Options)
	}
}

func TestJobKeepsAKindKataAddsLater(t *testing.T) {
	wire := g.CronJob{Definition: g.CronJobDefinitionResponse{Trigger: g.TriggerResponse{Kind: "webhook"}, Action: g.ActionResponse{Kind: "page"}}}
	got := cronjobclient.Job(wire).Definition
	if got.Trigger.Kind != "webhook" || got.Action.Kind != "page" {
		t.Errorf("kinds %q, %q", got.Trigger.Kind, got.Action.Kind)
	}
}

func TestWorkflowDefinitionReadsEveryFieldAndMissingValuesAsEmpty(t *testing.T) {
	about, command, prompt, retries := "nightly", "make", "review", int64(2)
	got := cronjobclient.WorkflowDefinition(g.CronWorkflowDefinitionResponse{
		Version: 1, About: &about, Options: map[string]any{"profile": "fast"},
		Steps: []g.CronWorkflowStepResponse{
			{Key: "build", Kind: "command", Command: &command, Retries: &retries},
			{Key: "review", Kind: "prompt", Prompt: &prompt, After: []string{"build"}, Options: map[string]any{"profile": "deep"}},
		},
	})
	if got.Version != 1 || got.About != "nightly" || got.Input != "" || got.Options.(map[string]any)["profile"] != "fast" || len(got.Steps) != 2 {
		t.Fatalf("definition %+v", got)
	}
	build, review := got.Steps[0], got.Steps[1]
	if build.Kind != cronjob.StepCommand || build.Command != "make" || build.Prompt != "" || build.RetryCount() != 2 || build.Options != nil {
		t.Errorf("command step %+v", build)
	}
	if review.Kind != cronjob.StepPrompt || review.Prompt != "review" || review.Retries != nil || review.After[0] != "build" || review.Options.(map[string]any)["profile"] != "deep" {
		t.Errorf("prompt step %+v", review)
	}
	if problem := cronjob.ValidateWorkflow(got); problem != nil {
		t.Errorf("the translated definition is refused: %s", problem.Message())
	}
}

func TestRunReadsEveryFieldAndMissingValuesAsEmpty(t *testing.T) {
	created := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	started := created.Add(time.Minute)
	in, out := int64(20), int64(4)
	job, def, wf, wfDef, key, issue, label, team, msg := "J", "DE", "W", "WE", "k1", "I", "box", "ann", "done"
	got := cronjobclient.Run(g.CronRun{
		UID: "R", JobUID: &job, DefinitionEventUID: &def, WorkflowUID: &wf, WorkflowDefinitionEventUID: &wfDef,
		OccurrenceKey: &key, IssueUID: &issue, Actor: "me", Teammate: &team, ExecutorLabel: &label, Status: "succeeded",
		Summary:  g.SummaryResponse{Message: &msg, InputTokens: &in, OutputTokens: &out},
		Revision: 2, CreatedAt: created, UpdatedAt: started, StartedAt: &started,
	})
	if got.UID != "R" || got.JobUID != job || got.DefinitionEventUID != def || got.WorkflowUID != wf || got.WorkflowDefinitionEventUID != wfDef ||
		got.OccurrenceKey != key || got.IssueUID != issue || got.Actor != "me" || got.Teammate != team || got.ExecutorLabel != label || got.Revision != 2 {
		t.Errorf("run %+v", got)
	}
	if got.Status != cronjob.RunSucceeded || got.Summary != msg || got.InputTokens == nil || *got.InputTokens != 20 || got.OutputTokens == nil || *got.OutputTokens != 4 {
		t.Errorf("status and summary %+v", got)
	}
	if !got.CreatedAt.Equal(created) || got.StartedAt == nil || !got.StartedAt.Equal(started) || got.EndedAt != nil {
		t.Errorf("times %+v", got)
	}
	bare := cronjobclient.Run(g.CronRun{UID: "R2", Status: "running"})
	if bare.JobUID != "" || bare.WorkflowDefinitionEventUID != "" || bare.Summary != "" || bare.InputTokens != nil || bare.Status != cronjob.RunRunning {
		t.Errorf("bare run %+v", bare)
	}
}

func TestRunReadsAStatusItDoesNotKnowAsUnknown(t *testing.T) {
	for _, status := range []string{"paused", "", "RUNNING"} {
		if got := cronjobclient.Run(g.CronRun{UID: "R", Status: status}).Status; got != cronjob.RunUnknown {
			t.Errorf("status %q read as %q, want unknown", status, got)
		}
	}
}
