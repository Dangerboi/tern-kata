package cronjob

// TriggerKind is how kata names when a Cron job fires. A kind kata adds later stays representable: a value outside the
// constants below is held as it came, and every question of this package answers conservatively for it.
type TriggerKind string

const (
	TriggerManual         TriggerKind = "manual"
	TriggerCron           TriggerKind = "cron"
	TriggerInterval       TriggerKind = "interval"
	TriggerOnce           TriggerKind = "once"
	TriggerIssueScheduled TriggerKind = "issue-scheduled"
	TriggerIssueDeadline  TriggerKind = "issue-deadline"
)

type ActionKind string

const (
	ActionNotify  ActionKind = "notify"
	ActionExecute ActionKind = "execute"
)

type IssuePolicyKind string

const (
	IssuePerRun   IssuePolicyKind = "per-run"
	IssueExisting IssuePolicyKind = "existing"
)
