package data

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"go.kenn.io/kata/pkg/client"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/worker"
)

// Gateway is the one way commands reach the kata daemon: every Cmd goes in and
// exactly one result Msg comes out, except Subscribe, whose stream sends many.
// It locates the daemon once per workspace and reuses the client, and locates
// again when the daemon cannot be reached.
type Gateway struct {
	locate Locator
	// host is this machine's host name for Attachment pointers; empty when unreadable.
	host string
	// deadline bounds discovery, the version Ping and the kind check of one connection attempt.
	deadline time.Duration

	mu      sync.Mutex
	clients map[string]*Client
	// current is the catalog name of the daemon the Pane last switched to; empty until it does.
	current string

	streamMu         sync.Mutex
	streamCancel     context.CancelFunc
	streamGeneration int64

	wakeMu         sync.Mutex
	wakeCancel     context.CancelFunc
	wakeGeneration int64
	now            func() time.Time
	wakePoll       time.Duration

	// workerDirs locates this machine's worker; runTern runs the tern program. Both are the real ones unless a test replaces them.
	workerDirs func() (worker.Dirs, error)
	runTern    func(context.Context, []string) ([]byte, error)

	// convertMu serializes the passes that move old Attachment pointers; converted holds the instance/Project pairs this process
	// has finished. startMu guards convertStarted, the workspaces whose Pane start has begun a pass. The passes run on
	// backgroundCtx, which Close ends, and background counts them.
	convertMu        sync.Mutex
	converted        map[string]bool
	startMu          sync.Mutex
	convertStarted   map[string]bool
	background       sync.WaitGroup
	backgroundCtx    context.Context
	backgroundCancel context.CancelFunc
}

func NewGateway(locate Locator) *Gateway {
	backgroundCtx, backgroundCancel := context.WithCancel(context.Background())
	return &Gateway{locate: locate, deadline: connectDeadline, clients: map[string]*Client{}, now: time.Now, wakePoll: time.Minute, workerDirs: worker.DefaultDirs, runTern: execTern,
		converted: map[string]bool{}, convertStarted: map[string]bool{}, backgroundCtx: backgroundCtx, backgroundCancel: backgroundCancel}
}

// WithHost sets this machine's host name, the one every Attachment pointer is written with and compared against.
func (g *Gateway) WithHost(host string) *Gateway {
	g.host = host
	return g
}

// Close ends the running stream and any pointer pass still running, and drops the cached clients.
func (g *Gateway) Close() {
	if g.backgroundCancel != nil {
		g.backgroundCancel()
	}
	g.background.Wait()
	g.streamMu.Lock()
	if g.streamCancel != nil {
		g.streamCancel()
	}
	g.streamMu.Unlock()
	g.wakeMu.Lock()
	if g.wakeCancel != nil {
		g.wakeCancel()
	}
	g.wakeMu.Unlock()
	g.mu.Lock()
	defer g.mu.Unlock()
	clear(g.clients)
}

// Run executes cmd and sends each result to messages until ctx ends.
// A Subscribe replaces the running stream; one older than the running one is ignored.
func (g *Gateway) Run(ctx context.Context, cmd model.Cmd, messages chan<- model.Msg) {
	d := declarationFor(cmd)
	if d.kind == longLived {
		d.stream(g, ctx, cmd, messages)
		return
	}
	for _, step := range d.before {
		step.run(g, ctx, cmd)
	}
	result := d.handle(g, ctx, cmd)
	for _, step := range d.after {
		result = step.run(g, ctx, cmd, result)
	}
	select {
	case messages <- result:
	case <-ctx.Done():
	}
}

// ok passes a result through, or the error alone: a failed read has no result to show.
func ok[T model.Msg](result T, err error) (model.Msg, error) {
	if err != nil {
		return nil, err
	}
	return result, nil
}

