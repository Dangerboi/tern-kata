package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"go.kenn.io/kata/pkg/client"
	g "go.kenn.io/kata/pkg/client/generated"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
	"github.com/Dangerboi/tern-kata/internal/model"
)

// kata's code for a write guarded on a definition_event_uid that has moved, whose error data carries the current job.
const cronConflict = "cron_conflict"

// SaveJob creates or replaces one Cron job. A create is a PUT to the uid the card minted, so a resend of one that already landed
// is no second job; a replace is guarded by the definition_event_uid the card read, which kata carries in the body. A guard that
// moved writes nothing and comes back as the current job in Changed.
func (c *Client) SaveJob(ctx context.Context, cmd model.SaveJobCmd) (model.JobWritten, error) {
	def := cmd.Definition
	if cmd.ParentRef != "" {
		parent, err := c.resolveIssue(ctx, cmd.ProjectID, cmd.ParentRef, "Parent")
		if err != nil {
			return model.JobWritten{}, err
		}
		policy := cronjob.IssuePolicy{Kind: cronjob.IssuePerRun}
		if def.Issue != nil {
			policy = *def.Issue
		}
		policy.ParentUID = parent
		def.Issue = &policy
	}
	if cmd.IssueRef != "" {
		issue, err := c.resolveIssue(ctx, cmd.ProjectID, cmd.IssueRef, "Target Issue")
		if err != nil {
			return model.JobWritten{}, err
		}
		def.Trigger.IssueUID = ""
		if model.DateTrigger(def.Trigger.Kind) {
			def.Trigger.IssueUID = issue
		}
		if def.Issue != nil && def.Issue.Kind == cronjob.IssueExisting {
			// Kata makes a job that executes report to the Issue its trigger fires on.
			policy := *def.Issue
			policy.UID = issue
			def.Issue = &policy
		}
	}
	if cmd.ReportRef != "" && def.Issue != nil && def.Issue.Kind == cronjob.IssueExisting {
		issue, err := c.resolveIssue(ctx, cmd.ProjectID, cmd.ReportRef, "Notified Issue")
		if err != nil {
			return model.JobWritten{}, err
		}
		policy := *def.Issue
		policy.UID = issue
		def.Issue = &policy
	}
	body := g.PutCronJobBody{Actor: &cmd.Actor, Name: cmd.Name, Definition: wireDefinition(def)}
	op := model.JobCreated
	if cmd.ExpectedEventUID != "" {
		body.ExpectedEventUID, op = &cmd.ExpectedEventUID, model.JobReplaced
	}
	response, err := c.ReplaceCronJobWithResponse(ctx, &g.ReplaceCronJobRequestOptions{
		PathParams: &g.ReplaceCronJobPath{ProjectID: cmd.ProjectID, CronUID: cmd.UID}, Body: &body,
	})
	parents := parentReader{c: c, projectID: cmd.ProjectID, read: map[string]readParent{}}
	if err != nil {
		current, found, readErr := c.conflictingJob(ctx, cmd.ProjectID, cmd.UID, err, &parents)
		switch {
		case readErr != nil:
			return model.JobWritten{}, readErr
		case !found:
			return model.JobWritten{}, daemonError(err)
		case op == model.JobCreated && current.Name == cmd.Name && reflect.DeepEqual(withoutEmptyMaps(current.Definition), withoutEmptyMaps(def)):
			// The same create, sent again after the first one landed.
			return model.JobWritten{ProjectID: cmd.ProjectID, UID: cmd.UID, Op: op, Job: &current}, nil
		}
		return model.JobWritten{ProjectID: cmd.ProjectID, UID: cmd.UID, Changed: &current}, nil
	}
	return c.writtenJob(ctx, cmd.ProjectID, op, response.JSON200, &parents)
}

// ArchiveJob archives the job guarded on the definition_event_uid the Pane read, which kata sends as a DELETE with a body.
func (c *Client) ArchiveJob(ctx context.Context, cmd model.ArchiveJobCmd) (model.JobWritten, error) {
	response, err := c.ArchiveCronJobWithResponse(ctx, &g.ArchiveCronJobRequestOptions{
		PathParams: &g.ArchiveCronJobPath{ProjectID: cmd.ProjectID, CronUID: cmd.UID},
		Body:       &g.ArchiveCronJobBody{Actor: &cmd.Actor, ExpectedEventUID: cmd.ExpectedEventUID},
	})
	parents := parentReader{c: c, projectID: cmd.ProjectID, read: map[string]readParent{}}
	if err != nil {
		return c.guardedFailure(ctx, cmd.ProjectID, cmd.UID, err, &parents)
	}
	return c.writtenJob(ctx, cmd.ProjectID, model.JobArchived, response.JSON200, &parents)
}

// RestoreJob restores an archived job, guarded like ArchiveJob.
func (c *Client) RestoreJob(ctx context.Context, cmd model.RestoreJobCmd) (model.JobWritten, error) {
	response, err := c.RestoreCronJobWithResponse(ctx, &g.RestoreCronJobRequestOptions{
		PathParams: &g.RestoreCronJobPath{ProjectID: cmd.ProjectID, CronUID: cmd.UID},
		Body:       &g.RestoreCronJobBody{Actor: &cmd.Actor, ExpectedEventUID: cmd.ExpectedEventUID},
	})
	parents := parentReader{c: c, projectID: cmd.ProjectID, read: map[string]readParent{}}
	if err != nil {
		return c.guardedFailure(ctx, cmd.ProjectID, cmd.UID, err, &parents)
	}
	return c.writtenJob(ctx, cmd.ProjectID, model.JobRestored, response.JSON200, &parents)
}

