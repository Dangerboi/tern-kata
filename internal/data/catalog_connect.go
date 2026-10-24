package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"go.kenn.io/kata/pkg/client"
)

// shortcut: kata's client cannot connect to a catalog entry by name, so the Locator below builds the connection from the entry itself (a remote one) or from `kata daemon locate` (a local one); delete it once pkg/client does.

// LocateEntry is the Locator for one entry of the daemon catalog, whatever workspace is asked for.
// A remote entry connects with its url and the token its token_env variable holds, honouring allow_insecure.
// A local entry is found with `kata --daemon <name> daemon locate --json`, which never starts a daemon,
// and connected at the address it prints.
func LocateEntry(entry CatalogEntry) Locator {
	return func(ctx context.Context, _ string) (Target, error) {
		if entry.Local {
			return locateLocalEntry(ctx, entry)
		}
		token, err := entry.Token()
		if err != nil {
			return Target{}, err
		}
		c, err := client.NewForTarget(ctx, entry.URL, client.TargetAuth{Token: token, AllowInsecure: entry.AllowInsecure})
		if err != nil {
			return Target{}, fmt.Errorf("daemon %q: %w", entry.Name, err)
		}
		return Target{Client: c, Remote: true}, nil
	}
}

func locateLocalEntry(ctx context.Context, entry CatalogEntry) (Target, error) {
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
	c, err := client.NewForTarget(ctx, base, client.TargetAuth{})
	if err != nil {
		return Target{}, fmt.Errorf("daemon %q: %w", entry.Name, err)
	}
	return Target{Client: c}, nil
}
