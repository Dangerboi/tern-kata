package data

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	oklogulid "github.com/oklog/ulid/v2"
)

// shortcut: kata's client does not expose its daemon catalog or [tui] preferences, so this reads <KATA_HOME>/config.toml itself; delete this file once pkg/client does.

// CatalogEntry is one [[daemon]] entry of kata's config.toml.
type CatalogEntry struct {
	Name          string
	URL           string
	TokenEnv      string
	AllowInsecure bool
	Local         bool
	Home          string
	InstanceUID   string
}

// Token returns the bearer token the entry's token_env variable holds; an entry without token_env has none.
func (e CatalogEntry) Token() (string, error) {
	if e.TokenEnv == "" {
		return "", nil
	}
	token := strings.TrimSpace(os.Getenv(e.TokenEnv))
	if token == "" {
		return "", fmt.Errorf("daemon %q: token_env %q is unset or empty", e.Name, e.TokenEnv)
	}
	return token, nil
}

// Catalog is what tern-kata reads of kata's client configuration.
type Catalog struct {
	// Active is the active_daemon name, empty when none is selected.
	Active string
	// Daemons are the entries in file order.
	Daemons []CatalogEntry
	// ConfirmQuit is [tui].confirm_quit; true unless the file sets it false.
	ConfirmQuit bool
}

// Entry finds the entry called name.
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
	TUI struct {
		ConfirmQuit *bool `toml:"confirm_quit"`
	} `toml:"tui"`
}

// kataHome is $KATA_HOME, else .kata under the user's home directory.
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
	cat := Catalog{Active: strings.TrimSpace(f.ActiveDaemon), ConfirmQuit: f.TUI.ConfirmQuit == nil || *f.TUI.ConfirmQuit}
	seen := map[string]bool{}
	for _, d := range f.Daemons {
		e := CatalogEntry{
			Name: strings.TrimSpace(d.Name), URL: strings.TrimSpace(d.URL), TokenEnv: strings.TrimSpace(d.TokenEnv),
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
