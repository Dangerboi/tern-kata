package data

import (
	"context"
	"errors"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

// deleteConfirmation is the value of kata's X-Kata-Confirm header: "DELETE " and the Issue's project#short id, the form kata's
// `kata delete --confirm` takes. kata refuses a delete without it (confirm_required) or with another (confirm_mismatch).
func deleteConfirmation(qualifiedID string) string { return "DELETE " + qualifiedID }

// deleteRaw soft-deletes the Issue with kata's own error and answer.
func (c *Client) deleteRaw(ctx context.Context, cmd model.DeleteIssueCmd) (issueWrite, error) {
	confirm := deleteConfirmation(cmd.QualifiedID)
	body := g.DeleteIssueBody{Actor: cmd.Actor}
	if reason := strings.TrimSpace(cmd.Reason); reason != "" {
		body.Reason = &reason
	}
	r, err := c.DeleteIssueWithResponse(ctx, &g.DeleteIssueRequestOptions{
		PathParams: &g.DeleteIssuePath{ProjectID: cmd.ProjectID, Ref: cmd.UID},
		Header:     &g.DeleteIssueHeaders{XKataConfirm: &confirm},
		Body:       &body,
	})
	if err != nil {
		return issueWrite{}, err
	}
	return written(r.JSON200.Changed, r.JSON200.Issue, nil), nil
}

// DeleteRecorded soft-deletes the Issue and also says what the delete did to the Undo history.
func (c *Client) DeleteRecorded(ctx context.Context, cmd model.DeleteIssueCmd) (model.WriteRecord, error) {
	done := c.recorder(ctx, model.UndoDelete, cmd.ProjectID, cmd.UID, cmd.Actor)
	res, err := c.deleteRaw(ctx, cmd)
	return done(res, ambiguousWrite(err)), daemonError(err)
}

// restoreRaw restores the Issue, which kata finds among the deleted ones, with kata's own error and answer.
func (c *Client) restoreRaw(ctx context.Context, projectID int64, uid, actor string) (issueWrite, error) {
	r, err := c.RestoreIssueWithResponse(ctx, &g.RestoreIssueRequestOptions{
		PathParams: &g.RestoreIssuePath{ProjectID: projectID, Ref: uid},
		Body:       &g.RestoreIssueBody{Actor: actor},
	})
	if err != nil {
		return issueWrite{}, err
	}
	return written(r.JSON200.Changed, r.JSON200.Issue, nil), nil
}

// RestoreIssue restores the Issue and reports whether kata changed it: kata answers an Issue that is not deleted with no change.
func (c *Client) RestoreIssue(ctx context.Context, cmd model.RestoreIssueCmd) (model.IssueRestored, error) {
	res, err := c.restoreRaw(ctx, cmd.ProjectID, cmd.UID, cmd.Actor)
	if err != nil {
		return model.IssueRestored{}, daemonError(err)
	}
	return model.IssueRestored{UID: cmd.UID, Changed: res.Changed}, nil
}

// showIssueIncludingDeleted is showIssue for an Issue that may be deleted.
func (c *Client) showIssueIncludingDeleted(ctx context.Context, projectID int64, uid string) (*g.ShowIssueResponseBody, error) {
	include := true
	r, err := c.ShowIssueWithResponse(ctx, &g.ShowIssueRequestOptions{
		PathParams: &g.ShowIssuePath{ProjectID: projectID, Ref: uid},
		Query:      &g.ShowIssueQuery{IncludeDeleted: &include},
	})
	if err != nil {
		return nil, err
	}
	if r.JSON200 == nil {
		return nil, errors.New("kata returned no Issue")
	}
	return r.JSON200, nil
}

// showForEntry reads the Issue an Undo entry concerns: a deleted Issue is read with deleted Issues included, since it is the
// state the entry's change left.
func (c *Client) showForEntry(ctx context.Context, e model.UndoEntry) (*g.ShowIssueResponseBody, error) {
	if e.Kind == model.UndoDelete {
		return c.showIssueIncludingDeleted(ctx, e.ProjectID, e.UID)
	}
	return c.showIssue(ctx, e.ProjectID, e.UID)
}

// LookupDeleted reads the Issue the reference names with deleted Issues included. A ULID is read without a Project.
func (c *Client) LookupDeleted(ctx context.Context, cmd model.LookupDeleted) (model.DeletedLooked, error) {
	var shown *g.ShowIssueResponseBody
	if cmd.ByUID {
		include := true
		r, err := c.ShowIssueByUIDWithResponse(ctx, &g.ShowIssueByUIDRequestOptions{
			PathParams: &g.ShowIssueByUIDPath{UID: cmd.Ref},
			Query:      &g.ShowIssueByUIDQuery{IncludeDeleted: &include},
		})
		if err != nil {
			return model.DeletedLooked{}, daemonError(err)
		}
		if r.JSON200 == nil {
			return model.DeletedLooked{}, errors.New("kata returned no Issue")
		}
		shown = r.JSON200
	} else {
		var err error
		if shown, err = c.showIssueIncludingDeleted(ctx, cmd.ProjectID, cmd.Ref); err != nil {
			return model.DeletedLooked{}, daemonError(err)
		}
	}
	i := shown.Issue
	return model.DeletedLooked{Ref: cmd.Typed, Issue: model.DeletedIssue{UID: i.UID, ProjectID: i.ProjectID, ShortID: i.ShortID, Title: i.Title, Deleted: i.DeletedAt != nil}}, nil
}
