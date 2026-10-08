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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	session, err := tern.Connect(ctx, tern.Options{App: "tern-kata", Version: "dev"})
	if errors.Is(err, tern.ErrUnsupported) {
		kata, err := exec.LookPath("kata")
		if err != nil {
			return err
		}
		return syscall.Exec(kata, append([]string{"kata", "tui"}, os.Args[1:]...), os.Environ())
	}
	if err != nil {
		return err
	}
	defer session.Close()
	surface, err := session.Open(tern.SurfaceOptions{ID: "kata", Mode: tern.Screen, Title: "kata", Discard: true})
	if err != nil {
		return err
	}
	defer surface.Close()
	workspace, err := os.Getwd()
	if err != nil {
		return err
	}
	messages := make(chan model.Msg)
	inputErrors := make(chan error, 1)
	go func() {
		for {
			input, err := session.Next(ctx)
			if err != nil {
				inputErrors <- err
				return
			}
			var msg model.Msg
			switch input := input.(type) {
			case tern.Key:
				msg = model.KeyMsg{Chord: input.String()}
			case *tern.Select:
				if input.ID == view.IssueListID {
					msg = model.SelectMsg{UID: strings.TrimPrefix(input.Item, view.IssueListID+".")}
				}
				if input.ID == view.CollectionsID {
					msg = model.CollectionPointerSelected{ID: input.Item}
				}
			case *tern.Action:
				if input.ID == view.RetryID && input.Act == string(model.RetryAction) {
					msg = model.Retry{}
				}
			case *tern.Resize:
				msg = model.ResizeMsg{Cols: input.Cols}
			case *tern.ErrorEvent:
				inputErrors <- fmt.Errorf("Tern surface: %s", input.Msg)
				return
			}
			if msg != nil {
				select {
				case messages <- msg:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	m := model.Model{Cols: session.Caps().Cols}
	runner := commandRunner{}
	defer runner.close()
	msg := model.Msg(model.Start{Workspace: workspace})
	for {
		previousSelection := m.SelectedUID
		var cmds []model.Cmd
		m, cmds = model.Update(m, msg)
		if m.Quitting {
			return nil
		}
		if err := surface.Render(view.Render(m)); err != nil {
			return err
		}
		if m.SelectedUID != "" && m.SelectedUID != previousSelection {
			if err := surface.Reveal(view.IssueID(m.SelectedUID), "nearest"); err != nil {
				return err
			}
		}
		for _, cmd := range cmds {
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
