package cronjob

// TriggerKind is how kata names when a Cron job fires. A kind kata adds later stays representable: a value outside the
// constants below is held as it came, and every question of this package answers conservatively for it.
type TriggerKind string

// The trigger kinds kata knows.
const (
	TriggerManual         TriggerKind = "manual"
	TriggerCron           TriggerKind = "cron"
	TriggerInterval       TriggerKind = "interval"
	TriggerOnce           TriggerKind = "once"
	TriggerIssueScheduled TriggerKind = "issue-scheduled"
	TriggerIssueDeadline  TriggerKind = "issue-deadline"
)

// ActionKind is how kata names what a Cron job does when it fires.
type ActionKind string

// The action kinds kata knows.
const (
	ActionNotify  ActionKind = "notify"
	ActionExecute ActionKind = "execute"
)

// IssuePolicyKind is how kata names which Issue a job's run or notification belongs to.
type IssuePolicyKind string

// The Issue policy kinds kata knows.
const (
	IssuePerRun   IssuePolicyKind = "per-run"
	IssueExisting IssuePolicyKind = "existing"
)
