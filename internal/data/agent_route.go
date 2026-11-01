package data

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// AgentRoute is the kata daemon the Pane talks to, as far as it differs from what a block Tern starts would pick by itself. Tern's
// daemon gives a block its own environment, not the Pane's, so `tern-kata agent` sets the kata variables from the route before it
// runs the agent: the agent's KATA_REF then names an Issue on the daemon its own `kata` commands reach.
// No flag carries a secret: a token never rides on the command line, which Tern shows as the block's title and a process list shows to
// everyone. The Pane writes the token it uses to a private file (startAgent) and the route names that file; the wrapper reads it.
type AgentRoute struct {
	// Home is the Pane's KATA_HOME; empty when the Pane has none.
	Home string
	// Daemon is the catalog entry the Pane switched to on the Daemons Page. It outranks Server, as it does for the Pane.
	Daemon string
	// Server is the Pane's KATA_SERVER, with AllowInsecure its KATA_ALLOW_INSECURE.
	Server        string
	AllowInsecure bool
	// TokenFile is the private file holding the bearer token the Pane uses for that daemon: a copy startAgent writes, or the Pane's own
	// KATA_AUTH_TOKEN_FILE. Empty when the Pane uses none.
	TokenFile string
	// Token is the token itself, in the Pane only: Args leaves it out, and startAgent moves it into TokenFile before the route is used.
	Token string
}

// IsZero reports whether the route changes nothing: the agent starts exactly as it did before routing existed.
func (r AgentRoute) IsZero() bool { return r == AgentRoute{} }

// zeroWithoutToken reports whether the route would change nothing but for the credentials the Pane has.
func (r AgentRoute) zeroWithoutToken() bool { r.Token, r.TokenFile = "", ""; return r.IsZero() }

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
// way once it has the same KATA_HOME. A Pane on the default daemon has a zero route, which starts the agent exactly as before.
// Once the route is not zero the agent gets the credential the Pane uses: a switched remote entry's token (its inline token or what its
// token_env holds in the Pane's environment), else the Pane's KATA_AUTH_TOKEN or KATA_AUTH_TOKEN_FILE. A token_env the Pane
// cannot resolve is an error, as it is for the Pane's own connection.
func (g *Gateway) CurrentAgentRoute() (AgentRoute, error) {
	g.mu.Lock()
	current := g.current
	g.mu.Unlock()
	route := AgentRoute{Home: os.Getenv("KATA_HOME"), Daemon: current}
	switch {
	case current != "":
		catalog, err := ReadCatalog()
		if err != nil {
			return AgentRoute{}, err
		}
		entry, found := catalog.Entry(current)
		if !found {
			return AgentRoute{}, fmt.Errorf("daemon %q is not in the daemon catalog", current)
		}
		if !entry.Local {
			if route.Token, err = entry.Token(); err != nil {
				return AgentRoute{}, err
			}
		}
		return route, nil
	case os.Getenv("KATA_SERVER") != "":
		route.Server = os.Getenv("KATA_SERVER")
		route.AllowInsecure = envTruthy("KATA_ALLOW_INSECURE")
	}
	if route.zeroWithoutToken() {
		return route, nil
	}
	if route.Token = strings.TrimSpace(os.Getenv("KATA_AUTH_TOKEN")); route.Token == "" {
		route.TokenFile = strings.TrimSpace(os.Getenv("KATA_AUTH_TOKEN_FILE"))
	}
	return route, nil
}

// envTruthy is kata's reading of KATA_ALLOW_INSECURE: "1" or "true" (any case), with surrounding space ignored.
func envTruthy(name string) bool {
	v := strings.TrimSpace(os.Getenv(name))
	return v == "1" || strings.EqualFold(v, "true")
}

// agentTokenSuffix names the token file beside a prompt file; removePrompt removes both.
const agentTokenSuffix = ".token"

// withTokenFile moves route.Token into a new private file beside promptFile and names it in route.TokenFile. A route with no Token
// is returned as it is.
func (r AgentRoute) withTokenFile(promptFile string) (AgentRoute, error) {
	if r.Token == "" {
		return r, nil
	}
	file := promptFile + agentTokenSuffix
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return r, err
	}
	_, err = f.WriteString(r.Token + "\n")
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(file)
		return r, fmt.Errorf("write the agent's token file: %w", err)
	}
	r.Token, r.TokenFile = "", file
	return r, nil
}

