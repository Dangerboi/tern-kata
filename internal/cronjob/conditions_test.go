package cronjob_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

func jobWithOptions(options map[string]any) cronjob.Job {
	return cronjob.Job{UID: "J", Definition: cronjob.Definition{Options: options}}
}

func TestAJobWithoutConditionsHasNone(t *testing.T) {
	for name, options := range map[string]map[string]any{
		"no options":      nil,
		"other options":   {"adapter": "tern-kata", "profile": "fast"},
		"a null key":      {"conditions": nil},
		"an empty object": {"conditions": map[string]any{}},
	} {
		t.Run(name, func(t *testing.T) {
			c, err := jobWithOptions(options).Conditions()
			if err != nil || c.Any() {
				t.Fatalf("conditions %+v, %v; want none and no error", c, err)
			}
		})
	}
}

func TestConditionsReadRunLimitsFromTheOptionsWhateverTheNumberType(t *testing.T) {
	var decoded map[string]any
	if err := json.Unmarshal([]byte(`{"conditions":{"runs_per_day":3,"runs_per_week":10}}`), &decoded); err != nil {
		t.Fatal(err)
	}
	for name, options := range map[string]map[string]any{
		"decoded JSON": decoded,
		"ints":         {"conditions": map[string]any{"runs_per_day": 3, "runs_per_week": 10}},
		"int64s":       {"conditions": map[string]any{"runs_per_day": int64(3), "runs_per_week": int64(10)}},
		"json numbers": {"conditions": map[string]any{"runs_per_day": json.Number("3"), "runs_per_week": json.Number("10")}},
	} {
		t.Run(name, func(t *testing.T) {
			c, err := jobWithOptions(options).Conditions()
			if err != nil || c != (cronjob.Conditions{RunsPerDay: 3, RunsPerWeek: 10}) || !c.Any() {
				t.Fatalf("conditions %+v, %v; want 3 a day and 10 a week", c, err)
			}
		})
	}
	c, err := jobWithOptions(map[string]any{"conditions": map[string]any{"runs_per_week": 2}}).Conditions()
	if err != nil || c != (cronjob.Conditions{RunsPerWeek: 2}) {
		t.Fatalf("conditions %+v, %v; want only the weekly limit", c, err)
	}
}

func TestConditionsOfTheWrongTypeAreAnErrorThatNamesThem(t *testing.T) {
	for name, tc := range map[string]struct {
		options map[string]any
		want    string
	}{
		"a string for the object":  {map[string]any{"conditions": "3 a day"}, "must be an object, not a string"},
		"a list for the object":    {map[string]any{"conditions": []any{1}}, "must be an object, not a list"},
		"a string limit":           {map[string]any{"conditions": map[string]any{"runs_per_day": "3"}}, "options.conditions.runs_per_day must be a whole number of at least 1"},
		"a fractional limit":       {map[string]any{"conditions": map[string]any{"runs_per_day": 2.5}}, "runs_per_day must be a whole number"},
		"a zero limit":             {map[string]any{"conditions": map[string]any{"runs_per_week": 0}}, "runs_per_week must be a whole number of at least 1"},
		"a negative limit":         {map[string]any{"conditions": map[string]any{"runs_per_week": -1}}, "runs_per_week must be a whole number of at least 1"},
		"a boolean limit":          {map[string]any{"conditions": map[string]any{"runs_per_day": true}}, "runs_per_day must be a whole number"},
		"a null limit":             {map[string]any{"conditions": map[string]any{"runs_per_day": nil}}, "runs_per_day must be a whole number"},
		"an unknown condition key": {map[string]any{"conditions": map[string]any{"runs_per_hour": 2}}, `unknown condition "runs_per_hour"`},
	} {
		t.Run(name, func(t *testing.T) {
			c, err := jobWithOptions(tc.options).Conditions()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v, want one containing %q", err, tc.want)
			}
			if c.Any() {
				t.Fatalf("conditions %+v read from options that do not parse", c)
			}
		})
	}
}

func TestATriggersDaysAreCountedInItsZone(t *testing.T) {
	for name, tc := range map[string]struct {
		trigger cronjob.Trigger
		want    string
	}{
		"a cron trigger's timezone":     {cronjob.Trigger{Kind: cronjob.TriggerCron, Cron: "0 9 * * *", Timezone: "Europe/Berlin"}, "Europe/Berlin"},
		"a cron directive":              {cronjob.Trigger{Kind: cronjob.TriggerCron, Cron: "CRON_TZ=Asia/Tokyo 0 9 * * *"}, "Asia/Tokyo"},
		"a cron trigger with no zone":   {cronjob.Trigger{Kind: cronjob.TriggerCron, Cron: "0 9 * * *"}, "UTC"},
		"an interval with a timezone":   {cronjob.Trigger{Kind: cronjob.TriggerInterval, IntervalSeconds: 3600, Timezone: "America/New_York"}, "America/New_York"},
		"an interval with no timezone":  {cronjob.Trigger{Kind: cronjob.TriggerInterval, IntervalSeconds: 3600}, "UTC"},
		"a timezone that does not load": {cronjob.Trigger{Kind: cronjob.TriggerOnce, Timezone: "Mars/Olympus"}, "UTC"},
		"an Issue date with a timezone": {cronjob.Trigger{Kind: cronjob.TriggerIssueDeadline, Timezone: "Europe/Berlin"}, "Europe/Berlin"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := tc.trigger.Location().String(); got != tc.want {
				t.Fatalf("zone %s, want %s", got, tc.want)
			}
		})
	}
}
