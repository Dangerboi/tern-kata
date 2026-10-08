package main

import (
	"context"

	"github.com/Dangerboi/tern-kata/data"
	"github.com/Dangerboi/tern-kata/model"
)

func execute(ctx context.Context, cmd model.Cmd, messages chan<- model.Msg) {
	var msg model.Msg
	switch cmd := cmd.(type) {
	case model.LoadProject:
		client, err := data.Discover(ctx, cmd.Workspace)
		if err != nil {
			msg = model.Failed{Err: err}
		} else {
			result, err := client.ReadProject(ctx, cmd.Workspace)
			if err != nil { msg = model.Failed{Err: err} } else { msg = result }
		}
	}
	if msg != nil {
		select { case messages <- msg: case <-ctx.Done(): }
	}
}
