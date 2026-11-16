package data

import (
	"context"
	"errors"
	"net/http"

	"go.kenn.io/kata/pkg/client"
	g "go.kenn.io/kata/pkg/client/generated"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// setDates writes the Dates card's changed keys in one Issue metadata patch, as kata tui does, guarded by If-Match on the revision
// of the Issue it read. The date keys read are compared with what the card opened with first (absent when it held none): a key
// changed since the card opened writes nothing, and the refusal names it. A refused revision reads the Issue again: when the date
// keys still hold what the card opened with, the patch goes again at the new revision, up to guardedAttempts times. The patch
// is all or nothing, so applied is every change or none. It returns the changes that changed the Issue and the Issue after
// them. ambiguous says the failure leaves the outcome of the write unknown.
func (c *Client) setDates(ctx context.Context, cmd model.SetDatesCmd) (applied, changed []model.DateChange, last issueWrite, ambiguous bool, err error) {
	if len(cmd.Changes) == 0 {
		return nil, nil, last, false, nil
	}
	patch := make(map[string]any, len(cmd.Changes))
	for _, change := range cmd.Changes {
		var value any
		if change.Value != "" {
			value = change.Value
		}
		patch[change.Key] = value
	}
	for range guardedAttempts {
		shown, e := c.showIssue(ctx, cmd.ProjectID, cmd.UID)
		if e != nil {
			return nil, nil, last, false, daemonError(e)
		}
		for _, change := range cmd.Changes {
			if held, present := shown.Issue.Metadata[change.Key]; present != (change.Was != "") || (present && held != change.Was) {
				return nil, nil, last, false, datesConflict(shown.Issue, change)
			}
		}
		tag := etag(shown.Issue.Revision)
		options, _ := metadataPatch(cmd.ProjectID, cmd.UID, cmd.Actor, patch)
		options.Header = &g.PatchIssueMetadataHeaders{IfMatch: &tag}
		r, e := c.PatchIssueMetadataWithResponse(ctx, options)
		switch {
		case client.StatusCode(e) == http.StatusPreconditionFailed:
			continue
		case e != nil:
			return nil, nil, last, ambiguousWrite(e), daemonError(e)
		}
		last = written(r.JSON200.Changed, r.JSON200.Issue, nil)
		if last.Changed {
			changed = cmd.Changes
		}
		return cmd.Changes, changed, last, false, nil
	}
	return nil, nil, last, false, errors.New("The Issue kept changing while saving; nothing was written. Try again.")
}

// SetDatesRecorded writes the Dates card's changes and says what the save did to the Undo history. It returns the changes
// written: all of them, or none when the save failed.
func (c *Client) SetDatesRecorded(ctx context.Context, cmd model.SetDatesCmd) ([]model.DateChange, model.WriteRecord, error) {
	done := c.recorder(ctx, model.UndoDates, cmd.ProjectID, cmd.UID, cmd.Actor)
	applied, changed, last, ambiguous, err := c.setDates(ctx, cmd)
	last.Changed = len(changed) > 0
	return applied, done(last, ambiguous, func(e *model.UndoEntry) { e.Dates = changed }), err
}

func datesConflict(issue g.ShowIssueOut, change model.DateChange) model.DatesConflict {
	conflict := model.DatesConflict{Label: change.Label, Key: change.Key}
	conflict.Held, _ = issue.Metadata[change.Key].(string)
	conflict.Shown = conflict.Held
	if conflict.Shown == "" {
		projected := issue.ScheduledOnDate
		if change.Key == model.DeadlineKey {
			projected = issue.DeadlineOnDate
		}
		if projected != nil {
			conflict.Shown = *projected
		}
	}
	return conflict
}
