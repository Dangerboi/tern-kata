package worker

import (
	"context"
	"time"
)

// retentionAge is how long a settled, fully reported run's folder is kept (ADR-0006 "Retention").
const retentionAge = 14 * 24 * time.Hour

// retain sweeps once at start and then daily. The next day's timer is taken before the sweep reads the clock, so a
// clock that moves while the sweep runs still wakes it; a timer taken after the sweep would be due a day after a
// moment the sweep has already passed, and a jump landing between the two would skip a whole day.
func (w *Worker) retain(ctx context.Context) {
	for {
		next := w.clock.After(24 * time.Hour)
		removed, err := w.store.Retain(w.clock.Now().Add(-retentionAge))
		for _, uid := range removed {
			w.opt.Log("run %s: settled more than %d days ago; its folder was removed", uid, int(retentionAge/(24*time.Hour)))
		}
		if err != nil {
			w.opt.Log("journals: removing old runs: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-next:
		}
	}
}
