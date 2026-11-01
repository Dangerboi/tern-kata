package data

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// AgentRoute is the kata daemon the Pane talks to, as far as it differs from what a block Tern starts would pick by itself. Tern's
// daemon gives a block its own environment, not the Pane's, so `tern-kata agent` sets the kata variables from the route before it
// runs the agent: the agent's KATA_REF then names an Issue on the daemon its own `kata` commands reach.
// No field is a secret: a token never rides on the command line, which Tern shows as the block's title and a process list shows to
// everyone. The wrapper resolves a catalog entry's token itself; TokenFile names a file, not its content.
type AgentRoute struct {
	// Home is the Pane's KATA_HOME; empty when the Pane has none.
	Home string
	// Daemon is the catalog entry the Pane switched to on the Daemons Page. It outranks Server, as it does for the Pane.
	Daemon string
	// Server is the Pane's KATA_SERVER, with TokenFile its KATA_AUTH_TOKEN_FILE and AllowInsecure its KATA_ALLOW_INSECURE.
	Server        string
	TokenFile     string
	AllowInsecure bool
}

// IsZero reports whether the route changes nothing: the agent starts exactly as it did before routing existed.
func (r AgentRoute) IsZero() bool { return r == AgentRoute{} }

// Args is the route as `tern-kata agent` flags; none for a zero route.
func (r AgentRoute) Args() []string {
	var args []string
	if r.Home != "" {
		args = append(args, "--home", r.Home)
	}
	if r.Daemon != "" {
		args = append(args, "--daemon", r.Daemon)
	}
	if r.Server != "" {
		args = append(args, "--server", r.Server)
	}
	if r.TokenFile != "" {
		args = append(args, "--token-file", r.TokenFile)
	}
	if r.AllowInsecure {
		args = append(args, "--allow-insecure")
	}
	return args
}

// CurrentAgentRoute is the route of the daemon the Pane talks to now, by the order the Pane's own connection follows: the entry it
// switched to, else KATA_SERVER; otherwise the daemon KATA_HOME (or the default home) selects, which the agent resolves the same
// way once it has the same KATA_HOME. A Pane on the default daemon has a zero route.
// The Pane's own token (KATA_AUTH_TOKEN) is not carried: the agent keeps whatever its block inherited, as it always did.
func (g *Gateway) CurrentAgentRoute() AgentRoute {
	g.mu.Lock()
	current := g.current
	g.mu.Unlock()
	route := AgentRoute{Home: os.Getenv("KATA_HOME"), Daemon: current}
	if current != "" {
		return route
	}
	if route.Server = os.Getenv("KATA_SERVER"); route.Server != "" {
		route.TokenFile = strings.TrimSpace(os.Getenv("KATA_AUTH_TOKEN_FILE"))
		route.AllowInsecure, _ = strconv.ParseBool(strings.TrimSpace(os.Getenv("KATA_ALLOW_INSECURE")))
	}
	return route
}

// Env resolves the route into what the agent's environment must say: set holds NAME=value entries, and unset the names to drop first
// (an inherited KATA_SERVER or token that would point the agent at another daemon, or hand it another daemon's credential).
//
// KATA_SERVER outranks a checkout's .kata.local.toml pin in kata's resolution (`kata --help`: "KATA_SERVER, workspace,
// active_daemon, local"), so a remote route wins over the pin of the agent's checkout: the agent talks to the Pane's daemon, the one
// its KATA_REF names. kata has no environment variable that names a local daemon, and KATA_SERVER takes only http(s), so a local
// entry is expressed by KATA_HOME alone: the entry's own home when it has one, else the current home, which must already select that
// entry; a local route a checkout's pin could still override is not expressible, and one that cannot be reached by KATA_HOME at all
// is refused rather than started against another daemon.
func (r AgentRoute) Env() (set, unset []string, err error) {
	if r.IsZero() {
		return nil, nil, nil
	}
	if r.Home != "" {
		set = append(set, "KATA_HOME="+r.Home)
	}
	if r.Daemon == "" {
		if r.Server != "" {
			set = append(set, "KATA_SERVER="+r.Server)
		}
		if r.TokenFile != "" {
			unset = append(unset, "KATA_AUTH_TOKEN")
			set = append(set, "KATA_AUTH_TOKEN_FILE="+r.TokenFile)
		}
		if r.AllowInsecure {
			set = append(set, "KATA_ALLOW_INSECURE=1")
		}
		return set, unset, nil
	}
	home := r.Home
	if home == "" {
		if home, err = kataHome(); err != nil {
			return nil, nil, err
		}
	}
	catalog, err := ReadCatalogAt(home)
	if err != nil {
		return nil, nil, err
	}
	entry, found := catalog.Entry(r.Daemon)
	if !found {
		return nil, nil, fmt.Errorf("daemon %q is not in the catalog of %s", r.Daemon, home)
	}
	unset = append(unset, "KATA_SERVER", "KATA_AUTH_TOKEN", "KATA_AUTH_TOKEN_FILE", "KATA_ALLOW_INSECURE")
	if !entry.Local {
		token, err := entry.Token()
		if err != nil {
			return nil, nil, err
		}
		set = append(set, "KATA_SERVER="+entry.URL)
		if token != "" {
			set = append(set, "KATA_AUTH_TOKEN="+token)
		}
		if entry.AllowInsecure {
			set = append(set, "KATA_ALLOW_INSECURE=1")
		}
		return set, unset, nil
	}
	if entry.Home != "" {
		return []string{"KATA_HOME=" + entry.Home}, unset, nil // the entry's own home replaces the Pane's: it holds the daemon's state
	}
	if active, _ := catalog.Entry(catalog.Active); catalog.Active != "" && catalog.Active != entry.Name && (!active.Local || active.Home != "") {
		return nil, nil, fmt.Errorf("daemon %q is a local daemon of %s, but the agent's kata would use active_daemon %q; kata has no variable that names a local daemon", entry.Name, home, catalog.Active)
	}
	return set, unset, nil
}
