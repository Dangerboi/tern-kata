package cronjob

import (
	"cmp"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// MaxIntervalSeconds is the longest interval kata accepts, which a Duration holds without wrapping.
const MaxIntervalSeconds = int64(1<<63-1) / 1e9

// HighRatePerDay is the most Issues a day a job may create before it is warned about.
const HighRatePerDay = 24

// maxPerWeek stops counting a week's occurrences: a count beyond it only needs to read as large.
const maxPerWeek = 100_000

// parser is kata's own: five fields and descriptors, never seconds, so an expression parses here as it does there.
var parser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// Step is an interval's period; false when it is not positive or longer than kata accepts.
func Step(seconds int64) (time.Duration, bool) {
	if seconds <= 0 || seconds > MaxIntervalSeconds {
		return 0, false
	}
	return time.Duration(seconds) * time.Second, true
}

// Zone splits a cron trigger into its zone and bare expression: an embedded TZ= or CRON_TZ= directive wins as it does in
// kata's parser, else the trigger's timezone, else UTC.
func Zone(expression, timezone string) (zone, bare string) {
	expression = strings.TrimSpace(expression)
	for _, prefix := range []string{"CRON_TZ=", "TZ="} {
		if rest, ok := strings.CutPrefix(expression, prefix); ok {
			zone, bare, _ = strings.Cut(rest, " ")
			return zone, strings.TrimSpace(bare)
		}
	}
	return cmp.Or(timezone, "UTC"), expression
}

// Schedule yields the occurrences of one trigger: a cron expression in its zone, or an interval counting from an anchor.
type Schedule struct {
	cron   cron.Schedule
	step   time.Duration
	anchor time.Time
}

// Interval counts from anchor in steps of seconds: the first occurrence is anchor + one step.
func Interval(seconds int64, anchor time.Time) (Schedule, error) {
	step, ok := Step(seconds)
	if !ok {
		return Schedule{}, errors.New("interval must be positive")
	}
	return Schedule{step: step, anchor: anchor.UTC()}, nil
}

// Cron reads a cron expression in its zone as kata does. An @every expression is an interval anchored at createdAt, the
// job's creation, when that is known; with the zero time it stays the parser's own schedule, which a native job's
// delivery uses.
func Cron(expression, timezone string, createdAt time.Time) (Schedule, error) {
	zone, bare := Zone(expression, timezone)
	schedule, err := parser.Parse("CRON_TZ=" + zone + " " + bare)
	if err != nil {
		return Schedule{}, fmt.Errorf("invalid cron expression: %w", err)
	}
	if every, ok := schedule.(cron.ConstantDelaySchedule); ok && every.Delay > 0 && !createdAt.IsZero() {
		return Schedule{step: every.Delay, anchor: createdAt.UTC()}, nil
	}
	return Schedule{cron: schedule}, nil
}

// Next is the first occurrence strictly after t, in UTC; the zero time when there is none.
func (s Schedule) Next(t time.Time) time.Time {
	if s.step > 0 {
		if t.Before(s.anchor) {
			return s.anchor.Add(s.step)
		}
		return s.anchor.Add((t.Sub(s.anchor)/s.step + 1) * s.step)
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
		return int(to.Sub(s.anchor)/s.step) - int(from.Sub(s.anchor)/s.step)
	}
	count := 0
	for next := s.Next(from); !next.IsZero() && !next.After(to) && count < limit; next = s.Next(next) {
		count++
	}
	return count
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
