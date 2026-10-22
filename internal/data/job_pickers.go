package data

import (
	"errors"
	"io/fs"
	"os"
	"slices"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/worker"
)

// loadWorkerPickers reads this machine's worker.toml with the worker's own parser, so a file the worker would refuse offers no
// picker either. A missing file is no error: the Job card takes free text then. A file that exists but does not read leaves the
// card on free text and says why.
func loadWorkerPickers() model.WorkerPickersLoaded {
	dirs, err := worker.DefaultDirs()
	if err != nil {
		return model.WorkerPickersLoaded{Pickers: model.WorkerPickers{Note: err.Error()}}
	}
	path := dirs.ConfigPath()
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return model.WorkerPickersLoaded{}
	}
	cfg, err := worker.LoadConfig(path)
	if err != nil {
		return model.WorkerPickersLoaded{Pickers: model.WorkerPickers{Note: err.Error()}}
	}
	profiles, checkouts := make([]string, 0, len(cfg.Profiles)), make([]string, 0, len(cfg.Checkouts))
	for name := range cfg.Profiles {
		profiles = append(profiles, name)
	}
	for key := range cfg.Checkouts {
		checkouts = append(checkouts, key)
	}
	slices.Sort(profiles)
	slices.Sort(checkouts)
	return model.WorkerPickersLoaded{Pickers: model.WorkerPickers{Read: true, Profiles: profiles, Checkouts: checkouts}}
}
