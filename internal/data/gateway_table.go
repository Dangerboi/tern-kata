package data

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// The Daemon gateway's command table declares, per command type, how the gateway handles it: the handler and the named
// steps that run after it. Run looks the declaration up and runs it; it never switches on a command's type. A command with
// no declaration fails with a message naming its type.

type kind int

const (
	answersOnce kind = iota
	longLived
)

type afterStep struct {
	name string
	run  func(g *Gateway, ctx context.Context, cmd model.Cmd, result model.Msg) model.Msg
}

type declaration struct {
	kind   kind
	after  []afterStep
	handle func(g *Gateway, ctx context.Context, cmd model.Cmd) model.Msg
	stream func(g *Gateway, ctx context.Context, cmd model.Cmd, messages chan<- model.Msg)
}

var (
	readDefaultFilterAfter = afterStep{"read the Default filter", func(g *Gateway, _ context.Context, _ model.Cmd, result model.Msg) model.Msg {
		if loaded, ok := result.(model.ProjectLoaded); ok && loaded.Project.UID != "" {
			loaded.Default, loaded.DefaultErr = g.loadFilters(loaded.Project.UID)
			return loaded
		}
		return result
	}}
)

var commands = map[reflect.Type]declaration{}

// local declares a command the gateway answers without the daemon, or with a handler of its own. Method expressions
// of Gateway fit it as they are.
func local[C model.Cmd, R model.Msg](handle func(g *Gateway, ctx context.Context, cmd C) R, after ...afterStep) {
	commands[reflect.TypeFor[C]()] = declaration{kind: answersOnce, after: after,
		handle: func(g *Gateway, ctx context.Context, cmd model.Cmd) model.Msg { return handle(g, ctx, cmd.(C)) }}
}

func withoutCtx[C model.Cmd, R model.Msg](handle func(*Gateway, C) R) func(*Gateway, context.Context, C) R {
	return func(g *Gateway, _ context.Context, cmd C) R { return handle(g, cmd) }
}

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
	return answerWithin(g, ctx, workspace, g.callBudget, fail, fn)
}

// answerWithin is answer with its own budget for the call; zero leaves it to ctx alone.
func answerWithin[R model.Msg](g *Gateway, ctx context.Context, workspace string, budget time.Duration, fail func(error) model.Msg, fn func(context.Context, *Client) (R, error)) model.Msg {
	result, err := callAsWithin(g, ctx, workspace, budget, fn)
	if err != nil {
		return fail(err)
	}
	return result
}

// workspaceOf reads the Workspace field every daemon command carries: the folder its daemon is found from. A command
// type without a string Workspace field fails when it is declared, not when it first runs.
func workspaceOf[C model.Cmd]() func(C) string {
	field, ok := reflect.TypeFor[C]().FieldByName("Workspace")
	if !ok || field.Type.Kind() != reflect.String {
		panic(fmt.Sprintf("data: %T has no string Workspace field", *new(C)))
	}
	return func(cmd C) string { return reflect.ValueOf(cmd).FieldByIndex(field.Index).String() }
}

// daemonHandler is the handler of a command that calls the daemon: the call against the client, and the failure its
// error becomes. The call and the failure both read the command, so a result and its failure share a shape.
func daemonHandler[C model.Cmd, R model.Msg](
	call func(c *Client, ctx context.Context, cmd C) (R, error),
	fail func(cmd C, err error) model.Msg,
) func(*Gateway, context.Context, C) model.Msg {
	workspace := workspaceOf[C]()
	return func(g *Gateway, ctx context.Context, cmd C) model.Msg {
		return answer(g, ctx, workspace(cmd), func(err error) model.Msg { return fail(cmd, err) },
			func(ctx context.Context, c *Client) (R, error) { return call(c, ctx, cmd) })
	}
}

func daemonCall[C model.Cmd, R model.Msg](
	call func(c *Client, ctx context.Context, cmd C) (R, error),
	fail func(cmd C, err error) model.Msg,
	after ...afterStep,
) {
	local(daemonHandler(call, fail), after...)
}

