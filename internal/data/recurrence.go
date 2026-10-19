package data

import (
	"context"
	"errors"
	"net/http"

	"github.com/Dangerboi/tern-kata/internal/model"
	"go.kenn.io/kata/pkg/client"
	g "go.kenn.io/kata/pkg/client/generated"
)

// CreateRecurrence makes the Issue the first Occurrence of a new recurrence. The template copies the Issue as it is now:
// title, body, priority and labels, never the owner, so an agent's claim on this Occurrence does not carry to the next.
func (c *Client) CreateRecurrence(ctx context.Context, cmd model.CreateRecurrenceCmd) error {
	current, err := c.ShowIssueWithResponse(ctx, &g.ShowIssueRequestOptions{PathParams: &g.ShowIssuePath{ProjectID: cmd.ProjectID, Ref: cmd.UID}})
	if err != nil {
		return daemonError(err)
	}
	if current.JSON200 == nil {
		return errors.New("kata returned no Issue before making it recur")
	}
	issue := current.JSON200.Issue
	_, err = c.CreateRecurrenceWithResponse(ctx, &g.CreateRecurrenceRequestOptions{
		PathParams: &g.CreateRecurrencePath{ProjectID: cmd.ProjectID},
		Body: &g.CreateRecurrenceBody{
			Actor: &cmd.Actor, Rrule: cmd.Rule, Dtstart: cmd.Start, Timezone: cmd.TimeZone, InitialIssueRef: &cmd.UID,
			Template: g.RecurrenceTemplateInput{Title: issue.Title, Body: &issue.Body, Priority: issue.Priority, Labels: issue.Labels},
		},
	})
	return daemonError(err)
}

var errRecurrenceStopped = errors.New("The recurrence was stopped; nothing was written.")

// currentRecurrence reads the recurrence as it stands now, with kata's raw error. A stopped (deleted) one is an error, not a result: kata answers 404 for it.
func (c *Client) currentRecurrence(ctx context.Context, projectID int64, uid string) (model.Recurrence, error) {
	shown, err := c.ShowRecurrenceWithResponse(ctx, &g.ShowRecurrenceRequestOptions{PathParams: &g.ShowRecurrencePath{ProjectID: projectID, RecurrenceUID: uid}})
	if client.StatusCode(err) == http.StatusNotFound {
		return model.Recurrence{}, errRecurrenceStopped
	}
	if err != nil {
		return model.Recurrence{}, err
	}
	if shown.JSON200 == nil || shown.JSON200.Recurrence.DeletedAt != nil {
		return model.Recurrence{}, errRecurrenceStopped
	}
	return translateLiveRecurrence(shown.JSON200.Recurrence), nil
}

// ChangeRecurrence is a Guarded write: it sends the terms, never the template, at the revision the recurrence has now, so a
// revision that moved only because an Occurrence was created does not matter. When the recurrence's terms differ from what the
// card opened with, nothing is written and the current recurrence comes back.
func (c *Client) ChangeRecurrence(ctx context.Context, cmd model.ChangeRecurrenceCmd) (*model.Recurrence, error) {
	var changed *model.Recurrence
	err := recurrenceWrite(func() error {
		current, err := c.currentRecurrence(ctx, cmd.ProjectID, cmd.RecurrenceUID)
		if err != nil {
			return err
		}
		if current.RecurrenceTerms != cmd.Opened {
			changed = &current
			return nil
		}
		tag := etag(current.Revision)
		_, err = c.PatchRecurrenceWithResponse(ctx, &g.PatchRecurrenceRequestOptions{
			PathParams: &g.PatchRecurrencePath{ProjectID: cmd.ProjectID, RecurrenceUID: cmd.RecurrenceUID},
			Header:     &g.PatchRecurrenceHeaders{IfMatch: &tag},
			Body:       &g.PatchRecurrenceBody{Actor: &cmd.Actor, Rrule: &cmd.Rule, Dtstart: &cmd.Start, Timezone: &cmd.TimeZone},
		})
		return err
	})
	return changed, err
}

// StopRecurrence deletes the recurrence at the revision it has now. One already stopped is an error and nothing is written.
func (c *Client) StopRecurrence(ctx context.Context, cmd model.StopRecurrenceCmd) error {
	return recurrenceWrite(func() error {
		current, err := c.currentRecurrence(ctx, cmd.ProjectID, cmd.RecurrenceUID)
		if err != nil {
			return err
		}
		_, err = c.DeleteRecurrenceWithResponse(ctx, &g.DeleteRecurrenceRequestOptions{
			PathParams: &g.DeleteRecurrencePath{ProjectID: cmd.ProjectID, RecurrenceUID: cmd.RecurrenceUID},
			Query:      &g.DeleteRecurrenceQuery{Actor: &cmd.Actor},
			Header:     &g.DeleteRecurrenceHeaders{IfMatch: etag(current.Revision)},
		})
		return err
	})
}