// Env resolves the route into what the agent's environment must say: set holds NAME=value entries, and unset the names to drop first.
// A route that is not zero always drops the block's own KATA_SERVER, KATA_AUTH_TOKEN, KATA_AUTH_TOKEN_FILE and KATA_ALLOW_INSECURE,
// so what the block inherited (the Tern daemon's default remote, and its credential) neither picks the daemon nor is sent to the
// Pane's. The token goes to the agent as KATA_AUTH_TOKEN, which kata ranks above every other credential source ([auth] in config.toml
// included), read from the route's token file.
//
// KATA_SERVER outranks a checkout's .kata.local.toml pin in kata's resolution (`kata --help`: "KATA_SERVER, workspace,
// active_daemon, local"), so a remote route wins over the pin of the agent's checkout: the agent talks to the Pane's daemon, the one
// its KATA_REF names. kata has no environment variable that names a local daemon, and KATA_SERVER takes only http(s), so a local
// entry is expressed by KATA_HOME alone: the entry's own home when it has one, else the current home, which must already select that
// entry. Such a route is refused when the checkout's pin would win over it (a url, or another daemon's name), when the home's
// active_daemon selects another daemon, and, for a remote entry with no token of its own, when the home's [auth] section holds a
// credential, which kata sends to every KATA_SERVER and the Pane does not send to an entry that carries none.
func (r AgentRoute) Env() (set, unset []string, err error) {
	if r.IsZero() {
		return nil, nil, nil
	}
	unset = []string{"KATA_SERVER", "KATA_AUTH_TOKEN", "KATA_AUTH_TOKEN_FILE", "KATA_ALLOW_INSECURE"}
	if r.Home != "" {
		set = append(set, "KATA_HOME="+r.Home)
	}
	token := ""
	if r.TokenFile != "" {
		if token, err = readTokenFile(r.TokenFile, false); err != nil {
			return nil, nil, err
		}
	}
	if r.Daemon == "" {
		if r.Server != "" {
			set = append(set, "KATA_SERVER="+r.Server)
		}
		if token != "" {
			set = append(set, "KATA_AUTH_TOKEN="+token)
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
	if !entry.Local {
		if token == "" {
			if token, err = entry.Token(); err != nil {
				return nil, nil, err
			}
		}
		if token == "" {
			if credential, err := homeAuthCredential(home); err != nil || credential != "" {
				return nil, nil, cmp.Or(err, fmt.Errorf("daemon %q carries no token, but %s has an [auth] %s that kata would send to it", entry.Name, filepath.Join(home, "config.toml"), credential))
			}
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
	if err := checkLocalRoute(catalog, entry, home); err != nil {
		return nil, nil, err
	}
	if entry.Home != "" {
		return []string{"KATA_HOME=" + entry.Home}, unset, nil // the entry's own home replaces the Pane's: it holds the daemon's state
	}
	return set, unset, nil
}

// checkLocalRoute refuses a local route that a variable cannot express: the checkout's pin would win over it, or the home's
// active_daemon selects another daemon than the entry.
func checkLocalRoute(catalog Catalog, entry CatalogEntry, home string) error {
	ctx, cancel := context.WithTimeout(context.Background(), connectDeadline)
	defer cancel()
	pin, err := workspacePin(ctx, "")
	if err != nil {
		return err
	}
	if pin.URL != "" || pin.Daemon != "" && (pin.Daemon != entry.Name || entry.Home != "") {
		return fmt.Errorf("daemon %q is a local daemon, which kata names only through KATA_HOME, and this checkout's .kata.local.toml pins %s; the pin would win", entry.Name, cmp.Or(pin.URL, "daemon "+pin.Daemon))
	}
	if entry.Home == "" && catalog.Active != "" && catalog.Active != entry.Name {
		if active, _ := catalog.Entry(catalog.Active); !active.Local || active.Home != "" {
			return fmt.Errorf("daemon %q is a local daemon of %s, but the agent's kata would use active_daemon %q; kata has no variable that names a local daemon", entry.Name, home, catalog.Active)
		}
	}
	return nil
}

// homeAuthCredential names the [auth] credential key (token, token_file or token_env) config.toml in home sets, "" when it sets none.
func homeAuthCredential(home string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(home, "config.toml"))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var f struct {
		Auth struct {
			Token     string `toml:"token"`
			TokenFile string `toml:"token_file"`
			TokenEnv  string `toml:"token_env"`
		} `toml:"auth"`
	}
	if _, err := toml.Decode(string(raw), &f); err != nil {
		return "", err
	}
	for _, key := range []struct{ name, value string }{{"token", f.Auth.Token}, {"token_file", f.Auth.TokenFile}, {"token_env", f.Auth.TokenEnv}} {
		if strings.TrimSpace(key.value) != "" {
			return key.name, nil
		}
	}
	return "", nil
}
