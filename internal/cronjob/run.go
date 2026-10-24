package cronjob

import "time"

// RunStatus is where a Run stands, as kata records it. A status kata adds later reads as RunUnknown (see ReadRunStatus), so a
// Run of a newer kata still shows and the Worker never mistakes it for one it knows.
type RunStatus string

// The Run statuses kata knows.
const (
	RunRunning   RunStatus = "running"
	RunSucceeded RunStatus = "succeeded"
	RunFailed    RunStatus = "failed"
	RunCancelled RunStatus = "cancelled"
	// RunUnknown is a Run an executor could not settle (held, or lost track of), and any status this package does not know.
	RunUnknown RunStatus = "unknown"
)

// ReadRunStatus is the status kata wrote as one of the constants above; any other text is RunUnknown.
func ReadRunStatus(s string) RunStatus {
	switch st := RunStatus(s); st {
	case RunRunning, RunSucceeded, RunFailed, RunCancelled:
		return st
	}
	return RunUnknown
}

// Run is kata's record of one execution of a Cron job or Workflow, as the Pane and the Worker both read it. A value kata leaves
// out is empty (or nil for a count and an instant).
type Run struct {
	UID string
	// JobUID and WorkflowUID are what the Run belongs to; DefinitionEventUID and WorkflowDefinitionEventUID are the versions of
	// them it ran.
	JobUID, DefinitionEventUID              string
	WorkflowUID, WorkflowDefinitionEventUID string
	OccurrenceKey, IssueUID                 string
	// Actor is who observed the Run first; Teammate and ExecutorLabel say which agent and machine did.
	Actor, Teammate, ExecutorLabel string
	Status                         RunStatus
	// Summary and the token counts are kata's summary. A count kata does not hold is nil.
	Summary                   string
	InputTokens, OutputTokens *int64
	Revision                  int64
	CreatedAt, UpdatedAt      time.Time
	StartedAt, EndedAt        *time.Time
}
