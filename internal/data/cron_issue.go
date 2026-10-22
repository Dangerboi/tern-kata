package data

import (
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// occurrenceKey is the metadata key kata's daemon writes on each Issue a native Cron job makes.
const occurrenceKey = "cron.occurrence"

// translateOccurrence reads an Issue's cron.occurrence metadata: nil when the Issue has none or it names no job. A scheduled
// time kata wrote in a shape that is not RFC 3339 leaves ScheduledAt zero rather than hiding the job.
func translateOccurrence(metadata map[string]any) *model.CronOccurrence {
	value, ok := metadata[occurrenceKey].(map[string]any)
	if !ok {
		return nil
	}
	jobUID, _ := value["job_uid"].(string)
	if jobUID == "" {
		return nil
	}
	occurrence := &model.CronOccurrence{JobUID: jobUID}
	occurrence.DefinitionEventUID, _ = value["definition_event_uid"].(string)
	if scheduled, ok := value["scheduled_at"].(string); ok {
		if at, err := time.Parse(time.RFC3339Nano, scheduled); err == nil {
			occurrence.ScheduledAt = at
		}
	}
	return occurrence
}
