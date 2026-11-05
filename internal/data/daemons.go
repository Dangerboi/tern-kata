package data

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.kenn.io/kata/pkg/client"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// startDeadline is how long connecting to a named daemon may take when kata starts a stopped local one first; kata itself waits up to five seconds for the daemon to answer.
const startDeadline = 30 * time.Second

// currentDaemon is the catalog name of the daemon workspace talks to: the entry the Gateway last switched to, else the name its live
// connection was resolved to (a workspace pin can differ from active_daemon, and a url pin names no entry), else what a connection
// would resolve to now: KATA_SERVER names no entry, otherwise the workspace pin or active_daemon by the same rules the Pane's own
// connection follows. "" when the connection is not a catalog entry.
func (g *Gateway) currentDaemon(ctx context.Context, catalog Catalog, workspace string) string {
	g.mu.Lock()
	current, c := g.current, g.clients[workspace]
	g.mu.Unlock()
	if current == "" && os.Getenv("KATA_SERVER") == "" && c != nil {
		return c.name
	}
	// The pin read runs git, so it stays outside g.mu.
	name, _ := resolveEntry(ctx, g.deadline, workspace, catalog, current)
	return name
}

// loadDaemons lists kata's daemon catalog for the Daemons Page. The names, endpoints and token flags are kata's own; allow_insecure
// is not in kata's client, so it comes from the file tern-kata reads for the agent route, and is left out when that read fails.
func (g *Gateway) loadDaemons(ctx context.Context, cmd model.LoadDaemons) model.Msg {
	home, err := kataHome()
	if err != nil {
		return model.DaemonsLoaded{Request: cmd.Request, Err: err}
	}
	listed, err := client.DaemonCatalog()
	if err != nil {
		return model.DaemonsLoaded{Request: cmd.Request, Err: err}
	}
	catalog, _ := ReadCatalog()
	current := g.currentDaemon(ctx, catalog, cmd.Workspace)
	loaded := model.DaemonsLoaded{Request: cmd.Request, ConfigPath: filepath.Join(home, "config.toml")}
	for _, d := range listed {
		entry, _ := catalog.Entry(d.Name)
		loaded.Entries = append(loaded.Entries, model.DaemonEntry{
			Name: d.Name, Endpoint: d.Endpoint, Token: d.HasToken, AllowInsecure: entry.AllowInsecure, Current: d.Name == current,
		})
	}
	return loaded
}

// switchDaemon connects to the catalog entry cmd names through kata's client and, only once that works, makes it the connection of
// every workspace and ends the event stream of the old one. kata starts a stopped local entry first. Nothing is written to disk.
// A failure changes nothing, so the old connection stays.
func (g *Gateway) switchDaemon(ctx context.Context, cmd model.SwitchDaemon) model.Msg {
	fail := func(err error) model.Msg {
		if errors.Is(err, client.ErrDaemonNotInCatalog) {
			err = errors.New("not in kata's daemon catalog")
		}
		prefix := fmt.Sprintf("daemon %q: ", cmd.Name)
		return model.DaemonSwitched{Request: cmd.Request, Name: cmd.Name, Err: errors.New(strings.TrimPrefix(err.Error(), prefix))}
	}
	discoverCtx, cancel := context.WithTimeout(ctx, startDeadline)
	defer cancel()
	target, err := LocateNamed(cmd.Name, kataExecutable())(discoverCtx, cmd.Workspace)
	if err != nil {
		return fail(err)
	}
	// The entry is for the connection's credentials-by-name display and the agent route; kata's client does not expose it.
	catalog, err := ReadCatalog()
	if err != nil {
		return fail(err)
	}
	entry, _ := catalog.Entry(cmd.Name)
	connectCtx, cancelConnect := context.WithTimeout(ctx, g.deadline)
	defer cancelConnect()
	c, err := connect(connectCtx, target)
	if err != nil {
		return fail(err)
	}
	c.name, c.entry, c.deadline = cmd.Name, entry, g.deadline
	g.install(cmd.Name, LocateNamed(cmd.Name, ""), cmd.Workspace, c)
	return model.DaemonSwitched{Request: cmd.Request, Name: cmd.Name, Daemon: c.describe(ctx)}
}

// install makes c, the connection to the catalog entry called name, the one every workspace uses from now on: the old daemon's
// pointer passes are ended and awaited first, so none can still write to it through the connection they hold; then the cached
// clients of the old daemon go, a reconnect locates the new one, and the old daemon's event stream ends.
func (g *Gateway) install(name string, locate Locator, workspace string, c *Client) {
	g.startMu.Lock()
	g.connCancel()
	g.background.Wait()
	g.mu.Lock()
	g.locate, g.current = locate, name
	g.epoch++
	clear(g.clients)
	g.clients[workspace] = c
	g.mu.Unlock()
	g.connCtx, g.connCancel = context.WithCancel(g.backgroundCtx)
	clear(g.convertStarted)
	g.startMu.Unlock()
	g.streamMu.Lock()
	if g.streamCancel != nil {
		g.streamCancel()
	}
	g.streamMu.Unlock()
}

// describe is the connection as the top bar names it: its catalog name, where it runs, its version, and the account the daemon says
// the Pane acts as. The account is best effort: a daemon that does not say leaves it empty.
func (c *Client) describe(ctx context.Context) model.Daemon {
	daemon := model.Daemon{Name: c.name, Kind: c.kind, Version: c.version}
	deadline := c.deadline
	if deadline == 0 {
		deadline = connectDeadline
	}
	ctx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	if instance, err := c.InstanceWithResponse(ctx); err == nil && instance.JSON200 != nil {
		if actor := instance.JSON200.Auth.Actor; actor != nil {
			daemon.Account = strings.TrimSpace(*actor)
		}
	}
	return daemon
}
