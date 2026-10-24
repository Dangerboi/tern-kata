package worker

import (
	"errors"
	"fmt"
	"slices"
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

// inFlightLocked counts the runs a goroutine is driving, the slots reserved for runs not yet bound to one, the
// accepted, non-held runs a transient kata failure left owing reports or settlement, and the journals of the last
// worker that recovery has not yet handed on; a held run is none of these. mu is held.
func (w *Worker) inFlightLocked() int {
	n := 0
	for _, ps := range w.projects {
		n += len(ps.running) + ps.pending
		for uid := range ps.owing {
			if !ps.running[uid] {
				n++
			}
		}
	}
	for uid := range w.owed {
		if !w.drivenLocked(uid) {
			n++
		}
	}
	return n
}

func (w *Worker) drivenLocked(uid string) bool {
	for _, ps := range w.projects {
		if ps.running[uid] || ps.owing[uid] {
			return true
		}
	}
	return false
}

// noteOwed records the unsettled, non-held journals of this executor in the Projects this worker serves, before the
// control socket opens: until recovery hands each to spawn or settles it, an idle stop waits for it.
func (w *Worker) noteOwed() {
	journals, _, err := w.store.Scan()
	if err != nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, j := range journals {
		if j.Executor != w.opt.Config.Executor || j.Phase == PhaseSettled || j.Outcome != nil && j.Outcome.Hold != "" {
			continue
		}
		if !slices.ContainsFunc(w.projects, func(ps *projectState) bool { return ps.cfg.Name == j.ProjectName }) {
			continue
		}
		if w.owed == nil {
			w.owed = map[string]bool{}
		}
		w.owed[j.RunUID] = true
	}
}

// handedOn drops uid from the journals noted at start: recovery has handed it on or it is settled.
func (w *Worker) handedOn(uid string) {
	w.mu.Lock()
	delete(w.owed, uid)
	w.mu.Unlock()
	w.finishDrain()
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
// worker and the supervision rule holds for the config folder.
func (w *Worker) restartsAfterStop() bool {
	return w.opt.Supervisor != "" && w.opt.Dirs.SupervisionOn()
}
