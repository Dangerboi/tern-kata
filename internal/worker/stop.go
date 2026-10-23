package worker

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// errStopping is why every admission is refused once a stop was asked.
var errStopping = errors.New("the worker is stopping")

// Stop modes: StopIdle refuses new work and exits once no run is in flight; StopNow ends the worker at once, as a
// terminate signal does, and the runs it interrupts are held `interrupted` at the next start (ADR-0006).
const (
	StopIdle = "idle"
	StopNow  = "now"
)

// StopAnswer is what a stop request answers: the mode now in force, how many runs are in flight (held runs are not),
// and whether the plugin will start a fresh worker afterwards.
type StopAnswer struct {
	Mode     string
	InFlight int
	Restarts bool
}

// Stop starts a drain (mode idle, the default) or ends the worker at once (mode now). A now during a drain upgrades it.
// Leases and the journals of runs it interrupts stay as they are (ADR-0003).
func (w *Worker) Stop(mode string) (StopAnswer, error) {
	switch mode {
	case "":
		mode = StopIdle
	case StopIdle, StopNow:
	default:
		return StopAnswer{}, fmt.Errorf("unknown stop mode %q: use idle or now", mode)
	}
	w.mu.Lock()
	if w.stopping != StopNow {
		w.stopping = mode
	}
	answer := StopAnswer{Mode: w.stopping, InFlight: w.inFlightLocked(), Restarts: w.restartsAfterStop()}
	w.mu.Unlock()
	return answer, nil
}

func (w *Worker) isStopping() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.stopping != ""
}

// inFlightLocked counts the runs a goroutine is driving and the accepted, non-held runs a transient kata failure
// left owing reports or settlement; a held run is neither. mu is held.
func (w *Worker) inFlightLocked() int {
	n := 0
	for _, ps := range w.projects {
		n += len(ps.running)
		for uid := range ps.owing {
			if !ps.running[uid] {
				n++
			}
		}
	}
	return n
}

// finishDrain ends the worker once a stop is due: a stop now, or a drain with no run left in flight. The control
// handler calls it after it has written its answer, since ending the worker closes the connections.
func (w *Worker) finishDrain() {
	w.mu.Lock()
	done := w.stopping == StopNow || w.stopping == StopIdle && w.inFlightLocked() == 0
	w.mu.Unlock()
	if done && w.cancel != nil {
		w.cancel()
	}
}

// restartsAfterStop is whether the plugin's host half starts a fresh worker once this one exits: it supervises this
// worker and nothing turned supervision off with worker.manual beside worker.toml.
func (w *Worker) restartsAfterStop() bool {
	if w.opt.Supervisor == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(filepath.Dir(w.opt.Dirs.ConfigPath()), "worker.manual"))
	return err != nil
}
