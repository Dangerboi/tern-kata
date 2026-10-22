package worker

import (
	"context"
	"time"
)

// retentionAge is how long a settled, fully reported run's folder is kept (ADR-0006 "Retention").
const retentionAge = 14 * 24 * time.Hour

// retain removes the old settled runs once at start and then every day until ctx ends.
func (w *Worker) retain(ctx context.Context) {
	for {
		removed, err := w.store.Retain(w.clock.Now().Add(-retentionAge))
		for _, uid := range removed {
			w.opt.Log("run %s: settled more than %d days ago; its folder was removed", uid, int(retentionAge/(24*time.Hour)))
		}
		if err != nil {
			w.opt.Log("journals: removing old runs: %v", err)
		}
		if !w.sleep(ctx, 24*time.Hour) {
			return
		}
	}
}
