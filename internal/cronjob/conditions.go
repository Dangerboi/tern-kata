package cronjob

import (
	"encoding/json"
	"fmt"
	"math"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"
)

// ConditionsKey is the options key that holds a job's conditions (ADR-0016).
const ConditionsKey = "conditions"

const (
	ConditionRunsPerDay   = "runs_per_day"
	ConditionRunsPerWeek  = "runs_per_week"
	ConditionSpendPerDay  = "spend_per_day"
	ConditionSpendPerWeek = "spend_per_week"
	ConditionNewCommits   = "new_commits"
)

// Conditions are what a due occurrence of an Adapter execute job must meet, read from options.conditions. The zero value
// has no condition. Every field is one condition and is off at its zero value, so a later condition is one more field
// and, when windowed, one more row in limits.
type Conditions struct {
	// RunsPerDay and RunsPerWeek are the most runs the job may start in a calendar day or week of its trigger's zone.
	RunsPerDay  int
	RunsPerWeek int
	// SpendPerDay and SpendPerWeek are the most, in US dollars, the job's runs may have cost omp in a day or week: a due
	// occurrence is skipped once the window's spend has reached it.
	SpendPerDay  float64
	SpendPerWeek float64
	// NewCommits asks for commits on the job's checkout branch since the job's last succeeded run; Folders, when set,
	// limits the commits that count to those that touch one of them (slash paths relative to the checkout, cleaned).
	NewCommits bool
	Folders    []string
}

// Limit is one windowed condition that is on: at most Runs runs started, or at most Spend dollars spent, in the
// calendar day or Monday-first week that contains an occurrence.
type Limit struct {
	// Condition is the key in options.conditions and the name a skip record carries.
	Condition string
	// Per is "day" or "week".
	Per   string
	Runs  int
	Spend float64
}

// Wording is the limit as the Pane shows it: "At most 3 runs per day", "At most $5 spent per week".
func (l Limit) Wording() string {
	if l.Runs != 0 {
		runs := " runs per "
		if l.Runs == 1 {
			runs = " run per "
		}
		return "At most " + strconv.Itoa(l.Runs) + runs + l.Per
	}
	return "At most $" + strconv.FormatFloat(l.Spend, 'f', -1, 64) + " spent per " + l.Per
}

type limitRow struct {
	Limit
	runs  *int
	spend *float64
}

// limits is the one table of windowed conditions, in the order options, problems, skips and the Pane list them. A later
// windowed condition is one more field of Conditions and one more row here.
func (c *Conditions) limits() []limitRow {
	return []limitRow{
		{Limit: Limit{Condition: ConditionRunsPerDay, Per: "day"}, runs: &c.RunsPerDay},
		{Limit: Limit{Condition: ConditionRunsPerWeek, Per: "week"}, runs: &c.RunsPerWeek},
		{Limit: Limit{Condition: ConditionSpendPerDay, Per: "day"}, spend: &c.SpendPerDay},
		{Limit: Limit{Condition: ConditionSpendPerWeek, Per: "week"}, spend: &c.SpendPerWeek},
	}
}

// Limits are the windowed conditions that are on, runs first and day before week.
func (c Conditions) Limits() []Limit {
	var out []Limit
	for _, r := range c.limits() {
		if r.runs != nil && *r.runs != 0 {
			r.Runs = *r.runs
		} else if r.spend != nil && *r.spend != 0 {
			r.Spend = *r.spend
		} else {
			continue
		}
		out = append(out, r.Limit)
	}
	return out
}

func (c Conditions) Any() bool { return len(c.Limits()) > 0 || c.NewCommits }

func conditionKeys() []string {
	var keys []string
	for _, r := range new(Conditions).limits() {
		keys = append(keys, r.Condition)
	}
	return append(keys, ConditionNewCommits)
}

