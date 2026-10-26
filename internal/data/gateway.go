package data

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
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
	// connecting holds, per workspace, a channel closed when the dial in flight for it ends.
	connecting map[string]chan struct{}
	// closed is set by Close, so a connect in flight caches nothing after it.
	closed bool
	// current is the catalog name of the daemon the Pane last switched to; empty until it does.
	current string
	// epoch counts daemon switches, so a connect that raced one is dropped rather than cached against the new daemon.
	epoch uint64

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
	// has finished. startMu guards convertStarted, the workspaces whose Pane start has begun a pass, and connCtx, the context of the
	// passes of the connection in use: a daemon switch ends it and waits for the passes before it installs the new connection.
	// background counts the passes. connCtx descends from backgroundCtx, which Close ends.
	convertMu        sync.Mutex
	converted        map[string]bool
	startMu          sync.Mutex
	convertStarted   map[string]bool
	connCtx          context.Context
	connCancel       context.CancelFunc
	background       sync.WaitGroup
	backgroundCtx    context.Context
	backgroundCancel context.CancelFunc
}

func NewGateway(locate Locator) *Gateway {
	backgroundCtx, backgroundCancel := context.WithCancel(context.Background())
	connCtx, connCancel := context.WithCancel(backgroundCtx)
	return &Gateway{locate: locate, deadline: connectDeadline, clients: map[string]*Client{}, connecting: map[string]chan struct{}{}, now: time.Now, wakePoll: time.Minute, workerDirs: worker.DefaultDirs, runTern: execTern,
		converted: map[string]bool{}, convertStarted: map[string]bool{}, connCtx: connCtx, connCancel: connCancel, backgroundCtx: backgroundCtx, backgroundCancel: backgroundCancel}
}

// WithHost sets this machine's host name, the one every Attachment pointer is written with and compared against.
func (g *Gateway) WithHost(host string) *Gateway {
	g.host = host
	return g
}

