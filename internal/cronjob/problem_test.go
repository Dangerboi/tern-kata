package cronjob_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

func withAction(j cronjob.Job, a cronjob.Action) cronjob.Job {
	j.Definition.Action = a
	return j
}

func prompt(j cronjob.Job) cronjob.Job {
	j.Definition.Action.Prompt = "do it"
	return j
}

func notifying(j cronjob.Job, recipient string) cronjob.Job {
	j.Definition.Action.Recipient = recipient
	return j
}

func TestProblem(t *testing.T) {
	const unschedulable = "its trigger cannot be scheduled: "
	hourly := cronjob.Trigger{Kind: cronjob.TriggerCron, Cron: "0 * * * *"}
	deadline := cronjob.Trigger{Kind: cronjob.TriggerIssueDeadline, IssueUID: "I"}
	cases := []struct {
		name   string
		job    cronjob.Job
		want   string
		prefix bool // want is the start of the reason: the rest is the parser's own complaint
	}{
		{"a clean execute job", prompt(job(cronjob.ActionExecute, perRun(), hourly)), "", false},
		{"a clean execute Workflow job", withWorkflow(job(cronjob.ActionExecute, perRun(), hourly)), "", false},
		{"a clean notify job", notifying(job(cronjob.ActionNotify, existingIssue(), hourly), "alice"), "", false},
		{"a clean notify job on an Issue date", notifying(job(cronjob.ActionNotify, nil, deadline), "alice"), "", false},
		{"a cron expression that does not parse", prompt(job(cronjob.ActionExecute, perRun(), cronjob.Trigger{Kind: cronjob.TriggerCron, Cron: "nope"})), unschedulable, true},
		{"an interval outside kata's bounds", prompt(job(cronjob.ActionExecute, perRun(), cronjob.Trigger{Kind: cronjob.TriggerInterval})), unschedulable, true},
		{"an interval with no creation time to count from", withoutCreation(prompt(job(cronjob.ActionExecute, perRun(), cronjob.Trigger{Kind: cronjob.TriggerInterval, IntervalSeconds: 60}))), unschedulable + "it has no creation time to count from", false},
		{"a once trigger without an RFC3339 instant", prompt(job(cronjob.ActionExecute, perRun(), cronjob.Trigger{Kind: cronjob.TriggerOnce, At: "tomorrow"})), unschedulable + "once requires an RFC3339 instant", false},
		{"issue-deadline on an execute job", prompt(job(cronjob.ActionExecute, perRun(), deadline)), "issue-deadline triggers only notify jobs; an execute job needs issue-scheduled", false},
		{"an execute job with neither a prompt nor a Workflow", job(cronjob.ActionExecute, perRun(), hourly), "it has neither a prompt nor a workflow", false},
		{"an execute job with a blank prompt", withAction(job(cronjob.ActionExecute, perRun(), hourly), cronjob.Action{Kind: cronjob.ActionExecute, Prompt: " \n"}), "it has neither a prompt nor a workflow", false},
		{"an execute job with no Issue policy", prompt(job(cronjob.ActionExecute, nil, hourly)), "it has no Issue policy", false},
		{"a notify job with no Issue", notifying(job(cronjob.ActionNotify, nil, hourly), "alice"), "does not name an Issue and a recipient to notify", false},
		{"a notify job with no recipient", job(cronjob.ActionNotify, existingIssue(), hourly), "does not name an Issue and a recipient to notify", false},
		{"a Native job", notifying(job(cronjob.ActionNotify, perRun(), hourly), "alice"), "creates an Issue per run: kata's daemon delivers it, the worker never runs it", false},
		{"a Native job's bad trigger is not its problem", notifying(job(cronjob.ActionNotify, perRun(), cronjob.Trigger{Kind: cronjob.TriggerCron, Cron: "nope"}), "alice"), "creates an Issue per run: kata's daemon delivers it, the worker never runs it", false},
		{"an action of a kind kata added later", job("page", perRun(), hourly), "it has an action kind this worker does not know", false},
		{"the trigger is reported before the action", job(cronjob.ActionExecute, nil, cronjob.Trigger{Kind: cronjob.TriggerCron, Cron: "nope"}), unschedulable, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.job.Problem()
			if tc.prefix {
				if !strings.HasPrefix(got, tc.want) || got == tc.want {
					t.Errorf("Problem() = %q; want %q and the parser's complaint", got, tc.want)
				}
				return
			}
			if got != tc.want {
				t.Errorf("Problem() = %q; want %q", got, tc.want)
			}
		})
	}
}

func TestActionProblemIgnoresTheTrigger(t *testing.T) {
	j := prompt(job(cronjob.ActionExecute, perRun(), cronjob.Trigger{Kind: cronjob.TriggerCron, Cron: "nope"}))
	if got := j.ActionProblem(); got != "" {
		t.Errorf("ActionProblem() = %q; want none: a manual run does not need a schedule", got)
	}
	if j.TriggerError() == nil {
		t.Error("TriggerError() = nil for an expression that does not parse")
	}
}

func TestTriggerKindsPerAction(t *testing.T) {
	all := []cronjob.TriggerKind{cronjob.TriggerManual, cronjob.TriggerCron, cronjob.TriggerInterval, cronjob.TriggerOnce, cronjob.TriggerIssueScheduled, cronjob.TriggerIssueDeadline, "webhook"}
	refused := map[cronjob.ActionKind][]cronjob.TriggerKind{
		cronjob.ActionExecute: {cronjob.TriggerIssueDeadline, "webhook"},
		cronjob.ActionNotify:  {"webhook"},
		"page":                all,
	}
	for _, action := range []cronjob.ActionKind{cronjob.ActionExecute, cronjob.ActionNotify, "page"} {
		for _, trigger := range all {
			want := !slices.Contains(refused[action], trigger)
			if got := cronjob.AllowsTrigger(action, trigger); got != want {
				t.Errorf("AllowsTrigger(%s, %s) = %v; want %v", action, trigger, got, want)
			}
			if got := slices.Contains(cronjob.TriggersFor(action), trigger); got != want {
				t.Errorf("TriggersFor(%s) holds %s = %v; want %v", action, trigger, got, want)
			}
		}
	}
	if got := cronjob.TriggersFor(cronjob.ActionNotify); got[len(got)-1] != cronjob.TriggerIssueDeadline {
		t.Errorf("TriggersFor(notify) = %v; want the deadline offered last", got)
	}
}
