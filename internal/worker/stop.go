package worker

import (
	"errors"
	"fmt"
	"slices"
)

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
	answer := StopAnswer{Mode: w.stopping, InFlight: w.inFlightLocked(), Restarts: w.opt.Dirs.RestartsAfterStop(w.opt.Supervisor)}
	w.mu.Unlock()
	return answer, nil
}

const (
	cmdStatus    = "status"
	cmdRunDetail = "run-detail"
	cmdStop      = "stop"
	cmdRun       = "run"
	cmdResume    = "resume"
	cmdAbandon   = "abandon"
	cmdResumeJob = "resume-job"
	cmdIdentity  = "identity"
)

// whileStopping is the one rule for what each command may do once a stop was asked (ADR-0012), by stop mode: idle
// drains and refuses what starts work, now ends the worker at once and refuses what changes runs. An abandon is
// accepted during an idle drain because it ends a run instead of starting one.
var whileStopping = map[string]struct{ idle, now bool }{
	cmdStatus:    {idle: true, now: true},
	cmdRunDetail: {idle: true, now: true},
	cmdStop:      {idle: true, now: true},
	cmdRun:       {},
	cmdResume:    {},
	cmdAbandon:   {idle: true},
	cmdResumeJob: {},
	cmdIdentity:  {idle: true, now: true},
}

// refusedWhileStopping is errStopping when command may not run in stop mode ("" while serving), nil otherwise.
func refusedWhileStopping(command, mode string) error {
	switch mode {
	case "":
		return nil
	case StopIdle:
		if whileStopping[command].idle {
			return nil
		}
	case StopNow:
		if whileStopping[command].now {
			return nil
		}
	}
	return errStopping
}

func (w *Worker) stopRefusal(command string) error {
	if w.halted.Load() {
		return refusedWhileStopping(command, StopNow)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return refusedWhileStopping(command, w.stopping)
}

func (w *Worker) isStopping() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.stopping != ""
}

// flight is one Run's entry in the in-flight record: whether a goroutine drives it and whether its lifecycle says it
// still owes a report or its settlement. An entry exists only while one of them holds.
type flight struct{ driven, owes bool }

// inFlightLocked counts the Runs in the in-flight record plus the slots reserved for Runs not yet bound to a uid. A
// held Run owes nothing, so it is counted only while a goroutine drives it. The stop answer and `worker status` both
// report this number. mu is held.
func (w *Worker) inFlightLocked() int {
	n := len(w.flight)
	for _, ps := range w.projects {
		n += ps.pending
	}
	return n
}

// seedFlight records the Runs the last worker owed: the Journals of this executor in the Projects this worker serves
// that the lifecycle says still owe. It runs before the control socket opens, so an idle stop asked at once waits for
// them until recovery hands each on.
func (w *Worker) seedFlight() {
	journals, _, _ := w.store.Scan() // the readable journals still seed; an unreadable one cannot be known to owe
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, j := range journals {
		if j.Executor != w.opt.Config.Executor || !j.owes() {
			continue
		}
		if !slices.ContainsFunc(w.projects, func(ps *projectState) bool { return ps.cfg.Name == j.ProjectName }) {
			continue
		}
		w.flight[j.RunUID] = flight{owes: true}
	}
}

func (w *Worker) updateFlight(uid string, change func(*flight)) {
	w.mu.Lock()
	f := w.flight[uid]
	change(&f)
	if f.driven || f.owes {
		w.flight[uid] = f
	} else {
		delete(w.flight, uid)
	}
	w.mu.Unlock()
	w.finishDrain()
}

// settleFlight clears uid's owing when the lifecycle says the Run no longer owes (settled or held since); it never
// makes a Run owe, since only the start seed and a goroutine's end do that.
func (w *Worker) settleFlight(uid string, owes bool) {
	w.updateFlight(uid, func(f *flight) { f.owes = f.owes && owes })
}

// handedOn is where recovery has handed uid on: it spawned a goroutine, settled the Run or held it, so the Journal
// decides whether the Run still owes.
func (w *Worker) handedOn(uid string) {
	j, err := w.store.Load(uid)
	w.settleFlight(uid, err == nil && j.owes())
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
