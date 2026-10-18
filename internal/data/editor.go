package data

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

// guardedAttempts bounds how often a guarded write is retried when the revision moves between reading the Issue and writing it.
const guardedAttempts = 3

// SaveDraft is a Guarded write: it conflicts only when the Issue's text differs from the text the
// Draft started against, and then reports a DraftConflict, not an error. A revision that moved for
// any other reason (owner, status, claim, expiry) is written against as it now stands.
func (c *Client) SaveDraft(ctx context.Context, cmd model.SaveDraft) (model.Msg, error) {
	actor := cmd.Actor
	for attempt := 1; ; attempt++ {
		current, err := c.ShowIssueWithResponse(ctx, &g.ShowIssueRequestOptions{PathParams: &g.ShowIssuePath{ProjectID: cmd.ProjectID, Ref: cmd.UID}})
		if err != nil {
			if current != nil {
				err = ReadError(current.Body, err)
			}
			return nil, err
		}
		if current.JSON200 == nil {
			return nil, fmt.Errorf("kata returned no Issue before saving Draft")
		}
		issue := current.JSON200.Issue
		if issue.Body != cmd.BaseText {
			return c.draftConflict(ctx, cmd.UID)
		}
		var status int
		var body []byte
		var werr error
		if cmd.Comment {
			response, err := c.CreateCommentWithResponse(ctx, &g.CreateCommentRequestOptions{PathParams: &g.CreateCommentPath{ProjectID: fmt.Sprint(cmd.ProjectID), Ref: cmd.UID}, Body: &g.CreateCommentBody{Body: cmd.Text, Actor: &actor}})
			werr = err
			if response != nil {
				status, body = response.StatusCode, response.Body
			}
		} else {
			response, err := c.EditIssueWithResponse(ctx, &g.EditIssueRequestOptions{PathParams: &g.EditIssuePath{ProjectID: fmt.Sprint(cmd.ProjectID), Ref: cmd.UID}, Body: &g.EditIssueBody{Body: &cmd.Text, Actor: &actor}}, ifMatch(issue.Revision))
			werr = err
			if response != nil {
				status, body = response.StatusCode, response.Body
			}
		}
		switch {
		case status == http.StatusPreconditionFailed && attempt < guardedAttempts:
			continue
		case status == http.StatusPreconditionFailed:
			return c.draftConflict(ctx, cmd.UID)
		case werr != nil:
			return nil, ReadError(body, werr)
		}
		return model.DraftSaved{UID: cmd.UID}, nil
	}
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

func etag(revision int64) string { return fmt.Sprintf("\"rev-%d\"", revision) }

func ifMatch(revision int64) func(context.Context, *http.Request) error {
	return func(_ context.Context, req *http.Request) error {
		req.Header.Set("If-Match", etag(revision))
		return nil
	}
}
