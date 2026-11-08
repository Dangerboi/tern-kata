package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Dangerboi/tern-kata/internal/data"
	"github.com/Dangerboi/tern-kata/internal/model"
)

// subcommandError is any failure of a subcommand, from its usage to its output: main exits 1 with the message, not the Pane's start-failure status.
type subcommandError struct{ error }

func (e subcommandError) Unwrap() error { return e.error }

// runSubcommand runs plan's migrate-attachments, recur, views, agents or message through the same gateway commands the Pane sends, printing results to out.
func runSubcommand(ctx context.Context, plan LaunchPlan, gateway *data.Gateway, out io.Writer) error {
	defer gateway.Close()
	workspace, err := os.Getwd()
	if err != nil {
		return subcommandError{err}
	}
	results := make(chan model.Msg, 1)
	send := func(cmd model.Cmd) model.Msg {
		gateway.Run(ctx, cmd, results)
		return <-results
	}
	if plan.Subcommand == "team" {
		return runTeam(plan, workspace, gateway.ReadTeamFiles, time.Now(), send, out)
	}
	if plan.Subcommand == "agents" || plan.Subcommand == "message" {
		running, err := gateway.ReadRunningAgents()
		if err != nil {
			return subcommandError{err}
		}
		if plan.Subcommand == "agents" {
			return runAgents(plan, running, out)
		}
		return runMessage(plan, workspace, running, send, out)
	}
	if plan.Subcommand == "views" {
		return runViews(plan, workspace, gateway.ViewsStore, send, out)
	}
	if plan.Subcommand == "recur" {
		return runRecur(plan, workspace, send, out)
	}
	if plan.Subcommand == "migrate-attachments" {
		migrated := send(model.MigrateAttachments{Workspace: workspace, Actor: plan.Actor}).(model.AttachmentsMigrated)
		if migrated.Err != nil && len(migrated.Moved) == 0 && len(migrated.Left) == 0 {
			return subcommandError{migrated.Err}
		}
		if _, err := fmt.Fprintln(out, migrated.Report()); err != nil {
			return err
		}
		if migrated.Err != nil {
			return subcommandError{migrated.Err}
		}
		return nil
	}
	return subcommandError{fmt.Errorf("unknown subcommand %q", plan.Subcommand)}
}

// runRecur is `recur`: print the schedule, make or change it from the flags, or stop it, through the commands the Recurrence
// card sends. The flags are read into terms before anything is written, so a refusal sends nothing.
func runRecur(plan LaunchPlan, workspace string, send func(model.Cmd) model.Msg, out io.Writer) error {
	load := func() (model.RecurTarget, error) {
		target := send(model.LoadRecurTarget{Workspace: workspace, Ref: plan.TargetRef}).(model.RecurTarget)
		return target, target.Err
	}
	say := func(line string) error {
		_, err := fmt.Fprintln(out, line)
		return err
	}
	// movedMeanwhile prints the schedule another writer left and refuses: the write did not land.
	movedMeanwhile := func(target model.RecurTarget, changed model.Recurrence) error {
		if err := say(target.WithRecurrence(changed).Report()); err != nil {
			return err
		}
		return subcommandError{model.ErrRecurrenceMovedMeanwhile}
	}
	target, err := load()
	if err != nil {
		return subcommandError{err}
	}
	switch {
	case plan.Stop:
		switch {
		case target.Recurrence == nil:
			return subcommandError{model.NotRecurringError(target.ShortID)}
		case target.Recurring() == nil:
			return subcommandError{model.RecurStoppedError(target.ShortID)}
		}
		stopped := send(model.StopRecurrenceCmd{Workspace: workspace, UID: target.UID, ProjectID: target.ProjectID, Actor: plan.Actor, RecurrenceUID: target.Recurrence.UID, Opened: target.Recurrence.RecurrenceTerms}).(model.RecurrenceResultMsg)
		if stopped.Err != nil {
			return subcommandError{stopped.Err}
		}
		if stopped.Changed != nil {
			return movedMeanwhile(target, *stopped.Changed)
		}
		return say(target.StopReport())
	case len(plan.RecurFlags) == 0:
		return say(target.Report())
	}
	if target.Recurrence != nil && target.Recurring() == nil {
		return subcommandError{model.RecurStoppedError(target.ShortID)}
	}
	terms, err := target.Terms(plan.RecurFlags)
	if err != nil {
		return subcommandError{err}
	}
	var written model.RecurrenceResultMsg
	if current := target.Recurring(); current != nil {
		written = send(model.ChangeRecurrenceCmd{Workspace: workspace, UID: target.UID, ProjectID: target.ProjectID, Actor: plan.Actor,
			RecurrenceUID: current.UID, RecurrenceTerms: terms, Opened: current.RecurrenceTerms}).(model.RecurrenceResultMsg)
	} else {
		written = send(model.CreateRecurrenceCmd{Workspace: workspace, UID: target.UID, ProjectID: target.ProjectID, Actor: plan.Actor, RecurrenceTerms: terms}).(model.RecurrenceResultMsg)
	}
	if written.Err != nil {
		return subcommandError{written.Err}
	}
	if written.Changed != nil {
		return movedMeanwhile(target, *written.Changed)
	}
	// Said once the write landed: a refused save must not read as if the stored Local zone was replaced.
	if note := target.ZoneNote(plan.RecurFlags); note != "" {
		if err := say(note); err != nil {
			return err
		}
	}
	target, err = load()
	if err != nil {
		return subcommandError{err}
	}
	return say(target.Report())
}
