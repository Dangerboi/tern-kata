package data

import (
	"context"
	"fmt"

	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

// SaveDraft is a Guarded write: it conflicts only when the Issue's text differs from the text the
// Draft started against, and then reports a DraftConflict, not an error. A revision that moved for
// any other reason (owner, status, claim, expiry) does not matter.
func (c *Client) SaveDraft(ctx context.Context, cmd model.SaveDraft) (model.Msg, error) {
	current, err := c.ShowIssueWithResponse(ctx, &g.ShowIssueRequestOptions{PathParams: &g.ShowIssuePath{ProjectID: cmd.ProjectID, Ref: cmd.UID}})
	if err != nil {
		return nil, daemonError(err)
	}
	if current.JSON200 == nil {
		return nil, fmt.Errorf("kata returned no Issue before saving Draft")
	}
	if current.JSON200.Issue.Body != cmd.BaseText {
		return c.draftConflict(ctx, cmd.UID)
	}
	actor := cmd.Actor
	// shortcut: check-then-write races (kata v0.19 honours no revision precondition on edits or comments); upgrade when it does.
	var werr error
	if cmd.Comment {
		_, werr = c.CreateCommentWithResponse(ctx, &g.CreateCommentRequestOptions{PathParams: &g.CreateCommentPath{ProjectID: fmt.Sprint(cmd.ProjectID), Ref: cmd.UID}, Body: &g.CreateCommentBody{Body: cmd.Text, Actor: &actor}})
	} else {
		_, werr = c.EditIssueWithResponse(ctx, &g.EditIssueRequestOptions{PathParams: &g.EditIssuePath{ProjectID: fmt.Sprint(cmd.ProjectID), Ref: cmd.UID}, Body: &g.EditIssueBody{Body: &cmd.Text, Actor: &actor}})
	}
	if werr != nil {
		return nil, daemonError(werr)
	}
	return model.DraftSaved{UID: cmd.UID}, nil
}

func (c *Client) draftConflict(ctx context.Context, uid string) (model.Msg, error) {
	detail, err := c.ReadDetail(ctx, uid)
	if err != nil {
		return nil, err
	}
	if detail.Selected.Issue == nil {
		return nil, fmt.Errorf("kata returned no current Issue for conflict")
	}
	return model.DraftConflict{UID: uid, Issue: *detail.Selected.Issue}, nil
}
