package cronjob

import (
	"cmp"
	"errors"
	"strings"
	"time"
)

// The reasons a Cron job cannot run as defined anywhere, whichever machine's Worker reads it. What depends on one Worker's
// worker.toml (a profile, checkout or secret it does not map) stays with that Worker.

// triggersOf are the triggers an adapter job may have; a deadline fires a notification only, as kata allows.
var triggersOf = map[ActionKind][]TriggerKind{
	ActionExecute: {TriggerCron, TriggerInterval, TriggerOnce, TriggerManual, TriggerIssueScheduled},
	ActionNotify:  {TriggerCron, TriggerInterval, TriggerOnce, TriggerManual, TriggerIssueScheduled, TriggerIssueDeadline},
}

// TriggersFor are the trigger kinds a job with this action may have, in the order a Job card offers them. A kind of action
// kata adds later allows none.
func TriggersFor(action ActionKind) []TriggerKind {
	return append([]TriggerKind(nil), triggersOf[action]...)
}

// AllowsTrigger says a job with this action may have a trigger of this kind.
func AllowsTrigger(action ActionKind, trigger TriggerKind) bool {
	for _, k := range triggersOf[action] {
		if k == trigger {
			return true
		}
	}
	return false
}

// OnceAt is the instant of a once trigger.
func OnceAt(t Trigger) (time.Time, error) {
	at, err := time.Parse(time.RFC3339, t.At)
	if err != nil {
		return time.Time{}, errors.New("once requires an RFC3339 instant")
	}
	return at.UTC(), nil
}

// TriggerError is why the job's trigger cannot be scheduled, nil when it can or schedules nothing by time: a cron
// expression that does not parse, an interval outside kata's bounds or with no creation time to count from, a once trigger
// without an RFC3339 instant. The error of a cron or interval trigger is the parser's own complaint.
func (j Job) TriggerError() error {
	t := j.Definition.Trigger
	switch t.Kind {
	case TriggerCron, TriggerInterval:
		if _, ok := j.Schedule(time.Time{}); ok {
			return nil
		}
		var err error
		if t.Kind == TriggerCron {
			_, err = Cron(t.Cron, t.Timezone, time.Time{})
		} else {
			_, err = Interval(t.IntervalSeconds, time.Time{})
		}
		return cmp.Or(err, errors.New("it has no creation time to count from"))
	case TriggerOnce:
		_, err := OnceAt(t)
		return err
	}
	return nil
}

// ActionProblem is why the job's action cannot run as defined, "" when it can: a Native job, which an adapter never runs; a
// notify job with no Issue and recipient to notify; an execute job with neither a prompt nor a Workflow, or with no Issue
// policy; an action of a kind kata added later. It is worded to follow "it", so the caller may quote it after a job's name.
func (j Job) ActionProblem() string {
	d := j.Definition
	switch d.Action.Kind {
	case ActionNotify:
		uid := d.Trigger.IssueUID
		if d.Issue != nil {
			if d.Issue.Kind != IssueExisting {
				return "creates an Issue per run: kata's daemon delivers it, the worker never runs it"
			}
			uid = d.Issue.UID
		}
		if uid == "" || d.Action.Recipient == "" {
			return "does not name an Issue and a recipient to notify"
		}
	case ActionExecute:
		switch {
		case d.Action.WorkflowUID == "" && strings.TrimSpace(d.Action.Prompt) == "":
			return "has neither a prompt nor a workflow"
		case d.Issue == nil:
			return "has no Issue policy"
		}
	default:
		return "has an action kind this worker does not know"
	}
	return ""
}

// Problem is why the job cannot run as defined on any machine, "" when it can: a trigger that cannot be scheduled,
// `issue-deadline` on an execute job, or an action problem. The text reads as a reason on its own.
func (j Job) Problem() string {
	if j.Native() {
		return j.ActionProblem()
	}
	if err := j.TriggerError(); err != nil {
		return "its trigger cannot be scheduled: " + err.Error()
	}
	d := j.Definition
	if d.Action.Kind == ActionExecute && d.Trigger.Kind == TriggerIssueDeadline {
		return "issue-deadline triggers only notify jobs; an execute job needs issue-scheduled"
	}
	if why := j.ActionProblem(); why != "" {
		if d.Action.Kind == ActionNotify {
			return why
		}
		return "it " + why
	}
	return ""
}
