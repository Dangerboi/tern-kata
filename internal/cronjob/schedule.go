package cronjob

import (
	"cmp"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/netresearch/go-cron"
)

// MaxIntervalSeconds is the longest interval kata accepts, which a Duration holds without wrapping.
const MaxIntervalSeconds = int64(1<<63-1) / 1e9

// HighRatePerDay is the most Issues a day a job may create before it is warned about.
const HighRatePerDay = 24

// maxPerWeek stops counting a week's occurrences: a count beyond it only needs to read as large.
const maxPerWeek = 100_000

// parser is kata's own: five fields, descriptors and crontab's either-day rule, never seconds, so an expression parses here as it does there.
var parser = cron.MustNewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor | cron.DowOrDom)

// Step is an interval's period; false when it is not positive or longer than kata accepts.
func Step(seconds int64) (time.Duration, bool) {
	if seconds <= 0 || seconds > MaxIntervalSeconds {
		return 0, false
	}
	return time.Duration(seconds) * time.Second, true
}

// Zone splits a cron trigger into its zone and bare expression: the trigger's timezone, else an embedded TZ= or CRON_TZ=
// directive, else UTC. A directive naming another zone than a non-empty timezone never wins: Cron refuses that trigger, and
// this reports the timezone, the zone kata's contract gives a portable cron trigger.
func Zone(expression, timezone string) (zone, bare string) {
	directive, bare := directiveOf(expression)
	if timezone != "" {
		return timezone, bare
	}
	return cmp.Or(directive, "UTC"), bare
}

// directiveOf splits off an embedded TZ= or CRON_TZ= directive: its zone, "" when there is none, and the bare expression.
func directiveOf(expression string) (zone, bare string) {
	expression = strings.TrimSpace(expression)
	for _, prefix := range []string{"CRON_TZ=", "TZ="} {
		if rest, ok := strings.CutPrefix(expression, prefix); ok {
			zone, bare, _ = strings.Cut(rest, " ")
			if n := len(zone); n >= 2 && (zone[0] == '"' || zone[0] == '\'') && zone[n-1] == zone[0] {
				zone = zone[1 : n-1]
			}
			return zone, strings.TrimSpace(bare)
		}
	}
	return "", expression
}

// ZoneConflictError is a cron expression whose embedded directive names another zone than the trigger's timezone.
type ZoneConflictError struct{ Directive, Timezone string }

func (e *ZoneConflictError) Error() string {
	return fmt.Sprintf("its cron expression names zone %s but the trigger's timezone is %s", e.Directive, e.Timezone)
}

// Schedule yields the occurrences of one trigger: a cron expression in its zone, or an interval counting from an anchor.
type Schedule struct {
	cron   cron.Schedule
	loc    *time.Location
	step   time.Duration
	anchor time.Time
}

// Zone is the zone a cron expression is read in, for showing its occurrences as the expression means them. False for an
// interval, which has no zone of its own.
func (s Schedule) Zone() (zone *time.Location, ok bool) { return s.loc, s.loc != nil }

// Interval counts from anchor in steps of seconds: the first occurrence is anchor + one step.
func Interval(seconds int64, anchor time.Time) (Schedule, error) {
	step, ok := Step(seconds)
	if !ok {
		return Schedule{}, errors.New("interval must be positive")
	}
	return Schedule{step: step, anchor: anchor.UTC()}, nil
}

// Cron reads a cron expression in its zone as kata does, except that a directive naming another zone than a non-empty
// timezone is an error, not a winner: kata's portable cron triggers use the trigger's timezone, and its native recurring jobs
// require the two to agree. An @every expression is an interval anchored at createdAt, the
// job's creation, when that is known; with the zero time it stays the parser's own schedule, which a native job's
// delivery uses.
func Cron(expression, timezone string, createdAt time.Time) (Schedule, error) {
	if directive, _ := directiveOf(expression); directive != "" && timezone != "" && directive != timezone {
		return Schedule{}, &ZoneConflictError{Directive: directive, Timezone: timezone}
	}
	zone, bare := Zone(expression, timezone)
	schedule, err := parser.Parse("CRON_TZ=" + zone + " " + bare)
	if err != nil {
		return Schedule{}, fmt.Errorf("invalid cron expression: %w", err)
	}
	if every, ok := schedule.(cron.ConstantDelaySchedule); ok && every.Delay > 0 && !createdAt.IsZero() {
		return Schedule{step: every.Delay, anchor: createdAt.UTC()}, nil
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return Schedule{}, fmt.Errorf("invalid cron expression: %w", err)
	}
	return Schedule{cron: schedule, loc: loc}, nil
}

// Next is the first occurrence strictly after t, in UTC; the zero time when there is none.
func (s Schedule) Next(t time.Time) time.Time {
	if s.step > 0 {
		if t.Before(s.anchor) {
			return s.anchor.Add(s.step)
		}
		steps := int64(t.Sub(s.anchor)/s.step) + 1
		return s.anchor.Add(time.Duration(steps) * s.step)
	}
	if next := s.cron.Next(t); !next.IsZero() {
		return next.UTC()
	}
	return time.Time{}
}

// Between is how many occurrences fall after from and up to to. An expression's are counted one by one up to limit, which a
// count beyond only needs to read as large.
func (s Schedule) Between(from, to time.Time, limit int) int {
	if !to.After(from) {
		return 0
	}
	if s.step > 0 {
		return s.stepsThrough(to) - s.stepsThrough(from)
	}
	count := 0
	for next := s.Next(from); !next.IsZero() && !next.After(to) && count < limit; next = s.Next(next) {
		count++
	}
	return count
}

// stepsThrough is how many of an interval's occurrences, one step after its anchor and every step after that, fall at or
// before t: none before the anchor, where Go's division toward zero would count backwards.
func (s Schedule) stepsThrough(t time.Time) int {
	if t.Before(s.anchor) {
		return 0
	}
	return int(t.Sub(s.anchor) / s.step)
}

// PerDay is about how many Issues a day the trigger makes one of per occurrence, counted over a week from now so a weekday
// schedule averages out.
func (s Schedule) PerDay(now time.Time) int {
	if s.step > 0 {
		return int((24*time.Hour + s.step/2) / s.step)
	}
	return (s.Between(now, now.Add(7*24*time.Hour), maxPerWeek) + 3) / 7
}

// HighRateWarning is the line for a job that makes perDay Issues a day: "" unless that is more than 24.
func HighRateWarning(perDay int) string {
	if perDay <= HighRatePerDay {
		return ""
	}
	return fmt.Sprintf("creates about %d Issues a day; consider an existing Issue", perDay)
}
