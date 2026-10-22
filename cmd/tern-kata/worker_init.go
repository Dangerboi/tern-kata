package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/Dangerboi/tern-kata/internal/data"
	"github.com/Dangerboi/tern-kata/internal/worker"
)

// setupWorkerConfig is `worker init` (add false) or `worker add` (add true): it writes worker.toml for the Project bound
// to env.Dir, or adds that Project and its checkout to the existing file, through the worker package's checked write.
func setupWorkerConfig(add bool, plan LaunchPlan, env workerEnv, out io.Writer) error {
	path := env.Dirs.ConfigPath()
	project, checkout, err := data.ResolveBinding(env.Dir)
	if err != nil {
		return subcommandError{err}
	}
	if project == "" {
		return subcommandError{fmt.Errorf("%s is not bound to a kata Project; run `kata init` there first", env.Dir)}
	}
	binding := worker.Binding{Project: project, Checkout: checkout}

	var key string
	if add {
		key, err = worker.AddProject(path, binding)
		if errors.Is(err, worker.ErrNoConfig) {
			err = fmt.Errorf("no worker config at %s; run `tern-kata worker init` first", path)
		}
	} else {
		setup := worker.Setup{
			Binding:      binding,
			Executor:     plan.InitFlags["executor"],
			Actor:        plan.InitFlags["actor"],
			DefaultActor: plan.Actor,
			Command:      plan.InitFlags["command"],
			Model:        plan.InitFlags["model"],
			Thinking:     plan.InitFlags["thinking"],
		}
		if setup.Executor == "" {
			setup.Executor = worker.ExecutorName(env.Hostname)
		}
		key, err = worker.InitConfig(path, setup)
		if errors.Is(err, worker.ErrConfigExists) {
			err = fmt.Errorf("%s already exists; use `tern-kata worker add` to add this Project to it", path)
		}
	}
	if err != nil {
		return subcommandError{err}
	}

	verb := "Wrote"
	if add {
		verb = "Updated"
	}
	fmt.Fprintf(out, "%s %s: serving Project %s from %s (checkout %s).\n%s\n", verb, path, project, checkout, key, worker.TrustNote)
	if add {
		fmt.Fprintln(out, "A running worker reads worker.toml only at start: end it (`tern-kata worker status` prints its process id) and the plugin starts a fresh one.")
	} else {
		fmt.Fprintln(out, "The plugin's host half starts the worker within 10 seconds (else run `tern-kata worker`); `tern-kata worker doctor` checks the setup.")
	}
	return nil
}
