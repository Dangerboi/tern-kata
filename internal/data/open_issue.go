package data

import (
	"context"
	"fmt"

	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

// ResolveIssue is the UID of the Issue with shortID in the Project, by the lookup `--issue` and kata:// links use.
func (c *Client) ResolveIssue(ctx context.Context, projectID int64, shortID string) (string, error) {
	response, err := c.ResolveUIIssueReferenceWithResponse(ctx, &g.ResolveUIIssueReferenceRequestOptions{Query: &g.ResolveUIIssueReferenceQuery{ProjectID: projectID, Ref: shortID}})
	if err != nil {
		if response != nil {
			return "", ReadError(response.Body, err)
		}
		return "", err
	}
	if response.JSON200 == nil {
		return "", fmt.Errorf("kata daemon returned no Issue reference")
	}
	return response.JSON200.Issue.UID, nil
}

func (c *Client) OpenIssue(ctx context.Context, cmd model.OpenIssue) (model.IssueOpened, error) {
	_, shortID, err := model.ParseIssueRef(cmd.Ref)
	if err != nil {
		return model.IssueOpened{}, err
	}
	uid, err := c.ResolveIssue(ctx, cmd.ProjectID, shortID)
	if err != nil {
		return model.IssueOpened{}, err
	}
	detail, err := c.ReadDetail(ctx, uid)
	if err != nil {
		return model.IssueOpened{}, err
	}
	return model.IssueOpened{Ref: cmd.Ref, Selected: detail.Selected}, nil
}
