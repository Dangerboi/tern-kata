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
