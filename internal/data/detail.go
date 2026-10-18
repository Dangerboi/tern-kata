package data

import (
	"context"
	"fmt"

	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

func (c *Client) ReadDetail(ctx context.Context, uid string) (model.DetailLoaded, error) {
	view, history := "all-open", true
	response, err := c.ReadUISnapshotWithResponse(ctx, &g.ReadUISnapshotRequestOptions{
		Query: &g.ReadUISnapshotQuery{View: &view, SelectedIssueUID: &uid, IncludeHistory: &history},
	})
	if err != nil {
		return model.DetailLoaded{}, daemonError(err)
	}
	if response.JSON200 == nil || response.JSON200.Selected == nil {
		return model.DetailLoaded{}, fmt.Errorf("kata daemon returned no detail for Issue %s", uid)
	}
	return model.DetailLoaded{UID: uid, Selected: response.JSON200.Selected}, nil
}