// hostCall declares a daemon call that needs this machine's Host id, read where the state folder is. A failure to read it is
// the command's failure, before any call.
func hostCall[C model.Cmd, R model.Msg](
	call func(c *Client, ctx context.Context, hostID string, cmd C) (R, error),
	fail func(cmd C, err error) model.Msg,
) {
	workspace := workspaceOf[C]()
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
		(*Client).ShareLanes,
		func(cmd model.ShareLanes, err error) model.Msg { return model.LanesShared{Share: cmd, Err: err} })
	local(withoutCtx((*Gateway).saveFilters))
	local(withoutCtx((*Gateway).loadSeen))
	local(withoutCtx((*Gateway).saveSeen))
	local(func(g *Gateway, _ context.Context, _ model.LoadViews) model.Msg { return g.readViews() })
	local(withoutCtx((*Gateway).writeView))
	local(withoutCtx((*Gateway).removeView))
	local(withoutCtx((*Gateway).loadWorkerPickers))
	local((*Gateway).readWorkflowText)
	local((*Gateway).loadWorker)
	local(withoutCtx((*Gateway).loadWorkerSetup))
	local(withoutCtx((*Gateway).saveWorkerSetup))
	local(withoutCtx((*Gateway).saveWorkerEdit))
	local((*Gateway).stopWorker)
	local(withoutCtx((*Gateway).saveAgentConfig))
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

	daemonCall(
		(*Client).CreateIssue,
		func(cmd model.CreateIssueCmd, err error) model.Msg { return model.IssueCreated{Err: err} })

	daemonCall(
		(*Client).OpenIssue,
		func(cmd model.OpenIssue, err error) model.Msg { return model.IssueOpenFailed{Ref: cmd.Ref, Err: err} })

	// shortcut: any failure counts as no Issue until the query changes (retrying would loop while the daemon is down); the full-text query that follows reports it.
	daemonCall(
		func(c *Client, ctx context.Context, cmd model.LookupSearchID) (model.SearchIDLooked, error) {
			uid, err := c.ResolveIssue(ctx, cmd.ProjectID, cmd.ShortID)
			return model.SearchIDLooked{Request: cmd.Request, ShortID: cmd.ShortID, UID: uid}, err
		},
		func(cmd model.LookupSearchID, _ error) model.Msg {
			return model.SearchIDLooked{Request: cmd.Request, ShortID: cmd.ShortID}
		})

	daemonCall(
		(*Client).SearchIssues,
		func(cmd model.SearchByMeaning, err error) model.Msg {
			return model.SearchByMeaningDone{Request: cmd.Request, RankedAnswer: model.RankedAnswer{Query: cmd.Query}, Err: err}
		})

	daemonCall(
		(*Client).SaveDraft,
		func(cmd model.SaveDraft, err error) model.Msg { return model.DraftFailed{UID: cmd.UID, Err: err} })

	daemonCall(
		func(c *Client, ctx context.Context, cmd model.CloseIssueCmd) (model.CloseResultMsg, error) {
			rec, err := c.CloseRecorded(ctx, cmd)
			return recorded(err, rec.Unknown, model.CloseResultMsg{UID: cmd.UID, Err: err, Record: rec})
		},
		func(cmd model.CloseIssueCmd, err error) model.Msg {
			return model.CloseResultMsg{UID: cmd.UID, Err: err}
		})

	daemonCall(
		func(c *Client, ctx context.Context, cmd model.MoveIssueCmd) (model.MoveResultMsg, error) {
			rec, err := c.MoveRecorded(ctx, cmd)
			return recorded(err, rec.Unknown || rec.Entry != nil, model.MoveResultMsg{UID: cmd.UID, Err: err, Record: rec})
		},
		func(cmd model.MoveIssueCmd, err error) model.Msg { return model.MoveResultMsg{UID: cmd.UID, Err: err} })

	daemonCall(
		(*Client).MoveToProject,
		func(cmd model.MoveToProjectCmd, err error) model.Msg {
			return model.ProjectMoveAnswer{UID: cmd.UID, ToProjectUID: cmd.ToProjectUID, DryRun: cmd.DryRun, Err: err}
		})

	daemonCall(
		func(c *Client, ctx context.Context, cmd model.CreateRecurrenceCmd) (model.RecurrenceResultMsg, error) {
			return model.RecurrenceResultMsg{UID: cmd.UID}, c.CreateRecurrence(ctx, cmd)
		},
		func(cmd model.CreateRecurrenceCmd, err error) model.Msg {
			return model.RecurrenceResultMsg{UID: cmd.UID, Err: err}
		})

	daemonCall(
		func(c *Client, ctx context.Context, cmd model.ChangeRecurrenceCmd) (model.RecurrenceResultMsg, error) {
			changed, err := c.ChangeRecurrence(ctx, cmd)
			return model.RecurrenceResultMsg{UID: cmd.UID, Changed: changed}, err
		},
		func(cmd model.ChangeRecurrenceCmd, err error) model.Msg {
			return model.RecurrenceResultMsg{UID: cmd.UID, Err: err}
		})

	daemonCall(
		func(c *Client, ctx context.Context, cmd model.StopRecurrenceCmd) (model.RecurrenceResultMsg, error) {
			changed, err := c.StopRecurrence(ctx, cmd)
			return model.RecurrenceResultMsg{UID: cmd.UID, Changed: changed}, err
		},
		func(cmd model.StopRecurrenceCmd, err error) model.Msg {
			return model.RecurrenceResultMsg{UID: cmd.UID, Err: err}
		})

	daemonCall(
		(*Client).ReadRecurTarget,
		func(cmd model.LoadRecurTarget, err error) model.Msg { return model.RecurTarget{Ref: cmd.Ref, Err: err} })

	daemonCall(
		func(c *Client, ctx context.Context, cmd model.ReopenIssueCmd) (model.CloseResultMsg, error) {
			rec, err := c.ReopenRecorded(ctx, cmd)
			return recorded(err, rec.Unknown, model.CloseResultMsg{UID: cmd.UID, Err: err, Record: rec})
		},
		func(cmd model.ReopenIssueCmd, err error) model.Msg {
			return model.CloseResultMsg{UID: cmd.UID, Err: err}
		})

	daemonCall(
		func(c *Client, ctx context.Context, cmd model.WriteField) (model.FieldWritten, error) {
			rec, err := c.WriteFieldRecorded(ctx, cmd)
			return recorded(err, rec.Unknown, model.FieldWritten{UID: cmd.UID, Err: err, Record: rec})
		},
		func(cmd model.WriteField, err error) model.Msg { return model.FieldWritten{UID: cmd.UID, Err: err} })

	// An undo's answer carries its own failure: the Issue changed or it did not, and a retry could write twice.
	daemonCall(
		func(c *Client, ctx context.Context, cmd model.UndoChange) (model.Msg, error) {
			return c.Undo(ctx, cmd), nil
		},
		func(cmd model.UndoChange, err error) model.Msg {
			return model.ChangeUndone{EntryID: cmd.Entry.ID, Result: model.UndoFailed, Err: err}
		})

	// A write has landed: the result and its error travel together, and nothing is retried.
	daemonCall(
		func(c *Client, ctx context.Context, cmd model.SetDatesCmd) (model.DatesResultMsg, error) {
			applied, rec, err := c.SetDatesRecorded(ctx, cmd)
			if len(applied) > 0 || rec.Unknown {
				return model.DatesResultMsg{UID: cmd.UID, Applied: applied, Err: err, Record: rec}, nil
			}
			return model.DatesResultMsg{UID: cmd.UID}, err
		},
		func(cmd model.SetDatesCmd, err error) model.Msg { return model.DatesResultMsg{UID: cmd.UID, Err: err} })

	daemonCall(
		func(c *Client, ctx context.Context, cmd model.DeleteIssueCmd) (model.IssueDeleted, error) {
			rec, err := c.DeleteRecorded(ctx, cmd)
			return recorded(err, rec.Unknown, model.IssueDeleted{UID: cmd.UID, Err: err, Record: rec})
		},
		func(cmd model.DeleteIssueCmd, err error) model.Msg { return model.IssueDeleted{UID: cmd.UID, Err: err} })

	daemonCall(
		(*Client).LookupDeleted,
		func(cmd model.LookupDeleted, err error) model.Msg {
			return model.DeletedLooked{Ref: cmd.Typed, Err: err}
		})

	daemonCall(
		(*Client).RestoreIssue,
		func(cmd model.RestoreIssueCmd, err error) model.Msg {
			return model.IssueRestored{UID: cmd.UID, Err: err}
		})

	// An upload takes as long as the file: ctx alone ends it.
	local(func(g *Gateway, ctx context.Context, cmd model.AttachFile) model.Msg {
		return answerWithin(g, ctx, cmd.Workspace, 0, func(err error) model.Msg { return model.FileAttached{UID: cmd.UID, Err: err} },
			func(ctx context.Context, c *Client) (model.FileAttached, error) { return c.AttachFile(ctx, cmd) })
	})

	daemonCall(
		func(c *Client, ctx context.Context, cmd model.UnlinkAttachment) (model.AttachmentUnlinked, error) {
			return model.AttachmentUnlinked{UID: cmd.UID, Name: cmd.Name}, c.UnlinkAttachment(ctx, cmd)
		},
		func(cmd model.UnlinkAttachment, err error) model.Msg {
			return model.AttachmentUnlinked{UID: cmd.UID, Name: cmd.Name, Err: err}
		})

	local((*Gateway).openAttachment)

	local(func(g *Gateway, ctx context.Context, cmd model.MigrateAttachments) model.Msg {
		return answerWithin(g, ctx, cmd.Workspace, 0, func(err error) model.Msg { return model.AttachmentsMigrated{Err: err} },
			func(ctx context.Context, c *Client) (model.Msg, error) {
				return c.MigrateAttachments(ctx, g.host, cmd), nil
			})
	})

	daemonCall(
		(*Client).FieldLabels,
		func(cmd model.LoadFieldLabels, err error) model.Msg {
			return model.FieldLabelsLoaded{UID: cmd.UID, Err: err}
		})

	daemonCall(
		func(c *Client, ctx context.Context, cmd model.CreateProject) (model.ProjectSaved, error) {
			project, err := c.CreateProject(ctx, cmd)
			return model.ProjectSaved{Project: project}, err
		},
		func(cmd model.CreateProject, err error) model.Msg { return model.ProjectSaved{Err: err} })

	daemonCall(
		func(c *Client, ctx context.Context, cmd model.BindProject) (model.ProjectSaved, error) {
			project, err := c.BindProject(ctx, cmd)
			return model.ProjectSaved{Project: project}, err
		},
		func(cmd model.BindProject, err error) model.Msg { return model.ProjectSaved{Err: err} })

	daemonCall(
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
	local((*Gateway).switchDocbank)

	// A LoadEmbeddingsStatus waits out its poll pause first, as a LoadCredentials does.
	loadEmbeddings := daemonHandler(
		func(c *Client, ctx context.Context, cmd model.LoadEmbeddingsStatus) (model.EmbeddingsStatusLoaded, error) {
			loaded, err := c.EmbeddingsStatus(ctx, cmd)
			loaded.Request = cmd.Request
			return loaded, err
		},
		func(cmd model.LoadEmbeddingsStatus, err error) model.Msg {
			return model.EmbeddingsStatusLoaded{Request: cmd.Request, Err: err}
		})
	local(func(g *Gateway, ctx context.Context, cmd model.LoadEmbeddingsStatus) model.Msg {
		g.pause(ctx, cmd.After)
		return loadEmbeddings(g, ctx, cmd)
	})

	daemonCall(
		(*Client).TurnOnEmbeddings,
		func(cmd model.TurnOnEmbeddings, err error) model.Msg {
			return model.EmbeddingsTurnedOn{Err: err, Connection: cmd.Connection}
		})

	daemonCall(
		func(c *Client, ctx context.Context, cmd model.LoadDetail) (model.DetailLoaded, error) {
			return c.ReadDetail(ctx, cmd.UID)
		},
		func(cmd model.LoadDetail, err error) model.Msg { return model.DetailFailed{UID: cmd.UID, Err: err} })

	daemonCall(
		(*Client).ListJobs,
		func(cmd model.LoadJobs, err error) model.Msg { return model.JobsLoaded{Request: cmd.Request, Err: err} })

	daemonCall(
		(*Client).ListRuns,
		func(cmd model.LoadRuns, err error) model.Msg {
			return model.RunsLoaded{Request: cmd.Request, Before: cmd.Before, Err: err}
		})

	daemonCall(
		(*Client).ReadAuthCapabilities,
		func(cmd model.LoadAuthCapabilities, err error) model.Msg {
			return model.AuthCapabilitiesLoaded{Request: cmd.Request, Account: cmd.Account, Err: err}
		})

	// A LoadCredentials waits out its refresh pause first. The wait checks the wall clock at least every wakePoll, so a
	// machine that slept through the pause refreshes soon after resuming.
	loadCredentials := daemonHandler(
		(*Client).ListCredentials,
		func(cmd model.LoadCredentials, err error) model.Msg {
			return model.CredentialsLoaded{Request: cmd.Request, Err: err}
		})
	local(func(g *Gateway, ctx context.Context, cmd model.LoadCredentials) model.Msg {
		g.pause(ctx, cmd.After)
		return loadCredentials(g, ctx, cmd)
	})

	daemonCall(
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
		func(c *Client, ctx context.Context, cmd model.RevokeTokenCmd) (model.TokenRevoked, error) {
			_, err := c.RevokeToken(ctx, cmd.ID)
			return model.TokenRevoked{ID: cmd.ID, Err: err}, nil
		},
		func(cmd model.RevokeTokenCmd, err error) model.Msg { return model.TokenRevoked{ID: cmd.ID, Err: err} })

	daemonCall(
		(*Client).ReadFederation,
		func(cmd model.LoadFederation, err error) model.Msg {
			return model.FederationLoaded{Request: cmd.Request, Err: err}
		})

	daemonCall(
		(*Client).LeaveFederation,
		func(cmd model.LeaveFederation, err error) model.Msg {
			return model.FederationLeft{Attempt: cmd.Attempt, Err: err}
		})

	daemonCall(
		(*Client).ListFederationHubs,
		func(cmd model.LoadFederationHubs, err error) model.Msg {
			return model.FederationHubsLoaded{Request: cmd.Request, Err: err}
		})

	daemonCall(
		(*Client).ReadFederationHubProjects,
		func(cmd model.LoadFederationHubProjects, err error) model.Msg {
			return model.FederationHubProjectsLoaded{Request: cmd.Request, Err: err}
		})

	daemonCall(
		(*Client).BrowseFederationHubs,
		func(cmd model.BrowseFederationHubs, err error) model.Msg {
			return model.FederationBrowsed{Request: cmd.Request, Err: err}
		})

	daemonCall(
		(*Client).EnrollFederation,
		func(cmd model.EnrollFederation, err error) model.Msg {
			return model.FederationEnrolled{Attempt: cmd.Attempt, Err: err}
		})

	daemonCall(
		(*Client).SetFederationSpokeName,
		func(cmd model.SetFederationSpokeName, err error) model.Msg {
			return model.FederationSpokeNamed{Attempt: cmd.Attempt, Hub: cmd.HubURL, Err: err}
		})

	daemonCall(
		(*Client).ShowRun,
		func(cmd model.ShowRun, err error) model.Msg { return model.RunShown{UID: cmd.UID, Err: err} })

	daemonCall(
		(*Client).SaveJob,
		func(cmd model.SaveJobCmd, err error) model.Msg {
			return model.JobWritten{ProjectID: cmd.ProjectID, UID: cmd.UID, Err: err}
		})

	daemonCall(
		(*Client).ArchiveJob,
		func(cmd model.ArchiveJobCmd, err error) model.Msg {
			return model.JobWritten{ProjectID: cmd.ProjectID, UID: cmd.UID, Err: err}
		})

	daemonCall(
		(*Client).RestoreJob,
		func(cmd model.RestoreJobCmd, err error) model.Msg {
			return model.JobWritten{ProjectID: cmd.ProjectID, UID: cmd.UID, Err: err}
		})

	daemonCall(
		func(c *Client, ctx context.Context, cmd model.LoadWorkflows) (model.WorkflowsLoaded, error) {
			workflows, err := c.ListWorkflows(ctx, cmd.ProjectID)
			return model.WorkflowsLoaded{ProjectUID: cmd.ProjectUID, Workflows: workflows}, err
		},
		func(cmd model.LoadWorkflows, err error) model.Msg {
			return model.WorkflowsLoaded{ProjectUID: cmd.ProjectUID, Err: err}
		},
		readWorkflowsAfter)

	daemonCall(
		func(c *Client, ctx context.Context, cmd model.LoadJobWorkflows) (model.JobWorkflowsLoaded, error) {
			workflows, err := c.ListWorkflows(ctx, cmd.ProjectID)
			return model.JobWorkflowsLoaded{ProjectID: cmd.ProjectID, Workflows: workflows}, err
		},
		func(cmd model.LoadJobWorkflows, err error) model.Msg {
			return model.JobWorkflowsLoaded{ProjectID: cmd.ProjectID, Err: err}
		})

	daemonCall(
		(*Client).ListAttention,
		func(cmd model.LoadAttention, err error) model.Msg {
			return model.AttentionLoaded{Request: cmd.Request, Err: err}
		})

	// A write has landed: the result and its error travel together, and nothing is retried.
	daemonCall(
		func(c *Client, ctx context.Context, cmd model.ClearAttention) (model.AttentionCleared, error) {
			cleared, err := c.ClearAttention(ctx, cmd)
			cleared.Connection = cmd.Connection
			return cleared, err
		},
		func(cmd model.ClearAttention, err error) model.Msg {
			return model.AttentionCleared{Connection: cmd.Connection, UID: cmd.UID, Err: err}
		})

	daemonCall(
		func(c *Client, ctx context.Context, cmd model.RaiseAgentAttention) (model.AgentAttentionWritten, error) {
			return model.AgentAttentionWritten{UID: cmd.UID}, c.RaiseAgentAttention(ctx, cmd)
		},
		func(cmd model.RaiseAgentAttention, err error) model.Msg {
			return model.AgentAttentionWritten{UID: cmd.UID, Err: err}
		})

	daemonCall(
		(*Client).ReadAgentIssue,
		func(cmd model.ReadAgentIssue, err error) model.Msg {
			return model.AgentIssueRead{UID: cmd.UID, Err: err}
		})

	daemonCall(
		(*Client).ReadSweepComments,
		func(cmd model.ReadSweepComments, err error) model.Msg {
			return model.SweepCommentsRead{UID: cmd.UID, Address: cmd.Address, Err: err}
		})

	daemonCall(
		func(c *Client, ctx context.Context, cmd model.ClearAgentAttention) (model.AgentAttentionWritten, error) {
			return model.AgentAttentionWritten{UID: cmd.UID, Clear: true}, c.ClearAgentAttention(ctx, cmd)
		},
		func(cmd model.ClearAgentAttention, err error) model.Msg {
			return model.AgentAttentionWritten{UID: cmd.UID, Clear: true, Err: err}
		})

	daemonCall(
		(*Client).SendAgentMessage,
		func(cmd model.SendAgentMessage, err error) model.Msg {
			return model.AgentMessageSent{UID: cmd.UID, Address: cmd.Address, Err: err}
		})

	// An agent on another host is told by its record, so the Host id is read where the state folder is.
	hostCall(
		(*Client).ReadTeamKids,
		func(_ model.LoadTeamKids, err error) model.Msg { return model.TeamKidsLoaded{Err: err} })

	daemonCall(
		(*Client).ResolveComment,
		func(cmd model.ResolveComment, err error) model.Msg { return model.CommentResolved{Err: err} })

	// Records of this host's own id are left out, so the Host id is read where the state folder is.
	hostCall(
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
		func(c *Client, ctx context.Context, cmd model.ForgetAgentRecord) (model.AgentRecordForgotten, error) {
			return model.AgentRecordForgotten{Host: cmd.Host}, c.ForgetAgentRecord(ctx, cmd)
		},
		func(cmd model.ForgetAgentRecord, err error) model.Msg {
			return model.AgentRecordForgotten{Host: cmd.Host, Err: err}
		})

	daemonCall(
		(*Client).SaveWorkflow,
		func(cmd model.SaveWorkflow, err error) model.Msg {
			return model.WorkflowWritten{ProjectUID: cmd.ProjectUID, Op: model.WorkflowOpSave, UID: cmd.UID, Name: cmd.Name, Err: err}
		},
		readWrittenWorkflowAfter)

	daemonCall(
		(*Client).ArchiveWorkflow,
		func(cmd model.ArchiveWorkflow, err error) model.Msg {
			return model.WorkflowWritten{ProjectUID: cmd.ProjectUID, Op: model.WorkflowOpArchive, UID: cmd.UID, Name: cmd.Name, Err: err}
		},
		readWrittenWorkflowAfter)

	daemonCall(
		(*Client).RestoreWorkflow,
		func(cmd model.RestoreWorkflow, err error) model.Msg {
			return model.WorkflowWritten{ProjectUID: cmd.ProjectUID, Op: model.WorkflowOpRestore, UID: cmd.UID, Name: cmd.Name, Err: err}
		},
		readWrittenWorkflowAfter)

	daemonCall(
		(*Client).ReadSnapshot,
		func(cmd model.ReadSnapshot, err error) model.Msg {
			return model.SnapshotFailed{Request: cmd.Request, Err: err}
		})
}

func declarationFor(cmd model.Cmd) declaration {
	if d, ok := commands[reflect.TypeOf(cmd)]; ok {
		return d
	}
	return declaration{kind: answersOnce, handle: func(_ *Gateway, _ context.Context, cmd model.Cmd) model.Msg {
		return model.Failed{Err: fmt.Errorf("tern-kata: unknown command %T", cmd)}
	}}
}
