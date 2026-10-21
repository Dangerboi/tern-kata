package main

import (
	"context"
	"os"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"golang.org/x/term"
)

// paneSize is the Pane's size as a ResizeMsg: its rows from the terminal and its row height from Tern's cell size.
func paneSize(cols int, cell tern.Cell) model.ResizeMsg {
	msg := model.ResizeMsg{Cols: cols}
	if _, rows, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
		msg.Rows, msg.RowPx = rows, int(cell.H)
	}
	return msg
}

// startInputReader forwards every Tern input to inputs; the Pane driver decides what each means.
func startInputReader(ctx context.Context, session paneSession, inputs chan<- any, inputErrors chan<- error) func() {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			input, err := session.Next(ctx)
			if err != nil {
				if ctx.Err() == nil {
					inputErrors <- err
				}
				return
			}
			var out any = input
			if resize, ok := input.(*tern.Resize); ok {
				out = paneSize(resize.Cols, resize.Cell)
			}
			select {
			case inputs <- out:
			case <-ctx.Done():
				return
			}
		}
	}()
	return func() { cancel(); <-done }
}
