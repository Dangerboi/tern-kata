package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/Dangerboi/tern-kata/internal/worker"
)

// workerHold is why the Worker step must not stop the Worker: --force went past a pre-flight that did not clear worker.toml.
type workerHold int

const (
	holdNone      workerHold = iota
	holdConfig               // the new binary refused worker.toml
	holdUnchecked            // the new binary could not check worker.toml at all
)

// updateWorker is the Worker step of `update`, run once the new binary is in place: a Worker the plugin restarts after a
// stop gets an idle stop, so it comes back as the new binary once its runs finish; any other running Worker is only
// named. A hold stops nothing, since the new binary may not load the config. It never runs `worker upgrade`. A failure is a
// warning on out.
func updateWorker(ctx context.Context, env updateEnv, version string, hold workerHold, out io.Writer) {
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
	case hold == holdUnchecked:
		fmt.Fprintln(out, "The worker was not stopped: the new release could not check worker.toml. Check it with `tern-kata worker edit`, then run `tern-kata worker stop`.")
	case hold == holdConfig:
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
