package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/Dangerboi/tern-kata/model"
	"github.com/Dangerboi/tern-kata/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "tern-kata:", err)
		os.Exit(1)
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
		return syscall.Exec(kata, append([]string{"kata", "tui"}, fallbackArgs...), actorEnvironment(override))
	}
	if err != nil {
		return err
	}
	defer func() { session.Close() }()
	if _, err := fmt.Fprint(os.Stdout, "\x1b]0;tern-kata\x07"); err != nil {
		return err
	}
	surface, err := session.Open(editorSurfaceOptions())
	if err != nil {
		return err
	}
	defer func() { surface.Close() }()
	workspace, err := os.Getwd()
	if err != nil {
		return err
	}
	messages := make(chan model.Msg)
	inputErrors := make(chan error, 1)
	stopInput := startInputReader(ctx, session, messages, inputErrors)
	defer func() { stopInput() }()
	m := model.Model{Cols: session.Caps().Cols, Actor: resolveActor(override)}
	runner := commandRunner{}
	defer runner.close()
	msg := model.Msg(model.Start{Workspace: workspace, IssueRef: issueRef})
	inputFocus := ""
eventLoop:
	for {
		previousSelection := m.SelectedUID
		previousFocus, previousSection, previousIndex := m.Focus, m.DetailSection, m.DetailIndex
		previousDetailLoading := m.DetailLoading
		var cmds []model.Cmd
		m, cmds = model.Update(m, msg)
		if m.Quitting {
			return nil
		}
		if err := surface.Render(view.Render(m)); err != nil {
			return err
		}
		if target := view.InputFocusID(m); target != inputFocus {
			if err := surface.Focus(target); err != nil {
				return err
			}
			inputFocus = target
		}
		if m.Err == nil && !m.Loading && m.Focus == model.ColumnList && m.SelectedUID != "" && m.SelectedUID != previousSelection {
			if err := surface.Reveal(view.IssueID(m.SelectedUID), "nearest"); err != nil {
				return err
			}
		}
		if m.Err == nil && !m.Loading && !m.DetailHidden && m.Focus == model.ColumnDetail && m.Detail != nil && m.Detail.Issue != nil && !m.DetailLoading &&
			(previousFocus != m.Focus || previousSection != m.DetailSection || previousIndex != m.DetailIndex || previousDetailLoading) {
			if err := surface.Reveal(view.DetailSectionID(m.DetailSection), "nearest"); err != nil {
				return err
			}
		}
		for _, cmd := range cmds {
			if external, ok := cmd.(model.OpenDraftEditor); ok {
				stopInput()
				surface.Close()
				session.Close()
				msg = editExternally(external)
				nextSession, connectErr := tern.Connect(ctx, editorOptions())
				if connectErr != nil {
					return connectErr
				}
				nextSurface, openErr := nextSession.Open(editorSurfaceOptions())
				if openErr != nil {
					nextSession.Close()
					return openErr
				}
				session, surface = nextSession, nextSurface
				inputFocus = ""
				stopInput = startInputReader(ctx, session, messages, inputErrors)
				continue eventLoop
			}
			runner.launch(ctx, cmd, messages)
		}
		select {
		case msg = <-messages:
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
