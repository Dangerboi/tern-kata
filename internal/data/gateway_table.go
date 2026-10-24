package data

import (
	"context"
	"fmt"
	"reflect"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// The Daemon gateway's command table declares, per command type, how the gateway handles it: the named steps that run
// before its handler, the handler, and the named steps that run after it. Run looks the declaration up and runs it;
// it never switches on a command's type. A command with no declaration fails with a message naming its type.

// kind says how Run executes a declaration: a local command or daemon call answers once, a long-lived command sends many
// results (or none) on its own schedule.
type kind int

const (
	// answersOnce handles the command and sends the one result.
	answersOnce kind = iota
	// longLived hands the command the message channel; it outlives Run's single answer.
	longLived
)

// beforeStep runs ahead of the handler of cmd.
type beforeStep struct {
	name string
	run  func(g *Gateway, ctx context.Context, cmd model.Cmd)
}

// afterStep runs once the handler has a result and returns the result to send, so it can add to it.
type afterStep struct {
	name string
	run  func(g *Gateway, ctx context.Context, cmd model.Cmd, result model.Msg) model.Msg
}

// declaration is the whole handling of one command type.
type declaration struct {
	kind   kind
	before []beforeStep
	after  []afterStep
	// handle produces the one result of an answersOnce command.
	handle func(g *Gateway, ctx context.Context, cmd model.Cmd) model.Msg
	// stream runs a longLived command until ctx ends.
	stream func(g *Gateway, ctx context.Context, cmd model.Cmd, messages chan<- model.Msg)
}

// The named steps.
var (
	// convertPointersBefore converts old Attachment pointers of the command's workspace ahead of the daemon call.
	convertPointersBefore = beforeStep{"convert old pointers", func(g *Gateway, ctx context.Context, cmd model.Cmd) {
		switch cmd := cmd.(type) {
		case model.AttachFile:
			g.convertPointers(ctx, cmd.Workspace, cmd.Actor)
		case model.DetachFile:
			g.convertPointers(ctx, cmd.Workspace, cmd.Actor)
		}
	}}

	// startPointerMoveAfter starts the Attachment pointer move once a Project loaded.
	startPointerMoveAfter = afterStep{"start the pointer move", func(g *Gateway, _ context.Context, cmd model.Cmd, result model.Msg) model.Msg {
		if loaded, ok := result.(model.ProjectLoaded); ok && loaded.Project.UID != "" {
			load := cmd.(model.LoadProject)
			g.startPointerMove(load.Workspace, load.Actor)
		}
		return result
	}}

	// readDefaultFilterAfter reads the Project's Default filter into the load once a Project loaded.
	readDefaultFilterAfter = afterStep{"read the Default filter", func(g *Gateway, _ context.Context, _ model.Cmd, result model.Msg) model.Msg {
		if loaded, ok := result.(model.ProjectLoaded); ok && loaded.Project.UID != "" {
			loaded.Default, loaded.DefaultErr = g.loadFilters(loaded.Project.UID)
			return loaded
		}
		return result
	}}
)

// commands is the table, keyed by the command's type.
var commands = map[reflect.Type]declaration{}

func typeOf[C model.Cmd]() reflect.Type { return reflect.TypeFor[C]() }

// local declares a command the gateway answers without the daemon, or with a handler of its own.
func local[C model.Cmd](handle func(g *Gateway, ctx context.Context, cmd C) model.Msg, before []beforeStep, after []afterStep) {
	commands[typeOf[C]()] = declaration{kind: answersOnce, before: before, after: after,
		handle: func(g *Gateway, ctx context.Context, cmd model.Cmd) model.Msg { return handle(g, ctx, cmd.(C)) }}
}

// longLivedCommand declares a command that sends its results itself.
func longLivedCommand[C model.Cmd](stream func(g *Gateway, ctx context.Context, cmd C, messages chan<- model.Msg)) {
	commands[typeOf[C]()] = declaration{kind: longLived,
		stream: func(g *Gateway, ctx context.Context, cmd model.Cmd, messages chan<- model.Msg) {
			stream(g, ctx, cmd.(C), messages)
		}}
}

// daemonCall declares a command that calls the daemon: how its workspace is found, the call against the client, and the
// failure its error becomes. The call and the failure both read the command, so a result and its failure share a shape.
// When no daemon is found the command still gets its declared failure.
func daemonCall[C model.Cmd](
	workspace func(C) string,
	call func(ctx context.Context, c *Client, cmd C) (model.Msg, error),
	fail func(cmd C, err error) model.Msg,
	before []beforeStep, after []afterStep,
) {
	daemonCallAt(workspace,
		func(ctx context.Context, _ string, c *Client, cmd C) (model.Msg, error) { return call(ctx, c, cmd) },
		fail, before, after)
}

// daemonCallAt is daemonCall for a call that also needs this machine's host name.
func daemonCallAt[C model.Cmd](
	workspace func(C) string,
	call func(ctx context.Context, host string, c *Client, cmd C) (model.Msg, error),
	fail func(cmd C, err error) model.Msg,
	before []beforeStep, after []afterStep,
) {
	local(func(g *Gateway, ctx context.Context, cmd C) model.Msg {
		msg, err := g.call(ctx, workspace(cmd), func(ctx context.Context, c *Client) (model.Msg, error) {
			return call(ctx, g.host, c, cmd)
		})
		if err != nil {
			return fail(cmd, err)
		}
		return msg
	}, before, after)
}

func init() {
	longLivedCommand(func(g *Gateway, ctx context.Context, cmd model.Subscribe, messages chan<- model.Msg) {
		g.stream(ctx, cmd, messages)
	})
	longLivedCommand(func(g *Gateway, ctx context.Context, cmd model.WakeAt, messages chan<- model.Msg) {
		g.wake(ctx, cmd, messages)
	})

	local(func(g *Gateway, _ context.Context, cmd model.LoadLanes) model.Msg { return g.loadLanes(cmd) }, nil, nil)
	local(func(g *Gateway, _ context.Context, cmd model.SaveLanes) model.Msg { return g.saveLanes(cmd) }, nil, nil)
	local(func(g *Gateway, _ context.Context, cmd model.SaveFilters) model.Msg { return g.saveFilters(cmd) }, nil, nil)
	local(func(_ *Gateway, _ context.Context, _ model.LoadWorkerPickers) model.Msg { return loadWorkerPickers() }, nil, nil)
	local(func(g *Gateway, ctx context.Context, cmd model.ReadWorkflowText) model.Msg {
		return g.readWorkflowText(ctx, cmd)
	}, nil, nil)
	local(func(g *Gateway, ctx context.Context, cmd model.LoadWorker) model.Msg { return g.loadWorker(ctx, cmd) }, nil, nil)
	local(func(g *Gateway, _ context.Context, cmd model.LoadWorkerSetup) model.Msg {
		return g.loadWorkerSetup(cmd)
	}, nil, nil)
	local(func(g *Gateway, _ context.Context, cmd model.SaveWorkerSetup) model.Msg {
		return g.saveWorkerSetup(cmd)
	}, nil, nil)
	local(func(g *Gateway, _ context.Context, cmd model.SaveWorkerEdit) model.Msg { return g.saveWorkerEdit(cmd) }, nil, nil)
	local(func(g *Gateway, ctx context.Context, cmd model.StopWorker) model.Msg { return g.stopWorker(ctx, cmd) }, nil, nil)
	local(func(g *Gateway, ctx context.Context, cmd model.SetWorkerSupervision) model.Msg {
		return g.setWorkerSupervision(ctx, cmd)
	}, nil, nil)
	local(func(g *Gateway, ctx context.Context, cmd model.LoadLocalRun) model.Msg {
		return g.loadLocalRun(ctx, cmd)
	}, nil, nil)
	local(func(g *Gateway, ctx context.Context, cmd model.RunJobNow) model.Msg { return g.runJobNow(ctx, cmd) }, nil, nil)
	local(func(g *Gateway, ctx context.Context, cmd model.ResumeLocalRun) model.Msg {
		return g.moveRun(ctx, "resume", cmd.UID)
	}, nil, nil)
	local(func(g *Gateway, ctx context.Context, cmd model.AbandonLocalRun) model.Msg {
		return g.moveRun(ctx, "abandon", cmd.UID)
	}, nil, nil)
	local(func(g *Gateway, ctx context.Context, cmd model.OpenRunSession) model.Msg {
		return g.openRunSession(ctx, cmd)
	}, nil, nil)

	// Daemon calls.
	daemonCall(
		func(cmd model.CreateIssueCmd) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.CreateIssueCmd) (model.Msg, error) {
			return ok(c.CreateIssue(ctx, cmd))
		},
		func(cmd model.CreateIssueCmd, err error) model.Msg { return model.IssueCreated{Err: err} },
		nil, nil)

	daemonCall(
		func(cmd model.OpenIssue) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.OpenIssue) (model.Msg, error) {
			return ok(c.OpenIssue(ctx, cmd))
		},
		func(cmd model.OpenIssue, err error) model.Msg { return model.IssueOpenFailed{Ref: cmd.Ref, Err: err} },
		nil, nil)

	// shortcut: any failure counts as no Issue until the query changes (retrying would loop while the daemon is down); the full-text query that follows reports it.
	daemonCall(
		func(cmd model.LookupSearchID) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.LookupSearchID) (model.Msg, error) {
			uid, err := c.ResolveIssue(ctx, cmd.ProjectID, cmd.ShortID)
			return ok(model.SearchIDLooked{Request: cmd.Request, ShortID: cmd.ShortID, UID: uid}, err)
		},
		func(cmd model.LookupSearchID, _ error) model.Msg {
			return model.SearchIDLooked{Request: cmd.Request, ShortID: cmd.ShortID}
		},
		nil, nil)

	daemonCall(
		func(cmd model.SaveDraft) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.SaveDraft) (model.Msg, error) {
			return c.SaveDraft(ctx, cmd)
		},
		func(cmd model.SaveDraft, err error) model.Msg { return model.DraftFailed{UID: cmd.UID, Err: err} },
		nil, nil)

	daemonCall(
		func(cmd model.CloseIssueCmd) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.CloseIssueCmd) (model.Msg, error) {
			rec, err := c.CloseRecorded(ctx, cmd)
			if err != nil && !rec.Unknown {
				return nil, err
			}
			return model.CloseResultMsg{UID: cmd.UID, Err: err, Record: rec}, nil
		},
		func(cmd model.CloseIssueCmd, err error) model.Msg {
			return model.CloseResultMsg{UID: cmd.UID, Err: err}
		},
		nil, nil)

	daemonCall(
		func(cmd model.MoveIssueCmd) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.MoveIssueCmd) (model.Msg, error) {
			rec, err := c.MoveRecorded(ctx, cmd)
			if err != nil && !rec.Unknown && rec.Entry == nil {
				return nil, err
			}
			return model.MoveResultMsg{UID: cmd.UID, Err: err, Record: rec}, nil
		},
		func(cmd model.MoveIssueCmd, err error) model.Msg { return model.MoveResultMsg{UID: cmd.UID, Err: err} },
		nil, nil)

	daemonCall(
		func(cmd model.CreateRecurrenceCmd) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.CreateRecurrenceCmd) (model.Msg, error) {
			return ok(model.RecurrenceResultMsg{UID: cmd.UID}, c.CreateRecurrence(ctx, cmd))
		},
		func(cmd model.CreateRecurrenceCmd, err error) model.Msg {
			return model.RecurrenceResultMsg{UID: cmd.UID, Err: err}
		},
		nil, nil)

	daemonCall(
		func(cmd model.ChangeRecurrenceCmd) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.ChangeRecurrenceCmd) (model.Msg, error) {
			changed, err := c.ChangeRecurrence(ctx, cmd)
			return ok(model.RecurrenceResultMsg{UID: cmd.UID, Changed: changed}, err)
		},
		func(cmd model.ChangeRecurrenceCmd, err error) model.Msg {
			return model.RecurrenceResultMsg{UID: cmd.UID, Err: err}
		},
		nil, nil)

	daemonCall(
		func(cmd model.StopRecurrenceCmd) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.StopRecurrenceCmd) (model.Msg, error) {
			return ok(model.RecurrenceResultMsg{UID: cmd.UID}, c.StopRecurrence(ctx, cmd))
		},
		func(cmd model.StopRecurrenceCmd, err error) model.Msg {
			return model.RecurrenceResultMsg{UID: cmd.UID, Err: err}
		},
		nil, nil)

	daemonCall(
		func(cmd model.LoadRecurTarget) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.LoadRecurTarget) (model.Msg, error) {
			return ok(c.ReadRecurTarget(ctx, cmd))
		},
		func(cmd model.LoadRecurTarget, err error) model.Msg { return model.RecurTarget{Ref: cmd.Ref, Err: err} },
		nil, nil)

	daemonCall(
		func(cmd model.ReopenIssueCmd) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.ReopenIssueCmd) (model.Msg, error) {
			rec, err := c.ReopenRecorded(ctx, cmd)
			if err != nil && !rec.Unknown {
				return nil, err
			}
			return model.CloseResultMsg{UID: cmd.UID, Err: err, Record: rec}, nil
		},
		func(cmd model.ReopenIssueCmd, err error) model.Msg {
			return model.CloseResultMsg{UID: cmd.UID, Err: err}
		},
		nil, nil)

	daemonCall(
		func(cmd model.WriteField) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.WriteField) (model.Msg, error) {
			rec, err := c.WriteFieldRecorded(ctx, cmd)
			if err != nil && !rec.Unknown {
				return nil, err
			}
			return model.FieldWritten{UID: cmd.UID, Err: err, Record: rec}, nil
		},
		func(cmd model.WriteField, err error) model.Msg { return model.FieldWritten{UID: cmd.UID, Err: err} },
		nil, nil)

	// An undo's answer carries its own failure: the Issue changed or it did not, and a retry could write twice.
	daemonCall(
		func(cmd model.UndoChange) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.UndoChange) (model.Msg, error) {
			return c.Undo(ctx, cmd), nil
		},
		func(cmd model.UndoChange, err error) model.Msg {
			return model.ChangeUndone{EntryID: cmd.Entry.ID, Result: model.UndoFailed, Err: err}
		},
		nil, nil)

	daemonCall(
		func(cmd model.RemoveLinkCmd) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.RemoveLinkCmd) (model.Msg, error) {
			return ok(model.LinkRemoved{UID: cmd.UID, LinkID: cmd.LinkID}, c.RemoveLink(ctx, cmd))
		},
		func(cmd model.RemoveLinkCmd, err error) model.Msg {
			return model.LinkRemoved{UID: cmd.UID, LinkID: cmd.LinkID, Err: err}
		},
		nil, nil)

	// A write has landed: the result and its error travel together, and nothing is retried.
	daemonCall(
		func(cmd model.SetDatesCmd) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.SetDatesCmd) (model.Msg, error) {
			applied, rec, err := c.SetDatesRecorded(ctx, cmd)
			if len(applied) > 0 || rec.Unknown {
				return model.DatesResultMsg{UID: cmd.UID, Applied: applied, Err: err, Record: rec}, nil
			}
			return ok(model.DatesResultMsg{UID: cmd.UID}, err)
		},
		func(cmd model.SetDatesCmd, err error) model.Msg { return model.DatesResultMsg{UID: cmd.UID, Err: err} },
		nil, nil)

	daemonCallAt(
		func(cmd model.AttachFile) string { return cmd.Workspace },
		func(ctx context.Context, host string, c *Client, cmd model.AttachFile) (model.Msg, error) {
			return ok(c.AttachFile(ctx, host, cmd))
		},
		func(cmd model.AttachFile, err error) model.Msg { return model.FileAttached{Ref: cmd.Ref, Err: err} },
		[]beforeStep{convertPointersBefore}, nil)

	// Cleared is final: the result and its error travel together, and nothing is retried.
	daemonCallAt(
		func(cmd model.DetachFile) string { return cmd.Workspace },
		func(ctx context.Context, host string, c *Client, cmd model.DetachFile) (model.Msg, error) {
			detached, err := c.DetachFile(ctx, host, cmd)
			if detached.Cleared {
				detached.Err = err
				return detached, nil
			}
			return ok(detached, err)
		},
		func(cmd model.DetachFile, err error) model.Msg {
			return model.FileDetached{Ref: cmd.Ref, Name: cmd.Name, Err: err}
		},
		[]beforeStep{convertPointersBefore}, nil)

	daemonCall(
		func(cmd model.LoadFieldLabels) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.LoadFieldLabels) (model.Msg, error) {
			return ok(c.FieldLabels(ctx, cmd))
		},
		func(cmd model.LoadFieldLabels, err error) model.Msg {
			return model.FieldLabelsLoaded{UID: cmd.UID, Err: err}
		},
		nil, nil)

	daemonCall(
		func(cmd model.CreateProject) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.CreateProject) (model.Msg, error) {
			project, err := c.CreateProject(ctx, cmd)
			return ok(model.ProjectSaved{Project: project}, err)
		},
		func(cmd model.CreateProject, err error) model.Msg { return model.ProjectSaved{Err: err} },
		nil, nil)

	daemonCall(
		func(cmd model.BindProject) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.BindProject) (model.Msg, error) {
			project, err := c.BindProject(ctx, cmd)
			return ok(model.ProjectSaved{Project: project}, err)
		},
		func(cmd model.BindProject, err error) model.Msg { return model.ProjectSaved{Err: err} },
		nil, nil)

	daemonCall(
		func(cmd model.LoadProject) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.LoadProject) (model.Msg, error) {
			return ok(c.ReadProject(ctx, cmd.Workspace))
		},
		func(cmd model.LoadProject, err error) model.Msg { return model.Failed{Err: err} },
		nil, []afterStep{startPointerMoveAfter, readDefaultFilterAfter})

	daemonCall(
		func(cmd model.LoadDetail) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.LoadDetail) (model.Msg, error) {
			return ok(c.ReadDetail(ctx, cmd.UID))
		},
		func(cmd model.LoadDetail, err error) model.Msg { return model.DetailFailed{UID: cmd.UID, Err: err} },
		nil, nil)

	daemonCall(
		func(cmd model.LoadJobs) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.LoadJobs) (model.Msg, error) {
			return ok(c.ListJobs(ctx, cmd))
		},
		func(cmd model.LoadJobs, err error) model.Msg { return model.JobsLoaded{Request: cmd.Request, Err: err} },
		nil, nil)

	daemonCall(
		func(cmd model.LoadRuns) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.LoadRuns) (model.Msg, error) {
			return ok(c.ListRuns(ctx, cmd))
		},
		func(cmd model.LoadRuns, err error) model.Msg {
			return model.RunsLoaded{Request: cmd.Request, Before: cmd.Before, Err: err}
		},
		nil, nil)

	daemonCall(
		func(cmd model.LoadAuthCapabilities) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.LoadAuthCapabilities) (model.Msg, error) {
			return ok(c.ReadAuthCapabilities(ctx, cmd))
		},
		func(cmd model.LoadAuthCapabilities, err error) model.Msg {
			return model.AuthCapabilitiesLoaded{Request: cmd.Request, Err: err}
		},
		nil, nil)

	daemonCall(
		func(cmd model.LoadCredentials) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.LoadCredentials) (model.Msg, error) {
			return ok(c.ListCredentials(ctx, cmd))
		},
		func(cmd model.LoadCredentials, err error) model.Msg {
			return model.CredentialsLoaded{Request: cmd.Request, Err: err}
		},
		[]beforeStep{pauseBeforeRefresh}, nil)

	daemonCall(
		func(cmd model.LoadFederation) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.LoadFederation) (model.Msg, error) {
			return ok(c.ReadFederation(ctx, cmd))
		},
		func(cmd model.LoadFederation, err error) model.Msg {
			return model.FederationLoaded{Request: cmd.Request, Err: err}
		},
		nil, nil)

	daemonCall(
		func(cmd model.ShowRun) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.ShowRun) (model.Msg, error) {
			return ok(c.ShowRun(ctx, cmd))
		},
		func(cmd model.ShowRun, err error) model.Msg { return model.RunShown{UID: cmd.UID, Err: err} },
		nil, nil)

	daemonCall(
		func(cmd model.SaveJobCmd) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.SaveJobCmd) (model.Msg, error) {
			return ok(c.SaveJob(ctx, cmd))
		},
		func(cmd model.SaveJobCmd, err error) model.Msg {
			return model.JobWritten{ProjectID: cmd.ProjectID, UID: cmd.UID, Err: err}
		},
		nil, nil)

	daemonCall(
		func(cmd model.ArchiveJobCmd) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.ArchiveJobCmd) (model.Msg, error) {
			return ok(c.ArchiveJob(ctx, cmd))
		},
		func(cmd model.ArchiveJobCmd, err error) model.Msg {
			return model.JobWritten{ProjectID: cmd.ProjectID, UID: cmd.UID, Err: err}
		},
		nil, nil)

	daemonCall(
		func(cmd model.RestoreJobCmd) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.RestoreJobCmd) (model.Msg, error) {
			return ok(c.RestoreJob(ctx, cmd))
		},
		func(cmd model.RestoreJobCmd, err error) model.Msg {
			return model.JobWritten{ProjectID: cmd.ProjectID, UID: cmd.UID, Err: err}
		},
		nil, nil)

	daemonCall(
		func(cmd model.LoadWorkflows) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.LoadWorkflows) (model.Msg, error) {
			workflows, err := c.ListWorkflows(ctx, cmd.ProjectID)
			return ok(model.WorkflowsLoaded{ProjectUID: cmd.ProjectUID, Workflows: workflows}, err)
		},
		func(cmd model.LoadWorkflows, err error) model.Msg {
			return model.WorkflowsLoaded{ProjectUID: cmd.ProjectUID, Err: err}
		},
		nil, []afterStep{readWorkflowsAfter})

	daemonCall(
		func(cmd model.LoadJobWorkflows) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.LoadJobWorkflows) (model.Msg, error) {
			workflows, err := c.ListWorkflows(ctx, cmd.ProjectID)
			return ok(model.JobWorkflowsLoaded{ProjectID: cmd.ProjectID, Workflows: workflows}, err)
		},
		func(cmd model.LoadJobWorkflows, err error) model.Msg {
			return model.JobWorkflowsLoaded{ProjectID: cmd.ProjectID, Err: err}
		},
		nil, nil)

	daemonCall(
		func(cmd model.LoadAttention) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.LoadAttention) (model.Msg, error) {
			return ok(c.ListAttention(ctx, cmd))
		},
		func(cmd model.LoadAttention, err error) model.Msg {
			return model.AttentionLoaded{Request: cmd.Request, Err: err}
		},
		nil, nil)

	// A write has landed: the result and its error travel together, and nothing is retried.
	daemonCall(
		func(cmd model.ClearAttention) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.ClearAttention) (model.Msg, error) {
			return ok(c.ClearAttention(ctx, cmd))
		},
		func(cmd model.ClearAttention, err error) model.Msg {
			return model.AttentionCleared{UID: cmd.UID, Err: err}
		},
		nil, nil)

	daemonCall(
		func(cmd model.SaveWorkflow) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.SaveWorkflow) (model.Msg, error) {
			return ok(c.SaveWorkflow(ctx, cmd))
		},
		func(cmd model.SaveWorkflow, err error) model.Msg {
			return model.WorkflowWritten{ProjectUID: cmd.ProjectUID, Op: model.WorkflowOpSave, UID: cmd.UID, Err: err}
		},
		nil, []afterStep{readWrittenWorkflowAfter})

	daemonCall(
		func(cmd model.ArchiveWorkflow) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.ArchiveWorkflow) (model.Msg, error) {
			return ok(c.ArchiveWorkflow(ctx, cmd))
		},
		func(cmd model.ArchiveWorkflow, err error) model.Msg {
			return model.WorkflowWritten{ProjectUID: cmd.ProjectUID, Op: model.WorkflowOpArchive, UID: cmd.UID, Err: err}
		},
		nil, []afterStep{readWrittenWorkflowAfter})

	daemonCall(
		func(cmd model.RestoreWorkflow) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.RestoreWorkflow) (model.Msg, error) {
			return ok(c.RestoreWorkflow(ctx, cmd))
		},
		func(cmd model.RestoreWorkflow, err error) model.Msg {
			return model.WorkflowWritten{ProjectUID: cmd.ProjectUID, Op: model.WorkflowOpRestore, UID: cmd.UID, Err: err}
		},
		nil, []afterStep{readWrittenWorkflowAfter})

	daemonCall(
		func(cmd model.ReadSnapshot) string { return cmd.Workspace },
		func(ctx context.Context, c *Client, cmd model.ReadSnapshot) (model.Msg, error) {
			return ok(c.ReadSnapshot(ctx, cmd))
		},
		func(cmd model.ReadSnapshot, err error) model.Msg {
			return model.SnapshotFailed{Request: cmd.Request, Err: err}
		},
		nil, nil)
}

// declarationFor is the declaration of cmd; a command without one fails with a message naming its type.
func declarationFor(cmd model.Cmd) declaration {
	if d, ok := commands[reflect.TypeOf(cmd)]; ok {
		return d
	}
	return declaration{kind: answersOnce, handle: func(_ *Gateway, _ context.Context, cmd model.Cmd) model.Msg {
		return model.Failed{Err: fmt.Errorf("tern-kata: unknown command %T", cmd)}
	}}
}