// guardedFailure is a failed guarded write: the current job when kata refused for a moved guard, else kata's error.
func (c *Client) guardedFailure(ctx context.Context, projectID int64, uid string, err error, parents *parentReader) (model.JobWritten, error) {
	current, found, readErr := c.conflictingJob(ctx, projectID, uid, err, parents)
	switch {
	case readErr != nil:
		return model.JobWritten{}, readErr
	case !found:
		return model.JobWritten{}, daemonError(err)
	}
	return model.JobWritten{ProjectID: projectID, UID: uid, Changed: &current}, nil
}

func (c *Client) writtenJob(ctx context.Context, projectID int64, op model.JobOp, body *g.CronJobResponseBody, parents *parentReader) (model.JobWritten, error) {
	if body == nil {
		return model.JobWritten{}, errors.New("kata daemon returned no Cron job")
	}
	job, err := parents.translate(ctx, body.Job)
	if err != nil {
		return model.JobWritten{}, err
	}
	return model.JobWritten{ProjectID: projectID, UID: job.UID, Op: op, Job: &job}, nil
}

// conflictingJob is the current job of a write kata refused as a conflict: the one in the error's data, else read afresh. found
// is false when err is not that conflict.
func (c *Client) conflictingJob(ctx context.Context, projectID int64, uid string, err error, parents *parentReader) (job model.CronJob, found bool, _ error) {
	envelope, ok := errors.AsType[g.ErrorEnvelope](err)
	if !ok || client.StatusCode(err) != http.StatusConflict || envelope.ErrorData.Code != cronConflict {
		return model.CronJob{}, false, nil
	}
	var wire g.CronJob
	if raw, marshalErr := json.Marshal(envelope.ErrorData.Data["current"]); marshalErr == nil && json.Unmarshal(raw, &wire) == nil && wire.UID != "" {
		job, readErr := parents.translate(ctx, wire)
		return job, readErr == nil, readErr
	}
	response, showErr := c.ShowCronJobWithResponse(ctx, &g.ShowCronJobRequestOptions{PathParams: &g.ShowCronJobPath{ProjectID: projectID, CronUID: uid}})
	if showErr != nil {
		return model.CronJob{}, false, daemonError(showErr)
	}
	if response.JSON200 == nil {
		return model.CronJob{}, false, errors.New("kata daemon returned no Cron job")
	}
	job, readErr := parents.translate(ctx, response.JSON200.Job)
	return job, readErr == nil, readErr
}

// resolveIssue is the full uid of the Issue a reference names: a short id with or without its #, or a uid. The Issue must be
// live. label names the card's field in the message when there is no such Issue.
func (c *Client) resolveIssue(ctx context.Context, projectID int64, ref, label string) (string, error) {
	ref = strings.TrimPrefix(strings.TrimSpace(ref), "#")
	response, err := c.ShowIssueWithResponse(ctx, &g.ShowIssueRequestOptions{PathParams: &g.ShowIssuePath{ProjectID: projectID, Ref: ref}})
	switch {
	case client.StatusCode(err) == http.StatusNotFound:
		return "", fmt.Errorf("%s: no Issue %q in this Project.", label, ref)
	case err != nil:
		return "", daemonError(err)
	case response.JSON200 == nil:
		return "", errors.New("kata daemon returned no parent Issue")
	}
	return response.JSON200.Issue.UID, nil
}

// wireDefinition is the model's definition as kata takes it: version 1 of a job, a field only when it holds something, except
// the ones whose zero is a value (an Issue-deadline trigger's lead).
func wireDefinition(d cronjob.Definition) g.CronJobDefinition {
	t, a := d.Trigger, d.Action
	wire := g.CronJobDefinition{
		Version: 1, Kind: "job", Enabled: d.Enabled, Overlap: d.Overlap, Catchup: d.Catchup,
		Trigger:      g.Trigger{Kind: string(t.Kind), Cron: textOrNil(t.Cron), At: textOrNil(t.At), IssueUID: textOrNil(t.IssueUID), Timezone: textOrNil(t.Timezone)},
		Action:       g.Action{Kind: string(a.Kind), Message: textOrNil(a.Message), Recipient: textOrNil(a.Recipient), Prompt: textOrNil(a.Prompt), WorkflowUID: textOrNil(a.WorkflowUID)},
		GraceSeconds: positiveOrNil(d.GraceSeconds), TimeoutSeconds: positiveOrNil(d.TimeoutSeconds), CheckoutKey: textOrNil(d.CheckoutKey),
		Options: d.Options,
	}
	if t.Kind == cronjob.TriggerInterval {
		wire.Trigger.IntervalSeconds = &t.IntervalSeconds
	}
	if t.Kind == cronjob.TriggerIssueDeadline {
		wire.Trigger.LeadSeconds = &t.LeadSeconds
	}
	if len(d.SecretRefs) > 0 {
		wire.SecretRefs = d.SecretRefs
	}
	if p := d.Issue; p != nil {
		wire.Issue = &g.IssuePolicy{
			Kind: string(p.Kind), UID: textOrNil(p.UID), ParentUID: textOrNil(p.ParentUID), Title: textOrNil(p.Title), Body: textOrNil(p.Body),
			ScheduledOffsetSeconds: p.ScheduledOffsetSeconds, DeadlineOffsetSeconds: p.DeadlineOffsetSeconds,
		}
	}
	return wire
}

// withoutEmptyMaps is d with an empty SecretRefs or Options as nil, how kata hands them back: it omits an empty object, so a
// definition sent with one reads back with none.
func withoutEmptyMaps(d cronjob.Definition) cronjob.Definition {
	if len(d.SecretRefs) == 0 {
		d.SecretRefs = nil
	}
	if len(d.Options) == 0 {
		d.Options = nil
	}
	return d
}

func textOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func positiveOrNil(n int64) *int64 {
	if n <= 0 {
		return nil
	}
	return &n
}
