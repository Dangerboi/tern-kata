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
	// Server is the Pane's KATA_SERVER, with AllowInsecure its KATA_ALLOW_INSECURE and TrustPrivate its KATA_TRUST_PRIVATE_NETWORK (read as
	// kata reads them: "1" or "true"). kata's client honours both when it connects, so the agent must see the same values.
	Server        string
	AllowInsecure bool
	TrustPrivate  bool
	// TokenFile is the private file startAgent writes for the bearer token the Pane uses (a copy: the Pane's KATA_AUTH_TOKEN, or a switched
	// entry's token). The wrapper reads it into the agent's KATA_AUTH_TOKEN and deletes it.
	TokenFile string
	// AuthFile is the Pane's own KATA_AUTH_TOKEN_FILE, passed on as it is (a path, never read or deleted here).
	AuthFile string
	// Token is the token itself, in the Pane only: Args leaves it out, and startAgent moves it into TokenFile before the route is used.
	Token string
}

// IsZero reports whether the route changes nothing: the agent starts exactly as it did before routing existed.
func (r AgentRoute) IsZero() bool { return r == AgentRoute{} }

// zeroWithoutToken reports whether the route would change nothing but for the credentials the Pane has.
func (r AgentRoute) zeroWithoutToken() bool {
	r.Token, r.TokenFile, r.AuthFile, r.TrustPrivate = "", "", "", false
	return r.IsZero()
}

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
	if r.AuthFile != "" {
		args = append(args, "--auth-file", r.AuthFile)
	}
	if r.AllowInsecure {
		args = append(args, "--allow-insecure")
	}
	if r.TrustPrivate {
		args = append(args, "--trust-private-network")
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
		if entry.Local {
			route.paneCredential() // a local entry with no token of its own is reached with the Pane's, as the Pane connects to it
			return route, nil
		}
		if route.Token, err = entry.Token(); err != nil {
			return AgentRoute{}, err
		}
		return route, nil
	case os.Getenv("KATA_SERVER") != "":
		route.Server = os.Getenv("KATA_SERVER")
		route.AllowInsecure = envTruthy("KATA_ALLOW_INSECURE")
	}
	if route.zeroWithoutToken() {
		return route, nil
	}
	route.TrustPrivate = envTruthy("KATA_TRUST_PRIVATE_NETWORK")
	route.paneCredential()
	return route, nil
}

// paneCredential sets the Pane's own bearer credential on the route: its KATA_AUTH_TOKEN, else its KATA_AUTH_TOKEN_FILE.
func (r *AgentRoute) paneCredential() {
	if r.Token = strings.TrimSpace(os.Getenv("KATA_AUTH_TOKEN")); r.Token == "" {
		r.AuthFile = strings.TrimSpace(os.Getenv("KATA_AUTH_TOKEN_FILE"))
	}
}

// envTruthy is kata's reading of KATA_ALLOW_INSECURE and KATA_TRUST_PRIVATE_NETWORK: "1" or "true" (any case), with surrounding space ignored.
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
// dir is the agent's checkout ("" for the working directory), whose .kata.local.toml pin kata reads.
// A route that is not zero always drops the block's own KATA_SERVER, KATA_AUTH_TOKEN, KATA_AUTH_TOKEN_FILE, KATA_ALLOW_INSECURE and
// KATA_TRUST_PRIVATE_NETWORK, so what the block inherited (the Tern daemon's default remote, its credential, its trust settings) neither
// picks the daemon nor is sent to the Pane's. The Pane's token goes to the agent as KATA_AUTH_TOKEN, which kata ranks above every other
// credential source ([auth] in config.toml included), read from the route's token file, which Env deletes once it has read it.
// The Pane's own KATA_AUTH_TOKEN_FILE goes on as KATA_AUTH_TOKEN_FILE, so kata ranks it as it ranks it for the Pane.
//
// KATA_SERVER outranks a checkout's .kata.local.toml pin in kata's resolution (`kata --help`: "KATA_SERVER, workspace,
// active_daemon, local"), so a remote route wins over the pin of the agent's checkout: the agent talks to the Pane's daemon, the one
// its KATA_REF names. kata has no environment variable that names a local daemon, and KATA_SERVER takes only http(s), so a local
// entry is expressed by KATA_HOME alone: the entry's own home when it has one, else the current home, which must already select that
// entry. Such a route is refused when the checkout's pin would win over it (a url, a daemon that is not this profile, or a daemon pin
// at all for an entry with no home, which kata rejects), and when the home's active_daemon selects another daemon. A pin that names
// the profile itself already selects it in the Pane's home, which then stays. A remote entry with no token of its own is refused when
// the home's [auth] section holds a credential, which kata sends to every KATA_SERVER and the Pane does not send to such an entry.
func (r AgentRoute) Env(dir string) (set, unset []string, err error) {
	return r.resolve(dir, true)
}

