package data

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"go.kenn.io/kit/safefileio"
)

// AgentRoute is the kata daemon the Pane talks to, as far as it differs from what a block Tern starts would pick by itself. Tern's
// daemon gives a block its own environment, not the Pane's, so `tern-kata agent` sets the kata variables from the route before it
// runs the agent: the agent's KATA_REF then names an Issue on the daemon its own `kata` commands reach.
// No flag carries a secret: a token never rides on the command line, which Tern shows as the block's title and a process list shows to
// everyone. The Pane writes the token it uses to a private file (startAgent) and the route names that file; the wrapper reads it.
type AgentRoute struct {
	// Home is the Pane's KATA_HOME, or the default home it resolves to once the route is not zero; empty for a zero route.
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
	// DB, Schema and DSN are the Pane's KATA_DB (made absolute), KATA_POSTGRES_SCHEMA and KATA_DSN, carried only to a route that keeps the
	// Pane's own home: kata hashes them into <KATA_HOME>/runtime/<hash> and starts a daemon when none runs there, so the block's own
	// values must never mix with another home. A DSN may hold a password, so it travels like the token: DSN is the Pane's copy, which
	// startAgent moves into DSNFile, a private file the wrapper reads into KATA_DSN and deletes.
	DB, Schema, DSN, DSNFile string
	// Creds are the Pane's values of the variables the home's config.toml names as credential sources (each [[daemon]] token_env and
	// [auth].token_env), as NAME=value lines; startAgent moves them into EnvFile, a private file the wrapper reads, sets under their own
	// names and deletes. kata then scopes each token to its entry for the agent exactly as for the Pane, in whatever checkout the agent
	// runs, which a single KATA_AUTH_TOKEN (kata sends it to every remote target) could not do.
	Creds, EnvFile string
}

// IsZero reports whether the route changes nothing: the agent starts exactly as it did before routing existed.
func (r AgentRoute) IsZero() bool { return r == AgentRoute{} }

