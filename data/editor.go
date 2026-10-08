package data

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Dangerboi/tern-kata/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

func (c *Client) SaveDraft(ctx context.Context, cmd model.SaveDraft) model.Msg {
	failed := func(err error) model.Msg { return model.DraftFailed{UID: cmd.UID, Err: err} }
	current, err := c.ShowIssueWithResponse(ctx, &g.ShowIssueRequestOptions{PathParams: &g.ShowIssuePath{ProjectID: cmd.ProjectID, Ref: cmd.UID}})
	if err != nil {
		if current != nil {
			err = ReadError(current.Body, err)
		}
		return failed(err)
	}
	if current.JSON200 == nil {
		return failed(fmt.Errorf("kata returned no Issue before saving Draft"))
	}
	conflict := func() model.Msg {
		detail, err := c.ReadDetail(ctx, cmd.UID)
		if err != nil {
			return failed(err)
		}
		if detail.Selected.Issue == nil {
			return failed(fmt.Errorf("kata returned no current Issue for conflict"))
		}
		return model.DraftConflict{UID: cmd.UID, Issue: *detail.Selected.Issue}
	}
	if current.JSON200.Issue.Revision != cmd.BaseRevision {
		return conflict()
	}
	actor := cmd.Actor
	if cmd.Comment {
		response, err := c.CreateCommentWithResponse(ctx, &g.CreateCommentRequestOptions{PathParams: &g.CreateCommentPath{ProjectID: fmt.Sprint(cmd.ProjectID), Ref: cmd.UID}, Body: &g.CreateCommentBody{Body: cmd.Text, Actor: &actor}})
		if response != nil && response.StatusCode == http.StatusPreconditionFailed {
			return conflict()
		}
		if err != nil {
			if response != nil {
				err = ReadError(response.Body, err)
			}
			return failed(err)
		}
	} else {
		// ponytail: check-then-write races; use atomic If-Match once kata enforces it on edits.
		response, err := c.EditIssueWithResponse(ctx, &g.EditIssueRequestOptions{PathParams: &g.EditIssuePath{ProjectID: fmt.Sprint(cmd.ProjectID), Ref: cmd.UID}, Body: &g.EditIssueBody{Body: &cmd.Text, Actor: &actor}}, func(_ context.Context, req *http.Request) error {
			req.Header.Set("If-Match", fmt.Sprintf("\"rev-%d\"", cmd.BaseRevision))
			return nil
		})
		if response != nil && response.StatusCode == http.StatusPreconditionFailed {
			return conflict()
		}
		if err != nil {
			if response != nil {
				err = ReadError(response.Body, err)
			}
			return failed(err)
		}
	}
	return model.DraftSaved{UID: cmd.UID}
}
