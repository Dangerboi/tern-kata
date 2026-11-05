package data

import (
	"context"
	"encoding/json"

	g "go.kenn.io/kata/pkg/client/generated"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// setDates patches each changed date key in order, through the Issue metadata patch, guarded on the value the Dates card opened
// with (absent when it held none). The values kata holds are read first: a key changed since the card opened writes nothing, for
// any key, and the refusal names it. A key that changes after that read still fails its own guard and stops the run there. It
// returns the changes written, also when a later one failed, those that changed the Issue, and the Issue after the last of them.
// ambiguous says the failure leaves the outcome of the write unknown.
func (c *Client) setDates(ctx context.Context, cmd model.SetDatesCmd) (applied, changed []model.DateChange, last issueWrite, ambiguous bool, err error) {
	if shown, e := c.showIssue(ctx, cmd.ProjectID, cmd.UID); e == nil {
		for _, change := range cmd.Changes {
			if held, present := shown.Issue.Metadata[change.Key]; present != (change.Was != "") || (present && held != change.Was) {
				return nil, nil, last, false, datesConflict(shown.Issue, change)
			}
		}
	}
	for _, change := range cmd.Changes {
		var value any
		if change.Value != "" {
			value = change.Value
		}
		var ifValue *string
		if change.Was != "" {
			text, e := json.Marshal(change.Was)
			if e != nil {
				return applied, changed, last, false, e
			}
			guard := string(text)
			ifValue = &guard
		}
		res, e := c.patchMetadataKey(ctx, cmd.ProjectID, cmd.UID, cmd.Actor, change.Key, value, ifValue)
		if e != nil {
			if daemonCode(e) == "metadata_guard_failed" {
				conflict := c.datesConflictRead(ctx, cmd, change)
				for _, done := range applied {
					conflict.Saved = append(conflict.Saved, done.Label)
				}
				return applied, changed, last, false, conflict
			}
			return applied, changed, last, ambiguousWrite(e), daemonError(e)
		}
		applied = append(applied, change)
		if res.Changed {
			changed, last = append(changed, change), res
		}
	}
	return applied, changed, last, false, nil
}

// SetDates writes the Dates card's changes. It returns the changes written, also when a later one failed.
func (c *Client) SetDates(ctx context.Context, cmd model.SetDatesCmd) ([]model.DateChange, error) {
	applied, _, _, _, err := c.setDates(ctx, cmd)
	return applied, err
}

// SetDatesRecorded is SetDates that also says what the save did to the Undo history.
func (c *Client) SetDatesRecorded(ctx context.Context, cmd model.SetDatesCmd) ([]model.DateChange, model.WriteRecord, error) {
	done := c.recorder(ctx, model.UndoDates, cmd.ProjectID, cmd.UID, cmd.Actor)
	applied, changed, last, ambiguous, err := c.setDates(ctx, cmd)
	last.Changed = len(changed) > 0
	return applied, done(last, ambiguous, func(e *model.UndoEntry) { e.Dates = changed }), err
}

// datesConflictRead is datesConflict for a guard kata refused: the value kata holds now is read for it. A failed read leaves the
// value out, and the refusal says so.
func (c *Client) datesConflictRead(ctx context.Context, cmd model.SetDatesCmd, change model.DateChange) model.DatesConflict {
	shown, err := c.showIssue(ctx, cmd.ProjectID, cmd.UID)
	if err != nil {
		return model.DatesConflict{Label: change.Label, Key: change.Key}
	}
	return datesConflict(shown.Issue, change)
}

// datesConflict is the refusal for a date key that moved since the Dates card opened, carrying the value the Issue holds now so
// the card can show it.
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
	conflict.Read = true
	return conflict
}
