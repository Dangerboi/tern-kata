package model

import (
	"cmp"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// A Cron job is kata's recurring or scheduled definition, one per Project. The gateway translates kata's wire shape into the
// types here; the Jobs Page, the Job card, the cron markers on Issues and the Runs Page read them.

// Trigger kinds, as kata names them.
const (
	TriggerManual         = "manual"
	TriggerCron           = "cron"
	TriggerInterval       = "interval"
	TriggerOnce           = "once"
	TriggerIssueScheduled = "issue-scheduled"
	TriggerIssueDeadline  = "issue-deadline"
)

// Action kinds and Issue policy kinds, as kata names them.
const (
	ActionNotify  = "notify"
	ActionExecute = "execute"
	IssuePerRun   = "per-run"
	IssueExisting = "existing"
)

// CronTrigger says when a Cron job fires. Only the fields of Kind are set; Timezone may be set for any Kind.
type CronTrigger struct {
	Kind string
	// Cron is the 5-field expression or descriptor of a cron trigger.
	Cron string
	// IntervalSeconds is an interval trigger's period.
	IntervalSeconds int64
	// At is a once trigger's RFC3339 instant.
	At string
	// IssueUID names the Issue whose scheduled date or deadline fires an Issue-date trigger.
	IssueUID string
	// LeadSeconds is how long before the deadline an issue-deadline trigger fires.
	LeadSeconds int64
	Timezone    string
}

// CronAction is what a Cron job does when it fires.
type CronAction struct {
	Kind string
	// Message and Recipient belong to a notify action, Prompt or WorkflowUID to an execute action.
	Message, Recipient, Prompt, WorkflowUID string
}

// CronIssuePolicy is which Issue a Cron job's run or notification belongs to: a fresh one per run, or an existing one.
type CronIssuePolicy struct {
	Kind string
	// UID is the existing Issue; ParentUID the parent of a native job's children.
	UID, ParentUID string
	// Title and Body make a per-run Issue; the offsets, in seconds, set its dates relative to the run.
	Title, Body                                   string
	ScheduledOffsetSeconds, DeadlineOffsetSeconds *int64
}

// CronDefinition is the part of a Cron job a Job card edits.
type CronDefinition struct {
	Enabled bool
	Trigger CronTrigger
	Action  CronAction
	// Issue is nil for a notify action with an Issue-date trigger.
	Issue *CronIssuePolicy
	// Overlap is "allow" or "forbid", Catchup "skip", "latest" or "all".
	Overlap, Catchup string
	GraceSeconds     int64
	TimeoutSeconds   int64
	CheckoutKey      string
	// SecretRefs maps an environment name to the worker's source name.
	SecretRefs map[string]string
	// Options is the adapter's opaque object, kept whole; Profile is its options.profile.
	Options map[string]any
	Profile string
}

// CronProgress is a native job's delivery state, kept by kata's daemon in the parent's cron.progress.<job uid> metadata.
// Occurrences are delivered strictly after Anchor, the job's creation; Through is how far delivery has got, the daemon's
// clock at its last pass, so it stops advancing while the job is paused.
type CronProgress struct{ Anchor, Through time.Time }

// CronJobParent is a native job's parent Issue as the gateway read it with the job.
type CronJobParent struct {
	UID, ShortID, Title string
	// Status is the Issue's status; Deleted says it is deleted or gone.
	Status  string
	Deleted bool
	// Progress is nil when the parent carries no readable progress for the job.
	Progress *CronProgress
}

// CronJob is a Cron job with, for a native one, its parent.
type CronJob struct {
	UID, Name, Author string
	Revision          int64
	// DefinitionEventUID guards every write to the job: it names the version this value holds.
	DefinitionEventUID   string
	CreatedAt, UpdatedAt time.Time
	// DeletedAt is set on an archived job.
	DeletedAt  *time.Time
	Definition CronDefinition
	// Parent is set on a native job.
	Parent *CronJobParent
}

// Native says kata's own daemon delivers the job: it notifies with a fresh Issue per run. Every other job belongs to an
// adapter, which kata never runs.
func (j CronJob) Native() bool {
	d := j.Definition
	return d.Action.Kind == ActionNotify && d.Issue != nil && d.Issue.Kind == IssuePerRun
}

// Archived says the job is archived (kata calls it deleted), which stops it until restored.
func (j CronJob) Archived() bool { return j.DeletedAt != nil }

// ParentUID is the Issue a native job's children hang from, empty for an adapter job.
func (j CronJob) ParentUID() string {
	if j.Native() {
		return j.Definition.Issue.ParentUID
	}
	return ""
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
	case j.Definition.Action.Kind == ActionNotify:
		return KindNotify
	case j.Definition.Action.WorkflowUID != "":
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
	from, count := j.Parent.Progress.Through, 0
	switch j.Definition.Trigger.Kind {
	case TriggerInterval:
		step, ok := intervalStep(j.Definition.Trigger.IntervalSeconds)
		if !ok || now.Before(from) {
			return 0
		}
		anchor := j.Parent.Progress.Anchor
		return int(now.Sub(anchor)/step) - int(from.Sub(anchor)/step)
	case TriggerCron:
		schedule, ok := j.Definition.Trigger.cronSchedule()
		if !ok {
			return 0
		}
		for next := schedule.Next(from); !next.IsZero() && !next.After(now) && count < maxMissedCount; next = schedule.Next(next) {
			count++
		}
	}
	return count
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
	t := j.Definition.Trigger
	switch t.Kind {
	case TriggerCron:
		schedule, ok := t.cronSchedule()
		if !ok {
			return time.Time{}, false
		}
		next := schedule.Next(now)
		return next, !next.IsZero()
	case TriggerInterval:
		step, ok := intervalStep(t.IntervalSeconds)
		anchor, anchored := j.intervalAnchor()
		if !ok || !anchored {
			return time.Time{}, false
		}
		return nextTick(anchor, step, now), true
	case TriggerOnce:
		at, err := time.Parse(time.RFC3339, t.At)
		return at, err == nil && at.After(now)
	}
	return time.Time{}, false
}

// intervalAnchor is the instant an interval trigger counts from: a native job's, kata's delivery anchor on its parent; an
// adapter job's, its creation, as the worker anchors it. False when that is not known.
func (j CronJob) intervalAnchor() (time.Time, bool) {
	if j.Native() {
		if j.Parent == nil || j.Parent.Progress == nil {
			return time.Time{}, false
		}
		return j.Parent.Progress.Anchor, true
	}
	return j.CreatedAt, !j.CreatedAt.IsZero()
}

// intervalStep is an interval trigger's period; false when it is not positive or longer than kata accepts, which a Duration
// could not hold without wrapping.
func intervalStep(seconds int64) (time.Duration, bool) {
	if seconds <= 0 || seconds > maxJobSeconds {
		return 0, false
	}
	return time.Duration(seconds) * time.Second, true
}

// nextTick is the first anchor + n*step after now, counting from the first step.
func nextTick(anchor time.Time, step time.Duration, now time.Time) time.Time {
	if now.Before(anchor) {
		return anchor.Add(step)
	}
	return anchor.Add((now.Sub(anchor)/step + 1) * step)
}

// cronParser is kata's own: five fields and descriptors, never seconds, so an expression parses here as it does there.
var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// zoneAndExpression splits a cron trigger into its zone and bare expression: an embedded TZ= or CRON_TZ= directive wins as
// it does in kata's parser, else the trigger's timezone, else UTC.
func (t CronTrigger) zoneAndExpression() (zone, expression string) {
	expression = strings.TrimSpace(t.Cron)
	for _, prefix := range []string{"CRON_TZ=", "TZ="} {
		if rest, ok := strings.CutPrefix(expression, prefix); ok {
			zone, expression, _ = strings.Cut(rest, " ")
			return zone, strings.TrimSpace(expression)
		}
	}
	return cmp.Or(t.Timezone, "UTC"), expression
}

// cronSchedule parses the cron trigger in its zone as kata does: the zone rides on the expression as a CRON_TZ= directive.
func (t CronTrigger) cronSchedule() (cron.Schedule, bool) {
	zone, expression := t.zoneAndExpression()
	schedule, err := cronParser.Parse("CRON_TZ=" + zone + " " + expression)
	return schedule, err == nil
}

// Words reads the trigger as a phrase: "every day at 09:00 Europe/Berlin", "every 15 minutes", "once at 2026-11-01 09:00 UTC",
// "manual", "at the Issue's scheduled date", "2 days before the Issue's deadline". It is empty where the trigger can't be read:
// an expression that does not parse, a once trigger without a valid instant.
func (t CronTrigger) Words() string {
	switch t.Kind {
	case TriggerManual:
		return "manual"
	case TriggerCron:
		if _, ok := t.cronSchedule(); !ok {
			return ""
		}
		zone, expression := t.zoneAndExpression()
		return cronWords(expression) + " " + zone
	case TriggerInterval:
		if t.IntervalSeconds <= 0 {
			return ""
		}
		return "every " + durationWords(t.IntervalSeconds, true)
	case TriggerOnce:
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
	case TriggerIssueScheduled:
		return "at the Issue's scheduled date"
	case TriggerIssueDeadline:
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
			if _, err := strconv.Atoi(n); err == nil {
				return "every " + n + " minutes"
			}
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
	for i, on := range set {
		if on {
			names = append(names, weekdayNames[i])
		}
	}
	return strings.Join(names, ", "), len(names) > 0
}
