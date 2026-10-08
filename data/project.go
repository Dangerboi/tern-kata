package data

import (
	"context"
	"fmt"

	"github.com/Dangerboi/tern-kata/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

func (c *Client) ReadProject(ctx context.Context, workspace string) (model.ProjectLoaded, error) {
	project, err := c.ResolveProjectWithResponse(ctx, &g.ResolveProjectRequestOptions{
		Body: &g.ResolveProjectBody{StartPath: &workspace},
	})
	if err != nil {
		if project != nil {
			return model.ProjectLoaded{}, ReadError(project.Body, err)
		}
		return model.ProjectLoaded{}, err
	}
	if project.JSON200 == nil {
		return model.ProjectLoaded{}, fmt.Errorf("kata daemon returned no Project")
	}
	resolved := project.JSON200.Project
	view := "project"
	snapshot, err := c.ReadUISnapshotWithResponse(ctx, &g.ReadUISnapshotRequestOptions{
		Query: &g.ReadUISnapshotQuery{View: &view, ProjectUID: &resolved.UID, Status: []string{"open"}},
	})
	if err != nil {
		if snapshot != nil {
			return model.ProjectLoaded{}, ReadError(snapshot.Body, err)
		}
		return model.ProjectLoaded{}, err
	}
	if snapshot.JSON200 == nil {
		return model.ProjectLoaded{}, fmt.Errorf("kata daemon returned no Issue collection")
	}
	etag := ""
	if snapshot.Headers200 != nil {
		etag = snapshot.Headers200.ETag
	}
	return model.ProjectLoaded{Project: resolved, Snapshot: *snapshot.JSON200, ETag: etag}, nil
}
