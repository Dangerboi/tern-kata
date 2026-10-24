package data

import (
	"context"
	"net/http"

	"github.com/Dangerboi/tern-kata/internal/model"
	"go.kenn.io/kata/pkg/client"
	g "go.kenn.io/kata/pkg/client/generated"
)

// MoveToProject asks kata to move the Issue to another Project. A dry run changes nothing and reports the Issue's current
// revision; kata assigns the new short id only when it really moves, so a preview cannot name it. The move is a Guarded write:
// a command with a Revision sends it as If-Match, and kata refuses one that is no longer the Issue's, which the answer reports
// as a conflict rather than an error. A command without one is only a dry run, which kata checks against the current revision.
func (c *Client) MoveToProject(ctx context.Context, cmd model.MoveToProjectCmd) (model.ProjectMoveAnswer, error) {
	answer := model.ProjectMoveAnswer{UID: cmd.UID, ToProjectUID: cmd.ToProjectUID, DryRun: cmd.DryRun}
	actor, dryRun := cmd.Actor, cmd.DryRun
	options := &g.MoveIssueRequestOptions{
		PathParams: &g.MoveIssuePath{ProjectID: cmd.ProjectID, Ref: cmd.UID},
		Body:       &g.MoveIssueBody{Actor: &actor, DryRun: &dryRun, ToProjectUID: cmd.ToProjectUID},
	}
	if cmd.Revision != 0 {
		guard := etag(cmd.Revision)
		options.Header = &g.MoveIssueHeaders{IfMatch: &guard}
	}
	response, err := c.MoveIssueWithResponse(ctx, options)
	if client.StatusCode(err) == http.StatusPreconditionFailed && daemonCode(err) == "revision_conflict" {
		answer.Conflict = true
		return answer, nil
	}
	if err != nil {
		return answer, daemonError(err)
	}
	if response.JSON200 != nil {
		answer.Revision = response.JSON200.Issue.Revision
		if response.JSON200.NewShortID != nil {
			answer.NewShortID = *response.JSON200.NewShortID
		}
	}
	return answer, nil
}
