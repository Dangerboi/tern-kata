package data

import (
	"context"
	"errors"

	"github.com/Dangerboi/tern-kata/internal/model"
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
