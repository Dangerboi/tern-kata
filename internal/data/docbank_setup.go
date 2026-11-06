package data

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// LocalDaemon is the kata daemon whose config `tern-kata docbank setup` writes: one whose config.toml is on this machine.
type LocalDaemon struct {
	// Base is the KATA_HOME kata's commands must run with: the one holding the daemon catalog.
	Base string
	// Home is the home of the daemon itself and holds the config.toml to edit: Base for the default daemon, a local profile's own
	// home for a profile.
	Home string
	// Name is the catalog name kata's --daemon flag takes; empty for the default daemon.
	Name string
	// DB and Schema are the route's KATA_DB and KATA_POSTGRES_SCHEMA, which kata's commands need to find the default daemon's
	// runtime; empty for a profile, which keeps its own storage.
	DB, Schema string
}

// ResolveLocalDaemon picks the daemon the Pane would use for route, as far as route's --home, --daemon, --server, --db and
// --schema (and the environment's KATA_HOME and KATA_SERVER) choose one: the default daemon of KATA_HOME or ~/.kata, or a
// catalog entry that is a local profile. A remote daemon is refused, since its config is not on this machine.
func ResolveLocalDaemon(route AgentRoute) (LocalDaemon, error) {
	remote := func(what string) error {
		return fmt.Errorf("%s is a remote kata daemon: its config.toml is not on this machine, so set Docbank up on the machine that runs it", what)
	}
	if route.Daemon == "" && route.Server != "" {
		return LocalDaemon{}, remote("--server " + route.Server)
	}
	if route.Daemon == "" && route.Home == "" && os.Getenv("KATA_SERVER") != "" {
		return LocalDaemon{}, remote("KATA_SERVER")
	}
	base := route.Home
	if base == "" {
		var err error
		if base, err = kataHome(); err != nil {
			return LocalDaemon{}, err
		}
	}
	base, err := filepath.Abs(base)
	if err != nil {
		return LocalDaemon{}, fmt.Errorf("resolve the kata home: %w", err)
	}
	if route.Daemon == "" {
		return LocalDaemon{Base: base, Home: base, DB: route.DB, Schema: route.Schema}, nil
	}
	catalog, err := ReadCatalogAt(base)
	if err != nil {
		return LocalDaemon{}, err
	}
	entry, found := catalog.Entry(route.Daemon)
	if !found {
		return LocalDaemon{}, fmt.Errorf("daemon %q is not in the catalog of %s", route.Daemon, base)
	}
	if !entry.Local {
		return LocalDaemon{}, remote(fmt.Sprintf("daemon %q", entry.Name))
	}
	home := cmp.Or(entry.Home, base)
	return LocalDaemon{Base: base, Home: home, Name: entry.Name}, nil
}

// AttachmentsConfigured connects to the daemon locate finds and asks whether it has a Docbank to store attachments in, which kata
// reads from its config.toml only at start. A daemon that cannot be reached or asked is an error, as every other failure is.
func AttachmentsConfigured(ctx context.Context, locate Locator, workspace string) (bool, error) {
	target, err := locate(ctx, workspace)
	if err != nil {
		return false, err
	}
	c, err := connect(ctx, target)
	if err != nil {
		return false, err
	}
	return c.attachmentsConfigured(ctx)
}
