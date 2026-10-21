package data

import (
	"context"
	"errors"
	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
	"strconv"
)

func (c *Client) CreateIssue(ctx context.Context, cmd model.CreateIssueCmd) (model.IssueCreated, error) {
	body := g.CreateIssueBody{Title: cmd.Title, Body: &cmd.Body, Actor: &cmd.Actor, Labels: cmd.Labels, Priority: cmd.Priority}
	if cmd.Owner != "" {
		body.Owner = &cmd.Owner
	}
	if cmd.ParentUID != "" {
		body.Links = []g.CreateInitialLinkBody{{Type: "parent", ToRef: cmd.ParentUID}}
	}
	if cmd.ScheduledOn != "" || cmd.DeadlineOn != "" {
		body.Metadata = map[string]any{}
		if cmd.ScheduledOn != "" {
			body.Metadata[model.ScheduledKey] = cmd.ScheduledOn
		}
		if cmd.DeadlineOn != "" {
			body.Metadata[model.DeadlineKey] = cmd.DeadlineOn
		}
	}
	response, err := c.CreateIssueWithResponse(ctx, &g.CreateIssueRequestOptions{PathParams: &g.CreateIssuePath{ProjectID: strconv.FormatInt(cmd.ProjectID, 10)}, Body: &body, Header: &g.CreateIssueHeaders{IdempotencyKey: idempotencyKey(cmd.Key)}})
	if err != nil {
		return model.IssueCreated{}, daemonError(err)
	}
	if response == nil || response.JSON200 == nil {
		return model.IssueCreated{}, errors.New("kata returned no created Issue")
	}
	return model.IssueCreated{UID: response.JSON200.Issue.UID}, nil
}

// idempotencyKey is the Idempotency-Key header value for key, nil when the write carries none.
func idempotencyKey(key string) *string {
	if key == "" {
		return nil
	}
	return &key
}
