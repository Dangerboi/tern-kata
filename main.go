package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
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
	session, err := tern.Connect(ctx, editorOptions())
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
	defer func() { session.Close() }()
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
	m := model.Model{Cols: session.Caps().Cols}
	runner := commandRunner{}
	defer runner.close()
	msg := model.Msg(model.Start{Workspace: workspace})
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
		if err := surface.Focus(view.InputFocusID(m)); err != nil { return err }
		if m.Focus == model.ColumnList && m.SelectedUID != "" && m.SelectedUID != previousSelection {
			if err := surface.Reveal(view.IssueID(m.SelectedUID), "nearest"); err != nil {
				return err
			}
		}
		if m.Focus == model.ColumnDetail && m.Detail != nil && m.Detail.Issue != nil && !m.DetailLoading &&
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
				session, err = tern.Connect(ctx, editorOptions())
				if err != nil { return err }
				surface, err = session.Open(editorSurfaceOptions())
				if err != nil { return err }
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
