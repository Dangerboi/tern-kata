package main

import (
	"context"
	"time"

	"github.com/Dangerboi/tern-kata/data"
	"github.com/Dangerboi/tern-kata/model"
)

type commandRunner struct {
	streamCancel     context.CancelFunc
	streamGeneration int64
}

func (r *commandRunner) launch(ctx context.Context, cmd model.Cmd, messages chan<- any) {
	if subscription, ok := cmd.(model.Subscribe); ok {
		if subscription.Generation <= r.streamGeneration {
			return
		}
		r.close()
		r.streamGeneration = subscription.Generation
		streamCtx, cancel := context.WithCancel(ctx)
		r.streamCancel = cancel
		go executeSubscription(streamCtx, subscription, messages)
		return
	}
	go execute(ctx, cmd, messages)
}

func (r *commandRunner) close() {
	if r.streamCancel != nil {
		r.streamCancel()
	}
}

func executeSubscription(ctx context.Context, cmd model.Subscribe, messages chan<- any) {
	send := func(msg model.Msg) bool {
		select {
		case messages <- msg:
			return true
		case <-ctx.Done():
			return false
		}
	}
	if cmd.Delay > 0 {
		timer := time.NewTimer(cmd.Delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return
		}
	}
	client, err := data.Discover(ctx, cmd.Workspace)
	if err == nil {
		defer client.CloseIdleConnections()
		err = client.ReadEvents(ctx, cmd, send)
	}
	if err != nil && ctx.Err() == nil {
		send(model.StreamDropped{Generation: cmd.Generation, Err: err})
	}
}
