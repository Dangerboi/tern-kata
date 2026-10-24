package model

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

// A Cron job is kata's recurring or scheduled definition, one per Project. The gateway translates kata's wire shape into the
// cronjob package's value; the model adds what only the Pane reads. The Jobs Page, the Job card, the cron markers on Issues
// and the Runs Page read it.

// CronJobParent is a native job's parent Issue as the gateway read it with the job.
type CronJobParent struct {
	UID, ShortID, Title string
	// Status is the Issue's status; Deleted says it is deleted or gone.
	Status  string
	Deleted bool
	// Progress is nil when the parent carries no readable progress for the job.
	Progress *cronjob.Progress
}

// CronJob is a Cron job with, for a native one, its parent.
type CronJob struct {
	cronjob.Job
	// Parent is set on a native job.
	Parent *CronJobParent
}

// Job kinds as the Jobs Page words them.
const (
	KindRecurringChild = "recurring child"
	KindAgentPrompt    = "agent prompt"
	KindWorkflow       = "workflow"
	KindNotify         = "notify"
)

// Kind is what the job does, in words: a native job makes a recurring child, an execute job runs a prompt or a workflow, any
// other notify job notifies.
func (j CronJob) Kind() string {
	switch {
	case j.Native():
		return KindRecurringChild
	case j.Notifies():
		return KindNotify
	case j.RunsWorkflow():
		return KindWorkflow
	}
	return KindAgentPrompt
}

// PauseReason is why a job that is not archived is not running; the zero value is none. It is derived on the client: kata
// stores no pause, a native job just stops being delivered while its parent is closed or gone, its Project archived or the
// job disabled.
type PauseReason string

const (
	NotPaused            PauseReason = ""
	PauseParentClosed    PauseReason = "parent closed"
	PauseParentDeleted   PauseReason = "parent deleted"
	PauseProjectArchived PauseReason = "Project archived"
	PauseDisabled        PauseReason = "disabled"
	PauseArchived        PauseReason = "archived"
)

// Pause is why j is not running, with the Project archived taken from the Project's own state: kata answers no job of an
// archived Project. An archived job is archived whatever else holds; a disabled one reads as disabled before its parent's
// state matters.
func (j CronJob) Pause(projectArchived bool) PauseReason {
	switch {
	case j.Archived():
		return PauseArchived
	case projectArchived:
		return PauseProjectArchived
	case !j.Definition.Enabled:
		return PauseDisabled
	case j.Parent != nil && j.Parent.Deleted:
		return PauseParentDeleted
	case j.Parent != nil && j.Parent.Status != "open":
		return PauseParentClosed
	}
	return NotPaused
}

// StateText is the job's state as a row reads it: "active", "disabled", "archived" or "paused: <reason>".
func (r PauseReason) StateText() string {
	switch r {
	case NotPaused:
		return "active"
	case PauseDisabled, PauseArchived:
		return string(r)
	}
	return "paused: " + string(r)
}

// Missed is how many occurrences of a native job fall after its progress and up to now: what delivery owes it once it runs
// again. It is zero for an adapter job, a job with no readable progress and a trigger with no occurrences.
func (j CronJob) Missed(now time.Time) int {
	if !j.Native() || j.Parent == nil || j.Parent.Progress == nil {
		return 0
	}
	schedule, ok := j.schedule()
	if !ok {
		return 0
	}
	return schedule.Between(j.Parent.Progress.Through, now, maxMissedCount)
}

// maxMissedCount stops counting missed cron occurrences: a count beyond it only needs to read as large.
const maxMissedCount = 10000

