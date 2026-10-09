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
	"time"

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
		previous := m
		var cmds []model.Cmd
		m, cmds = model.Update(m, msg)
		if m.Quitting {
			return nil
		}
		if err := surface.Render(view.Render(m, time.Now())); err != nil {
			return err
		}
		if target := view.InputFocusID(m); target != inputFocus {
			if err := surface.Focus(target); err != nil {
				return err
			}
			inputFocus = target
		}
		if !rowClick(msg) && !m.Loading && m.Focus == model.ColumnList && m.Shows(model.ColumnList) && m.SelectedUID != "" && m.SelectedUID != previousSelection {
			if err := surface.Reveal(view.IssueID(m.SelectedUID), "nearest"); err != nil {
				return err
			}
		}
		if model.RevealsDetail(previous, m, msg) {
			if err := surface.Reveal(view.DetailRevealID(m), "nearest"); err != nil {
				return err
			}
		}
		for _, cmd := range cmds {
			if scroll, ok := cmd.(model.Scroll); ok {
				target := view.IssueListID
				switch scroll.Column {
				case model.ColumnCollections:
					target = view.CollectionsID
				case model.ColumnDetail:
					target = view.DetailID
				}
				by := "page-up"
				if scroll.Down {
					by = "page-down"
				}
				if scroll.Start {
					by = "start"
				}
				if err := surface.Scroll(target, by); err != nil {
					return err
				}
				continue
			}
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

// rowClick reports a click on an Issue row. Tern has already put that row under
// the pointer, so revealing it again would scroll the list from under it.
func rowClick(msg model.Msg) bool {
	switch msg := msg.(type) {
	case model.SelectMsg:
		return true
	case model.ActionMsg:
		return msg.Action == model.ToggleExpansion
	}
	return false
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
