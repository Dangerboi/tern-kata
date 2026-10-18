package data

import (
	"context"
	"errors"
	"net/http"

	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

// Close is a Guarded write: an Issue that is still open closes at the revision it has now, whatever
// else changed since the Close card opened. One that was closed meanwhile is an error and nothing is written.
func (c *Client) Close(ctx context.Context, cmd model.CloseIssueCmd) error {
	body := cmd.Body
	for attempt := 1; ; attempt++ {
		current, err := c.ShowIssueWithResponse(ctx, &g.ShowIssueRequestOptions{PathParams: &g.ShowIssuePath{ProjectID: cmd.ProjectID, Ref: cmd.UID}})
		if err != nil {
			if current != nil {
				err = ReadError(current.Body, err)
			}
			return err
		}
		if current.JSON200 == nil {
			return errors.New("kata returned no Issue before closing")
		}
		if current.JSON200.Issue.Status == "closed" {
			return errors.New("Issue was closed meanwhile; nothing was written")
		}
		tag := etag(current.JSON200.Issue.Revision)
		response, err := c.CloseIssueWithResponse(ctx, &g.CloseIssueRequestOptions{PathParams: &g.CloseIssuePath{ProjectID: cmd.ProjectID, Ref: cmd.UID}, Header: &g.CloseIssueHeaders{IfMatch: &tag}, Body: &body})
		if response != nil && response.StatusCode == http.StatusPreconditionFailed && attempt < guardedAttempts {
			continue
		}
		if response != nil && response.StatusCode >= 400 {
			return ReadError(response.Body, err)
		}
		return err
	}
}

func (c *Client) Reopen(ctx context.Context, cmd model.ReopenIssueCmd) error {
	actor := cmd.Actor
	response, err := c.ReopenIssueWithResponse(ctx, &g.ReopenIssueRequestOptions{PathParams: &g.ReopenIssuePath{ProjectID: cmd.ProjectID, Ref: cmd.UID}, Body: &g.ReopenIssueBody{Actor: &actor}})
	if response != nil && response.StatusCode >= 400 {
		return ReadError(response.Body, err)
	}
	return err
}