// NextRun is when the job fires next after now. It reports false where that can't be worked out: a job that is not running,
// an expression that does not parse, a manual or Issue-date trigger, a once trigger in the past, and an interval with no
// anchor to count from.
func (j CronJob) NextRun(now time.Time, projectArchived bool) (time.Time, bool) {
	if j.Pause(projectArchived) != NotPaused {
		return time.Time{}, false
	}
	if t := j.Definition.Trigger; t.Kind == cronjob.TriggerOnce {
		at, err := time.Parse(time.RFC3339, t.At)
		return at, err == nil && at.After(now)
	}
	schedule, ok := j.schedule()
	if !ok {
		return time.Time{}, false
	}
	next := schedule.Next(now)
	return next, !next.IsZero()
}

// schedule is the occurrences of the job's trigger, by the rule of the cronjob package; the delivery anchor is the one this
// Pane read from the parent.
func (j CronJob) schedule() (cronjob.Schedule, bool) { return j.Job.Schedule(j.deliveryAnchor()) }

// intervalAnchor is the instant an interval trigger of the job counts from.
func (j CronJob) intervalAnchor() (time.Time, bool) { return j.Job.IntervalAnchor(j.deliveryAnchor()) }

// deliveryAnchor is the delivery anchor on a native job's parent, the zero time when the Pane has none.
func (j CronJob) deliveryAnchor() time.Time {
	if j.Parent == nil || j.Parent.Progress == nil {
		return time.Time{}
	}
	return j.Parent.Progress.Anchor
}

// HighRateWarning is the line the Jobs Page shows for a job that makes more than 24 Issues a day, "" for any other.
func (j CronJob) HighRateWarning(now time.Time) string {
	return cronjob.HighRateWarning(j.IssuesPerDay(now))
}

// zoneAndExpression splits a cron trigger into its zone and bare expression.
func zoneAndExpression(t cronjob.Trigger) (zone, expression string) {
	return cronjob.Zone(t.Cron, t.Timezone)
}

// validCron reports whether the cron trigger parses.
func validCron(t cronjob.Trigger) bool {
	_, err := cronjob.Cron(t.Cron, t.Timezone, time.Time{})
	return err == nil
}

// Words reads the trigger as a phrase: "every day at 09:00 Europe/Berlin", "every 15 minutes", "once at 2026-11-01 09:00 UTC",
// "manual", "at the Issue's scheduled date", "2 days before the Issue's deadline". It is empty where the trigger can't be read:
// an expression that does not parse, a once trigger without a valid instant.
func triggerWords(t cronjob.Trigger) string {
	switch t.Kind {
	case cronjob.TriggerManual:
		return "manual"
	case cronjob.TriggerCron:
		if !validCron(t) {
			return ""
		}
		zone, expression := zoneAndExpression(t)
		return cronWords(expression) + " " + zone
	case cronjob.TriggerInterval:
		if t.IntervalSeconds <= 0 {
			return ""
		}
		return "every " + durationWords(t.IntervalSeconds, true)
	case cronjob.TriggerOnce:
		at, err := time.Parse(time.RFC3339, t.At)
		if err != nil {
			return ""
		}
		zone := at.Location().String()
		if t.Timezone != "" {
			if loc, err := time.LoadLocation(t.Timezone); err == nil {
				at, zone = at.In(loc), t.Timezone
			}
		} else if _, offset := at.Zone(); offset == 0 {
			zone = "UTC"
		} else {
			zone = at.Format("-07:00")
		}
		return "once at " + at.Format("2006-01-02 15:04") + " " + zone
	case cronjob.TriggerIssueScheduled:
		return "at the Issue's scheduled date"
	case cronjob.TriggerIssueDeadline:
		if t.LeadSeconds <= 0 {
			return "at the Issue's deadline"
		}
		return durationWords(t.LeadSeconds, false) + " before the Issue's deadline"
	}
	return ""
}

// durationWords reads seconds in the largest unit that divides them: "90 minutes", "2 days". A single unit reads as "day" when
// bare is set (after "every") and "1 day" otherwise.
func durationWords(seconds int64, bare bool) string {
	for _, unit := range []struct {
		name string
		secs int64
	}{{"day", 86400}, {"hour", 3600}, {"minute", 60}, {"second", 1}} {
		if seconds%unit.secs != 0 {
			continue
		}
		n := seconds / unit.secs
		switch {
		case n == 1 && bare:
			return unit.name
		case n == 1:
			return "1 " + unit.name
		}
		return strconv.FormatInt(n, 10) + " " + unit.name + "s"
	}
	return strconv.FormatInt(seconds, 10) + " seconds"
}

