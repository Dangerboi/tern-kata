package data

import (
	"context"
	"fmt"
	"github.com/Dangerboi/tern-kata/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

func (c *Client) Close(ctx context.Context, cmd model.CloseIssueCmd) error {
	body := cmd.Body
	etag := fmt.Sprintf("\"rev-%d\"", cmd.Revision)
	response, err := c.CloseIssueWithResponse(ctx, &g.CloseIssueRequestOptions{PathParams: &g.CloseIssuePath{ProjectID: cmd.ProjectID, Ref: cmd.UID}, Header: &g.CloseIssueHeaders{IfMatch: &etag}, Body: &body})
	if response != nil && response.StatusCode >= 400 {
		return ReadError(response.Body, err)
	}
	return err
}

func (c *Client) Reopen(ctx context.Context, cmd model.ReopenIssueCmd) error {
	actor := cmd.Actor
	response, err := c.ReopenIssueWithResponse(ctx, &g.ReopenIssueRequestOptions{PathParams: &g.ReopenIssuePath{ProjectID: cmd.ProjectID, Ref: cmd.UID}, Body: &g.ReopenIssueBody{Actor: &actor}})
	if response != nil && response.StatusCode >= 400 {
		return ReadError(response.Body, err)
	}
	return err
}
