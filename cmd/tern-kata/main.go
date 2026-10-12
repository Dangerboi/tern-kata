package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/data"
	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "tern-kata:", err)
		os.Exit(1)
	}
}

func run() error {
	plan, err := PlanLaunch(os.Args[1:], lookupEnv)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	session, err := tern.Connect(ctx, editorOptions())
	if errors.Is(err, tern.ErrUnsupported) {
		kata, err := exec.LookPath("kata")
		if err != nil {
			return err
		}
		return execKata(kata, plan.FallbackArgs, plan.FallbackEnv(os.Environ()))
	}
	if err != nil {
		return err
	}
	defer func() { session.Close() }()
	if _, err := fmt.Fprint(os.Stdout, "\x1b]0;tern-kata\x07"); err != nil {
		return err
	}
	surface, err := openSurface(session)
	if err != nil {
		return err
	}
	defer func() { surface.Close() }()
	workspace, err := os.Getwd()
	if err != nil {
		return err
	}
	inputs := make(chan any)
	inputErrors := make(chan error, 1)
	stopInput := startInputReader(ctx, session, inputs, inputErrors)
	defer func() { stopInput() }()
	state := driver.State{Model: model.Model{Cols: session.Caps().Cols, Actor: plan.Actor}}
	results := make(chan model.Msg)
	gateway := data.NewGateway(data.LocateWithKata)
	defer gateway.Close()
	input := any(model.Start{Workspace: workspace, IssueRef: plan.IssueRef})
eventLoop:
	for {
		out := driver.Step(state, input, time.Now())
		state = out.State
		if out.Err != nil {
			return out.Err
		}
		if out.Quit {
			return nil
		}
		if out.View != nil {
			if err := surface.Render(*out.View); err != nil {
				return err
			}
		}
		// One Send per op, so each goes out in its own frame as before: Tern needs the first tab reveal laid out before the second.
		for _, op := range out.Ops {
			if err := surface.Send(op); err != nil {
				return err
			}
		}
		for _, cmd := range out.Cmds {
			if external, ok := cmd.(model.OpenDraftEditor); ok {
				stopInput()
				surface.Close()
				session.Close()
				input = editExternally(external)
				nextSession, connectErr := tern.Connect(ctx, editorOptions())
				if connectErr != nil {
					return connectErr
				}
				nextSurface, openErr := openSurface(nextSession)
				if openErr != nil {
					nextSession.Close()
					return openErr
				}
				session, surface = nextSession, nextSurface
				state = state.Fresh()
				stopInput = startInputReader(ctx, session, inputs, inputErrors)
				continue eventLoop
			}
			go gateway.Run(ctx, cmd, results)
		}
		select {
		case input = <-inputs:
		case input = <-results:
		case err := <-inputErrors:
			if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}
	}
}

// lookupEnv reads the environment and answers gitUserNameKey from `git config user.name`.
func lookupEnv(key string) (string, bool) {
	if key != gitUserNameKey {
		return os.LookupEnv(key)
	}
	out, err := exec.Command("git", "config", "user.name").Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}
