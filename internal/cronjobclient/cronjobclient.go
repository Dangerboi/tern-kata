// Package cronjobclient is the one translation from kata's generated client types into the values of package cronjob. Only the
// Daemon gateway and the Worker import it; the cronjob package itself, and so the model, never see a client type.
package cronjobclient

import (
	g "go.kenn.io/kata/pkg/client/generated"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

// Str reads a value kata leaves out as empty.
func Str(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Int reads a value kata leaves out as zero.
func Int(n *int64) int64 {
	if n == nil {
		return 0
	}
	return *n
}

// Job is kata's Cron job as the cronjob package holds it.
func Job(j g.CronJob) cronjob.Job {
	d := j.Definition
	def := cronjob.Definition{
		Enabled: d.Enabled,
		Trigger: cronjob.Trigger{
			Kind: cronjob.TriggerKind(d.Trigger.Kind), Cron: Str(d.Trigger.Cron), IntervalSeconds: Int(d.Trigger.IntervalSeconds),
			At: Str(d.Trigger.At), IssueUID: Str(d.Trigger.IssueUID), LeadSeconds: Int(d.Trigger.LeadSeconds), Timezone: Str(d.Trigger.Timezone),
		},
		Action: cronjob.Action{
			Kind: cronjob.ActionKind(d.Action.Kind), Message: Str(d.Action.Message), Recipient: Str(d.Action.Recipient),
			Prompt: Str(d.Action.Prompt), WorkflowUID: Str(d.Action.WorkflowUID),
		},
		Overlap: d.Overlap, Catchup: d.Catchup, GraceSeconds: Int(d.GraceSeconds), TimeoutSeconds: Int(d.TimeoutSeconds),
		CheckoutKey: Str(d.CheckoutKey), SecretRefs: d.SecretRefs, Options: d.Options,
	}
	def.Profile, _ = d.Options["profile"].(string)
	if i := d.Issue; i != nil {
		def.Issue = &cronjob.IssuePolicy{
			Kind: cronjob.IssuePolicyKind(i.Kind), UID: Str(i.UID), ParentUID: Str(i.ParentUID), Title: Str(i.Title), Body: Str(i.Body),
			ScheduledOffsetSeconds: i.ScheduledOffsetSeconds, DeadlineOffsetSeconds: i.DeadlineOffsetSeconds,
		}
	}
	return cronjob.Job{
		UID: j.UID, Name: j.Name, Author: j.Author, Revision: j.Revision, DefinitionEventUID: j.DefinitionEventUID,
		ProjectID: j.ProjectID, CreatedAt: j.CreatedAt, UpdatedAt: j.UpdatedAt, DeletedAt: j.DeletedAt, Definition: def,
	}
}

// WorkflowDefinition is kata's Workflow definition as the cronjob package holds it, ready for cronjob.ValidateWorkflow.
// Options stay nil when kata sends none, so an absent value and an empty object read the same.
func WorkflowDefinition(d g.CronWorkflowDefinitionResponse) cronjob.WorkflowDefinition {
	out := cronjob.WorkflowDefinition{Version: d.Version, About: Str(d.About), Input: Str(d.Input), Options: options(d.Options), Steps: make([]cronjob.WorkflowStep, len(d.Steps))}
	for i, s := range d.Steps {
		out.Steps[i] = cronjob.WorkflowStep{
			Key: s.Key, Kind: cronjob.StepKind(s.Kind), Command: Str(s.Command), Prompt: Str(s.Prompt),
			After: s.After, Retries: s.Retries, Options: options(s.Options),
		}
	}
	return out
}

// options is a definition's options, nil when kata sends none.
func options(m map[string]any) any {
	if m == nil {
		return nil
	}
	return m
}
