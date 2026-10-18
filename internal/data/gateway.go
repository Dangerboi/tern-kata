package data

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// Gateway is the one way commands reach the kata daemon: every Cmd goes in and
// exactly one result Msg comes out, except Subscribe, whose stream sends many.
// It locates the daemon once per workspace and reuses the client, and locates
// again when the daemon cannot be reached.
type Gateway struct {
	locate Locator

	mu      sync.Mutex
	clients map[string]*Client

	streamMu         sync.Mutex
	streamCancel     context.CancelFunc
	streamGeneration int64
}

func NewGateway(locate Locator) *Gateway {
	return &Gateway{locate: locate, clients: map[string]*Client{}}
}

// Close ends the running stream and drops the cached clients.
func (g *Gateway) Close() {
	g.streamMu.Lock()
	if g.streamCancel != nil {
		g.streamCancel()
	}
	g.streamMu.Unlock()
	g.mu.Lock()
	defer g.mu.Unlock()
	clear(g.clients)
}

// Run executes cmd and sends each result to messages until ctx ends.
// A Subscribe replaces the running stream; one older than the running one is ignored.
func (g *Gateway) Run(ctx context.Context, cmd model.Cmd, messages chan<- model.Msg) {
	if subscription, ok := cmd.(model.Subscribe); ok {
		g.stream(ctx, subscription, messages)
		return
	}
	select {
	case messages <- g.do(ctx, cmd):
	case <-ctx.Done():
	}
}

// request is how one Cmd reaches the daemon and how its failure reads as a Msg.
type request struct {
	workspace string
	run       func(context.Context, *Client) (model.Msg, error)
	fail      func(error) model.Msg
}

// ok passes a result through, or the error alone: a failed read has no result to show.
func ok[T model.Msg](result T, err error) (model.Msg, error) {
	if err != nil {
		return nil, err
	}
	return result, nil
}

