package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"go.kenn.io/kata/pkg/client"
)

// shortcut: kata's client cannot connect to a catalog entry by name, so the Locator below builds the connection from the entry itself (a remote one) or from `kata daemon locate` (a local one); delete it once pkg/client does.

// LocateEntry is the Locator for one entry of the daemon catalog, whatever workspace is asked for.
// Every entry connects with the token it carries (inline, or what its token_env variable holds) and its allow_insecure, as kata's
// own --daemon resolution does. A remote entry connects at its url.
// A local entry with no token of its own falls back as kata's resolver does: KATA_AUTH_TOKEN, then [auth] in config.toml, then the owner
// token the daemon persisted in its home.
// A local entry is found with `kata --daemon <name> daemon locate --json`, which never starts a daemon,
// and connected at the address it prints.
func LocateEntry(entry CatalogEntry) Locator {
	return func(ctx context.Context, _ string) (Target, error) {
		token, err := entry.Token()
		if err != nil {
			return Target{}, err
		}
		auth := client.TargetAuth{Token: token, AllowInsecure: entry.AllowInsecure}
		if entry.Local {
			if auth.Token == "" {
				if auth.Token, err = implicitLocalToken(entry); err != nil {
					return Target{}, err
				}
			}
			return locateLocalEntry(ctx, entry, auth)
		}
		c, err := client.NewForTarget(ctx, entry.URL, auth)
		if err != nil {
			return Target{}, fmt.Errorf("daemon %q: %w", entry.Name, err)
		}
		return Target{Client: c, Remote: true}, nil
	}
}

// implicitLocalToken is the token kata's resolver uses for a local daemon whose entry carries none: KATA_AUTH_TOKEN, else the first of
// [auth].token, token_file and token_env in <KATA_HOME>/config.toml, else the owner token in the entry's home (KATA_HOME without one),
// which exists only when the daemon persists its auth (kata knows that from the daemon's runtime record, which is not read here: no file means none). It is empty when there is none.
func implicitLocalToken(entry CatalogEntry) (string, error) {
	if token := strings.TrimSpace(os.Getenv("KATA_AUTH_TOKEN")); token != "" {
		return token, nil
	}
	home, err := kataHome()
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(filepath.Join(home, "config.toml"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("daemon %q: %w", entry.Name, err)
	}
	var f struct {
		Auth struct {
			Token     string `toml:"token"`
			TokenFile string `toml:"token_file"`
			TokenEnv  string `toml:"token_env"`
		} `toml:"auth"`
	}
	if err == nil {
		if _, err := toml.Decode(string(raw), &f); err != nil {
			return "", fmt.Errorf("daemon %q: parse config.toml: %w", entry.Name, err)
		}
	}
	if token := strings.TrimSpace(f.Auth.Token); token != "" {
		return token, nil
	}
	if file := strings.TrimSpace(f.Auth.TokenFile); file != "" {
		return readTokenFile(entry.Name, file)
	}
	if name := strings.TrimSpace(f.Auth.TokenEnv); name != "" {
		token := strings.TrimSpace(os.Getenv(name))
		if token == "" {
			return "", fmt.Errorf("daemon %q: [auth] token_env %q is unset or empty", entry.Name, name)
		}
		return token, nil
	}
	if entry.Home != "" {
		home = entry.Home
	}
	return readTokenFile(entry.Name, filepath.Join(home, "auth-token"))
}

// readTokenFile is the trimmed content of a credential file; a missing file holds no token.
func readTokenFile(daemon, path string) (string, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("daemon %q: read token file: %w", daemon, err)
	}
	return strings.TrimSpace(string(raw)), nil
}

func locateLocalEntry(ctx context.Context, entry CatalogEntry, auth client.TargetAuth) (Target, error) {
	raw, err := exec.CommandContext(ctx, "kata", "--daemon", entry.Name, "daemon", "locate", "--json").Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			// kata prints a failure's JSON on stdout and may add text on stderr.
			var failed struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			if json.Unmarshal(raw, &failed) == nil && failed.Error.Message != "" {
				return Target{}, fmt.Errorf("daemon %q: %s", entry.Name, failed.Error.Message)
			}
			if msg := strings.TrimSpace(string(exit.Stderr)); msg != "" {
				return Target{}, fmt.Errorf("daemon %q: %s", entry.Name, msg)
			}
		}
		return Target{}, fmt.Errorf("daemon %q: kata daemon locate: %w", entry.Name, err)
	}
	var located struct {
		Network        string `json:"network"`
		Address        string `json:"address"`
		RequestBaseURL string `json:"request_base_url"`
	}
	if err := json.Unmarshal(raw, &located); err != nil {
		return Target{}, fmt.Errorf("daemon %q: kata daemon locate response: %w", entry.Name, err)
	}
	base := located.RequestBaseURL
	if located.Network == "unix" || base == "" {
		base = located.Address
		if located.Network != "unix" && !strings.Contains(base, "://") {
			base = "http://" + base
		}
	}
	c, err := client.NewForTarget(ctx, base, auth)
	if err != nil {
		return Target{}, fmt.Errorf("daemon %q: %w", entry.Name, err)
	}
	return Target{Client: c}, nil
}
