package cronjob

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
)

// ConditionsKey is the options key that holds a job's conditions (ADR-0016).
const ConditionsKey = "conditions"

// Condition names, as a skip record and a refusal name the condition that said no.
const (
	ConditionRunsPerDay  = "runs_per_day"
	ConditionRunsPerWeek = "runs_per_week"
)

// Conditions are what a due occurrence of an Adapter execute job must meet, read from options.conditions. The zero value
// has no condition. Every field is one condition and is off at its zero value, so a later condition is one more field
// and one more key in conditionKeys.
type Conditions struct {
	// RunsPerDay and RunsPerWeek are the most runs the job may start in a calendar day or week of its trigger's zone.
	RunsPerDay  int
	RunsPerWeek int
}

// Any reports whether at least one condition is on.
func (c Conditions) Any() bool { return c != Conditions{} }

// conditionKeys are the keys options.conditions may hold, in the order a problem names them.
var conditionKeys = []string{ConditionRunsPerDay, ConditionRunsPerWeek}

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
	return c, nil
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
