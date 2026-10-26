package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/Dangerboi/tern-kata/internal/data"
	"github.com/Dangerboi/tern-kata/internal/model"
)

// subcommandError is any failure of attach or detach, from its usage to its output: main exits 1 with the message, not the Pane's start-failure status.
type subcommandError struct{ error }

func (e subcommandError) Unwrap() error { return e.error }

// runSubcommand runs plan's attach or detach through the same gateway commands the Pane sends, printing results to out.
// Files are attached in order, and the first failure stops the rest.
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
	if plan.Subcommand == "recur" {
		return runRecur(plan, workspace, send, out)
	}
	if plan.Subcommand == "detach" {
		detached := send(model.DetachFile{Workspace: workspace, Ref: plan.TargetRef, Actor: plan.Actor, Name: plan.Operands[0]}).(model.FileDetached)
		if detached.Cleared {
			if _, err := fmt.Fprintln(out, detached.Report()); err != nil {
				return subcommandError{err}
			}
		}
		if detached.Err != nil {
			return subcommandError{detached.Err}
		}
		return nil
	}
	for _, file := range plan.Operands {
		attached := send(model.AttachFile{Workspace: workspace, Ref: plan.TargetRef, Actor: plan.Actor, Path: file}).(model.FileAttached)
		if attached.Err != nil {
			return subcommandError{attached.Err}
		}
		if _, err := fmt.Fprintln(out, attached.Path); err != nil {
			return subcommandError{err}
		}
	}
	return nil
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
		stopped := send(model.StopRecurrenceCmd{Workspace: workspace, UID: target.UID, ProjectID: target.ProjectID, Actor: plan.Actor, RecurrenceUID: target.Recurrence.UID}).(model.RecurrenceResultMsg)
		if stopped.Err != nil {
			return subcommandError{stopped.Err}
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
	if note := target.ZoneNote(plan.RecurFlags); note != "" {
		if err := say(note); err != nil {
			return subcommandError{err}
		}
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
		if err := say(target.WithRecurrence(*written.Changed).Report()); err != nil {
			return subcommandError{err}
		}
		return subcommandError{model.ErrRecurrenceMovedMeanwhile}
	}
	target, err = load()
	if err != nil {
		return subcommandError{err}
	}
	return say(target.Report())
}
