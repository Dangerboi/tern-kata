package data

import (
	"github.com/Dangerboi/tern-kata/internal/model"
)

// loadWorkerPickers reads this machine's worker.toml with the worker's own parser, so a file the worker would refuse offers no
// picker either. The Project's own checkouts and profiles lists narrow what is offered. A missing file is no error: the Job card takes free text then. A file that exists but does not read leaves the
// card on free text and says why.
func (g *Gateway) loadWorkerPickers(cmd model.LoadWorkerPickers) model.WorkerPickersLoaded {
	dirs, err := g.workerDirs()
	if err != nil {
		return model.WorkerPickersLoaded{Pickers: model.WorkerPickers{Note: err.Error()}}
	}
	cfg, missing, err := readWorkerConfig(dirs.ConfigPath())
	switch {
	case missing:
		return model.WorkerPickersLoaded{}
	case err != nil:
		return model.WorkerPickersLoaded{Pickers: model.WorkerPickers{Note: err.Error()}}
	}
	profiles, checkouts := cfg.PickerChoices(cmd.Project)
	return model.WorkerPickersLoaded{Pickers: model.WorkerPickers{Read: true, Profiles: profiles, Checkouts: checkouts}}
}
