package cronjob_test

import (
	"encoding/json"
	"math"
	"reflect"
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
			if err != nil || !reflect.DeepEqual(c, cronjob.Conditions{RunsPerDay: 3, RunsPerWeek: 10}) || !c.Any() {
				t.Fatalf("conditions %+v, %v; want 3 a day and 10 a week", c, err)
			}
		})
	}
	c, err := jobWithOptions(map[string]any{"conditions": map[string]any{"runs_per_week": 2}}).Conditions()
	if err != nil || !reflect.DeepEqual(c, cronjob.Conditions{RunsPerWeek: 2}) {
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

func TestNewCommitsIsReadAsTrueOrAsAnObjectWithFolders(t *testing.T) {
	var decoded map[string]any
	if err := json.Unmarshal([]byte(`{"conditions":{"new_commits":{"folders":["docs/","./src//api","docs"]},"runs_per_day":2}}`), &decoded); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		options map[string]any
		want    cronjob.Conditions
	}{
		"true":              {map[string]any{"conditions": map[string]any{"new_commits": true}}, cronjob.Conditions{NewCommits: true}},
		"false is off":      {map[string]any{"conditions": map[string]any{"new_commits": false}}, cronjob.Conditions{}},
		"an empty object":   {map[string]any{"conditions": map[string]any{"new_commits": map[string]any{}}}, cronjob.Conditions{NewCommits: true}},
		"folders, cleaned":  {decoded, cronjob.Conditions{NewCommits: true, Folders: []string{"docs", "src/api"}, RunsPerDay: 2}},
		"one folder as any": {map[string]any{"conditions": map[string]any{"new_commits": map[string]any{"folders": []any{"web"}}}}, cronjob.Conditions{NewCommits: true, Folders: []string{"web"}}},
	} {
		t.Run(name, func(t *testing.T) {
			c, err := jobWithOptions(tc.options).Conditions()
			if err != nil || !reflect.DeepEqual(c, tc.want) {
				t.Fatalf("conditions %+v, %v; want %+v", c, err, tc.want)
			}
		})
	}
	off, _ := jobWithOptions(map[string]any{"conditions": map[string]any{"new_commits": false}}).Conditions()
	if off.Any() {
		t.Fatal("new_commits false counts as a condition")
	}
}

func TestNewCommitsOfTheWrongShapeIsAnErrorThatNamesIt(t *testing.T) {
	for name, tc := range map[string]struct {
		value any
		want  string
	}{
		"a string":            {"yes", "options.conditions.new_commits must be true or an object with folders, not a string"},
		"a number":            {1, "must be true or an object with folders, not a number"},
		"null":                {nil, "must be true or an object with folders, not null"},
		"an unknown key":      {map[string]any{"branch": "main"}, `unknown key "branch"`},
		"folders as a string": {map[string]any{"folders": "docs"}, "folders must be a non-empty list of folders, not a string"},
		"folders empty":       {map[string]any{"folders": []any{}}, "folders must be a non-empty list"},
		"a folder number":     {map[string]any{"folders": []any{3}}, "folders must hold strings, not a number"},
		"a blank folder":      {map[string]any{"folders": []any{" "}}, "a folder is empty"},
		"an absolute folder":  {map[string]any{"folders": []any{"/etc"}}, "must be a path under the checkout"},
		"a folder above":      {map[string]any{"folders": []any{"../x"}}, "must be a path under the checkout"},
		"a folder that exits": {map[string]any{"folders": []any{"a/../.."}}, "must be a path under the checkout"},
		"the checkout itself": {map[string]any{"folders": []any{"."}}, "is the whole checkout"},
		"a control character": {map[string]any{"folders": []any{"a\nb"}}, "control character"},
	} {
		t.Run(name, func(t *testing.T) {
			c, err := jobWithOptions(map[string]any{"conditions": map[string]any{"new_commits": tc.value}}).Conditions()
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

func TestConditionsReadSpendLimitsInDollarsWhateverTheNumberType(t *testing.T) {
	var decoded map[string]any
	if err := json.Unmarshal([]byte(`{"conditions":{"spend_per_day":2.5,"spend_per_week":10}}`), &decoded); err != nil {
		t.Fatal(err)
	}
	for name, options := range map[string]map[string]any{
		"decoded JSON": decoded,
		"floats":       {"conditions": map[string]any{"spend_per_day": 2.5, "spend_per_week": 10.0}},
		"ints":         {"conditions": map[string]any{"spend_per_day": 2.5, "spend_per_week": 10}},
		"int64s":       {"conditions": map[string]any{"spend_per_day": 2.5, "spend_per_week": int64(10)}},
		"json numbers": {"conditions": map[string]any{"spend_per_day": json.Number("2.5"), "spend_per_week": json.Number("10")}},
	} {
		t.Run(name, func(t *testing.T) {
			c, err := jobWithOptions(options).Conditions()
			if err != nil || !reflect.DeepEqual(c, cronjob.Conditions{SpendPerDay: 2.5, SpendPerWeek: 10}) || !c.Any() {
				t.Fatalf("conditions %+v, %v; want $2.50 a day and $10 a week", c, err)
			}
		})
	}
	c, err := jobWithOptions(map[string]any{"conditions": map[string]any{"runs_per_day": 3, "spend_per_week": 0.05}}).Conditions()
	if err != nil || !reflect.DeepEqual(c, cronjob.Conditions{RunsPerDay: 3, SpendPerWeek: 0.05}) {
		t.Fatalf("conditions %+v, %v; want a run limit beside a spend limit", c, err)
	}
}

func TestSpendLimitsThatAreNotAnAmountAboveZeroAreAnErrorThatNamesThem(t *testing.T) {
	for name, tc := range map[string]struct {
		key   string
		value any
	}{
		"a string":       {"spend_per_day", "5"},
		"zero":           {"spend_per_day", 0},
		"a negative":     {"spend_per_week", -1.5},
		"a boolean":      {"spend_per_week", true},
		"a null":         {"spend_per_day", nil},
		"an infinity":    {"spend_per_day", math.Inf(1)},
		"a bad number":   {"spend_per_day", json.Number("abc")},
		"not a number":   {"spend_per_week", math.NaN()},
		"a list of them": {"spend_per_day", []any{1}},
	} {
		t.Run(name, func(t *testing.T) {
			c, err := jobWithOptions(map[string]any{"conditions": map[string]any{tc.key: tc.value}}).Conditions()
			want := "options.conditions." + tc.key + " must be an amount in dollars above 0"
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("error %v, want one containing %q", err, want)
			}
			if c.Any() {
				t.Fatalf("conditions %+v read from options that do not parse", c)
			}
		})
	}
}

func TestRunLimitsSpendLimitsAndNewCommitsAreReadTogether(t *testing.T) {
	c, err := jobWithOptions(map[string]any{"conditions": map[string]any{
		"runs_per_day": 2, "spend_per_week": 5.0, "new_commits": map[string]any{"folders": []any{"docs"}},
	}}).Conditions()
	want := cronjob.Conditions{RunsPerDay: 2, SpendPerWeek: 5, NewCommits: true, Folders: []string{"docs"}}
	if err != nil || !reflect.DeepEqual(c, want) {
		t.Fatalf("conditions %+v, %v; want %+v", c, err, want)
	}
	if only := (cronjob.Conditions{SpendPerDay: 1}); !only.Any() {
		t.Fatal("a spend limit alone is not a condition")
	}
}
