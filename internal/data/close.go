package data

import (
	"context"
	"errors"

	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

// closeRaw sends the close with kata's own error and answer.
func (c *Client) closeRaw(ctx context.Context, cmd model.CloseIssueCmd) (issueWrite, error) {
	if cmd.Key == "" {
		// Without its key kata answers 200 for an Issue already closed, which would read as a close that happened.
		return issueWrite{}, localFailure{errors.New("A close needs an idempotency key.")}
	}
	body := closeBody(cmd)
	r, err := c.CloseIssueWithResponse(ctx, &g.CloseIssueRequestOptions{PathParams: &g.CloseIssuePath{ProjectID: cmd.ProjectID, Ref: cmd.UID}, Header: &g.CloseIssueHeaders{IdempotencyKey: idempotencyKey(cmd.Key)}, Body: &body})
	if err != nil {
		return issueWrite{}, err
	}
	return written(r.JSON200.Changed, r.JSON200.Issue, nil), nil
}

func closeError(err error) error {
	if daemonCode(err) == "issue_already_closed" {
		return errors.New("Issue was closed meanwhile; nothing was written")
	}
	return daemonError(err)
}

// Close is a Guarded write that kata enforces: under its Idempotency-Key kata closes an Issue that is still open, whatever
// else changed since the Close card opened, replays a close it already stored under that key, and refuses one that another
// request closed. The close carries no revision, because kata fingerprints it and a revision would defeat the replay.
func (c *Client) Close(ctx context.Context, cmd model.CloseIssueCmd) error {
	_, err := c.closeRaw(ctx, cmd)
	return closeError(err)
}

// CloseRecorded is Close that also says what the close did to the Undo history.
func (c *Client) CloseRecorded(ctx context.Context, cmd model.CloseIssueCmd) (model.WriteRecord, error) {
	done := c.recorder(ctx, model.UndoClose, cmd.ProjectID, cmd.UID, cmd.Actor)
	res, err := c.closeRaw(ctx, cmd)
	return done(res, ambiguousWrite(err)), closeError(err)
}

func (c *Client) reopenRaw(ctx context.Context, cmd model.ReopenIssueCmd) (issueWrite, error) {
	actor := cmd.Actor
	r, err := c.ReopenIssueWithResponse(ctx, &g.ReopenIssueRequestOptions{PathParams: &g.ReopenIssuePath{ProjectID: cmd.ProjectID, Ref: cmd.UID}, Body: &g.ReopenIssueBody{Actor: &actor}})
	if err != nil {
		return issueWrite{}, err
	}
	return written(r.JSON200.Changed, r.JSON200.Issue, nil), nil
}

func (c *Client) Reopen(ctx context.Context, cmd model.ReopenIssueCmd) error {
	_, err := c.reopenRaw(ctx, cmd)
	return daemonError(err)
}

// ReopenRecorded is Reopen that also says what the reopen did to the Undo history.
func (c *Client) ReopenRecorded(ctx context.Context, cmd model.ReopenIssueCmd) (model.WriteRecord, error) {
	done := c.recorder(ctx, model.UndoReopen, cmd.ProjectID, cmd.UID, cmd.Actor)
	res, err := c.reopenRaw(ctx, cmd)
	return done(res, ambiguousWrite(err)), daemonError(err)
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
