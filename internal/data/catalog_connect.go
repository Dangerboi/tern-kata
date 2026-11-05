package data

import (
	"context"
	"os/exec"

	"go.kenn.io/kata/pkg/client"
)

// kataExecutable is the kata program that starts a stopped local daemon on a deliberate switch to it, empty when there is none on
// PATH (a stopped daemon is then reported, not started). Tests replace it.
var kataExecutable = func() string {
	path, _ := exec.LookPath("kata")
	return path
}

// LocateNamed is the Locator for the daemon catalog entry called name, whatever workspace is asked for. kata's own discovery picks the
// entry's credentials and address, as `kata --daemon <name>` does. startWith is a kata executable that starts a stopped local entry
// (`daemon recover` for a profile, `daemon start` otherwise) before connecting; empty never starts one, and a remote is never started.
func LocateNamed(name, startWith string) Locator {
	return func(ctx context.Context, _ string) (Target, error) {
		c, err := client.Discover(ctx, client.DiscoverOptions{Daemon: name, StartWith: startWith})
		if err != nil {
			return Target{}, err
		}
		discovered, _ := c.Discovered()
		return Target{Client: c, Remote: discovered.Remote}, nil
	}
}
