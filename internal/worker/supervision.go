package worker

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// ConfigFileName is the worker's config file, beside the supervision marker.
const ConfigFileName = "worker.toml"

// ManualFileName is the supervision marker's name, the one place it is spelled in Go. Every text that names the marker
// is built from it. The Luau host half keeps the one other copy of the rule, pinned by the supervision smoke step.
const ManualFileName = "worker.manual"

// SupervisionRule is the rule in one sentence, for texts that explain it (ADR-0012). The Luau host half (host.luau) applies
// it with its own copy; the supervision smoke step (smoke/supervision.sh) checks both against the same files.
const SupervisionRule = "the plugin starts and restarts the worker exactly when " + ConfigFileName + " exists and " + ManualFileName + " does not"

// SupervisionOnWords names what leaves supervision on, for the sentence `worker stop` prints when nothing will start the worker again.
const SupervisionOnWords = "a " + ConfigFileName + " without " + ManualFileName + " beside it"

// SupervisionOffWords names what turns supervision off, for the paragraph `worker status` prints about a supervised worker.
const SupervisionOffWords = ManualFileName + " beside " + ConfigFileName

// ManualPath is the supervision marker.
func (d Dirs) ManualPath() string { return filepath.Join(d.Config, ManualFileName) }

// ManualPresent is whether the marker exists, whatever else is there: the Worker card's read.
func (d Dirs) ManualPresent() bool {
	_, err := os.Stat(d.ManualPath())
	return err == nil
}

// SupervisionOn is whether the plugin starts and restarts the worker: worker.toml exists and the marker does not. The host
// half (host.luau) decides the same from the same files; the supervision smoke step (smoke/supervision.sh) pins them.
func (d Dirs) SupervisionOn() bool {
	if _, err := os.Stat(d.ConfigPath()); err != nil {
		return false
	}
	return !d.ManualPresent()
}

// SetSupervision turns the plugin's supervision off by creating the marker (and the config folder when it is missing) or
// on by removing it; either, repeated, is no error.
func (d Dirs) SetSupervision(on bool) error {
	if on {
		if err := os.Remove(d.ManualPath()); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(d.Config, 0o700); err != nil {
		return err
	}
	return os.WriteFile(d.ManualPath(), nil, 0o600)
}
