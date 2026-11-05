package data

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	g "go.kenn.io/kata/pkg/client/generated"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// ulid is the shape of a kata Issue UID; any other ref is a short id, optionally behind "<project>#".
var ulid = regexp.MustCompile(`(?i)^[0-9A-HJKMNP-TV-Z]{26}$`)

// issueTarget is an Issue named by a ref, read once, with the Project the workspace is bound to.
type issueTarget struct {
	issue   g.ShowIssueOut
	binding model.Project
}

// readIssueTarget reads the Issue ref names, as kata show takes it, and the Project the workspace is bound to.
func (c *Client) readIssueTarget(ctx context.Context, workspace, ref string) (issueTarget, error) {
	loaded, err := c.ReadProject(ctx, workspace)
	if err != nil {
		return issueTarget{}, err
	}
	target := issueTarget{binding: loaded.Project}
	ref = strings.TrimSpace(ref)
	if ulid.MatchString(ref) {
		response, err := c.ShowIssueByUIDWithResponse(ctx, &g.ShowIssueByUIDRequestOptions{PathParams: &g.ShowIssueByUIDPath{UID: ref}})
		if err != nil {
			return issueTarget{}, daemonError(err)
		}
		if response.JSON200 == nil {
			return issueTarget{}, errors.New("kata daemon returned no Issue")
		}
		target.issue = response.JSON200.Issue
		return target, nil
	}
	projectName, shortID, qualified := strings.Cut(ref, "#")
	if !qualified {
		projectName, shortID = "", ref
	}
	projectID := target.binding.ID
	if projectName != "" {
		projects, err := c.projectNames(ctx)
		if err != nil {
			return issueTarget{}, err
		}
		projectID = 0
		for id, name := range projects {
			if name == projectName {
				projectID = id
			}
		}
		if projectID == 0 {
			return issueTarget{}, fmt.Errorf("kata Project %q not found", projectName)
		}
	} else if target.binding.UID == "" {
		return issueTarget{}, fmt.Errorf("This workspace is not bound to a kata Project, so %q does not name an Issue; give <project>#<short id> or the Issue's ULID.", ref)
	}
	response, err := c.ShowIssueWithResponse(ctx, &g.ShowIssueRequestOptions{PathParams: &g.ShowIssuePath{ProjectID: projectID, Ref: shortID}})
	if err != nil {
		return issueTarget{}, daemonError(err)
	}
	if response.JSON200 == nil {
		return issueTarget{}, errors.New("kata daemon returned no Issue")
	}
	target.issue = response.JSON200.Issue
	return target, nil
}

// projectNames is the name of every Project by id.
func (c *Client) projectNames(ctx context.Context) (map[int64]string, error) {
	response, err := c.ListProjectsWithResponse(ctx, &g.ListProjectsRequestOptions{})
	if err != nil {
		return nil, daemonError(err)
	}
	if response.JSON200 == nil {
		return nil, errors.New("kata daemon returned no Projects")
	}
	names := map[int64]string{}
	for _, p := range response.JSON200.Projects {
		names[p.ID] = p.Name
	}
	return names, nil
}
