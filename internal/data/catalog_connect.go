package data

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"go.kenn.io/kata/pkg/client"
)

// kataExecutable is the kata program that starts a stopped local daemon on a deliberate switch to it, empty when there is none on
// PATH (a stopped daemon is then reported, not started). A relative PATH hit counts as none: exec.LookPath reports it with
// exec.ErrDot, and kata runs StartWith without looking it up again. Tests replace it.
var kataExecutable = func() string {
	path, err := exec.LookPath("kata")
	if err != nil {
		return ""
	}
	return path
}

// LocateNamed is the Locator for the daemon catalog entry called name, whatever workspace is asked for. A remote entry connects at its
// url with the token it carries (inline, or what its token_env variable holds) and its allow_insecure, and never with KATA_AUTH_TOKEN:
// kata's client would let that variable replace a remote entry's token, sending the local daemon's token to a remote host. A local
// entry goes through kata's own discovery, as `kata --daemon <name>` does. startWith is a kata executable that starts a stopped local
// entry (`daemon recover` for a profile, `daemon start` otherwise) before connecting; empty never starts one, and a remote is never started.
func LocateNamed(name, startWith string) Locator {
	return func(ctx context.Context, _ string) (Target, error) {
		if catalog, err := ReadCatalog(); err == nil {
			if entry, ok := catalog.Entry(name); ok && !entry.Local {
				return locateRemoteEntry(ctx, entry)
			}
		}
		c, err := client.Discover(ctx, client.DiscoverOptions{Daemon: name, StartWith: startWith})
		if err != nil {
			return Target{}, err
		}
		discovered, _ := c.Discovered()
		return Target{Client: c, Remote: discovered.Remote}, nil
	}
}

// locateRemoteEntry connects to a remote catalog entry with only the credentials the entry itself carries.
func locateRemoteEntry(ctx context.Context, entry CatalogEntry) (Target, error) {
	token, err := entry.Token()
	if err != nil {
		return Target{}, err
	}
	c, err := client.NewForTarget(ctx, strings.TrimRight(entry.URL, "/"), client.TargetAuth{Token: token, AllowInsecure: entry.AllowInsecure})
	if err != nil {
		return Target{}, fmt.Errorf("daemon %q: %w", entry.Name, err)
	}
	return Target{Client: c, Remote: true}, nil
}
