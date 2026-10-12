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
	response, err := c.CreateIssueWithResponse(ctx, &g.CreateIssueRequestOptions{PathParams: &g.CreateIssuePath{ProjectID: strconv.FormatInt(cmd.ProjectID, 10)}, Body: &body})
	if err != nil {
		if response != nil {
			err = ReadError(response.Body, err)
		}
		return model.IssueCreated{}, err
	}
	if response == nil || response.JSON200 == nil {
		return model.IssueCreated{}, errors.New("kata returned no created Issue")
	}
	return model.IssueCreated{UID: response.JSON200.Issue.UID}, nil
}
