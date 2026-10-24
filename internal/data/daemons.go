package data

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// startDeadline is how long starting a stopped local daemon may take; kata itself waits up to five seconds for the daemon to answer.
const startDeadline = 30 * time.Second

// currentDaemon is the catalog name of the daemon this Gateway talks to: the entry it last switched to, else the catalog's
// active_daemon, which kata's own discovery honours unless KATA_SERVER names a daemon. "" when the connection is not a catalog entry.
func (g *Gateway) currentDaemon(catalog Catalog) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.currentLocked(catalog)
}

func (g *Gateway) currentLocked(catalog Catalog) string {
	if g.current != "" {
		return g.current
	}
	if os.Getenv("KATA_SERVER") != "" {
		return ""
	}
	return catalog.Active
}

// loadDaemons reads the catalog for the Daemons Page.
func (g *Gateway) loadDaemons(_ context.Context, cmd model.LoadDaemons) model.Msg {
	home, err := kataHome()
	if err != nil {
		return model.DaemonsLoaded{Request: cmd.Request, Err: err}
	}
	catalog, err := ReadCatalog()
	if err != nil {
		return model.DaemonsLoaded{Request: cmd.Request, Err: err}
	}
	current := g.currentDaemon(catalog)
	loaded := model.DaemonsLoaded{Request: cmd.Request, ConfigPath: filepath.Join(home, "config.toml")}
	for _, e := range catalog.Daemons {
		loaded.Entries = append(loaded.Entries, model.DaemonEntry{
			Name: e.Name, Endpoint: e.endpoint(), Token: e.InlineToken != "" || e.TokenEnv != "",
			AllowInsecure: e.AllowInsecure, Current: e.Name == current,
		})
	}
	return loaded
}

// endpoint is "local" for a local entry, else the host of its url, as kata tui shows it.
func (e CatalogEntry) endpoint() string {
	if e.Local {
		return "local"
	}
	if u, err := url.Parse(e.URL); err == nil && u.Host != "" {
		return u.Host
	}
	return e.URL
}

// switchDaemon connects to the catalog entry cmd names and, only once that works, makes it the connection of every workspace and
// ends the event stream of the old one. A stopped local entry is started first: locating it fails while it is stopped. Nothing
// is written to disk. A failure changes nothing, so the old connection stays.
func (g *Gateway) switchDaemon(ctx context.Context, cmd model.SwitchDaemon) model.Msg {
	fail := func(err error) model.Msg {
		prefix := fmt.Sprintf("daemon %q: ", cmd.Name)
		return model.DaemonSwitched{Request: cmd.Request, Name: cmd.Name, Err: errors.New(strings.TrimPrefix(err.Error(), prefix))}
	}
	catalog, err := ReadCatalog()
	if err != nil {
		return fail(err)
	}
	entry, found := catalog.Entry(cmd.Name)
	if !found {
		return fail(errors.New("not in kata's daemon catalog"))
	}
	locate := LocateEntry(entry)
	located := func() (Target, error) {
		locateCtx, cancel := context.WithTimeout(ctx, g.deadline)
		defer cancel()
		return locate(locateCtx, cmd.Workspace)
	}
	target, err := located()
	if err != nil && entry.Local {
		if err = startLocalDaemon(ctx, entry); err == nil {
			target, err = located()
		}
	}
	if err != nil {
		return fail(err)
	}
	connectCtx, cancel := context.WithTimeout(ctx, g.deadline)
	defer cancel()
	c, err := connect(connectCtx, target)
	if err != nil {
		return fail(err)
	}
	c.name = entry.Name
	g.install(entry.Name, locate, cmd.Workspace, c)
	return model.DaemonSwitched{Request: cmd.Request, Name: entry.Name, Daemon: c.describe(ctx)}
}

// install makes c, the connection to the catalog entry called name, the one every workspace uses from now on: the cached clients of
// the old daemon go, a reconnect locates the new one, and the old daemon's event stream and pointer passes end.
func (g *Gateway) install(name string, locate Locator, workspace string, c *Client) {
	g.mu.Lock()
	g.locate, g.current = locate, name
	clear(g.clients)
	g.clients[workspace] = c
	g.mu.Unlock()
	g.streamMu.Lock()
	if g.streamCancel != nil {
		g.streamCancel()
	}
	g.streamMu.Unlock()
	g.startMu.Lock()
	clear(g.convertStarted)
	g.startMu.Unlock()
}

// startLocalDaemon starts the local daemon entry names, as kata tui does for a deliberate switch to a stopped one: kata's own
// `kata daemon start`, with KATA_HOME set to the profile's home when the entry has one and the current home otherwise. The child
// gets no KATA_DSN, KATA_DB or KATA_SERVER, which would point the command at another database or daemon than the one the entry names.
func startLocalDaemon(ctx context.Context, entry CatalogEntry) error {
	ctx, cancel := context.WithTimeout(ctx, startDeadline)
	defer cancel()
	cmd := exec.CommandContext(ctx, "kata", "daemon", "start")
	cmd.Env = startEnvironment(os.Environ(), entry.Home)
	// The daemon is detached, but a pipe it inherited would hold the output open: give up on it shortly after kata exits.
	cmd.WaitDelay = 2 * time.Second
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Run(); err != nil {
		if text := strings.TrimSpace(output.String()); text != "" {
			return fmt.Errorf("kata daemon start: %s", text)
		}
		return fmt.Errorf("kata daemon start: %w", err)
	}
	return nil
}

// startEnvironment is env without the variables that point kata at another database or daemon, and with KATA_HOME set to home when
// home is not empty.
func startEnvironment(env []string, home string) []string {
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		switch name {
		case "KATA_DSN", "KATA_DB", "KATA_SERVER":
			continue
		case "KATA_HOME":
			if home != "" {
				continue
			}
		}
		out = append(out, kv)
	}
	if home != "" {
		out = append(out, "KATA_HOME="+home)
	}
	return out
}

// describe is the connection as the top bar names it: its catalog name, where it runs, its version, and the account the daemon says
// the Pane acts as. The account is best effort: a daemon that does not say leaves it empty.
func (c *Client) describe(ctx context.Context) model.Daemon {
	daemon := model.Daemon{Name: c.name, Kind: c.kind, Version: c.version}
	if instance, err := c.InstanceWithResponse(ctx); err == nil && instance.JSON200 != nil {
		if actor := instance.JSON200.Auth.Actor; actor != nil {
			daemon.Account = strings.TrimSpace(*actor)
		}
	}
	return daemon
}
