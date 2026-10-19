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
	if plan.Subcommand == "detach" {
		detached := send(model.DetachFile{Workspace: workspace, Ref: plan.TargetRef, Actor: plan.Actor, Name: plan.Operands[0]}).(model.FileDetached)
		if detached.Err != nil {
			return subcommandError{detached.Err}
		}
		if _, err := fmt.Fprintln(out, detached.Report()); err != nil {
			return subcommandError{err}
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
