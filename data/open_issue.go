package data

import (
	"context"
	"fmt"

	"github.com/Dangerboi/tern-kata/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

func (c *Client) OpenIssue(ctx context.Context, cmd model.OpenIssue) (model.IssueOpened, error) {
	_, shortID, err := model.ParseIssueRef(cmd.Ref)
	if err != nil {
		return model.IssueOpened{}, err
	}
	response, err := c.ResolveUIIssueReferenceWithResponse(ctx, &g.ResolveUIIssueReferenceRequestOptions{Query: &g.ResolveUIIssueReferenceQuery{ProjectID: cmd.ProjectID, Ref: shortID}})
	if err != nil {
		if response != nil {
			return model.IssueOpened{}, ReadError(response.Body, err)
		}
		return model.IssueOpened{}, err
	}
	if response.JSON200 == nil {
		return model.IssueOpened{}, fmt.Errorf("kata daemon returned no Issue reference")
	}
	detail, err := c.ReadDetail(ctx, response.JSON200.Issue.UID)
	if err != nil {
		return model.IssueOpened{}, err
	}
	return model.IssueOpened{Ref: cmd.Ref, Selected: detail.Selected}, nil
}
