package main

import (
	"context"

	"github.com/Dangerboi/tern-kata/data"
	"github.com/Dangerboi/tern-kata/model"
)

// saveProject runs a Project write against the daemon that serves workspace.
func saveProject(ctx context.Context, workspace string, save func(*data.Client) (model.Project, error)) model.ProjectSaved {
	client, err := data.Discover(ctx, workspace)
	if err != nil {
		return model.ProjectSaved{Err: err}
	}
	defer client.CloseIdleConnections()
	project, err := save(client)
	return model.ProjectSaved{Project: project, Err: err}
}

func execute(ctx context.Context, cmd model.Cmd, messages chan<- any) {
	var msg model.Msg
	switch cmd := cmd.(type) {
	case model.CreateIssueCmd:
		client, err := data.Discover(ctx, cmd.Workspace)
		if err != nil {
			msg = model.IssueCreated{Err: err}
		} else {
			defer client.CloseIdleConnections()
			msg = client.CreateIssue(ctx, cmd)
		}
	case model.OpenIssue:
		client, err := data.Discover(ctx, cmd.Workspace)
		if err != nil {
			msg = model.IssueOpenFailed{Ref: cmd.Ref, Err: err}
		} else {
			defer client.CloseIdleConnections()
			result, err := client.OpenIssue(ctx, cmd)
			if err != nil {
				msg = model.IssueOpenFailed{Ref: cmd.Ref, Err: err}
			} else {
				msg = result
			}
		}
	case model.LookupSearchID:
		// shortcut: any failure counts as no Issue until the query changes (retrying would loop while the daemon is down); the full-text query that follows reports it.
		result := model.SearchIDLooked{Request: cmd.Request, ShortID: cmd.ShortID}
		if client, err := data.Discover(ctx, cmd.Workspace); err == nil {
			defer client.CloseIdleConnections()
			result.UID, _ = client.ResolveIssue(ctx, cmd.ProjectID, cmd.ShortID)
		}
		msg = result
	case model.SaveDraft:
		client, err := data.Discover(ctx, cmd.Workspace)
		if err != nil {
			msg = model.DraftFailed{UID: cmd.UID, Err: err}
		} else {
			defer client.CloseIdleConnections()
			msg = client.SaveDraft(ctx, cmd)
		}
	case model.CloseIssueCmd:
		client, err := data.Discover(ctx, cmd.Workspace)
		if err == nil {
			defer client.CloseIdleConnections()
			err = client.Close(ctx, cmd)
		}
		msg = model.CloseResultMsg{UID: cmd.UID, Err: err}
	case model.ReopenIssueCmd:
		client, err := data.Discover(ctx, cmd.Workspace)
		if err == nil {
			defer client.CloseIdleConnections()
			err = client.Reopen(ctx, cmd)
		}
		msg = model.CloseResultMsg{UID: cmd.UID, Err: err}
	case model.WriteField:
		client, err := data.Discover(ctx, cmd.Workspace)
		if err == nil {
			defer client.CloseIdleConnections()
			err = client.WriteField(ctx, cmd)
		}
		msg = model.FieldWritten{UID: cmd.UID, Err: err}
	case model.LoadFieldLabels:
		client, err := data.Discover(ctx, cmd.Workspace)
		result := model.FieldLabelsLoaded{UID: cmd.UID}
		if err == nil {
			defer client.CloseIdleConnections()
			result, err = client.FieldLabels(ctx, cmd)
		}
		result.UID, result.Err = cmd.UID, err
		msg = result
	case model.CreateProject:
		msg = saveProject(ctx, cmd.Workspace, func(c *data.Client) (model.Project, error) { return c.CreateProject(ctx, cmd) })
	case model.BindProject:
		msg = saveProject(ctx, cmd.Workspace, func(c *data.Client) (model.Project, error) { return c.BindProject(ctx, cmd) })
	case model.LoadProject:
		client, err := data.Discover(ctx, cmd.Workspace)
		if err != nil {
			msg = model.Failed{Err: err}
		} else {
			defer client.CloseIdleConnections()
			result, err := client.ReadProject(ctx, cmd.Workspace)
			if err != nil {
				msg = model.Failed{Err: err}
			} else {
				msg = result
			}
		}
	case model.LoadDetail:
		client, err := data.Discover(ctx, cmd.Workspace)
		if err != nil {
			msg = model.DetailFailed{UID: cmd.UID, Err: err}
		} else {
			defer client.CloseIdleConnections()
			result, err := client.ReadDetail(ctx, cmd.UID)
			if err != nil {
				msg = model.DetailFailed{UID: cmd.UID, Err: err}
			} else {
				msg = result
			}
		}
	case model.LoadCollection:
		client, err := data.Discover(ctx, cmd.Workspace)
		if err != nil {
			msg = model.Failed{Err: err}
		} else {
			defer client.CloseIdleConnections()
			result, err := client.ReadCollection(ctx, cmd)
			if err != nil {
				msg = model.Failed{Err: err}
			} else {
				msg = result
			}
		}
	case model.RefreshSnapshot:
		client, err := data.Discover(ctx, cmd.Workspace)
		if err != nil {
			msg = model.SnapshotFailed{Request: cmd.Request, Err: err}
		} else {
			defer client.CloseIdleConnections()
			result, err := client.ReadRefresh(ctx, cmd)
			if err != nil {
				msg = model.SnapshotFailed{Request: cmd.Request, Err: err}
			} else {
				msg = result
			}
		}
	}
	if msg != nil {
		select {
		case messages <- msg:
		case <-ctx.Done():
		}
	}
}