func plan(cmd model.Cmd) (request, bool) {
	switch cmd := cmd.(type) {
	case model.CreateIssueCmd:
		return request{cmd.Workspace,
			func(ctx context.Context, c *Client) (model.Msg, error) { return ok(c.CreateIssue(ctx, cmd)) },
			func(err error) model.Msg { return model.IssueCreated{Err: err} }}, true
	case model.OpenIssue:
		return request{cmd.Workspace,
			func(ctx context.Context, c *Client) (model.Msg, error) { return ok(c.OpenIssue(ctx, cmd)) },
			func(err error) model.Msg { return model.IssueOpenFailed{Ref: cmd.Ref, Err: err} }}, true
	case model.LookupSearchID:
		// shortcut: any failure counts as no Issue until the query changes (retrying would loop while the daemon is down); the full-text query that follows reports it.
		return request{cmd.Workspace,
			func(ctx context.Context, c *Client) (model.Msg, error) {
				uid, err := c.ResolveIssue(ctx, cmd.ProjectID, cmd.ShortID)
				return ok(model.SearchIDLooked{Request: cmd.Request, ShortID: cmd.ShortID, UID: uid}, err)
			},
			func(error) model.Msg { return model.SearchIDLooked{Request: cmd.Request, ShortID: cmd.ShortID} }}, true
	case model.SaveDraft:
		return request{cmd.Workspace,
			func(ctx context.Context, c *Client) (model.Msg, error) { return c.SaveDraft(ctx, cmd) },
			func(err error) model.Msg { return model.DraftFailed{UID: cmd.UID, Err: err} }}, true
	case model.CloseIssueCmd:
		return request{cmd.Workspace,
			func(ctx context.Context, c *Client) (model.Msg, error) {
				return ok(model.CloseResultMsg{UID: cmd.UID}, c.Close(ctx, cmd))
			},
			func(err error) model.Msg { return model.CloseResultMsg{UID: cmd.UID, Err: err} }}, true
	case model.CreateRecurrenceCmd:
		return request{cmd.Workspace,
			func(ctx context.Context, c *Client) (model.Msg, error) {
				return ok(model.RecurrenceResultMsg{UID: cmd.UID}, c.CreateRecurrence(ctx, cmd))
			},
			func(err error) model.Msg { return model.RecurrenceResultMsg{UID: cmd.UID, Err: err} }}, true
	case model.ChangeRecurrenceCmd:
		return request{cmd.Workspace,
			func(ctx context.Context, c *Client) (model.Msg, error) {
				changed, err := c.ChangeRecurrence(ctx, cmd)
				return ok(model.RecurrenceResultMsg{UID: cmd.UID, Changed: changed}, err)
			},
			func(err error) model.Msg { return model.RecurrenceResultMsg{UID: cmd.UID, Err: err} }}, true
	case model.StopRecurrenceCmd:
		return request{cmd.Workspace,
			func(ctx context.Context, c *Client) (model.Msg, error) {
				return ok(model.RecurrenceResultMsg{UID: cmd.UID}, c.StopRecurrence(ctx, cmd))
			},
			func(err error) model.Msg { return model.RecurrenceResultMsg{UID: cmd.UID, Err: err} }}, true
	case model.ReopenIssueCmd:
		return request{cmd.Workspace,
			func(ctx context.Context, c *Client) (model.Msg, error) {
				return ok(model.CloseResultMsg{UID: cmd.UID}, c.Reopen(ctx, cmd))
			},
			func(err error) model.Msg { return model.CloseResultMsg{UID: cmd.UID, Err: err} }}, true
	case model.WriteField:
		return request{cmd.Workspace,
			func(ctx context.Context, c *Client) (model.Msg, error) {
				return ok(model.FieldWritten{UID: cmd.UID}, c.WriteField(ctx, cmd))
			},
			func(err error) model.Msg { return model.FieldWritten{UID: cmd.UID, Err: err} }}, true
	case model.LoadFieldLabels:
		return request{cmd.Workspace,
			func(ctx context.Context, c *Client) (model.Msg, error) { return ok(c.FieldLabels(ctx, cmd)) },
			func(err error) model.Msg { return model.FieldLabelsLoaded{UID: cmd.UID, Err: err} }}, true
	case model.CreateProject:
		return request{cmd.Workspace,
			func(ctx context.Context, c *Client) (model.Msg, error) {
				project, err := c.CreateProject(ctx, cmd)
				return ok(model.ProjectSaved{Project: project}, err)
			},
			func(err error) model.Msg { return model.ProjectSaved{Err: err} }}, true
	case model.BindProject:
		return request{cmd.Workspace,
			func(ctx context.Context, c *Client) (model.Msg, error) {
				project, err := c.BindProject(ctx, cmd)
				return ok(model.ProjectSaved{Project: project}, err)
			},
			func(err error) model.Msg { return model.ProjectSaved{Err: err} }}, true
	case model.LoadProject:
		return request{cmd.Workspace,
			func(ctx context.Context, c *Client) (model.Msg, error) { return ok(c.ReadProject(ctx, cmd.Workspace)) },
			func(err error) model.Msg { return model.Failed{Err: err} }}, true
	case model.LoadDetail:
		return request{cmd.Workspace,
			func(ctx context.Context, c *Client) (model.Msg, error) { return ok(c.ReadDetail(ctx, cmd.UID)) },
			func(err error) model.Msg { return model.DetailFailed{UID: cmd.UID, Err: err} }}, true
	case model.ReadSnapshot:
		return request{cmd.Workspace,
			func(ctx context.Context, c *Client) (model.Msg, error) { return ok(c.ReadSnapshot(ctx, cmd)) },
			func(err error) model.Msg { return model.SnapshotFailed{Request: cmd.Request, Err: err} }}, true
	}
	return request{}, false
}

func (g *Gateway) do(ctx context.Context, cmd model.Cmd) model.Msg {
	req, known := plan(cmd)
	if !known {
		return model.Failed{Err: fmt.Errorf("tern-kata: unknown command %T", cmd)}
	}
	msg, err := g.call(ctx, req.workspace, req.run)
	if err != nil {
		return req.fail(err)
	}
	return msg
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
	g.mu.Lock()
	defer g.mu.Unlock()
	if c := g.clients[workspace]; c != nil {
		return c, nil
	}
	target, err := g.locate(ctx, workspace)
	if err != nil {
		return nil, err
	}
	c, err := connect(ctx, target)
	if err != nil {
		return nil, err
	}
	g.clients[workspace] = c
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
