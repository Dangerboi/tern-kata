package data

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	oklogulid "github.com/oklog/ulid/v2"
)

// sameURL says whether two daemon URLs name one endpoint as the daemon does: the scheme and host in lower case, a default port
// (80 for http, 443 for https) dropped, a trailing slash ignored.
func sameURL(a, b string) bool { return canonicalURL(a) == canonicalURL(b) }

func canonicalURL(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return strings.TrimRight(raw, "/")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port := u.Port(); port != "" && !(port == "80" && u.Scheme == "http") && !(port == "443" && u.Scheme == "https") {
		host += ":" + port
	}
	u.Host = host
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = ""
	return u.String()
}

// shortcut: kata's client lists the catalog (client.DaemonCatalog) and connects a local entry by name (client.Discover) but exposes no entry's url, token_env,
// inline token, allow_insecure, home or instance_uid, and no [tui] preferences. tern-kata needs those to set a started agent's environment
// (agent_route.go), to reach a remote entry or federation hub with only its own token (catalog_connect.go, federation*.go; kata's Discover would let KATA_AUTH_TOKEN replace a remote entry's token) and name it in a join, to tell which entry a connection is (gateway.go) and for
// [tui].confirm_quit (cmd/tern-kata/main.go), so this reads <KATA_HOME>/config.toml itself for them; delete this file once pkg/client does.

type CatalogEntry struct {
	Name          string
	URL           string
	TokenEnv      string
	AllowInsecure bool
	Local         bool
	Home          string
	InstanceUID   string
	InlineToken   string
}

// Token returns the bearer token the entry carries: its inline token, else what its token_env variable holds; an entry with
// neither has none.
func (e CatalogEntry) Token() (string, error) {
	if e.InlineToken != "" {
		return e.InlineToken, nil
	}
	if e.TokenEnv == "" {
		return "", nil
	}
	token := strings.TrimSpace(os.Getenv(e.TokenEnv))
	if token == "" {
		return "", fmt.Errorf("daemon %q: token_env %q is unset or empty", e.Name, e.TokenEnv)
	}
	return token, nil
}

// hasToken says the entry names a token, inline or through token_env, which Token sends.
func (e CatalogEntry) hasToken() bool { return e.InlineToken != "" || e.TokenEnv != "" }

type Catalog struct {
	Active  string
	Daemons []CatalogEntry
	// ConfirmQuit is [tui].confirm_quit; true unless the file sets it false, and true when [tui] does not decode.
	ConfirmQuit bool
}

func (c Catalog) Entry(name string) (CatalogEntry, bool) {
	for _, e := range c.Daemons {
		if e.Name == name {
			return e, true
		}
	}
	return CatalogEntry{}, false
}

type catalogFile struct {
	ActiveDaemon string `toml:"active_daemon"`
	Daemons      []struct {
		Name          string `toml:"name"`
		Local         bool   `toml:"local"`
		URL           string `toml:"url"`
		Home          string `toml:"home"`
		InstanceUID   string `toml:"instance_uid"`
		Token         string `toml:"token"`
		TokenEnv      string `toml:"token_env"`
		AllowInsecure bool   `toml:"allow_insecure"`
	} `toml:"daemon"`
}

// tuiFile is the [tui] table, decoded apart from the daemon catalog: kata ignores it, so a bad value there must not fail the catalog.
type tuiFile struct {
	TUI struct {
		ConfirmQuit *bool `toml:"confirm_quit"`
	} `toml:"tui"`
}

func kataHome() (string, error) {
	if v := os.Getenv("KATA_HOME"); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	return filepath.Join(home, ".kata"), nil
}

// ReadCatalog reads kata's config.toml. A missing file is an empty catalog; a file that cannot be read or fails kata's own validation is an error naming its path.
func ReadCatalog() (Catalog, error) {
	home, err := kataHome()
	if err != nil {
		return Catalog{}, err
	}
	return ReadCatalogAt(home)
}

func ReadCatalogAt(home string) (Catalog, error) {
	path := filepath.Join(home, "config.toml")
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Catalog{ConfirmQuit: true}, nil
	}
	if err != nil {
		return Catalog{}, fmt.Errorf("read %s: %w", path, err)
	}
	var f catalogFile
	if _, err := toml.Decode(string(raw), &f); err != nil {
		return Catalog{}, fmt.Errorf("parse %s: %w", path, err)
	}
	// A [tui] that does not decode leaves the default: confirm before quitting.
	var tui tuiFile
	if _, err := toml.Decode(string(raw), &tui); err != nil {
		tui = tuiFile{}
	}
	cat := Catalog{Active: strings.TrimSpace(f.ActiveDaemon), ConfirmQuit: tui.TUI.ConfirmQuit == nil || *tui.TUI.ConfirmQuit}
	seen := map[string]bool{}
	for _, d := range f.Daemons {
		e := CatalogEntry{
			Name: strings.TrimSpace(d.Name), URL: strings.TrimSpace(d.URL), TokenEnv: strings.TrimSpace(d.TokenEnv), InlineToken: strings.TrimSpace(d.Token),
			AllowInsecure: d.AllowInsecure, Local: d.Local,
			Home: strings.TrimSpace(d.Home), InstanceUID: strings.TrimSpace(d.InstanceUID),
		}
		if err := validateCatalogEntry(&e, strings.TrimSpace(d.Token) != "", seen); err != nil {
			return Catalog{}, fmt.Errorf("%s: %w", path, err)
		}
		seen[e.Name] = true
		cat.Daemons = append(cat.Daemons, e)
	}
	if cat.Active != "" && !seen[cat.Active] {
		return Catalog{}, fmt.Errorf("%s: active_daemon %q is not in daemon catalog", path, cat.Active)
	}
	return cat, nil
}

func validateCatalogEntry(e *CatalogEntry, inlineToken bool, seen map[string]bool) error {
	switch {
	case e.Name == "":
		return errors.New("daemon: name is required")
	case seen[e.Name]:
		return fmt.Errorf("daemon: duplicate daemon name %q", e.Name)
	case e.Local == (e.URL != ""):
		return fmt.Errorf("daemon %q: exactly one of local or url is required", e.Name)
	case inlineToken && e.TokenEnv != "":
		return fmt.Errorf("daemon %q: token and token_env are mutually exclusive", e.Name)
	}
	if e.Home == "" && e.InstanceUID == "" {
		return nil
	}
	if !e.Local {
		return fmt.Errorf("daemon %q: home and instance_uid are only valid for local profiles", e.Name)
	}
	if e.Home == "" || e.InstanceUID == "" {
		return fmt.Errorf("daemon %q: a local profile requires both home and instance_uid", e.Name)
	}
	if _, err := oklogulid.ParseStrict(e.InstanceUID); err != nil {
		return fmt.Errorf("daemon %q: instance_uid must be a valid ULID", e.Name)
	}
	if strings.HasPrefix(e.Home, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("daemon %q: resolve user home: %w", e.Name, err)
		}
		e.Home = filepath.Join(home, e.Home[2:])
	}
	if !filepath.IsAbs(e.Home) {
		return fmt.Errorf("daemon %q: profile home must be an absolute path or start with ~/", e.Name)
	}
	e.Home = filepath.Clean(e.Home)
	return nil
}