// zeroWithoutToken reports whether the route would change nothing but for the credentials the Pane has.
func (r AgentRoute) zeroWithoutToken() bool {
	r.Token, r.TokenFile, r.AuthFile, r.TrustPrivate = "", "", "", false
	r.DB, r.Schema, r.DSN, r.DSNFile, r.Creds, r.EnvFile = "", "", "", "", "", ""
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
	if r.DB != "" {
		args = append(args, "--db", r.DB)
	}
	if r.Schema != "" {
		args = append(args, "--schema", r.Schema)
	}
	if r.DSNFile != "" {
		args = append(args, "--dsn-file", r.DSNFile)
	}
	if r.EnvFile != "" {
		args = append(args, "--env-file", r.EnvFile)
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
	route := AgentRoute{Daemon: current}
	if home := os.Getenv("KATA_HOME"); home != "" {
		// Absolute: the agent's block starts in its checkout, not in the Pane's working directory.
		var err error
		if route.Home, err = filepath.Abs(home); err != nil {
			return AgentRoute{}, fmt.Errorf("resolve KATA_HOME: %w", err)
		}
	}
	switch {
	case current != "":
		var err error
		if route.Home, err = paneHome(); err != nil {
			return AgentRoute{}, err
		}
		catalog, err := ReadCatalog()
		if err != nil {
			return AgentRoute{}, err
		}
		entry, found := catalog.Entry(current)
		if !found {
			return AgentRoute{}, fmt.Errorf("daemon %q is not in the daemon catalog", current)
		}
		if route.Token, err = entry.Token(); err != nil {
			return AgentRoute{}, err
		}
		if entry.Local {
			if route.Token == "" {
				route.paneCredential() // a local entry with no token of its own is reached with the Pane's, as the Pane connects to it
			}
			if err := route.paneStorage(); err != nil {
				return AgentRoute{}, err
			}
			route.Creds = paneCredentialEnv(route.Home, true)
		}
		return route, nil
	case os.Getenv("KATA_SERVER") != "":
		route.Server = os.Getenv("KATA_SERVER")
		route.AllowInsecure = envTruthy("KATA_ALLOW_INSECURE")
	}
	if route.zeroWithoutToken() {
		return route, nil
	}
	// The route always names the Pane's home, so the block's own KATA_HOME (and the [auth] and catalog of that home) never decides
	// which credential the agent sends or which catalog its daemon resolves in.
	if route.Home == "" {
		var err error
		if route.Home, err = paneHome(); err != nil {
			return AgentRoute{}, err
		}
	}
	route.TrustPrivate = envTruthy("KATA_TRUST_PRIVATE_NETWORK")
	route.paneCredential()
	route.Creds = paneCredentialEnv(route.Home, route.Server == "")
	if route.Server == "" {
		if err := route.paneStorage(); err != nil {
			return AgentRoute{}, err
		}
	}
	return route, nil
}

// paneHome is the Pane's KATA_HOME made absolute, else the default home.
func paneHome() (string, error) {
	home, err := kataHome()
	if err != nil {
		return "", err
	}
	return filepath.Abs(home)
}

// paneStorage sets the Pane's KATA_DB, KATA_POSTGRES_SCHEMA and KATA_DSN on the route.
func (r *AgentRoute) paneStorage() error {
	if db := strings.TrimSpace(os.Getenv("KATA_DB")); db != "" {
		abs, err := filepath.Abs(db)
		if err != nil {
			return fmt.Errorf("resolve KATA_DB: %w", err)
		}
		r.DB = abs
	}
	r.Schema = strings.TrimSpace(os.Getenv("KATA_POSTGRES_SCHEMA"))
	r.DSN = strings.TrimSpace(os.Getenv("KATA_DSN"))
	return nil
}

// paneCredentialEnv is the Pane's values of the credential variables home's config.toml names, as NAME=value lines: every
// [[daemon]] token_env and [auth].token_env, or only [auth].token_env when all is false (a KATA_SERVER route reaches no catalog entry,
// so the other entries' tokens stay out of the agent). Unset variables, ownedNames, and values spanning lines are left out.
func paneCredentialEnv(home string, all bool) string {
	var names []string
	if all {
		if catalog, err := ReadCatalogAt(home); err == nil {
			for _, d := range catalog.Daemons {
				names = append(names, d.TokenEnv)
			}
		}
	}
	if raw, err := os.ReadFile(filepath.Join(home, "config.toml")); err == nil {
		var f struct {
			Auth struct {
				TokenEnv string `toml:"token_env"`
			} `toml:"auth"`
		}
		if _, err := toml.Decode(string(raw), &f); err == nil {
			names = append(names, f.Auth.TokenEnv)
		}
	}
	var out strings.Builder
	seen := map[string]bool{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		value := strings.TrimSpace(os.Getenv(name))
		if name == "" || seen[name] || !credentialName(name) || value == "" || strings.ContainsAny(value, "\r\n") {
			continue
		}
		seen[name] = true
		out.WriteString(name + "=" + value + "\n")
	}
	return out.String()
}

// ownedNames are the variables the route, the wrapper or kata's own routing and identity settings own (kata v0.20.0 refuses them as a
// local profile's credential references too); an env file may not set them. Other KATA_ names are legitimate token_env names.
var ownedNames = []string{
	"KATA_HOME", "KATA_SERVER", "KATA_AUTH_TOKEN", "KATA_AUTH_TOKEN_FILE", "KATA_ALLOW_INSECURE", "KATA_TRUST_PRIVATE_NETWORK",
	"KATA_DSN", "KATA_DB", "KATA_POSTGRES_SCHEMA", "KATA_POSTGRES_SCHEMA_MODE", "KATA_POSTGRES_SCHEMA_OWNER", "KATA_POSTGRES_ALLOW_INSECURE",
	"KATA_AUTOSTART", "KATA_AUTOSTART_IDLE_TIMEOUT", "KATA_SKIP_DAEMON_VERSION_CHECK", "KATA_HTTP_TIMEOUT", "KATA_LISTEN",
	"KATA_AUTHOR", "KATA_TEAMMATE", "KATA_INBOX_USER", "KATA_REF", "KATA_SESSION_ID", "KATA_TRUSTED_ACTOR_HEADER", "KATA_TRUSTED_PROXY_LISTENERS",
}

// credentialName reports whether name is an environment variable name the wrapper may set from an env file: a plain identifier that is
// not one of ownedNames (compared case-insensitively).
func credentialName(name string) bool {
	if name == "" || slices.ContainsFunc(ownedNames, func(owned string) bool { return strings.EqualFold(owned, name) }) {
		return false
	}
	for i, c := range name {
		if c != '_' && (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') && (i == 0 || c < '0' || c > '9') {
			return false
		}
	}
	return true
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

// agentTokenSuffix names the token file beside a prompt file; removePrompt removes it with the DSN and env files.
const agentTokenSuffix = ".token"

// agentDSNSuffix names the DSN file beside a prompt file; removePrompt removes it too.
const (
	agentDSNSuffix = ".dsn"
	agentEnvSuffix = ".env"
)

// withSecretFiles moves route.Token, route.DSN and route.Creds into new private files beside promptFile (created as kata's own credential files
// are, so they pass its checks on every OS) and names them in route.TokenFile, route.DSNFile and route.EnvFile. Nothing else changes.
func (r AgentRoute) withSecretFiles(promptFile string) (AgentRoute, error) {
	var written []string
	put := func(secret, suffix string, name *string) error {
		if secret == "" {
			return nil
		}
		file := promptFile + suffix
		f, err := safefileio.CreatePrivateFile(file)
		if err != nil {
			return fmt.Errorf("create the agent's private file: %w", err)
		}
		written = append(written, file)
		_, err = f.WriteString(secret + "\n")
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return fmt.Errorf("write the agent's private file: %w", err)
		}
		*name = file
		return nil
	}
	if err := put(r.Token, agentTokenSuffix, &r.TokenFile); err != nil {
		removeAll(written)
		return r, err
	}
	if err := put(r.DSN, agentDSNSuffix, &r.DSNFile); err != nil {
		removeAll(written)
		return r, err
	}
	if err := put(r.Creds, agentEnvSuffix, &r.EnvFile); err != nil {
		removeAll(written)
		return r, err
	}
	r.Token, r.DSN, r.Creds = "", "", ""
	return r, nil
}

func removeAll(files []string) {
	for _, f := range files {
		_ = os.Remove(f)
	}
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
	unset = []string{"KATA_SERVER", "KATA_AUTH_TOKEN", "KATA_AUTH_TOKEN_FILE", "KATA_ALLOW_INSECURE", "KATA_TRUST_PRIVATE_NETWORK", "KATA_DSN", "KATA_DB", "KATA_POSTGRES_SCHEMA"}
	if r.Home != "" {
		set = append(set, "KATA_HOME="+r.Home)
	}
	token := r.Token
	if r.TokenFile != "" && wrapper {
		if token, err = readTokenFile(r.TokenFile); err != nil {
			return nil, nil, err
		}
		_ = os.Remove(r.TokenFile) // the file's job is done: it must not outlive the agent's start
	}
	dsn := r.DSN
	if r.DSNFile != "" && wrapper {
		if dsn, err = readTokenFile(r.DSNFile); err != nil {
			return nil, nil, err
		}
		_ = os.Remove(r.DSNFile)
	}
	storage := func() { // only for a route that keeps the Pane's home
		if r.DB != "" {
			set = append(set, "KATA_DB="+r.DB)
		}
		if r.Schema != "" {
			set = append(set, "KATA_POSTGRES_SCHEMA="+r.Schema)
		}
		if dsn != "" {
			set = append(set, "KATA_DSN="+dsn)
		}
	}
	var creds []string
	if r.EnvFile != "" && wrapper {
		text, err := readTokenFile(r.EnvFile)
		if err != nil {
			return nil, nil, err
		}
		_ = os.Remove(r.EnvFile)
		for _, line := range strings.Split(text, "\n") {
			if name, _, ok := strings.Cut(line, "="); ok && credentialName(name) {
				creds = append(creds, line)
			}
		}
	}
	credential := func() {
		set = append(set, creds...)
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
		if r.Server == "" {
			storage()
		}
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
		creds = nil                               // that home's catalog is the one kata reads, not the Pane's: the Pane's token variables are none of its business
		set = []string{"KATA_HOME=" + entry.Home} // the entry's own home replaces the Pane's: it holds the daemon's state, and the Pane's storage settings are not its own
	} else {
		storage()
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