// Preflight runs the refusals Env makes, for the Pane to report before it starts a block, without reading or deleting a token file.
func (r AgentRoute) Preflight(dir string) error {
	_, _, err := r.resolve(dir, false)
	return err
}

func (r AgentRoute) resolve(dir string, wrapper bool) (set, unset []string, err error) {
	if r.IsZero() {
		return nil, nil, nil
	}
	unset = []string{"KATA_SERVER", "KATA_AUTH_TOKEN", "KATA_AUTH_TOKEN_FILE", "KATA_ALLOW_INSECURE", "KATA_TRUST_PRIVATE_NETWORK"}
	if r.Home != "" {
		set = append(set, "KATA_HOME="+r.Home)
	}
	token := r.Token
	if r.TokenFile != "" && wrapper {
		token, err = readTokenFile(r.TokenFile, false)
		_ = os.Remove(r.TokenFile) // the file's job is done: it must not outlive the agent's start
		if err != nil {
			return nil, nil, err
		}
	}
	credential := func() {
		if token != "" {
			set = append(set, "KATA_AUTH_TOKEN="+token)
		}
		if r.AuthFile != "" {
			set = append(set, "KATA_AUTH_TOKEN_FILE="+r.AuthFile)
		}
	}
	if r.Daemon == "" {
		if r.Server != "" {
			set = append(set, "KATA_SERVER="+r.Server)
		}
		credential()
		if r.AllowInsecure {
			set = append(set, "KATA_ALLOW_INSECURE=1")
		}
		if r.TrustPrivate {
			set = append(set, "KATA_TRUST_PRIVATE_NETWORK=1")
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
	pinned, err := checkLocalRoute(catalog, entry, home, dir)
	if err != nil {
		return nil, nil, err
	}
	if entry.Home != "" && !pinned {
		set = []string{"KATA_HOME=" + entry.Home} // the entry's own home replaces the Pane's: it holds the daemon's state
	}
	credential()
	return set, unset, nil
}

// checkLocalRoute refuses a local route that a variable cannot express: the checkout's pin would win over it, or the home's
// active_daemon selects another daemon than the entry. pinned reports that the checkout's pin names this profile, which the Pane's
// home already selects.
func checkLocalRoute(catalog Catalog, entry CatalogEntry, home, dir string) (pinned bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), connectDeadline)
	defer cancel()
	pin, err := workspacePin(ctx, dir)
	if err != nil {
		return false, err
	}
	pinned = pin.Daemon != "" && pin.Daemon == entry.Name && entry.Home != ""
	if pin.URL != "" || pin.Daemon != "" && !pinned {
		return false, fmt.Errorf("daemon %q is a local daemon, which kata names only through KATA_HOME, and this checkout's .kata.local.toml pins %s; the pin would win", entry.Name, cmp.Or(pin.URL, "daemon "+pin.Daemon))
	}
	if !pinned && entry.Home == "" && catalog.Active != "" && catalog.Active != entry.Name {
		if active, _ := catalog.Entry(catalog.Active); !active.Local || active.Home != "" {
			return false, fmt.Errorf("daemon %q is a local daemon of %s, but the agent's kata would use active_daemon %q; kata has no variable that names a local daemon", entry.Name, home, catalog.Active)
		}
	}
	return pinned, nil
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
