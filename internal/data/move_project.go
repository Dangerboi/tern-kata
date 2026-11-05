package data

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
	"go.kenn.io/kata/pkg/client"
	g "go.kenn.io/kata/pkg/client/generated"
)

// MoveToProject asks kata to move the Issue to another Project. A dry run changes nothing and reports the Issue's current
// revision; kata assigns the new short id only when it really moves, so a preview cannot name it. The move is a Guarded write:
// a command with a Revision sends it as If-Match, and kata refuses one that is no longer the Issue's, which the answer reports
// as a conflict rather than an error. A command without one is only a dry run, which kata checks against the current revision.
// kata moves the Issue with its descendants unless the command says Single; a preview lists the descendants that move and a
// move counts the Issues that moved.
func (c *Client) MoveToProject(ctx context.Context, cmd model.MoveToProjectCmd) (model.ProjectMoveAnswer, error) {
	answer := model.ProjectMoveAnswer{UID: cmd.UID, ToProjectUID: cmd.ToProjectUID, DryRun: cmd.DryRun}
	actor, dryRun := cmd.Actor, cmd.DryRun
	body := &g.MoveIssueBody{Actor: &actor, DryRun: &dryRun, ToProjectUID: cmd.ToProjectUID}
	if cmd.Single {
		body.Single = &cmd.Single
	}
	options := &g.MoveIssueRequestOptions{
		PathParams: &g.MoveIssuePath{ProjectID: cmd.ProjectID, Ref: cmd.UID},
		Body:       body,
	}
	if cmd.Revision != 0 {
		guard := etag(cmd.Revision)
		options.Header = &g.MoveIssueHeaders{IfMatch: &guard}
	}
	response, err := c.MoveIssueWithResponse(ctx, options)
	if client.StatusCode(err) == http.StatusPreconditionFailed && daemonCode(err) == "revision_conflict" {
		answer.Conflict = true
		return answer, nil
	}
	if err != nil {
		answer.Subtree, answer.Err = moveRefusal(err)
		return answer, nil
	}
	if response.JSON200 != nil {
		answer.Revision = response.JSON200.Issue.Revision
		answer.Count = max(len(response.JSON200.Issues), 1)
		for _, issue := range response.JSON200.Issues {
			if issue.UID == cmd.UID {
				continue
			}
			moved := model.MovedIssue{ShortID: issue.ShortID}
			if issue.ProjectUID != nil {
				moved.ProjectUID = *issue.ProjectUID
			}
			answer.Descendants = append(answer.Descendants, moved)
		}
		if response.JSON200.NewShortID != nil {
			answer.NewShortID = *response.JSON200.NewShortID
		}
	}
	return answer, nil
}

// moveRefusal words kata's refusal of a move. The two refusals that come from the subtree are reworded and reported as such, so
// the card stays on the chosen Project and offers "Only this Issue" or a new preview; a refusal that names a descendant in its error data is reported as a subtree refusal too, in kata's words; any other refusal keeps kata's words.
func moveRefusal(err error) (subtree bool, refusal error) {
	switch daemonCode(err) {
	case "issue_move_descendant_archived":
		who := "a descendant"
		if message, _, found := strings.Cut(daemonError(err).Error(), ": "); found {
			if ref, ok := strings.CutPrefix(message, "descendant "); ok {
				who = "descendant " + ref
			}
		}
		return true, fmt.Errorf("%s is in an archived Project, so the subtree cannot move. Restore that Project, remove its parent link, or switch on \"Only this Issue\".", who)
	case "issue_move_tree_changed":
		return true, errors.New("The Issue's descendants changed while kata checked the move. Nothing was moved; try again.")
	}
	// Any other refusal that names a descendant (a pinned recurrence, an import-mapping collision) also comes from the subtree.
	if envelope, ok := errors.AsType[g.ErrorEnvelope](err); ok {
		if _, named := envelope.ErrorData.Data["descendant"]; named {
			return true, daemonError(err)
		}
	}
	return false, daemonError(err)
}
