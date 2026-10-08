package data

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Dangerboi/tern-kata/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

func (c *Client) ReadProject(ctx context.Context, workspace string) (model.ProjectLoaded, error) {
	response, err := c.ResolveProjectWithResponse(ctx, &g.ResolveProjectRequestOptions{Body: &g.ResolveProjectBody{StartPath: &workspace}})
	var project model.Project
	if err != nil {
		if response == nil {
			return model.ProjectLoaded{}, err
		}
		var envelope g.ErrorEnvelope
		if json.Unmarshal(response.Body, &envelope) != nil || envelope.ErrorData.Code != "project_not_initialized" {
			return model.ProjectLoaded{}, ReadError(response.Body, err)
		}
	} else {
		if response.JSON200 == nil {
			return model.ProjectLoaded{}, fmt.Errorf("kata daemon returned no Project")
		}
		project = response.JSON200.Project
	}
	collection := model.Collection{View: "inbox", Title: "Inbox"}
	if project.UID != "" {
		collection = model.Collection{View: "project", ProjectUID: project.UID, Title: project.Name}
	}
	snapshot, err := c.ReadCollection(ctx, model.LoadCollection{Collection: collection})
	if err != nil {
		return model.ProjectLoaded{}, err
	}
	return model.ProjectLoaded{Project: project, Snapshot: snapshot.Snapshot, ETag: snapshot.ETag, LocalDate: snapshot.LocalDate, TimeZone: snapshot.TimeZone}, nil
}
