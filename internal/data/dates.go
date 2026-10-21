package data

import (
	"context"
	"encoding/json"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// SetDates patches each changed date key in order, through the Issue metadata patch, guarded on the value the Dates card opened
// with (absent when it held none). A key changed meanwhile is not overwritten: it stops the run with the refusal naming it. It
// returns the changes written, also when a later one failed.
func (c *Client) SetDates(ctx context.Context, cmd model.SetDatesCmd) ([]model.DateChange, error) {
	var applied []model.DateChange
	for _, change := range cmd.Changes {
		var value any
		if change.Value != "" {
			value = change.Value
		}
		var ifValue *string
		if change.Was != "" {
			text, err := json.Marshal(change.Was)
			if err != nil {
				return applied, err
			}
			guard := string(text)
			ifValue = &guard
		}
		if err := c.patchMetadataKey(ctx, cmd.ProjectID, cmd.UID, cmd.Actor, change.Key, value, ifValue); err != nil {
			if daemonCode(err) == "metadata_guard_failed" {
				return applied, model.DatesChangedError(change.Label)
			}
			return applied, daemonError(err)
		}
		applied = append(applied, change)
	}
	return applied, nil
}
