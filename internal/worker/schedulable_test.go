package worker

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

func notifyJob(trigger cronjob.Trigger, issue *cronjob.IssuePolicy, recipient string) cronjob.Job {
	return cronjob.Job{Name: "n", Definition: cronjob.Definition{
		Enabled: true,
		Trigger: trigger,
		Action:  cronjob.Action{Kind: cronjob.ActionNotify, Recipient: recipient},
		Issue:   issue,
	}}
}

func withMessage(job cronjob.Job, message string) cronjob.Job {
	job.Definition.Action.Message = message
	return job
}

// Kata's date sweeper notifies the trigger Issue's owner or author for the zero-lead alias, so the worker plans that job
// only when it notifies some other Issue, a later moment, a named recipient or with a message of its own (the sweeper
// sends only its generic text).
func TestSchedulableSkipsOnlyTheNotifyJobTheSweeperAlreadyDelivers(t *testing.T) {
	const trigger, other = "01TRIGGERISSUE", "01OTHERISSUE"
	for _, kind := range []cronjob.TriggerKind{cronjob.TriggerIssueScheduled, cronjob.TriggerIssueDeadline} {
		date := func(lead int64) cronjob.Trigger {
			return cronjob.Trigger{Kind: kind, IssueUID: trigger, LeadSeconds: lead}
		}
		cases := []struct {
			name string
			job  cronjob.Job
			want bool
		}{
			{"same Issue, issue omitted", notifyJob(date(0), nil, currentOwnerOrAuthor), false},
			{"same Issue, issue named", notifyJob(date(0), &cronjob.IssuePolicy{Kind: cronjob.IssueExisting, UID: trigger}, currentOwnerOrAuthor), false},
			{"another Issue", notifyJob(date(0), &cronjob.IssuePolicy{Kind: cronjob.IssueExisting, UID: other}, currentOwnerOrAuthor), true},
			{"a lead", notifyJob(date(600), nil, currentOwnerOrAuthor), true},
			{"a message of its own", withMessage(notifyJob(date(0), nil, currentOwnerOrAuthor), "Ship it today"), true},
			{"a blank message", withMessage(notifyJob(date(0), nil, currentOwnerOrAuthor), "  "), false},
			{"a named recipient", notifyJob(date(0), nil, "bob"), true},
		}
		for _, c := range cases {
			if got := schedulable(c.job); got != c.want {
				t.Errorf("%s %s: schedulable = %v, want %v", kind, c.name, got, c.want)
			}
		}
	}
}

func TestNotifyPlanTargetsTheIssueTheJobNotifies(t *testing.T) {
	job := notifyJob(cronjob.Trigger{Kind: cronjob.TriggerIssueDeadline, IssueUID: "01TRIGGERISSUE"},
		&cronjob.IssuePolicy{Kind: cronjob.IssueExisting, UID: "01OTHERISSUE"}, currentOwnerOrAuthor)
	_, policy, err := notifyPlan(job)
	if err != nil || policy.UID != "01OTHERISSUE" {
		t.Fatalf("notifyPlan = %q, %v; want the notified Issue, not the trigger's", policy.UID, err)
	}
}
