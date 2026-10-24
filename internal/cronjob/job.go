package cronjob

import "time"

// Trigger says when a Cron job fires. Only the fields of Kind are set; Timezone may be set for any Kind.
type Trigger struct {
	Kind TriggerKind
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

// Action is what a Cron job does when it fires.
type Action struct {
	Kind ActionKind
	// Message and Recipient belong to a notify action, Prompt or WorkflowUID to an execute action.
	Message, Recipient, Prompt, WorkflowUID string
}

// IssuePolicy is which Issue a Cron job's run or notification belongs to: a fresh one per run, or an existing one.
type IssuePolicy struct {
	Kind IssuePolicyKind
	// UID is the existing Issue; ParentUID the parent of a Native job's children.
	UID, ParentUID string
	// Title and Body make a per-run Issue; the offsets, in seconds, set its dates relative to the run.
	Title, Body                                   string
	ScheduledOffsetSeconds, DeadlineOffsetSeconds *int64
}

// Definition is the part of a Cron job a Job card edits.
type Definition struct {
	Enabled bool
	Trigger Trigger
	Action  Action
	// Issue is nil for a notify action with an Issue-date trigger.
	Issue *IssuePolicy
	// Overlap is "allow" or "forbid", Catchup "skip", "latest" or "all".
	Overlap, Catchup string
	GraceSeconds     int64
	TimeoutSeconds   int64
	CheckoutKey      string
	// SecretRefs maps an environment name to the Worker's source name.
	SecretRefs map[string]string
	// Options is the adapter's opaque object, kept whole; Profile is its options.profile.
	Options map[string]any
	Profile string
}

// Job is a Cron job as kata stores it: one per Project.
type Job struct {
	UID, Name, Author string
	Revision          int64
	// DefinitionEventUID guards every write to the job: it names the version this value holds.
	DefinitionEventUID   string
	ProjectID            int64
	CreatedAt, UpdatedAt time.Time
	// DeletedAt is set on an archived job (kata calls it deleted).
	DeletedAt  *time.Time
	Definition Definition
}

// Native says kata's own daemon delivers the job: it notifies with a fresh Issue per run, whether or not it is enabled. Every
// other job is an Adapter job, which kata never runs.
func (j Job) Native() bool {
	d := j.Definition
	return d.Action.Kind == ActionNotify && d.Issue != nil && d.Issue.Kind == IssuePerRun
}

// Notifies says the job's action is a notification.
func (j Job) Notifies() bool { return j.Definition.Action.Kind == ActionNotify }

// RunsWorkflow says the job executes a Workflow rather than a prompt.
func (j Job) RunsWorkflow() bool {
	a := j.Definition.Action
	return a.Kind == ActionExecute && a.WorkflowUID != ""
}

// Archived says the job is archived, which stops it until restored.
func (j Job) Archived() bool { return j.DeletedAt != nil }

// ParentUID is the Issue a Native job's children hang from, empty for an Adapter job.
func (j Job) ParentUID() string {
	if j.Native() {
		return j.Definition.Issue.ParentUID
	}
	return ""
}

// IntervalAnchor is the instant an interval trigger counts from: a Native job's, kata's delivery anchor on its parent, which
// the caller reads and passes (the zero time when it has none); an Adapter job's, its creation. False when that is not known.
func (j Job) IntervalAnchor(deliveryAnchor time.Time) (time.Time, bool) {
	if j.Native() {
		return deliveryAnchor, !deliveryAnchor.IsZero()
	}
	return j.CreatedAt, !j.CreatedAt.IsZero()
}

// Schedule is the occurrences of a cron or interval trigger, with the anchor rule written here once:
//   - an Adapter job's interval and @every count from its creation;
//   - a Native job's interval counts from deliveryAnchor, the delivery anchor on its parent (the zero time when unknown, which
//     leaves the schedule unknown);
//   - a Native job's @every stays unanchored on purpose, as kata's own trigger documentation has it: it is the parser's own
//     schedule.
//
// A cron expression is read in its zone, the directive winning. False for any other kind of trigger, an expression that does
// not parse, and an interval with no anchor known.
func (j Job) Schedule(deliveryAnchor time.Time) (Schedule, bool) {
	t := j.Definition.Trigger
	switch t.Kind {
	case TriggerCron:
		var created time.Time
		if !j.Native() {
			created = j.CreatedAt
		}
		schedule, err := Cron(t.Cron, t.Timezone, created)
		return schedule, err == nil
	case TriggerInterval:
		anchor, ok := j.IntervalAnchor(deliveryAnchor)
		if !ok {
			return Schedule{}, false
		}
		schedule, err := Interval(t.IntervalSeconds, anchor)
		return schedule, err == nil
	}
	return Schedule{}, false
}

// IssuesPerDay is about how many Issues a day an enabled execute job with a fresh Issue per run creates, counted from its
// schedule. It is zero for a job that makes none: a disabled one, a Native or notify job, one that reports to an existing
// Issue, and a trigger without a rate.
func (j Job) IssuesPerDay(now time.Time) int {
	d := j.Definition
	if !d.Enabled || d.Action.Kind != ActionExecute || d.Issue == nil || d.Issue.Kind != IssuePerRun {
		return 0
	}
	if k := d.Trigger.Kind; k != TriggerCron && k != TriggerInterval {
		return 0
	}
	schedule, ok := j.Schedule(time.Time{})
	if !ok {
		return 0
	}
	return schedule.PerDay(now)
}
