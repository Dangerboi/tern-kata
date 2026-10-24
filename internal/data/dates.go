package data

import (
	"context"
	"encoding/json"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// setDates patches each changed date key in order, through the Issue metadata patch, guarded on the value the Dates card opened
// with (absent when it held none). A key changed meanwhile is not overwritten: it stops the run with the refusal naming it. It
// returns the changes written, also when a later one failed, those that changed the Issue, and the Issue after the last of them.
// ambiguous says the failure leaves the outcome of the write unknown.
func (c *Client) setDates(ctx context.Context, cmd model.SetDatesCmd) (applied, changed []model.DateChange, last issueWrite, ambiguous bool, err error) {
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
				return applied, changed, last, false, model.DatesChangedError(change.Label)
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