// Conditions reads options.conditions once, typed. An absent key (or a null) is no conditions. A value that does not
// parse (not an object, an unknown key, a limit that is not a whole number of at least 1) is an error that names it: a
// condition that cannot be read is a condition that cannot be checked, never one that is dropped.
func (j Job) Conditions() (Conditions, error) {
	raw, ok := j.Definition.Options[ConditionsKey]
	if !ok || raw == nil {
		return Conditions{}, nil
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		return Conditions{}, fmt.Errorf("options.%s must be an object, not %s", ConditionsKey, kindOf(raw))
	}
	keys := conditionKeys()
	for key := range obj {
		if !slices.Contains(keys, key) {
			return Conditions{}, fmt.Errorf("options.%s has an unknown condition %q (known: %s)", ConditionsKey, key, strings.Join(keys, ", "))
		}
	}
	var c Conditions
	for _, r := range c.limits() {
		v, present := obj[r.Condition]
		if !present {
			continue
		}
		if r.runs != nil {
			n, err := wholeNumber(v)
			if err != nil || n < 1 {
				return Conditions{}, fmt.Errorf("options.%s.%s must be a whole number of at least 1, not %v", ConditionsKey, r.Condition, v)
			}
			*r.runs = n
			continue
		}
		d, err := readDollars(v)
		if err != nil {
			return Conditions{}, fmt.Errorf("options.%s.%s must be an amount in dollars above 0, not %v", ConditionsKey, r.Condition, v)
		}
		*r.spend = d
	}
	if v, present := obj[ConditionNewCommits]; present {
		var err error
		if c.NewCommits, c.Folders, err = readNewCommits(v); err != nil {
			return Conditions{}, err
		}
	}
	return c, nil
}

func readDollars(v any) (float64, error) {
	f, err := numberValue(v)
	if err != nil {
		return 0, err
	}
	if !(f > 0) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("not above 0")
	}
	return f, nil
}

func numberValue(v any) (float64, error) {
	switch n := v.(type) {
	case int:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case float64:
		return n, nil
	case json.Number:
		return n.Float64()
	}
	return 0, fmt.Errorf("not a number")
}

// readNewCommits reads options.conditions.new_commits: true (or false, which is off), or an object whose only key is
// folders, a non-empty list of folders under the checkout.
func readNewCommits(v any) (on bool, folders []string, err error) {
	prefix := fmt.Sprintf("options.%s.%s", ConditionsKey, ConditionNewCommits)
	switch n := v.(type) {
	case bool:
		return n, nil, nil
	case map[string]any:
		for key := range n {
			if key != "folders" {
				return false, nil, fmt.Errorf("%s has an unknown key %q (known: folders)", prefix, key)
			}
		}
		raw, present := n["folders"]
		if !present {
			return true, nil, nil
		}
		list, ok := raw.([]any)
		if !ok || len(list) == 0 {
			return false, nil, fmt.Errorf("%s.folders must be a non-empty list of folders, not %s", prefix, kindOf(raw))
		}
		for _, item := range list {
			name, ok := item.(string)
			if !ok {
				return false, nil, fmt.Errorf("%s.folders must hold strings, not %s", prefix, kindOf(item))
			}
			folder, err := cleanFolder(name)
			if err != nil {
				return false, nil, fmt.Errorf("%s.folders: %w", prefix, err)
			}
			if !slices.Contains(folders, folder) {
				folders = append(folders, folder)
			}
		}
		return true, folders, nil
	}
	return false, nil, fmt.Errorf("%s must be true or an object with folders, not %s", prefix, kindOf(v))
}

func cleanFolder(name string) (string, error) {
	folder := path.Clean(strings.TrimSpace(name))
	switch {
	case strings.TrimSpace(name) == "":
		return "", fmt.Errorf("a folder is empty")
	case strings.ContainsAny(folder, "\x00\n\r"):
		return "", fmt.Errorf("folder %q holds a control character", name)
	case path.IsAbs(folder), folder == "..", strings.HasPrefix(folder, "../"):
		return "", fmt.Errorf("folder %q must be a path under the checkout", name)
	case folder == ".":
		return "", fmt.Errorf("folder %q is the whole checkout: leave folders out instead", name)
	}
	return folder, nil
}

func wholeNumber(v any) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	}
	f, err := numberValue(v)
	if err != nil {
		return 0, err
	}
	if f != math.Trunc(f) || math.Abs(f) > 1e9 {
		return 0, fmt.Errorf("not a whole number")
	}
	return int(f), nil
}

func kindOf(v any) string {
	switch v.(type) {
	case string:
		return "a string"
	case bool:
		return "a boolean"
	case []any:
		return "a list"
	case map[string]any:
		return "an object"
	case nil:
		return "null"
	case float64, int, int64, json.Number:
		return "a number"
	}
	return fmt.Sprintf("%T", v)
}

// Location is the zone the trigger's days and weeks are counted in: a cron trigger's zone as Zone reads it, otherwise the
// trigger's timezone, and UTC when there is none or it does not load.
func (t Trigger) Location() *time.Location {
	zone := t.Timezone
	if t.Kind == TriggerCron {
		zone, _ = Zone(t.Cron, t.Timezone)
	}
	if zone == "" {
		return time.UTC
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return time.UTC
	}
	return loc
}
