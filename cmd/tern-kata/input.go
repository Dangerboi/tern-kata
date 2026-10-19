package main

import (
	"context"

	"github.com/stencil-hq/tern-sdk/go/tern"
)

// startInputReader forwards every Tern input to inputs; the Pane driver decides what each means.
func startInputReader(ctx context.Context, session interface {
	Next(ctx context.Context) (tern.Input, error)
}, inputs chan<- any, inputErrors chan<- error) func() {
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
			select {
			case inputs <- input:
			case <-ctx.Done():
				return
			}
		}
	}()
	return func() { cancel(); <-done }
}
