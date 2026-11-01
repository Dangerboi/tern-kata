package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/Dangerboi/tern-kata/internal/worker"
)

// updateWorker is the Worker step of `update`, run once the new binary is in place: a Worker the plugin restarts after a
// stop gets an idle stop, so it comes back as the new binary once its runs finish; any other running Worker is only
// named. forcedConfig (--force went past a refusal of worker.toml) stops nothing, since the new binary may not load the
// config. It never runs `worker upgrade`. A failure is a warning on out.
func updateWorker(ctx context.Context, env updateEnv, version string, forcedConfig bool, out io.Writer) {
	resp, err := worker.Ask(ctx, env.Dirs, worker.Request{Op: "status"})
	if errors.Is(err, worker.ErrNoWorker) {
		return
	}
	if err == nil && !resp.OK {
		err = errors.New(resp.Error)
	}
	if err != nil {
		fmt.Fprintf(out, "warning: could not ask the worker about the update: %v\n", err)
		return
	}
	switch {
	case forcedConfig:
		fmt.Fprintln(out, "The worker was not stopped: worker.toml does not load under "+version+". Run `tern-kata worker upgrade` (or `worker edit`), then `tern-kata worker stop`.")
	case resp.Status.Supervisor != "" && env.Dirs.SupervisionOn():
		stop, err := worker.Ask(ctx, env.Dirs, worker.Request{Op: "stop", Mode: worker.StopIdle})
		if err == nil && !stop.OK {
			err = errors.New(stop.Error)
		}
		if err != nil {
			fmt.Fprintf(out, "warning: could not stop the worker: %v\n", err)
			return
		}
		_ = writeStopAnswer(out, stop.Stop)
	default:
		fmt.Fprintf(out, "The worker was started by hand or supervision is off: restart it to use %s\n", version)
	}
}
