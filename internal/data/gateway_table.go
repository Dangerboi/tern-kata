package data

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// The Daemon gateway's command table declares, per command type, how the gateway handles it: the handler and the named
// steps that run after it. Run looks the declaration up and runs it; it never switches on a command's type. A command with
// no declaration fails with a message naming its type.

// kind says how Run executes a declaration: a local command or daemon call answers once, a long-lived command sends many
// results (or none) on its own schedule.
type kind int

const (
	// answersOnce handles the command and sends the one result.
	answersOnce kind = iota
	// longLived hands the command the message channel; it outlives Run's single answer.
	longLived
)

// afterStep runs once the handler has a result and returns the result to send, so it can add to it.
type afterStep struct {
	name string
	run  func(g *Gateway, ctx context.Context, cmd model.Cmd, result model.Msg) model.Msg
}

// declaration is the whole handling of one command type.
type declaration struct {
	kind  kind
	after []afterStep
	// handle produces the one result of an answersOnce command.
	handle func(g *Gateway, ctx context.Context, cmd model.Cmd) model.Msg
	// stream runs a longLived command until ctx ends.
	stream func(g *Gateway, ctx context.Context, cmd model.Cmd, messages chan<- model.Msg)
}

// The named steps.
var (
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

// local declares a command the gateway answers without the daemon, or with a handler of its own. Method expressions
// of Gateway fit it as they are.
func local[C model.Cmd, R model.Msg](handle func(g *Gateway, ctx context.Context, cmd C) R, after ...afterStep) {
	commands[reflect.TypeFor[C]()] = declaration{kind: answersOnce, after: after,
		handle: func(g *Gateway, ctx context.Context, cmd model.Cmd) model.Msg { return handle(g, ctx, cmd.(C)) }}
}

// withoutCtx adapts a Gateway method that takes no ctx to local.
func withoutCtx[C model.Cmd, R model.Msg](handle func(*Gateway, C) R) func(*Gateway, context.Context, C) R {
	return func(g *Gateway, _ context.Context, cmd C) R { return handle(g, cmd) }
}

// longLivedCommand declares a command that sends its results itself.
func longLivedCommand[C model.Cmd](stream func(g *Gateway, ctx context.Context, cmd C, messages chan<- model.Msg)) {
	commands[reflect.TypeFor[C]()] = declaration{kind: longLived,
		stream: func(g *Gateway, ctx context.Context, cmd model.Cmd, messages chan<- model.Msg) {
			stream(g, ctx, cmd.(C), messages)
		}}
}

// answer runs fn against the daemon for workspace and returns its result; the failure of the call is what fail makes of
// its error. When no daemon is found the command still gets its declared failure. A failed call has no result to show,
// so fn may return its result and its error together.
func answer[R model.Msg](g *Gateway, ctx context.Context, workspace string, fail func(error) model.Msg, fn func(context.Context, *Client) (R, error)) model.Msg {
	result, err := callAs(g, ctx, workspace, fn)
	if err != nil {
		return fail(err)
	}
	return result
}

// daemonHandler is the handler of a command that calls the daemon: how its workspace is found, the call against the client,
// and the failure its error becomes. The call and the failure both read the command, so a result and its failure share a
// shape.
func daemonHandler[C model.Cmd, R model.Msg](
	workspace func(C) string,
	call func(c *Client, ctx context.Context, cmd C) (R, error),
	fail func(cmd C, err error) model.Msg,
) func(*Gateway, context.Context, C) model.Msg {
	return func(g *Gateway, ctx context.Context, cmd C) model.Msg {
		return answer(g, ctx, workspace(cmd), func(err error) model.Msg { return fail(cmd, err) },
			func(ctx context.Context, c *Client) (R, error) { return call(c, ctx, cmd) })
	}
}

// daemonCall declares a command that calls the daemon; see daemonHandler.
func daemonCall[C model.Cmd, R model.Msg](
	workspace func(C) string,
	call func(c *Client, ctx context.Context, cmd C) (R, error),
	fail func(cmd C, err error) model.Msg,
	after ...afterStep,
) {
	local(daemonHandler(workspace, call, fail), after...)
}

// hostCall declares a daemon call that needs this machine's Host id, read where the state folder is. A failure to read it is
// the command's failure, before any call.
func hostCall[C model.Cmd, R model.Msg](
	workspace func(C) string,
	call func(c *Client, ctx context.Context, hostID string, cmd C) (R, error),
	fail func(cmd C, err error) model.Msg,
) {
	local(func(g *Gateway, ctx context.Context, cmd C) model.Msg {
		failed := func(err error) model.Msg { return fail(cmd, err) }
		state, err := g.stateDir()
		if err != nil {
			return failed(err)
		}
		hostID, err := ReadHostID(state)
		if err != nil {
			return failed(err)
		}
		return answer(g, ctx, workspace(cmd), failed, func(ctx context.Context, c *Client) (R, error) {
			return call(c, ctx, hostID, cmd)
		})
	})
}

// recorded is the result of a write that records itself in the Undo history. When the write failed, the failure travels
// with the result only if landed says a change reached the Issue (or may have); otherwise the error alone fails the command,
// and a retry is safe.
func recorded[R model.Msg](err error, landed bool, result R) (R, error) {
	if err != nil && !landed {
		var none R
		return none, err
	}
	return result, nil
}

func init() {
	longLivedCommand((*Gateway).stream)
	longLivedCommand((*Gateway).wake)

	local((*Gateway).loadLanes)
	local(withoutCtx((*Gateway).saveLanes))
	daemonCall(
		func(cmd model.ShareLanes) string { return cmd.Workspace },
		(*Client).ShareLanes,
		func(cmd model.ShareLanes, err error) model.Msg { return model.LanesShared{Share: cmd, Err: err} })
	local(withoutCtx((*Gateway).saveFilters))
	local(withoutCtx((*Gateway).loadSeen))
	local(withoutCtx((*Gateway).saveSeen))
	local(func(g *Gateway, _ context.Context, _ model.LoadViews) model.Msg { return g.readViews() })
	local(withoutCtx((*Gateway).writeView))
	local(withoutCtx((*Gateway).removeView))
	local(func(_ *Gateway, _ context.Context, _ model.LoadWorkerPickers) model.Msg { return loadWorkerPickers() })
	local((*Gateway).readWorkflowText)
	local((*Gateway).loadWorker)
	local(withoutCtx((*Gateway).loadWorkerSetup))
	local(withoutCtx((*Gateway).saveWorkerSetup))
	local(withoutCtx((*Gateway).saveWorkerEdit))
	local((*Gateway).stopWorker)
	local(withoutCtx((*Gateway).saveAgentConfig))
	local((*Gateway).setDocbankSupervision)
	local((*Gateway).setWorkerSupervision)
	local((*Gateway).loadLocalRun)
	local((*Gateway).runJobNow)
	local((*Gateway).resumeJob)
	local(func(g *Gateway, ctx context.Context, cmd model.ResumeLocalRun) model.Msg {
		return g.moveRun(ctx, "resume", cmd.UID)
	})
	local(func(g *Gateway, ctx context.Context, cmd model.AbandonLocalRun) model.Msg {
		return g.moveRun(ctx, "abandon", cmd.UID)
	})
	local((*Gateway).openRunSession)

	local((*Gateway).startAgent)
	local((*Gateway).startTeam)
	local((*Gateway).driveTeams)
	local(withoutCtx((*Gateway).setTeamPaused))
	local((*Gateway).refreshAgents)
	local((*Gateway).readAgentSessions)
	local((*Gateway).waitAgentSessions)
	local((*Gateway).openAgent)
	local(withoutCtx((*Gateway).setAgentOpenFlag))
	local((*Gateway).sendAgentBack)
	daemonCall(
		func(cmd model.HandOffAgent) string { return cmd.Workspace },
		func(c *Client, ctx context.Context, cmd model.HandOffAgent) (model.AgentHandedOff, error) {
			return model.AgentHandedOff{UID: cmd.UID}, c.HandOffAgent(ctx, cmd)
		},
		func(cmd model.HandOffAgent, err error) model.Msg { return model.AgentHandedOff{UID: cmd.UID, Err: err} })
	local((*Gateway).closeAgent)
	local((*Gateway).stopAgent)
	local((*Gateway).interruptAgent)
	local((*Gateway).focusAgent)
	local((*Gateway).pruneAgents)
	local(withoutCtx((*Gateway).saveAgentStates))

	// Daemon calls.
	daemonCall(
		func(cmd model.CreateIssueCmd) string { return cmd.Workspace },
		(*Client).CreateIssue,
		func(cmd model.CreateIssueCmd, err error) model.Msg { return model.IssueCreated{Err: err} })

	daemonCall(
		func(cmd model.OpenIssue) string { return cmd.Workspace },
		(*Client).OpenIssue,
		func(cmd model.OpenIssue, err error) model.Msg { return model.IssueOpenFailed{Ref: cmd.Ref, Err: err} })

	// shortcut: any failure counts as no Issue until the query changes (retrying would loop while the daemon is down); the full-text query that follows reports it.
	daemonCall(
		func(cmd model.LookupSearchID) string { return cmd.Workspace },
		func(c *Client, ctx context.Context, cmd model.LookupSearchID) (model.SearchIDLooked, error) {
			uid, err := c.ResolveIssue(ctx, cmd.ProjectID, cmd.ShortID)
			return model.SearchIDLooked{Request: cmd.Request, ShortID: cmd.ShortID, UID: uid}, err
		},
		func(cmd model.LookupSearchID, _ error) model.Msg {
			return model.SearchIDLooked{Request: cmd.Request, ShortID: cmd.ShortID}
		})

	daemonCall(
		func(cmd model.SaveDraft) string { return cmd.Workspace },
		(*Client).SaveDraft,
		func(cmd model.SaveDraft, err error) model.Msg { return model.DraftFailed{UID: cmd.UID, Err: err} })

	daemonCall(
		func(cmd model.CloseIssueCmd) string { return cmd.Workspace },
		func(c *Client, ctx context.Context, cmd model.CloseIssueCmd) (model.CloseResultMsg, error) {
			rec, err := c.CloseRecorded(ctx, cmd)
			return recorded(err, rec.Unknown, model.CloseResultMsg{UID: cmd.UID, Err: err, Record: rec})
		},
		func(cmd model.CloseIssueCmd, err error) model.Msg {
			return model.CloseResultMsg{UID: cmd.UID, Err: err}
		})

	daemonCall(
		func(cmd model.MoveIssueCmd) string { return cmd.Workspace },
		func(c *Client, ctx context.Context, cmd model.MoveIssueCmd) (model.MoveResultMsg, error) {
			rec, err := c.MoveRecorded(ctx, cmd)
			return recorded(err, rec.Unknown || rec.Entry != nil, model.MoveResultMsg{UID: cmd.UID, Err: err, Record: rec})
		},
		func(cmd model.MoveIssueCmd, err error) model.Msg { return model.MoveResultMsg{UID: cmd.UID, Err: err} })

	daemonCall(
		func(cmd model.MoveToProjectCmd) string { return cmd.Workspace },
		(*Client).MoveToProject,
		func(cmd model.MoveToProjectCmd, err error) model.Msg {
			return model.ProjectMoveAnswer{UID: cmd.UID, ToProjectUID: cmd.ToProjectUID, DryRun: cmd.DryRun, Err: err}
		})

	daemonCall(
		func(cmd model.CreateRecurrenceCmd) string { return cmd.Workspace },
		func(c *Client, ctx context.Context, cmd model.CreateRecurrenceCmd) (model.RecurrenceResultMsg, error) {
			return model.RecurrenceResultMsg{UID: cmd.UID}, c.CreateRecurrence(ctx, cmd)
		},
		func(cmd model.CreateRecurrenceCmd, err error) model.Msg {
			return model.RecurrenceResultMsg{UID: cmd.UID, Err: err}
		})

	daemonCall(
		func(cmd model.ChangeRecurrenceCmd) string { return cmd.Workspace },
		func(c *Client, ctx context.Context, cmd model.ChangeRecurrenceCmd) (model.RecurrenceResultMsg, error) {
			changed, err := c.ChangeRecurrence(ctx, cmd)
			return model.RecurrenceResultMsg{UID: cmd.UID, Changed: changed}, err
		},
		func(cmd model.ChangeRecurrenceCmd, err error) model.Msg {
			return model.RecurrenceResultMsg{UID: cmd.UID, Err: err}
		})

	daemonCall(
		func(cmd model.StopRecurrenceCmd) string { return cmd.Workspace },
		func(c *Client, ctx context.Context, cmd model.StopRecurrenceCmd) (model.RecurrenceResultMsg, error) {
			changed, err := c.StopRecurrence(ctx, cmd)
			return model.RecurrenceResultMsg{UID: cmd.UID, Changed: changed}, err
		},
		func(cmd model.StopRecurrenceCmd, err error) model.Msg {
			return model.RecurrenceResultMsg{UID: cmd.UID, Err: err}
		})

	daemonCall(
		func(cmd model.LoadRecurTarget) string { return cmd.Workspace },
		(*Client).ReadRecurTarget,
		func(cmd model.LoadRecurTarget, err error) model.Msg { return model.RecurTarget{Ref: cmd.Ref, Err: err} })

	daemonCall(
		func(cmd model.ReopenIssueCmd) string { return cmd.Workspace },
		func(c *Client, ctx context.Context, cmd model.ReopenIssueCmd) (model.CloseResultMsg, error) {
			rec, err := c.ReopenRecorded(ctx, cmd)
			return recorded(err, rec.Unknown, model.CloseResultMsg{UID: cmd.UID, Err: err, Record: rec})
		},
		func(cmd model.ReopenIssueCmd, err error) model.Msg {
			return model.CloseResultMsg{UID: cmd.UID, Err: err}
		})

	daemonCall(
		func(cmd model.WriteField) string { return cmd.Workspace },
		func(c *Client, ctx context.Context, cmd model.WriteField) (model.FieldWritten, error) {
			rec, err := c.WriteFieldRecorded(ctx, cmd)
			return recorded(err, rec.Unknown, model.FieldWritten{UID: cmd.UID, Err: err, Record: rec})
		},
		func(cmd model.WriteField, err error) model.Msg { return model.FieldWritten{UID: cmd.UID, Err: err} })

	// An undo's answer carries its own failure: the Issue changed or it did not, and a retry could write twice.
	daemonCall(
		func(cmd model.UndoChange) string { return cmd.Workspace },
		func(c *Client, ctx context.Context, cmd model.UndoChange) (model.Msg, error) {
			return c.Undo(ctx, cmd), nil
		},
		func(cmd model.UndoChange, err error) model.Msg {
			return model.ChangeUndone{EntryID: cmd.Entry.ID, Result: model.UndoFailed, Err: err}
		})

	// A write has landed: the result and its error travel together, and nothing is retried.
	daemonCall(
		func(cmd model.SetDatesCmd) string { return cmd.Workspace },
		func(c *Client, ctx context.Context, cmd model.SetDatesCmd) (model.DatesResultMsg, error) {
			applied, rec, err := c.SetDatesRecorded(ctx, cmd)
			if len(applied) > 0 || rec.Unknown {
				return model.DatesResultMsg{UID: cmd.UID, Applied: applied, Err: err, Record: rec}, nil
			}
			return model.DatesResultMsg{UID: cmd.UID}, err
		},
		func(cmd model.SetDatesCmd, err error) model.Msg { return model.DatesResultMsg{UID: cmd.UID, Err: err} })

	daemonCall(
		func(cmd model.DeleteIssueCmd) string { return cmd.Workspace },
		func(c *Client, ctx context.Context, cmd model.DeleteIssueCmd) (model.IssueDeleted, error) {
			rec, err := c.DeleteRecorded(ctx, cmd)
			return recorded(err, rec.Unknown, model.IssueDeleted{UID: cmd.UID, Err: err, Record: rec})
		},
		func(cmd model.DeleteIssueCmd, err error) model.Msg { return model.IssueDeleted{UID: cmd.UID, Err: err} })

	daemonCall(
		func(cmd model.LookupDeleted) string { return cmd.Workspace },
		(*Client).LookupDeleted,
		func(cmd model.LookupDeleted, err error) model.Msg {
			return model.DeletedLooked{Ref: cmd.Typed, Err: err}
		})

	daemonCall(
		func(cmd model.RestoreIssueCmd) string { return cmd.Workspace },
		(*Client).RestoreIssue,
		func(cmd model.RestoreIssueCmd, err error) model.Msg {
			return model.IssueRestored{UID: cmd.UID, Err: err}
		})

	daemonCall(
		func(cmd model.AttachFile) string { return cmd.Workspace },
		(*Client).AttachFile,
		func(cmd model.AttachFile, err error) model.Msg { return model.FileAttached{UID: cmd.UID, Err: err} })

	daemonCall(
		func(cmd model.UnlinkAttachment) string { return cmd.Workspace },
		func(c *Client, ctx context.Context, cmd model.UnlinkAttachment) (model.AttachmentUnlinked, error) {
			return model.AttachmentUnlinked{UID: cmd.UID, Name: cmd.Name}, c.UnlinkAttachment(ctx, cmd)
		},
		func(cmd model.UnlinkAttachment, err error) model.Msg {
			return model.AttachmentUnlinked{UID: cmd.UID, Name: cmd.Name, Err: err}
		})

	local((*Gateway).openAttachment)

	local(func(g *Gateway, ctx context.Context, cmd model.MigrateAttachments) model.Msg {
		return answer(g, ctx, cmd.Workspace, func(err error) model.Msg { return model.AttachmentsMigrated{Err: err} },
			func(ctx context.Context, c *Client) (model.Msg, error) {
				return c.MigrateAttachments(ctx, g.host, cmd), nil
			})
	})

	daemonCall(
		func(cmd model.LoadFieldLabels) string { return cmd.Workspace },
		(*Client).FieldLabels,
		func(cmd model.LoadFieldLabels, err error) model.Msg {
			return model.FieldLabelsLoaded{UID: cmd.UID, Err: err}
		})

	daemonCall(
		func(cmd model.CreateProject) string { return cmd.Workspace },
		func(c *Client, ctx context.Context, cmd model.CreateProject) (model.ProjectSaved, error) {
			project, err := c.CreateProject(ctx, cmd)
			return model.ProjectSaved{Project: project}, err
		},
		func(cmd model.CreateProject, err error) model.Msg { return model.ProjectSaved{Err: err} })

	daemonCall(
		func(cmd model.BindProject) string { return cmd.Workspace },
		func(c *Client, ctx context.Context, cmd model.BindProject) (model.ProjectSaved, error) {
			project, err := c.BindProject(ctx, cmd)
			return model.ProjectSaved{Project: project}, err
		},
		func(cmd model.BindProject, err error) model.Msg { return model.ProjectSaved{Err: err} })

	daemonCall(
		func(cmd model.LoadProject) string { return cmd.Workspace },
		func(c *Client, ctx context.Context, cmd model.LoadProject) (model.ProjectLoaded, error) {
			loaded, err := c.LoadProject(ctx, cmd.Workspace, cmd.Switched)
			loaded.Connection = cmd.Connection
			return loaded, err
		},
		func(cmd model.LoadProject, err error) model.Msg { return model.Failed{Err: err} },
		readDefaultFilterAfter)

	local((*Gateway).loadDaemons)
	local((*Gateway).switchDaemon)
	local((*Gateway).loadConfigFiles)

	daemonCall(
		func(cmd model.LoadDetail) string { return cmd.Workspace },
		func(c *Client, ctx context.Context, cmd model.LoadDetail) (model.DetailLoaded, error) {
			return c.ReadDetail(ctx, cmd.UID)
		},
		func(cmd model.LoadDetail, err error) model.Msg { return model.DetailFailed{UID: cmd.UID, Err: err} })

	daemonCall(
		func(cmd model.LoadJobs) string { return cmd.Workspace },
		(*Client).ListJobs,
		func(cmd model.LoadJobs, err error) model.Msg { return model.JobsLoaded{Request: cmd.Request, Err: err} })

	daemonCall(
		func(cmd model.LoadRuns) string { return cmd.Workspace },
		(*Client).ListRuns,
		func(cmd model.LoadRuns, err error) model.Msg {
			return model.RunsLoaded{Request: cmd.Request, Before: cmd.Before, Err: err}
		})

	daemonCall(
		func(cmd model.LoadAuthCapabilities) string { return cmd.Workspace },
		(*Client).ReadAuthCapabilities,
		func(cmd model.LoadAuthCapabilities, err error) model.Msg {
			return model.AuthCapabilitiesLoaded{Request: cmd.Request, Account: cmd.Account, Err: err}
		})

	// A LoadCredentials waits out its refresh pause first. The wait checks the wall clock at least every wakePoll, so a
	// machine that slept through the pause refreshes soon after resuming.
	loadCredentials := daemonHandler(
		func(cmd model.LoadCredentials) string { return cmd.Workspace },
		(*Client).ListCredentials,
		func(cmd model.LoadCredentials, err error) model.Msg {
			return model.CredentialsLoaded{Request: cmd.Request, Err: err}
		})
	local(func(g *Gateway, ctx context.Context, cmd model.LoadCredentials) model.Msg {
		if cmd.After > 0 {
			g.pause(ctx, cmd.After)
		}
		return loadCredentials(g, ctx, cmd)
	})

	daemonCall(
		func(cmd model.CreateTokenCmd) string { return cmd.Workspace },
		func(c *Client, ctx context.Context, cmd model.CreateTokenCmd) (model.TokenCreated, error) {
			done, err := c.CreateToken(ctx, cmd)
			done.Connection = cmd.Connection
			return done, err
		},
		func(cmd model.CreateTokenCmd, err error) model.Msg {
			return model.TokenCreated{Connection: cmd.Connection, Err: err}
		})

	// A revoke's answer carries its own failure: the token was revoked or it was not, and nothing is retried.
	daemonCall(
		func(cmd model.RevokeTokenCmd) string { return cmd.Workspace },
		func(c *Client, ctx context.Context, cmd model.RevokeTokenCmd) (model.TokenRevoked, error) {
			_, err := c.RevokeToken(ctx, cmd.ID)
			return model.TokenRevoked{ID: cmd.ID, Err: err}, nil
		},
		func(cmd model.RevokeTokenCmd, err error) model.Msg { return model.TokenRevoked{ID: cmd.ID, Err: err} })

	daemonCall(
		func(cmd model.LoadFederation) string { return cmd.Workspace },
		(*Client).ReadFederation,
		func(cmd model.LoadFederation, err error) model.Msg {
			return model.FederationLoaded{Request: cmd.Request, Err: err}
		})

	daemonCall(
		func(cmd model.LeaveFederation) string { return cmd.Workspace },
		(*Client).LeaveFederation,
		func(cmd model.LeaveFederation, err error) model.Msg {
			return model.FederationLeft{Attempt: cmd.Attempt, Err: err}
		})

	daemonCall(
		func(cmd model.LoadFederationHubs) string { return cmd.Workspace },
		(*Client).ListFederationHubs,
		func(cmd model.LoadFederationHubs, err error) model.Msg {
			return model.FederationHubsLoaded{Request: cmd.Request, Err: err}
		})

	daemonCall(
		func(cmd model.LoadFederationHubProjects) string { return cmd.Workspace },
		(*Client).ReadFederationHubProjects,
		func(cmd model.LoadFederationHubProjects, err error) model.Msg {
			return model.FederationHubProjectsLoaded{Request: cmd.Request, Err: err}
		})

	daemonCall(
		func(cmd model.BrowseFederationHubs) string { return cmd.Workspace },
		(*Client).BrowseFederationHubs,
		func(cmd model.BrowseFederationHubs, err error) model.Msg {
			return model.FederationBrowsed{Request: cmd.Request, Err: err}
		})

	daemonCall(
		func(cmd model.EnrollFederation) string { return cmd.Workspace },
		(*Client).EnrollFederation,
		func(cmd model.EnrollFederation, err error) model.Msg {
			return model.FederationEnrolled{Attempt: cmd.Attempt, Err: err}
		})

	daemonCall(
		func(cmd model.SetFederationSpokeName) string { return cmd.Workspace },
		(*Client).SetFederationSpokeName,
		func(cmd model.SetFederationSpokeName, err error) model.Msg {
			return model.FederationSpokeNamed{Attempt: cmd.Attempt, Hub: cmd.HubURL, Err: err}
		})

	daemonCall(
		func(cmd model.ShowRun) string { return cmd.Workspace },
		(*Client).ShowRun,
		func(cmd model.ShowRun, err error) model.Msg { return model.RunShown{UID: cmd.UID, Err: err} })

	daemonCall(
		func(cmd model.SaveJobCmd) string { return cmd.Workspace },
		(*Client).SaveJob,
		func(cmd model.SaveJobCmd, err error) model.Msg {
			return model.JobWritten{ProjectID: cmd.ProjectID, UID: cmd.UID, Err: err}
		})

	daemonCall(
		func(cmd model.ArchiveJobCmd) string { return cmd.Workspace },
		(*Client).ArchiveJob,
		func(cmd model.ArchiveJobCmd, err error) model.Msg {
			return model.JobWritten{ProjectID: cmd.ProjectID, UID: cmd.UID, Err: err}
		})

	daemonCall(
		func(cmd model.RestoreJobCmd) string { return cmd.Workspace },
		(*Client).RestoreJob,
		func(cmd model.RestoreJobCmd, err error) model.Msg {
			return model.JobWritten{ProjectID: cmd.ProjectID, UID: cmd.UID, Err: err}
		})

	daemonCall(
		func(cmd model.LoadWorkflows) string { return cmd.Workspace },
		func(c *Client, ctx context.Context, cmd model.LoadWorkflows) (model.WorkflowsLoaded, error) {
			workflows, err := c.ListWorkflows(ctx, cmd.ProjectID)
			return model.WorkflowsLoaded{ProjectUID: cmd.ProjectUID, Workflows: workflows}, err
		},
		func(cmd model.LoadWorkflows, err error) model.Msg {
			return model.WorkflowsLoaded{ProjectUID: cmd.ProjectUID, Err: err}
		},
		readWorkflowsAfter)

	daemonCall(
		func(cmd model.LoadJobWorkflows) string { return cmd.Workspace },
		func(c *Client, ctx context.Context, cmd model.LoadJobWorkflows) (model.JobWorkflowsLoaded, error) {
			workflows, err := c.ListWorkflows(ctx, cmd.ProjectID)
			return model.JobWorkflowsLoaded{ProjectID: cmd.ProjectID, Workflows: workflows}, err
		},
		func(cmd model.LoadJobWorkflows, err error) model.Msg {
			return model.JobWorkflowsLoaded{ProjectID: cmd.ProjectID, Err: err}
		})

	daemonCall(
		func(cmd model.LoadAttention) string { return cmd.Workspace },
		(*Client).ListAttention,
		func(cmd model.LoadAttention, err error) model.Msg {
			return model.AttentionLoaded{Request: cmd.Request, Err: err}
		})

	// A write has landed: the result and its error travel together, and nothing is retried.
	daemonCall(
		func(cmd model.ClearAttention) string { return cmd.Workspace },
		func(c *Client, ctx context.Context, cmd model.ClearAttention) (model.AttentionCleared, error) {
			cleared, err := c.ClearAttention(ctx, cmd)
			cleared.Connection = cmd.Connection
			return cleared, err
		},
		func(cmd model.ClearAttention, err error) model.Msg {
			return model.AttentionCleared{Connection: cmd.Connection, UID: cmd.UID, Err: err}
		})

	daemonCall(
		func(cmd model.RaiseAgentAttention) string { return cmd.Workspace },
		func(c *Client, ctx context.Context, cmd model.RaiseAgentAttention) (model.AgentAttentionWritten, error) {
			return model.AgentAttentionWritten{UID: cmd.UID}, c.RaiseAgentAttention(ctx, cmd)
		},
		func(cmd model.RaiseAgentAttention, err error) model.Msg {
			return model.AgentAttentionWritten{UID: cmd.UID, Err: err}
		})

	daemonCall(
		func(cmd model.ReadAgentIssue) string { return cmd.Workspace },
		(*Client).ReadAgentIssue,
		func(cmd model.ReadAgentIssue, err error) model.Msg {
			return model.AgentIssueRead{UID: cmd.UID, Err: err}
		})

	daemonCall(
		func(cmd model.ReadSweepComments) string { return cmd.Workspace },
		(*Client).ReadSweepComments,
		func(cmd model.ReadSweepComments, err error) model.Msg {
			return model.SweepCommentsRead{UID: cmd.UID, Address: cmd.Address, Err: err}
		})

	daemonCall(
		func(cmd model.ClearAgentAttention) string { return cmd.Workspace },
		func(c *Client, ctx context.Context, cmd model.ClearAgentAttention) (model.AgentAttentionWritten, error) {
			return model.AgentAttentionWritten{UID: cmd.UID, Clear: true}, c.ClearAgentAttention(ctx, cmd)
		},
		func(cmd model.ClearAgentAttention, err error) model.Msg {
			return model.AgentAttentionWritten{UID: cmd.UID, Clear: true, Err: err}
		})

	daemonCall(
		func(cmd model.SendAgentMessage) string { return cmd.Workspace },
		(*Client).SendAgentMessage,
		func(cmd model.SendAgentMessage, err error) model.Msg {
			return model.AgentMessageSent{UID: cmd.UID, Address: cmd.Address, Err: err}
		})

	// An agent on another host is told by its record, so the Host id is read where the state folder is.
	hostCall(
		func(cmd model.LoadTeamKids) string { return cmd.Workspace },
		(*Client).ReadTeamKids,
		func(_ model.LoadTeamKids, err error) model.Msg { return model.TeamKidsLoaded{Err: err} })

	daemonCall(
		func(cmd model.ResolveComment) string { return cmd.Workspace },
		(*Client).ResolveComment,
		func(cmd model.ResolveComment, err error) model.Msg { return model.CommentResolved{Err: err} })

	// Records of this host's own id are left out, so the Host id is read where the state folder is.
	hostCall(
		func(cmd model.LoadRemoteAgents) string { return cmd.Workspace },
		(*Client).ReadRemoteAgents,
		func(_ model.LoadRemoteAgents, err error) model.Msg { return model.RemoteAgentsLoaded{Err: err} })

	// An exited agent's record goes beside its address: nobody reads either again.
	local(func(g *Gateway, ctx context.Context, cmd model.ClearAgentAddress) model.Msg {
		cleared := model.AgentAddressCleared{UID: cmd.UID, Block: cmd.Block}
		return answer(g, ctx, cmd.Workspace, func(err error) model.Msg { cleared.Err = err; return cleared },
			func(ctx context.Context, c *Client) (model.AgentAddressCleared, error) {
				var clearErr error
				if cmd.Address != "" {
					clearErr = c.ClearAgentAddress(ctx, cmd)
				}
				return cleared, errors.Join(clearErr, g.removeAgentRecordOf(ctx, c, cmd.ProjectID, cmd.UID, cmd.Block))
			})
	})

	daemonCall(
		func(cmd model.ForgetAgentRecord) string { return cmd.Workspace },
		func(c *Client, ctx context.Context, cmd model.ForgetAgentRecord) (model.AgentRecordForgotten, error) {
			return model.AgentRecordForgotten{Host: cmd.Host}, c.ForgetAgentRecord(ctx, cmd)
		},
		func(cmd model.ForgetAgentRecord, err error) model.Msg {
			return model.AgentRecordForgotten{Host: cmd.Host, Err: err}
		})

	daemonCall(
		func(cmd model.SaveWorkflow) string { return cmd.Workspace },
		(*Client).SaveWorkflow,
		func(cmd model.SaveWorkflow, err error) model.Msg {
			return model.WorkflowWritten{ProjectUID: cmd.ProjectUID, Op: model.WorkflowOpSave, UID: cmd.UID, Name: cmd.Name, Err: err}
		},
		readWrittenWorkflowAfter)

	daemonCall(
		func(cmd model.ArchiveWorkflow) string { return cmd.Workspace },
		(*Client).ArchiveWorkflow,
		func(cmd model.ArchiveWorkflow, err error) model.Msg {
			return model.WorkflowWritten{ProjectUID: cmd.ProjectUID, Op: model.WorkflowOpArchive, UID: cmd.UID, Name: cmd.Name, Err: err}
		},
		readWrittenWorkflowAfter)

	daemonCall(
		func(cmd model.RestoreWorkflow) string { return cmd.Workspace },
		(*Client).RestoreWorkflow,
		func(cmd model.RestoreWorkflow, err error) model.Msg {
			return model.WorkflowWritten{ProjectUID: cmd.ProjectUID, Op: model.WorkflowOpRestore, UID: cmd.UID, Name: cmd.Name, Err: err}
		},
		readWrittenWorkflowAfter)

	daemonCall(
		func(cmd model.ReadSnapshot) string { return cmd.Workspace },
		(*Client).ReadSnapshot,
		func(cmd model.ReadSnapshot, err error) model.Msg {
			return model.SnapshotFailed{Request: cmd.Request, Err: err}
		})
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
