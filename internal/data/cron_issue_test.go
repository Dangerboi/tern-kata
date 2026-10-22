package data

import (
	"encoding/json"
	"testing"
	"time"

	g "go.kenn.io/kata/pkg/client/generated"
)

// Native delivery runs only in kata's daemon loop, so a cron-made Issue is a canned payload as kata sends it.
func TestGatewayTranslatesACronMadeIssuesOccurrence(t *testing.T) {
	var wire g.UIIssue
	payload := `{"id":9,"uid":"01CHILD","short_id":"abcd","title":"Tick — 2026-10-22T14:00:00Z","status":"open","metadata":{
		"cron.occurrence":{"version":1,"job_uid":"01JOB","definition_event_uid":"01EVENT","scheduled_at":"2026-10-22T14:00:00Z"}}}`
	if err := json.Unmarshal([]byte(payload), &wire); err != nil {
		t.Fatal(err)
	}
	detail := translateDetail(&g.UISelectedAuthority{Issue: &wire}, false)
	got := detail.Issue.CronOccurrence
	if got == nil || got.JobUID != "01JOB" || got.DefinitionEventUID != "01EVENT" || !got.ScheduledAt.Equal(time.Date(2026, 10, 22, 14, 0, 0, 0, time.UTC)) {
		t.Fatalf("occurrence %+v", got)
	}
	if !detail.Issue.CronMade() {
		t.Fatal("an Issue with cron.occurrence is cron-made")
	}
}

func TestGatewayAnIssueWithoutOccurrenceMetadataIsNotCronMade(t *testing.T) {
	for name, metadata := range map[string]map[string]any{
		"none":         nil,
		"other keys":   {"cron.progress.01JOB": map[string]any{"anchor": "2026-10-22T14:00:00Z"}},
		"no job":       {"cron.occurrence": map[string]any{"version": 1.0}},
		"not an object": {"cron.occurrence": "x"},
	} {
		if got := translateIssue(&g.UIIssue{Metadata: metadata}); got.CronMade() {
			t.Fatalf("%s: marked cron-made %+v", name, got.CronOccurrence)
		}
	}
}
