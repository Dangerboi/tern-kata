package data

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"go.kenn.io/kata/pkg/client"
	g "go.kenn.io/kata/pkg/client/generated"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// errNoSuchIssue marks a ref that names no Issue: kata's 404 or 400 on the read, an unknown Project, or a bare ref in an unbound
// workspace. Its message is the underlying one.
type errNoSuchIssue struct{ error }

func (e errNoSuchIssue) Unwrap() error { return e.error }

// issueReadError is daemonError, marked errNoSuchIssue when kata says there is no such Issue.
func issueReadError(err error) error {
	switch client.StatusCode(err) {
	case http.StatusNotFound, http.StatusBadRequest:
		return errNoSuchIssue{daemonError(err)}
	}
	return daemonError(err)
}

// ulid is the shape of a kata Issue UID; any other ref is a short id, optionally behind "<project>#".
var ulid = regexp.MustCompile(`(?i)^[0-9A-HJKMNP-TV-Z]{26}$`)

type issueTarget struct {
	issue   g.ShowIssueOut
	binding model.Project
}

// readIssueTarget reads the Issue ref names, as kata show takes it, and the Project the workspace is bound to.
func (c *Client) readIssueTarget(ctx context.Context, workspace, ref string) (issueTarget, error) {
	given := ref
	ref = strings.TrimSpace(ref)
	projectName, shortID, qualified := strings.Cut(ref, "#")
	if ref == "" || qualified && shortID == "" {
		return issueTarget{}, errNoSuchIssue{fmt.Errorf("%q is not a valid issue ref", given)}
	}
	loaded, err := c.ReadProject(ctx, workspace)
	if err != nil {
		return issueTarget{}, err
	}
	target := issueTarget{binding: loaded.Project}
	if ulid.MatchString(ref) {
		response, err := c.ShowIssueByUIDWithResponse(ctx, &g.ShowIssueByUIDRequestOptions{PathParams: &g.ShowIssueByUIDPath{UID: ref}})
		if err != nil {
			return issueTarget{}, issueReadError(err)
		}
		target.issue = response.JSON200.Issue
		return target, nil
	}
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
			return issueTarget{}, errNoSuchIssue{fmt.Errorf("kata Project %q not found", projectName)}
		}
	} else if target.binding.UID == "" {
		return issueTarget{}, errNoSuchIssue{fmt.Errorf("This workspace is not bound to a kata Project, so %q does not name an Issue; give <project>#<short id> or the Issue's ULID.", ref)}
	}
	response, err := c.ShowIssueWithResponse(ctx, &g.ShowIssueRequestOptions{PathParams: &g.ShowIssuePath{ProjectID: projectID, Ref: shortID}})
	if err != nil {
		return issueTarget{}, issueReadError(err)
	}
	target.issue = response.JSON200.Issue
	return target, nil
}

func (c *Client) projectNames(ctx context.Context) (map[int64]string, error) {
	response, err := c.ListProjectsWithResponse(ctx, &g.ListProjectsRequestOptions{})
	if err != nil {
		return nil, daemonError(err)
	}
	names := map[int64]string{}
	for _, p := range response.JSON200.Projects {
		names[p.ID] = p.Name
	}
	return names, nil
}
