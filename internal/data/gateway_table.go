package data

import (
	"context"
	"reflect"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// The Daemon gateway's command table declares, per command type, how the gateway handles it: the named steps that run
// before its handler, the handler, and the named steps that run after it. Run looks the declaration up and runs it;
// it never switches on a command's type. A command with no declaration is a daemon call: it falls through to plan,
// the request builder.

// kind says how Run executes a declaration: a local command or daemon call answers once, a long-lived command sends many
// results (or none) on its own schedule.
type kind int

const (
	// answersOnce handles the command and sends the one result.
	answersOnce kind = iota
	// longLived hands the command the message channel; it outlives Run's single answer.
	longLived
)

// beforeStep runs ahead of the handler of cmd.
type beforeStep struct {
	name string
	run  func(g *Gateway, ctx context.Context, cmd model.Cmd)
}

// afterStep runs once the handler has a result and returns the result to send, so it can add to it.
type afterStep struct {
	name string
	run  func(g *Gateway, ctx context.Context, cmd model.Cmd, result model.Msg) model.Msg
}

// declaration is the whole handling of one command type.
type declaration struct {
	kind   kind
	before []beforeStep
	after  []afterStep
	// handle produces the one result of an answersOnce command.
	handle func(g *Gateway, ctx context.Context, cmd model.Cmd) model.Msg
	// stream runs a longLived command until ctx ends.
	stream func(g *Gateway, ctx context.Context, cmd model.Cmd, messages chan<- model.Msg)
}

// The named steps.
var (
	// convertPointersBefore converts old Attachment pointers of the command's workspace ahead of the daemon call.
	convertPointersBefore = beforeStep{"convert old pointers", func(g *Gateway, ctx context.Context, cmd model.Cmd) {
		switch cmd := cmd.(type) {
		case model.AttachFile:
			g.convertPointers(ctx, cmd.Workspace, cmd.Actor)
		case model.DetachFile:
			g.convertPointers(ctx, cmd.Workspace, cmd.Actor)
		}
	}}

	// startPointerMoveAfter starts the Attachment pointer move once a Project loaded.
	startPointerMoveAfter = afterStep{"start the pointer move", func(g *Gateway, _ context.Context, cmd model.Cmd, result model.Msg) model.Msg {
		if loaded, ok := result.(model.ProjectLoaded); ok && loaded.Project.UID != "" {
			load := cmd.(model.LoadProject)
			g.startPointerMove(load.Workspace, load.Actor)
		}
		return result
	}}

	// readDefaultFilterAfter reads the Project's Default filter into the load once a Project loaded.
	readDefaultFilterAfter = afterStep{"read the Default filter", func(g *Gateway, _ context.Context, _ model.Cmd, result model.Msg) model.Msg {
		if loaded, ok := result.(model.ProjectLoaded); ok && loaded.Project.UID != "" {
			loaded.Default, loaded.DefaultErr = g.loadFilters(loaded.Project.UID)
			return loaded
		}
		return result
	}}
)

// commands is the table, keyed by the command's type. Daemon calls are not in it yet.
var commands = map[reflect.Type]declaration{}

func typeOf[C model.Cmd]() reflect.Type { return reflect.TypeFor[C]() }

// local declares a command the gateway answers without the daemon, or with a handler of its own.
func local[C model.Cmd](handle func(g *Gateway, ctx context.Context, cmd C) model.Msg, before []beforeStep, after []afterStep) {
	commands[typeOf[C]()] = declaration{kind: answersOnce, before: before, after: after,
		handle: func(g *Gateway, ctx context.Context, cmd model.Cmd) model.Msg { return handle(g, ctx, cmd.(C)) }}
}

// longLivedCommand declares a command that sends its results itself.
func longLivedCommand[C model.Cmd](stream func(g *Gateway, ctx context.Context, cmd C, messages chan<- model.Msg)) {
	commands[typeOf[C]()] = declaration{kind: longLived,
		stream: func(g *Gateway, ctx context.Context, cmd model.Cmd, messages chan<- model.Msg) {
			stream(g, ctx, cmd.(C), messages)
		}}
}

func init() {
	longLivedCommand(func(g *Gateway, ctx context.Context, cmd model.Subscribe, messages chan<- model.Msg) {
		g.stream(ctx, cmd, messages)
	})
	longLivedCommand(func(g *Gateway, ctx context.Context, cmd model.WakeAt, messages chan<- model.Msg) {
		g.wake(ctx, cmd, messages)
	})

	local(func(g *Gateway, _ context.Context, cmd model.LoadLanes) model.Msg { return g.loadLanes(cmd) }, nil, nil)
	local(func(g *Gateway, _ context.Context, cmd model.SaveLanes) model.Msg { return g.saveLanes(cmd) }, nil, nil)
	local(func(g *Gateway, _ context.Context, cmd model.SaveFilters) model.Msg { return g.saveFilters(cmd) }, nil, nil)
	local(func(_ *Gateway, _ context.Context, _ model.LoadWorkerPickers) model.Msg { return loadWorkerPickers() }, nil, nil)
	local(func(g *Gateway, ctx context.Context, cmd model.LoadWorker) model.Msg { return g.loadWorker(ctx, cmd) }, nil, nil)
	local(func(g *Gateway, _ context.Context, cmd model.LoadWorkerSetup) model.Msg {
		return g.loadWorkerSetup(cmd)
	}, nil, nil)
	local(func(g *Gateway, _ context.Context, cmd model.SaveWorkerSetup) model.Msg {
		return g.saveWorkerSetup(cmd)
	}, nil, nil)
	local(func(g *Gateway, _ context.Context, cmd model.SaveWorkerEdit) model.Msg { return g.saveWorkerEdit(cmd) }, nil, nil)
	local(func(g *Gateway, ctx context.Context, cmd model.StopWorker) model.Msg { return g.stopWorker(ctx, cmd) }, nil, nil)
	local(func(g *Gateway, ctx context.Context, cmd model.SetWorkerSupervision) model.Msg {
		return g.setWorkerSupervision(ctx, cmd)
	}, nil, nil)
	local(func(g *Gateway, ctx context.Context, cmd model.LoadLocalRun) model.Msg {
		return g.loadLocalRun(ctx, cmd)
	}, nil, nil)
	local(func(g *Gateway, ctx context.Context, cmd model.RunJobNow) model.Msg { return g.runJobNow(ctx, cmd) }, nil, nil)
	local(func(g *Gateway, ctx context.Context, cmd model.ResumeLocalRun) model.Msg {
		return g.moveRun(ctx, "resume", cmd.UID)
	}, nil, nil)
	local(func(g *Gateway, ctx context.Context, cmd model.AbandonLocalRun) model.Msg {
		return g.moveRun(ctx, "abandon", cmd.UID)
	}, nil, nil)
	local(func(g *Gateway, ctx context.Context, cmd model.OpenRunSession) model.Msg {
		return g.openRunSession(ctx, cmd)
	}, nil, nil)

	// These three still reach the daemon through the request builder; the steps around the call are declared here.
	viaDaemon := func(g *Gateway, ctx context.Context, cmd model.Cmd) model.Msg { return g.do(ctx, cmd) }
	commands[typeOf[model.LoadProject]()] = declaration{kind: answersOnce, handle: viaDaemon,
		after: []afterStep{startPointerMoveAfter, readDefaultFilterAfter}}
	commands[typeOf[model.AttachFile]()] = declaration{kind: answersOnce, handle: viaDaemon, before: []beforeStep{convertPointersBefore}}
	commands[typeOf[model.DetachFile]()] = declaration{kind: answersOnce, handle: viaDaemon, before: []beforeStep{convertPointersBefore}}
}

// declarationFor is the declaration of cmd; a command without one is a daemon call.
func declarationFor(cmd model.Cmd) declaration {
	if d, ok := commands[reflect.TypeOf(cmd)]; ok {
		return d
	}
	return declaration{kind: answersOnce, handle: func(g *Gateway, ctx context.Context, cmd model.Cmd) model.Msg { return g.do(ctx, cmd) }}
}
