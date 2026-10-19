package data

import (
	"context"
	"fmt"

	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

// readChildCounts is kata's direct-child count of each Issue with children,
// whatever the filters left in the snapshot: one list request whose rows carry
// child_counts, scoped to the Project when the Collection is one (the snapshot has none of its own). The model asks for it
// only when an event could have changed a count.
func (c *Client) readChildCounts(ctx context.Context, snapshot model.Snapshot, projectUID *string) (map[string]model.ChildCounts, error) {
	query := &g.ListAllIssuesQuery{}
	if projectUID != nil {
		if entry, ok := model.CatalogEntry(snapshot, *projectUID); ok {
			query.ProjectID = &entry.Project.ID
		}
	}
	response, err := c.ListAllIssuesWithResponse(ctx, &g.ListAllIssuesRequestOptions{Query: query})
	if err != nil {
		return nil, daemonError(err)
	}
	if response.JSON200 == nil {
		return nil, fmt.Errorf("kata daemon returned no Issue list")
	}
	counts := make(map[string]model.ChildCounts)
	for _, issue := range response.JSON200.Issues {
		if issue.ChildCounts != nil {
			counts[issue.UID] = model.ChildCounts{Open: int(issue.ChildCounts.Open), Total: int(issue.ChildCounts.Total)}
		}
	}
	return counts, nil
}
