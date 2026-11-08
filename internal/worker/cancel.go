package worker

import (
	"errors"
	"sync"
)

// errRunCancelled stops an attempt that was about to start: a cancel request was saved first, so nothing is started.
var errRunCancelled = errors.New("the run was cancelled before its process started")

type cancelSignal struct {
	ch   chan struct{}
	once sync.Once
}

func (c *cancelSignal) fire() { c.once.Do(func() { close(c.ch) }) }

func (w *Worker) cancelSignalOf(uid string) *cancelSignal {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancels[uid] == nil {
		w.cancels[uid] = &cancelSignal{ch: make(chan struct{})}
	}
	return w.cancels[uid]
}

func (w *Worker) dropCancelSignal(uid string) {
	w.mu.Lock()
	delete(w.cancels, uid)
	w.mu.Unlock()
}

// cancelRunning asks a running run to stop (ADR-0008 "Cancellation"): the request is saved first, then the process
// group gets SIGTERM and SIGKILL after 10 s, and the run ends `cancelled` once the group is proven gone.
func (w *Worker) cancelRunning(ps *projectState, uid, by string) (*Journal, error) {
	updated, err := w.store.Update(uid, "cancel-requested", func(j *Journal) error {
		return j.requestCancel(by, w.clock.Now().UTC())
	})
	if err != nil {
		w.noteHook(err)
		return nil, err
	}
	w.cancelSignalOf(uid).fire()
	w.spawn(ps, uid) // a run waiting to start has a goroutine only when something drives it
	return updated, nil
}

// concludeCancel ends a cancelled run: its outcome is `cancelled` and the steps that never ran are skipped. The
// tokens the stopped attempt and its resumed predecessors used are kept.
func (w *Worker) concludeCancel(j *Journal) error {
	return w.record(j.RunUID, "cancelled", (*Journal).concludeCancel)
}