// Close ends the running stream and any pointer pass still running, and drops the cached clients.
func (g *Gateway) Close() {
	// Under startMu so a pass start either counts before Wait or sees the cancel and starts nothing.
	g.startMu.Lock()
	if g.backgroundCancel != nil {
		g.backgroundCancel()
	}
	g.startMu.Unlock()
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
	g.closed = true
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
	// A receiver that is ready gets the result even once ctx has ended: a subcommand waits on a buffered channel for the one
	// answer, and a random pick of ctx.Done() there would leave it waiting for good.
	select {
	case messages <- result:
		return
	default:
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

// errGatewayClosed is what a call gets once Close has run: no connection may be cached after it.
var errGatewayClosed = errors.New("gateway closed")

func (g *Gateway) client(ctx context.Context, workspace string) (*Client, error) {
	// The deadline counts from the call, so a command queued behind another's connect gives up after one deadline in all.
	connectCtx, cancel := context.WithTimeout(ctx, g.deadline)
	defer cancel()
	unavailable := func(err error) error {
		if connectCtx.Err() != nil && ctx.Err() == nil {
			return fmt.Errorf("%w\n%s", client.ErrDaemonUnavailable, noDaemonHint)
		}
		return err
	}
	for {
		g.mu.Lock()
		if g.closed {
			g.mu.Unlock()
			return nil, errGatewayClosed
		}
		c, locate, current, epoch := g.clients[workspace], g.locate, g.current, g.epoch
		if c != nil {
			g.mu.Unlock()
			return c, nil
		}
		// One dial per workspace: kata's client offers no way to release the connection of a duplicate, so a second
		// caller waits for the first and then reads its client (or retries when that dial failed).
		if wait := g.connecting[workspace]; wait != nil {
			g.mu.Unlock()
			select {
			case <-wait:
				continue
			case <-connectCtx.Done():
				return nil, unavailable(connectCtx.Err())
			}
		}
		done := make(chan struct{})
		if g.connecting == nil {
			g.connecting = map[string]chan struct{}{}
		}
		g.connecting[workspace] = done
		g.mu.Unlock()

		c, err := g.dial(ctx, connectCtx, locate, current, workspace)

		g.mu.Lock()
		delete(g.connecting, workspace)
		close(done)
		switch {
		case err != nil:
			g.mu.Unlock()
			return nil, unavailable(err)
		case g.closed:
			g.mu.Unlock()
			return nil, errGatewayClosed
		case g.epoch != epoch:
			// A switch landed while this connected: start over against the new daemon.
			g.mu.Unlock()
			continue
		}
		g.clients[workspace] = c
		g.mu.Unlock()
		return c, nil
	}
}

// dial resolves the entry and connects, outside g.mu: the pin read runs git, so a slow one holds up this workspace only.
func (g *Gateway) dial(ctx, connectCtx context.Context, locate Locator, current, workspace string) (*Client, error) {
	catalog, _ := ReadCatalog()
	name, entry := resolveEntry(ctx, g.deadline, workspace, catalog, current)
	c, err := connectLocated(connectCtx, locate, workspace)
	if err != nil {
		return nil, err
	}
	c.name, c.entry, c.deadline = name, entry, g.deadline
	return c, nil
}

// connect locates the daemon for workspace with ctx and dials it.
func connectLocated(ctx context.Context, locate Locator, workspace string) (*Client, error) {
	target, err := locate(ctx, workspace)
	if err != nil {
		return nil, err
	}
	return connect(ctx, target)
}

// resolveEntry names the catalog entry the located daemon is, as kata's discovery picked it: the entry the Pane switched to
// (current), else KATA_SERVER (no entry), else the workspace's .kata.local.toml pin, else active_daemon. A [server].daemon pin names
// its entry; a [server].url pin stays unnamed, as kata reaches it with the global token and never an entry's. active_daemon counts
// for a remote or a local profile (home and instance_uid); a plain `local = true` entry falls through to the default local daemon,
// so stays unnamed. A pin kata would refuse to resolve leaves the connection unnamed, so no entry's token is ever sent to another
// daemon. It takes no lock: the pin read runs git.
func resolveEntry(ctx context.Context, deadline time.Duration, workspace string, catalog Catalog, current string) (string, CatalogEntry) {
	if current != "" || os.Getenv("KATA_SERVER") != "" {
		entry, _ := catalog.Entry(current)
		return current, entry
	}
	// Its own bounded context: the remainder of the connect's after locate and dial may be too short for git, and a failed
	// tracked-file check inside a worktree would leave a valid pin unnamed.
	pinCtx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	pin, err := workspacePin(pinCtx, workspace)
	if err != nil || pin.URL != "" {
		return "", CatalogEntry{}
	}
	name := pin.Daemon
	if name == "" {
		name = catalog.Active
	}
	entry, found := catalog.Entry(name)
	if !found || pin.Daemon == "" && entry.Local && entry.Home == "" {
		return "", CatalogEntry{}
	}
	return name, entry
}

// workspacePin reads [server] of the .kata.local.toml kata honours for workspace (the working directory when empty), by kata's
// rules: the file counts only at or below the nearest directory holding .kata.toml or .git, never when git tracks it, and a file
// that does not parse (or names both url and daemon) is an error, as it aborts kata's own resolution.
// shortcut: kata's public client does not say which daemon it chose, so the pin is read again here; delete it once it does.
func workspacePin(ctx context.Context, workspace string) (pin struct{ URL, Daemon string }, err error) {
	dir := workspace
	if dir == "" {
		if dir, err = os.Getwd(); err != nil {
			return pin, err
		}
	}
	root := ""
	for {
		if root == "" {
			if _, statErr := os.Stat(filepath.Join(dir, ".kata.local.toml")); statErr == nil {
				root = dir
			}
		}
		if hasMarker(dir, ".kata.toml") || hasMarker(dir, ".git") {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return pin, nil
		}
		dir = parent
	}
	if root == "" {
		return pin, nil
	}
	file := filepath.Join(root, ".kata.local.toml")
	tracked, determined := localConfigTracked(ctx, root)
	switch {
	case determined && tracked:
		return pin, nil
	case !determined && gitWorktreePresent(root):
		return pin, fmt.Errorf("cannot verify the git-tracked status of %s", file)
	}
	var cfg struct {
		Version int `toml:"version"`
		Server  struct {
			URL    string `toml:"url"`
			Daemon string `toml:"daemon"`
		} `toml:"server"`
	}
	if _, err := toml.DecodeFile(file, &cfg); err != nil {
		return pin, err
	}
	pin.URL, pin.Daemon = strings.TrimSpace(cfg.Server.URL), strings.TrimSpace(cfg.Server.Daemon)
	if cfg.Version != 1 || pin.URL != "" && pin.Daemon != "" {
		return pin, fmt.Errorf("%s: unsupported version or both server.url and server.daemon", file)
	}
	return pin, nil
}

func hasMarker(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

// localConfigTracked asks git whether root's .kata.local.toml is committed, as kata does: the same git runner (no inherited GIT_*
// variables, no global or system config, safe.directory honoured) and a case-insensitive pathspec scoped to the one file.
// determined is false when git could not answer, including when ctx ends first.
func localConfigTracked(ctx context.Context, root string) (tracked, determined bool) {
	out, err := projectGit.Output(ctx, root, "ls-files", "-z", "--", ":(icase).kata.local.toml")
	if err != nil {
		return false, false
	}
	for entry := range strings.SplitSeq(string(out), "\x00") {
		if entry != "" && strings.EqualFold(entry[strings.LastIndex(entry, "/")+1:], ".kata.local.toml") {
			return true, true
		}
	}
	return false, true
}

// gitWorktreePresent reports whether dir sits in a git worktree by walking for .git, on both the lexical and symlink-resolved path;
// an unresolvable path counts as inside one.
func gitWorktreePresent(dir string) bool {
	if dotGitInAncestors(dir) {
		return true
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return true
	}
	return resolved != dir && dotGitInAncestors(resolved)
}

func dotGitInAncestors(dir string) bool {
	for {
		if hasMarker(dir, ".git") {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
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
