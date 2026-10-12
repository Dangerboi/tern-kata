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
		// Pane contract with window.luau: the Pane title is `tern-kata` (set in run) and this binary's own failures
		// exit 3; Go exits 2 on a panic. The plugin reads 2 and 3 as "failed to start", any other early failing status
		// as "not installed", so a failure must never exit with some other status.
		os.Exit(3)
	}
}

func run() error {
	override, args, err := actorArgs(os.Args[1:])
	if err != nil {
		return err
	}
	issueRef, fallbackArgs, err := launchArgs(args)
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
		return execKata(kata, fallbackArgs, actorEnvironment(override))
	}
	if err != nil {
		return err
	}
	defer func() { session.Close() }()
	// Pane contract with window.luau: this title identifies a kata Pane.
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
	state := driver.State{Model: model.Model{Cols: session.Caps().Cols, Actor: resolveActor(override)}}
	results := make(chan model.Msg)
	gateway := data.NewGateway(data.LocateWithKata)
	defer gateway.Close()
	input := any(model.Start{Workspace: workspace, IssueRef: issueRef})
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

func launchArgs(args []string) (string, []string, error) {
	ref := ""
	issueSet := false
	fallback := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--issue" {
			issueSet = true
			i++
			if i == len(args) {
				return "", nil, fmt.Errorf("--issue requires <project>/<short_id>")
			}
			ref = args[i]
		} else if value, ok := strings.CutPrefix(arg, "--issue="); ok {
			issueSet = true
			ref = value
		} else {
			fallback = append(fallback, arg)
		}
	}
	if issueSet {
		project, shortID, err := model.ParseIssueRef(ref)
		if err != nil {
			return "", nil, err
		}
		fallback = append(fallback, "--project", project, project+"#"+shortID)
	}
	return ref, fallback, nil
}
