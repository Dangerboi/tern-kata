package data

import (
	"context"
	"errors"

	g "go.kenn.io/kata/pkg/client/generated"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// remoteAgentsPage is how many Issues one read of a Project's open Issues asks for.
const remoteAgentsPage = 200

// eachOpenIssuePage calls fn with each page of the open Issues of a Project, in order, until the daemon names no next page or fn fails.
func (c *Client) eachOpenIssuePage(ctx context.Context, projectID int64, fn func(page *g.ListIssuesResp) error) error {
	open, limit := g.ListIssuesQueryStatusOpen, int64(remoteAgentsPage)
	var cursor *string
	for {
		page, err := c.ListIssuesWithResponse(ctx, &g.ListIssuesRequestOptions{
			PathParams: &g.ListIssuesPath{ProjectID: projectID}, Query: &g.ListIssuesQuery{Status: &open, Limit: &limit, Cursor: cursor}})
		if err != nil {
			return daemonError(err)
		}
		if page.JSON200 == nil {
			return errors.New("kata daemon returned no Issues")
		}
		if err := fn(page); err != nil {
			return err
		}
		if cursor = page.JSON200.NextCursor; cursor == nil || *cursor == "" {
			return nil
		}
	}
}

// ReadRemoteAgents reads the agents other hosts published, as `tern-kata agents` and `tern-kata message` list them: the Agent
// records, left out when they carry hostID (this host's own agents come from agents.json). cmd.Issue names one Issue; without it
// the open Issues of the Project of cmd.ScopeRef, else of the workspace's bound Project, are read page by page.
func (c *Client) ReadRemoteAgents(ctx context.Context, hostID string, cmd model.LoadRemoteAgents) (model.RemoteAgentsLoaded, error) {
	if cmd.Issue != "" {
		target, err := c.readIssueTarget(ctx, cmd.Workspace, cmd.Issue)
		if _, missing := errors.AsType[errNoSuchIssue](err); missing {
			return model.RemoteAgentsLoaded{NotFound: true}, nil
		}
		if err != nil {
			return model.RemoteAgentsLoaded{}, err
		}
		i := target.issue
		if len(model.AgentRecords(i.Metadata)) == 0 {
			return model.RemoteAgentsLoaded{}, nil
		}
		names, err := c.projectNames(ctx)
		if err != nil {
			return model.RemoteAgentsLoaded{}, err
		}
		return model.RemoteAgentsLoaded{Agents: remoteAgentsOn(names[i.ProjectID]+"#"+i.ShortID, i.UID, i.ProjectID, i.Metadata, hostID)}, nil
	}
	projectID, err := c.remoteScope(ctx, cmd)
	if err != nil {
		return model.RemoteAgentsLoaded{}, err
	}
	var agents []model.IssueAgent
	err = c.eachOpenIssuePage(ctx, projectID, func(page *g.ListIssuesResp) error {
		for _, i := range page.JSON200.Issues {
			agents = append(agents, remoteAgentsOn(i.QualifiedID, i.UID, i.ProjectID, i.Metadata, hostID)...)
		}
		return nil
	})
	if err != nil {
		return model.RemoteAgentsLoaded{}, err
	}
	return model.RemoteAgentsLoaded{Agents: agents}, nil
}

// remoteScope is the id of the Project a scan reads: the one cmd.ScopeRef names, else the workspace's bound one.
func (c *Client) remoteScope(ctx context.Context, cmd model.LoadRemoteAgents) (int64, error) {
	if cmd.ScopeRef != "" {
		target, err := c.readIssueTarget(ctx, cmd.Workspace, cmd.ScopeRef)
		if err != nil {
			return 0, err
		}
		return target.issue.ProjectID, nil
	}
	loaded, err := c.ReadProject(ctx, cmd.Workspace)
	if err != nil {
		return 0, err
	}
	if loaded.Project.UID == "" {
		return 0, model.ErrNoRemoteScope
	}
	return loaded.Project.ID, nil
}

// remoteAgentsOn lists the agents the records in an Issue's metadata publish, except those of hostID.
func remoteAgentsOn(ref, uid string, projectID int64, metadata map[string]any, hostID string) []model.IssueAgent {
	var agents []model.IssueAgent
	for _, r := range model.AgentRecords(metadata) {
		if r.HostID != hostID {
			agents = append(agents, model.RemoteIssueAgent(r, ref, uid, projectID))
		}
	}
	return agents
}
