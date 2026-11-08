package main

import (
	"context"
	"os"
	"sync"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"golang.org/x/term"
)

func paneSize(cols int) model.ResizeMsg {
	msg := model.ResizeMsg{Cols: cols}
	if _, rows, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
		msg.Rows = rows
	}
	return msg
}

func startInputReader(ctx context.Context, session paneSession, inputs chan<- any, inputErrors chan<- error) func() {
	ctx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Go(func() {
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
				out = paneSize(resize.Cols)
			}
			select {
			case inputs <- out:
			case <-ctx.Done():
				return
			}
		}
	})
	wg.Go(func() { watchRows(ctx, heightChanges(ctx), inputs) })
	return func() { cancel(); wg.Wait() }
}

// watchRows sends the Pane's rows on each change until ctx ends: Tern reports a resize only when the width changes, so a Pane made
// taller or shorter at the same width is seen only in its terminal's size.
func watchRows(ctx context.Context, changes <-chan struct{}, inputs chan<- any) {
	_, last, _ := term.GetSize(int(os.Stdout.Fd()))
	for {
		select {
		case <-changes:
		case <-ctx.Done():
			return
		}
		_, rows, err := term.GetSize(int(os.Stdout.Fd()))
		if err != nil || rows == last {
			continue
		}
		last = rows
		select {
		case inputs <- model.PaneRowsMsg{Rows: rows}:
		case <-ctx.Done():
			return
		}
	}
}
