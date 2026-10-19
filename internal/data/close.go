package data

import (
	"context"
	"errors"

	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

// Close is a Guarded write that kata enforces: under its Idempotency-Key kata closes an Issue that is still open, whatever
// else changed since the Close card opened, replays a close it already stored under that key, and refuses one that another
// request closed. The close carries no revision, because kata fingerprints it and a revision would defeat the replay.
func (c *Client) Close(ctx context.Context, cmd model.CloseIssueCmd) error {
	body := closeBody(cmd)
	_, err := c.CloseIssueWithResponse(ctx, &g.CloseIssueRequestOptions{PathParams: &g.CloseIssuePath{ProjectID: cmd.ProjectID, Ref: cmd.UID}, Header: &g.CloseIssueHeaders{IdempotencyKey: idempotencyKey(cmd.Key)}, Body: &body})
	if daemonCode(err) == "issue_already_closed" {
		return errors.New("Issue was closed meanwhile; nothing was written")
	}
	return daemonError(err)
}

func (c *Client) Reopen(ctx context.Context, cmd model.ReopenIssueCmd) error {
	actor := cmd.Actor
	_, err := c.ReopenIssueWithResponse(ctx, &g.ReopenIssueRequestOptions{PathParams: &g.ReopenIssuePath{ProjectID: cmd.ProjectID, Ref: cmd.UID}, Body: &g.ReopenIssueBody{Actor: &actor}})
	return daemonError(err)
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