// call runs fn against the daemon for workspace. When the daemon cannot be
// reached it locates the daemon again and tries once more before giving up.
func (g *Gateway) call(ctx context.Context, workspace string, fn func(context.Context, *Client) (model.Msg, error)) (model.Msg, error) {
	for attempt := 0; ; attempt++ {
		c, err := g.client(ctx, workspace)
		if err != nil {
			return nil, err
		}
		msg, err := fn(ctx, c)
		if err == nil || attempt > 0 || ctx.Err() != nil || !unreachable(err) {
			return msg, err
		}
		g.forget(workspace, c)
	}
}

// unreachable reports a failed dial. Nothing was sent, so a write is safe to retry.
func unreachable(err error) bool {
	var op *net.OpError
	return errors.As(err, &op) && op.Op == "dial"
}

func (g *Gateway) client(ctx context.Context, workspace string) (*Client, error) {
	// The deadline counts from the call, so a command queued behind another's connect gives up after one deadline in all.
	connectCtx, cancel := context.WithTimeout(ctx, g.deadline)
	defer cancel()
	g.mu.Lock()
	defer g.mu.Unlock()
	if c := g.clients[workspace]; c != nil {
		return c, nil
	}
	// shortcut: the lock is held while connecting, so one hung daemon holds up other workspaces until the deadline.
	c, err := g.connect(connectCtx, workspace)
	if err != nil {
		if connectCtx.Err() != nil && ctx.Err() == nil {
			err = fmt.Errorf("%w\n%s", client.ErrDaemonUnavailable, noDaemonHint)
		}
		return nil, err
	}
	g.clients[workspace] = c
	return c, nil
}

func (g *Gateway) connect(ctx context.Context, workspace string) (*Client, error) {
	target, err := g.locate(ctx, workspace)
	if err != nil {
		return nil, err
	}
	c, err := connect(ctx, target)
	if err != nil {
		return nil, err
	}
	catalog, _ := ReadCatalog()
	c.name = g.currentLocked(catalog)
	return c, nil
}

func (g *Gateway) forget(workspace string, c *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.clients[workspace] == c {
		delete(g.clients, workspace)
	}
}

func (g *Gateway) stream(ctx context.Context, cmd model.Subscribe, messages chan<- model.Msg) {
	g.streamMu.Lock()
	if cmd.Generation <= g.streamGeneration {
		g.streamMu.Unlock()
		return
	}
	if g.streamCancel != nil {
		g.streamCancel()
	}
	g.streamGeneration = cmd.Generation
	ctx, cancel := context.WithCancel(ctx)
	g.streamCancel = cancel
	g.streamMu.Unlock()
	defer cancel()
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
	_, err := g.call(ctx, cmd.Workspace, func(ctx context.Context, c *Client) (model.Msg, error) {
		return nil, c.ReadEvents(ctx, cmd, send)
	})
	if err != nil && ctx.Err() == nil {
		send(model.StreamDropped{Generation: cmd.Generation, Err: err})
	}
}

// wake answers DayEnded once the wall clock reaches cmd.At. It checks the wall clock at least every wakePoll, so a machine that
// slept past At answers soon after resuming, and ends when ctx ends or a newer wake replaces it.
func (g *Gateway) wake(ctx context.Context, cmd model.WakeAt, messages chan<- model.Msg) {
	g.wakeMu.Lock()
	if cmd.Generation <= g.wakeGeneration {
		g.wakeMu.Unlock()
		return
	}
	g.wakeGeneration = cmd.Generation
	if g.wakeCancel != nil {
		g.wakeCancel()
	}
	ctx, cancel := context.WithCancel(ctx)
	g.wakeCancel = cancel
	g.wakeMu.Unlock()
	defer cancel()
	for {
		// Round(0) drops the monotonic reading, which does not advance while the machine sleeps.
		left := cmd.At.Round(0).Sub(g.now().Round(0))
		if left <= 0 {
			break
		}
		timer := time.NewTimer(min(left, g.wakePoll))
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return
		}
	}
	select {
	case messages <- model.DayEnded{Day: cmd.Day}:
	case <-ctx.Done():
	}
}
