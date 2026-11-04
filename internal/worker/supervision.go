package worker

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ConfigFileName is the worker's config file, beside the supervision marker.
const ConfigFileName = "worker.toml"

// ManualFileName is the supervision marker's name, the one place it is spelled. Every text that names the marker
// is built from it.
const ManualFileName = "worker.manual"

// SupervisionRule is the rule in one sentence, for texts that explain it (ADR-0012). The Luau host half (host.luau) holds no copy:
// it asks `tern-kata worker supervised`, which answers from SupervisionOn.
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

// SupervisionOn is whether the plugin starts and restarts the worker: worker.toml exists and the marker does not. The one
// place the rule is decided: the host half (host.luau) asks `tern-kata worker supervised`, which answers from here.
func (d Dirs) SupervisionOn() bool {
	if _, err := os.Stat(d.ConfigPath()); err != nil {
		return false
	}
	return !d.ManualPresent()
}

// supervisedConfigMaxBytes bounds the worker.toml text SupervisedConfig returns.
const supervisedConfigMaxBytes = 1 << 20

// SupervisedConfig is SupervisionOn together with worker.toml's text (at most supervisedConfigMaxBytes; empty when it
// cannot be read), which the host half compares to see that the file changed since the worker last started.
func (d Dirs) SupervisedConfig() (text string, on bool) {
	if !d.SupervisionOn() {
		return "", false
	}
	f, err := os.Open(d.ConfigPath())
	if err != nil {
		return "", true
	}
	defer f.Close()
	b, _ := io.ReadAll(io.LimitReader(f, supervisedConfigMaxBytes))
	return string(b), true
}

// identityWait bounds RunningWorker's question, well inside the 10 s the host half gives `worker supervised`.
const identityWait = 2 * time.Second

// RunningWorker is the worker that answers the control socket, and whether one does. The host half asks it with the
// Supervision rule: after a plugin reload or a Tern update's handoff, a worker its earlier load started holds worker.lock,
// and a second one would only be refused. A worker that answers but not the identity request (one from an older release,
// or one too busy to answer in time) still runs: worker.lock names its process.
func RunningWorker(ctx context.Context, d Dirs) (Identity, bool) {
	ctx, cancel := context.WithTimeout(ctx, identityWait)
	defer cancel()
	resp, err := Ask(ctx, d, Request{Op: cmdIdentity})
	if errors.Is(err, ErrNoWorker) {
		return Identity{}, false
	}
	if err == nil && resp.OK && resp.Identity != nil {
		return *resp.Identity, true
	}
	holder, _ := os.ReadFile(d.Lock())
	pid, _ := strconv.Atoi(strings.TrimSpace(string(holder)))
	return Identity{PID: pid}, true
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
