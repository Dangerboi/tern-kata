package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
	_ "time/tzdata" // zone database for hosts without one, such as Windows

	"github.com/Dangerboi/tern-kata/internal/data"
	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "tern-kata:", err)
		// Pane contract with window.luau: the Pane title is `tern-kata` (set in run) and this binary's own failures
		// exit 3; Go exits 2 on a panic. The plugin reads 2 and 3 as "failed to start", 1 as either (Windows reports every
		// failing launch as 1), and any other early failing status as "not installed", so a failure must never exit 1.
		os.Exit(3)
	}
}

func run() error {
	plan, err := PlanLaunch(os.Args[1:], lookupEnv, buildVersion())
	if err != nil {
		return err
	}
	if plan.Version != "" {
		_, err := fmt.Fprintln(os.Stdout, "tern-kata", plan.Version)
		return err
	}
	if plan.Help {
		_, err := fmt.Fprint(os.Stdout, usage)
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
		return execKata(kata, plan.FallbackArgs, os.Environ(), plan.fallbackAuthor)
	}
	if err != nil {
		return err
	}
	// Pane contract with window.luau: this title identifies a kata Pane.
	if _, err := fmt.Fprint(os.Stdout, "\x1b]0;tern-kata\x07"); err != nil {
		session.Close()
		return err
	}
	surface, err := openSurface(session)
	if err != nil {
		session.Close()
		return err
	}
	term := newTerminal(ctx, session, surface, connectPane)
	defer term.Close()
	workspace, err := os.Getwd()
	if err != nil {
		return err
	}
	state := driver.State{Model: model.Model{Cols: term.Caps().Cols, Actor: plan.Actor}}
	results := make(chan model.Msg)
	gateway := data.NewGateway(data.LocateWithKata)
	defer gateway.Close()
	input := any(model.Start{Workspace: workspace, IssueRef: plan.IssueRef, Nonce: rand.Text()})
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
			if err := term.Render(*out.View); err != nil {
				return err
			}
		}
		for _, op := range out.Ops {
			if err := term.Send(op); err != nil {
				return err
			}
		}
		for _, cmd := range out.Cmds {
			go gateway.Run(ctx, cmd, results)
		}
		if out.Editor != nil {
			closed, err := editInTerminal(term, *out.Editor, editorCommand)
			if err != nil {
				return err
			}
			input, state = closed, state.Fresh()
			continue
		}
		select {
		case input = <-term.Inputs():
		case input = <-results:
		case err := <-term.InputErrors():
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