var weekdayNames = []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}

// cronWords reads a cron expression that already parsed. The common shapes get a phrase; any other reads as the expression.
func cronWords(expression string) string {
	if strings.HasPrefix(expression, "@") {
		if every, ok := strings.CutPrefix(expression, "@every "); ok {
			if d, err := time.ParseDuration(every); err == nil && d >= time.Second {
				return "every " + durationWords(int64(d/time.Second), true)
			}
			return "cron " + expression
		}
		switch expression {
		case "@yearly", "@annually":
			return "every year on 1 Jan at 00:00"
		case "@monthly":
			return "on day 1 of every month at 00:00"
		case "@weekly":
			return "every Sun at 00:00"
		case "@daily", "@midnight":
			return "every day at 00:00"
		case "@hourly":
			return "every hour"
		}
		return "cron " + expression
	}
	f := strings.Fields(expression)
	if len(f) != 5 {
		return "cron " + expression
	}
	minute, hour, dom, month, dow := f[0], f[1], f[2], f[3], f[4]
	if month != "*" {
		return "cron " + expression
	}
	clock, timed := "", false
	if m, err := strconv.Atoi(minute); err == nil {
		if h, err := strconv.Atoi(hour); err == nil {
			clock, timed = fmt.Sprintf("%02d:%02d", h, m), true
		}
	}
	switch {
	case dom == "*" && dow == "*" && hour == "*" && minute == "*":
		return "every minute"
	case dom == "*" && dow == "*" && hour == "*":
		if n, ok := strings.CutPrefix(minute, "*/"); ok {
			if step, err := strconv.Atoi(n); err == nil && step > 0 && step < 60 && 60%step == 0 {
				return "every " + n + " minutes" // any other step restarts each hour, so it is not an even period
			}
			return "cron " + expression
		}
		if m, err := strconv.Atoi(minute); err == nil {
			return fmt.Sprintf("every hour at minute %d", m)
		}
	case dom == "*" && dow == "*" && timed:
		return "every day at " + clock
	case dom == "*" && timed:
		if days, ok := weekdayList(dow); ok {
			return "every " + days + " at " + clock
		}
	case dow == "*" && timed:
		if d, err := strconv.Atoi(dom); err == nil {
			return fmt.Sprintf("on day %d of every month at %s", d, clock)
		}
	}
	return "cron " + expression
}

// weekdayList reads a cron day-of-week field of numbers, names, ranges and lists as "Mon, Wed" or, for Monday to Friday,
// "weekday".
func weekdayList(field string) (string, bool) {
	var set [7]bool
	day := func(s string) (int, bool) {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 && n <= 7 {
			return n % 7, true
		}
		for i, name := range weekdayNames {
			if strings.EqualFold(s, name) {
				return i, true
			}
		}
		return 0, false
	}
	for part := range strings.SplitSeq(field, ",") {
		from, to, isRange := strings.Cut(part, "-")
		a, ok := day(from)
		if !ok {
			return "", false
		}
		b := a
		if isRange {
			if b, ok = day(to); !ok || b < a && to != "7" {
				return "", false
			}
			if to == "7" {
				b = 7
			}
		}
		for d := a; d <= b; d++ {
			set[d%7] = true
		}
	}
	if set == [7]bool{false, true, true, true, true, true, false} {
		return "weekday", true
	}
	var names []string
	for _, i := range []int{1, 2, 3, 4, 5, 6, 0} { // Monday first, so a weekend reads "Sat, Sun"
		if set[i] {
			names = append(names, weekdayNames[i])
		}
	}
	return strings.Join(names, ", "), len(names) > 0
}
