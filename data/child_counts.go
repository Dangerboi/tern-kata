package data

import (
	"context"
	"fmt"

	"github.com/Dangerboi/tern-kata/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

// readChildCounts is kata's direct-child count of each Issue with children,
// whatever the filters left in the snapshot: one list request whose rows carry
// child_counts, scoped to the Project when the Collection is one (the snapshot
// has none of its own). Only a snapshot that changed needs it: the ETag moves
// with every event.
func (c *Client) readChildCounts(ctx context.Context, snapshot g.UISnapshotResponseBody, projectUID *string) (map[string]g.ChildCounts, error) {
	query := &g.ListAllIssuesQuery{}
	if projectUID != nil {
		if entry, ok := model.CatalogEntry(snapshot, *projectUID); ok {
			query.ProjectID = &entry.Project.ID
		}
	}
	response, err := c.ListAllIssuesWithResponse(ctx, &g.ListAllIssuesRequestOptions{Query: query})
	if err != nil {
		if response != nil {
			return nil, ReadError(response.Body, err)
		}
		return nil, err
	}
	if response.JSON200 == nil {
		return nil, fmt.Errorf("kata daemon returned no Issue list")
	}
	counts := make(map[string]g.ChildCounts)
	for _, issue := range response.JSON200.Issues {
		if issue.ChildCounts != nil {
			counts[issue.UID] = *issue.ChildCounts
		}
	}
	return counts, nil
}
