package data

import (
	"context"
	"errors"
	"net/http"

	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

// Close is a Guarded write: an Issue that is still open closes at the revision it has now, whatever
// else changed since the Close card opened. One that was closed meanwhile is an error and nothing is written.
func (c *Client) Close(ctx context.Context, cmd model.CloseIssueCmd) error {
	body := closeBody(cmd)
	for attempt := 1; ; attempt++ {
		current, err := c.ShowIssueWithResponse(ctx, &g.ShowIssueRequestOptions{PathParams: &g.ShowIssuePath{ProjectID: cmd.ProjectID, Ref: cmd.UID}})
		if err != nil {
			if current != nil {
				err = ReadError(current.Body, err)
			}
			return err
		}
		if current.JSON200 == nil {
			return errors.New("kata returned no Issue before closing")
		}
		if current.JSON200.Issue.Status == "closed" {
			return errors.New("Issue was closed meanwhile; nothing was written")
		}
		tag := etag(current.JSON200.Issue.Revision)
		response, err := c.CloseIssueWithResponse(ctx, &g.CloseIssueRequestOptions{PathParams: &g.CloseIssuePath{ProjectID: cmd.ProjectID, Ref: cmd.UID}, Header: &g.CloseIssueHeaders{IfMatch: &tag}, Body: &body})
		if response != nil && response.StatusCode == http.StatusPreconditionFailed && attempt < guardedAttempts {
			continue
		}
		if response != nil && response.StatusCode >= 400 {
			return ReadError(response.Body, err)
		}
		return err
	}
}

func (c *Client) Reopen(ctx context.Context, cmd model.ReopenIssueCmd) error {
	actor := cmd.Actor
	response, err := c.ReopenIssueWithResponse(ctx, &g.ReopenIssueRequestOptions{PathParams: &g.ReopenIssuePath{ProjectID: cmd.ProjectID, Ref: cmd.UID}, Body: &g.ReopenIssueBody{Actor: &actor}})
	if response != nil && response.StatusCode >= 400 {
		return ReadError(response.Body, err)
	}
	return err
}

// closeBody is kata's close request for a plain close command.
func closeBody(cmd model.CloseIssueCmd) g.CloseIssueBody {
	reason := g.CloseActionRequestBodyReason(cmd.Reason)
	protocol := g.CloseActionRequestBodyRetryProtocol("close-v1")
	body := g.CloseIssueBody{Actor: &cmd.Actor, Reason: &reason, RetryProtocol: &protocol}
	if cmd.Quick {
		source := g.CloseActionRequestBodySource("tui")
		body.Source = &source
		return body
	}
	body.Message = &cmd.Message
	for _, e := range cmd.Evidence {
		value := e.Value
		evidence := g.Evidence{Type: string(e.Kind)}
		switch e.Kind {
		case model.EvidenceCommit:
			evidence.Sha = &value
		case model.EvidencePR:
			evidence.URL = &value
		case model.EvidenceTest:
			evidence.Command = &value
		case model.EvidenceAudit:
			evidence.Rationale = &value
		default:
			evidence.IssueRef = &value
		}
		body.Evidence = append(body.Evidence, evidence)
	}
	return body
}
