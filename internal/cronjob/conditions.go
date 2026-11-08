package cronjob

import (
	"encoding/json"
	"fmt"
	"math"
	"path"
	"slices"
	"strings"
	"time"
)

// ConditionsKey is the options key that holds a job's conditions (ADR-0016).
const ConditionsKey = "conditions"

// Condition names, as a skip record and a refusal name the condition that said no.
const (
	ConditionRunsPerDay   = "runs_per_day"
	ConditionRunsPerWeek  = "runs_per_week"
	ConditionSpendPerDay  = "spend_per_day"
	ConditionSpendPerWeek = "spend_per_week"
	ConditionNewCommits   = "new_commits"
)

// Conditions are what a due occurrence of an Adapter execute job must meet, read from options.conditions. The zero value
// has no condition. Every field is one condition and is off at its zero value, so a later condition is one more field,
// one more key in conditionKeys, one more test in Any, and one more row in the worker's condition table.
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

// Any reports whether at least one condition is on.
func (c Conditions) Any() bool {
	return c.RunsPerDay != 0 || c.RunsPerWeek != 0 || c.SpendPerDay != 0 || c.SpendPerWeek != 0 || c.NewCommits
}

// conditionKeys are the keys options.conditions may hold, in the order a problem names them.
var conditionKeys = []string{ConditionRunsPerDay, ConditionRunsPerWeek, ConditionSpendPerDay, ConditionSpendPerWeek, ConditionNewCommits}

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
	for key := range obj {
		if !slices.Contains(conditionKeys, key) {
			return Conditions{}, fmt.Errorf("options.%s has an unknown condition %q (known: %s)", ConditionsKey, key, strings.Join(conditionKeys, ", "))
		}
	}
	var c Conditions
	for _, f := range []struct {
		key string
		to  *int
	}{{ConditionRunsPerDay, &c.RunsPerDay}, {ConditionRunsPerWeek, &c.RunsPerWeek}} {
		v, present := obj[f.key]
		if !present {
			continue
		}
		n, err := wholeNumber(v)
		if err != nil || n < 1 {
			return Conditions{}, fmt.Errorf("options.%s.%s must be a whole number of at least 1, not %v", ConditionsKey, f.key, v)
		}
		*f.to = n
	}
	for _, f := range []struct {
		key string
		to  *float64
	}{{ConditionSpendPerDay, &c.SpendPerDay}, {ConditionSpendPerWeek, &c.SpendPerWeek}} {
		v, present := obj[f.key]
		if !present {
			continue
		}
		d, err := readDollars(v)
		if err != nil {
			return Conditions{}, fmt.Errorf("options.%s.%s must be an amount in dollars above 0, not %v", ConditionsKey, f.key, v)
		}
		*f.to = d
	}
	if v, present := obj[ConditionNewCommits]; present {
		var err error
		if c.NewCommits, c.Folders, err = readNewCommits(v); err != nil {
			return Conditions{}, err
		}
	}
	return c, nil
}

// readDollars reads a JSON number that is a finite amount above 0, whatever Go type the decoder gave it.
func readDollars(v any) (float64, error) {
	var f float64
	switch n := v.(type) {
	case int:
		f = float64(n)
	case int64:
		f = float64(n)
	case float64:
		f = n
	case json.Number:
		var err error
		if f, err = n.Float64(); err != nil {
			return 0, err
		}
	default:
		return 0, fmt.Errorf("not a number")
	}
	if !(f > 0) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("not above 0")
	}
	return f, nil
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

// cleanFolder is a folder under the checkout as a clean slash path; one that leaves the checkout, or is the checkout
// itself, is an error.
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

// wholeNumber reads a JSON number that holds an integer, whatever Go type the decoder gave it.
func wholeNumber(v any) (int, error) {
	var f float64
	switch n := v.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case float64:
		f = n
	case json.Number:
		var err error
		if f, err = n.Float64(); err != nil {
			return 0, err
		}
	default:
		return 0, fmt.Errorf("not a number")
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
