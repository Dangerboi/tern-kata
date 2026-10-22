package data

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/worker"
)

// workerManualFile is the marker beside worker.toml that turns the plugin's supervision of the worker off.
const workerManualFile = "worker.manual"

// loadWorkerSetup is the one read the Worker card and the Jobs Page banner stand on: the Project the Pane's folder is bound to and
// its checkout, whether this machine's worker.toml exists, loads (the worker's own parser) and lists that Project with which
// checkouts, and whether worker.manual is present.
func (g *Gateway) loadWorkerSetup(cmd model.LoadWorkerSetup) model.WorkerSetupLoaded {
	dirs, err := g.workerDirs()
	if err != nil {
		return model.WorkerSetupLoaded{Err: err}
	}
	setup := model.WorkerSetup{Path: dirs.ConfigPath()}
	if setup.Project, setup.Checkout, err = ResolveBinding(cmd.Workspace); err != nil {
		return model.WorkerSetupLoaded{Err: err}
	}
	if _, err := os.Stat(filepath.Join(dirs.Config, workerManualFile)); err == nil {
		setup.Manual = true
	}
	if _, err := os.Stat(setup.Path); errors.Is(err, fs.ErrNotExist) {
		return model.WorkerSetupLoaded{Setup: setup}
	} else if err != nil {
		return model.WorkerSetupLoaded{Err: err}
	}
	setup.Exists = true
	cfg, err := worker.LoadConfig(setup.Path)
	if err != nil {
		setup.LoadErr = err.Error()
		return model.WorkerSetupLoaded{Setup: setup}
	}
	setup.Executor = cfg.Executor
	for _, p := range cfg.Projects {
		if p.Name != setup.Project {
			continue
		}
		setup.Lists = true
		for _, key := range p.Checkouts {
			if dir, ok := cfg.Checkouts[key]; ok {
				setup.Checkouts = append(setup.Checkouts, dir)
			}
		}
		slices.Sort(setup.Checkouts)
	}
	return model.WorkerSetupLoaded{Setup: setup}
}

// saveWorkerSetup writes worker.toml through the worker package's checked write: a new file for the Project the Pane's folder is
// bound to, or that Project added to the file that is there. Nothing is written without the trust choice.
func (g *Gateway) saveWorkerSetup(cmd model.SaveWorkerSetup) model.WorkerSetupSaved {
	saved := model.WorkerSetupSaved{Serve: cmd.Serve}
	fail := func(err error) model.WorkerSetupSaved { saved.Err = err; return saved }
	if !cmd.Trusted {
		return fail(errors.New("Trust: choose Yes to confirm. " + worker.TrustNote))
	}
	dirs, err := g.workerDirs()
	if err != nil {
		return fail(err)
	}
	project, checkout, err := ResolveBinding(cmd.Workspace)
	switch {
	case err != nil:
		return fail(err)
	case project == "":
		return fail(fmt.Errorf("%s is not bound to a kata Project; run `kata init` there first", cmd.Workspace))
	}
	saved.Project = project
	binding := worker.Binding{Project: project, Checkout: checkout}
	path := dirs.ConfigPath()
	if cmd.Serve {
		saved.Key, err = worker.AddProject(path, binding)
	} else {
		saved.Key, err = worker.InitConfig(path, worker.Setup{Binding: binding, Executor: cmd.Executor, Actor: cmd.Actor, DefaultActor: cmd.DefaultActor,
			Command: cmd.Command, Model: cmd.Model, Thinking: cmd.Thinking})
	}
	saved.Err = err
	return saved
}

// stopWorker sends the stop request: mode idle drains, mode now ends the worker at once. No worker answering is not an error here, it is
// what the stop was after.
func (g *Gateway) stopWorker(ctx context.Context, cmd model.StopWorker) model.WorkerStopped {
	resp, err := g.ask(ctx, worker.Request{Op: "stop", Mode: cmd.Mode})
	switch {
	case errors.Is(err, worker.ErrNoWorker):
		return model.WorkerStopped{Mode: cmd.Mode, Gone: true}
	case err != nil:
		return model.WorkerStopped{Mode: cmd.Mode, Err: err}
	case resp.Stop == nil:
		return model.WorkerStopped{Mode: cmd.Mode, Err: errors.New("the worker accepted the stop but gave no answer")}
	}
	return model.WorkerStopped{Mode: resp.Stop.Mode, InFlight: resp.Stop.InFlight, Restarts: resp.Stop.Restarts}
}
