package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Dangerboi/tern-kata/internal/data"
	"github.com/Dangerboi/tern-kata/internal/worker"
)

// workerEnv is what the worker subcommands need from the machine, so tests give them their own folders and daemon.
type workerEnv struct {
	Dirs worker.Dirs
	// Locate finds the kata daemon the way the Pane does; the worker has no workspace, so it is asked about none.
	Locate data.Locator
	Clock  worker.Clock
	// Log receives the foreground worker's log lines.
	Log     io.Writer
	Version string
	// Supervisor is who started this worker, from TERN_KATA_SUPERVISOR; empty when it was started by hand.
	Supervisor string
	// Dir is the directory `worker init` and `worker add` read the Binding from, and Hostname names the machine's executor.
	Dir, Hostname string
}

// newWorkerEnv is the production environment: tern-kata's user folders and kata's own discovery.
// Only `worker init` and `worker add` read the working directory, so the other forms work from a removed one.
func newWorkerEnv(plan LaunchPlan) (workerEnv, error) {
	dirs, err := worker.DefaultDirs()
	if err != nil {
		return workerEnv{}, subcommandError{err}
	}
	var dir string
	if plan.Subcommand == "worker" && len(plan.Operands) == 1 && (plan.Operands[0] == "init" || plan.Operands[0] == "add") {
		if dir, err = os.Getwd(); err != nil {
			return workerEnv{}, subcommandError{err}
		}
	}
	host, _ := os.Hostname()
	return workerEnv{Dirs: dirs, Locate: data.LocateWithKata, Log: printableWriter{os.Stderr}, Version: buildVersion(), Supervisor: os.Getenv("TERN_KATA_SUPERVISOR"), Dir: dir, Hostname: host}, nil
}

// isWorkerCommand reports whether the plan is `run` or one of the `worker` forms, which need no workspace or gateway.
func isWorkerCommand(plan LaunchPlan) bool {
	return plan.Subcommand == "run" || plan.Subcommand == "worker"
}

// runWorkerCommand runs `run <job-uid>`, `worker status`, `worker doctor` or the foreground worker.
func runWorkerCommand(ctx context.Context, plan LaunchPlan, env workerEnv, out io.Writer) error {
	switch {
	case plan.Subcommand == "run":
		return runJobNow(ctx, plan, env, out)
	case len(plan.Operands) == 1 && (plan.Operands[0] == "init" || plan.Operands[0] == "add"):
		return setupWorkerConfig(plan.Operands[0] == "add", plan, env, out)
	case len(plan.Operands) == 1 && plan.Operands[0] == "edit":
		return editWorkerConfig(ctx, env, out)
	case len(plan.Operands) == 1 && plan.Operands[0] == "upgrade":
		return upgradeWorkerConfig(ctx, env, out)
	case len(plan.Operands) == 1 && plan.Operands[0] == "status":
		resp, err := worker.Ask(ctx, env.Dirs, worker.Request{Op: "status"})
		if err != nil {
			return subcommandError{err}
		}
		if !resp.OK {
			return subcommandError{errors.New(resp.Error)}
		}
		if plan.JSON {
			return writeJSON(out, resp.Status)
		}
		_, err = fmt.Fprint(out, resp.Status.String())
		return err
	case len(plan.Operands) == 1 && plan.Operands[0] == "stop":
		return stopWorker(ctx, plan, env, out)
	case len(plan.Operands) == 2:
		return settleHeldRun(ctx, plan, env, out)
	case len(plan.Operands) == 1:
		return runDoctor(ctx, plan, env, out)
	}
	return runForeground(ctx, plan, env, out)
}

// stopWorker is `worker stop [--now]`: it asks the running worker to drain (or, with --now, to end at once) and says what
// the worker answered.
func stopWorker(ctx context.Context, plan LaunchPlan, env workerEnv, out io.Writer) error {
	mode := worker.StopIdle
	if plan.Now {
		mode = worker.StopNow
	}
	resp, err := worker.Ask(ctx, env.Dirs, worker.Request{Op: "stop", Mode: mode})
	if errors.Is(err, worker.ErrNoWorker) {
		return subcommandError{errors.New("no tern-kata worker is running on this machine: there is nothing to stop")}
	}
	if err != nil {
		return subcommandError{err}
	}
	if !resp.OK {
		return subcommandError{errors.New(resp.Error)}
	}
	a := resp.Stop
	runs := fmt.Sprintf("%d %s in flight", a.InFlight, plural(a.InFlight, "run", "runs"))
	switch {
	case a.Mode == worker.StopNow && a.InFlight == 0:
		_, err = fmt.Fprintf(out, "Stopping now: the worker is ending; %s.", runs)
	case a.Mode == worker.StopNow:
		_, err = fmt.Fprintf(out, "Stopping now: the worker ends at once; its %s will be held interrupted and can be resumed once a worker runs again.", runs)
	case a.InFlight == 0:
		_, err = fmt.Fprintf(out, "Stopping when idle: %s, so the worker exits now.", runs)
	default:
		_, err = fmt.Fprintf(out, "Stopping when idle: the worker admits nothing new and exits once its runs in flight have finished (%s now). `tern-kata worker stop --now` ends it at once.", runs)
	}
	if err != nil {
		return err
	}
	if a.Restarts {
		_, err = fmt.Fprintln(out, " The plugin will start a fresh worker.")
	} else {
		_, err = fmt.Fprintln(out, " Nothing will start it again: start one with `tern-kata worker`, or let the plugin ("+worker.SupervisionOnWords+").")
	}
	return err
}

// settleHeldRun is `worker resume <run-uid>` or `worker abandon <run-uid>`: it asks the running worker to continue or
// give up a held run (or, for abandon, to stop a running one or settle one whose local state is lost) and prints what
// the worker saved.
func settleHeldRun(ctx context.Context, plan LaunchPlan, env workerEnv, out io.Writer) error {
	verb, uid := plan.Operands[0], plan.Operands[1]
	resp, err := worker.Ask(ctx, env.Dirs, worker.Request{Op: verb, Run: uid, Actor: plan.Actor})
	if err != nil {
		return subcommandError{err}
	}
	if !resp.OK {
		return subcommandError{errors.New(resp.Error)}
	}
	t := resp.Transition
	if verb == "resume" {
		_, err = fmt.Fprintf(out, "Run %s of job %q in Project %s resumed: the worker continues its omp session; follow it with `tern-kata worker status`.\n", t.RunUID, t.Job, t.Project)
		return err
	}
	switch t.State {
	case "cancelling":
		_, err = fmt.Fprintf(out, "Run %s of job %q in Project %s is being stopped: the worker signals its process group (SIGTERM, then SIGKILL after 10 s) and kata records it cancelled once it is gone.\n", t.RunUID, t.Job, t.Project)
	case "unknown":
		_, err = fmt.Fprintf(out, "Run %s in Project %s had lost its local state: kata now records it unknown (\"local state lost\") and it is never rerun.\n", t.RunUID, t.Project)
	case "acknowledged":
		_, err = fmt.Fprintf(out, "Run %s: its damaged journal is acknowledged. The folder stays in quarantine and keeps vetoing the run.\n", t.RunUID)
	default:
		_, err = fmt.Fprintf(out, "Run %s of job %q in Project %s abandoned: kata will record it cancelled and its slot is free.\n", t.RunUID, t.Job, t.Project)
	}
	return err
}

// runJobNow asks the running worker for a manual occurrence and prints the run's uid once the worker has saved it.
func runJobNow(ctx context.Context, plan LaunchPlan, env workerEnv, out io.Writer) error {
	resp, err := worker.Ask(ctx, env.Dirs, worker.Request{Op: "run", Job: plan.TargetRef})
	if err != nil {
		return subcommandError{err}
	}
	if !resp.OK {
		return subcommandError{errors.New(resp.Error)}
	}
	_, err = fmt.Fprintf(out, "Run %s of job %q in Project %s accepted; follow it with `tern-kata worker status`.\n", resp.Run.RunUID, resp.Run.JobName, resp.Run.Project)
	return err
}

// upgradeWorkerConfig is `worker upgrade`: it adds the secrets and commands keys the Projects' jobs need to worker.toml,
// all or nothing, and says what it added and what it could not.
func upgradeWorkerConfig(ctx context.Context, env workerEnv, out io.Writer) error {
	path := env.Dirs.ConfigPath()
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return subcommandError{fmt.Errorf("no worker config at %s; run `tern-kata worker init` first", path)}
	}
	if err != nil {
		return subcommandError{err}
	}
	cfg, err := worker.LoadConfig(path)
	if err != nil {
		return subcommandError{err}
	}
	target, err := env.Locate(ctx, "")
	if err != nil {
		return subcommandError{err}
	}
	text, report, err := worker.Upgrade(ctx, worker.NewKata(target.Client), cfg, string(raw))
	if err != nil {
		return subcommandError{err}
	}
	for _, line := range report {
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}
	if text == string(raw) {
		_, err = fmt.Fprintln(out, "worker.toml is unchanged.")
		return err
	}
	if err := worker.ReplaceConfig(path, string(raw), text); err != nil {
		return subcommandError{err}
	}
	return reportEdit(ctx, env, path, out)
}

func runDoctor(ctx context.Context, plan LaunchPlan, env workerEnv, out io.Writer) error {
	opts := worker.DoctorOptions{Dirs: env.Dirs, Clock: env.Clock, Actor: plan.Actor}
	if target, err := env.Locate(ctx, ""); err != nil {
		opts.KataErr = err
	} else {
		opts.Kata = worker.NewKata(target.Client)
	}
	report := worker.Doctor(ctx, opts)
	if _, err := fmt.Fprint(out, report); err != nil {
		return err
	}
	if n := report.Problems(); n > 0 {
		return subcommandError{fmt.Errorf("worker doctor found %d %s", n, map[bool]string{true: "problem", false: "problems"}[n == 1])}
	}
	return nil
}

// runForeground is `tern-kata worker`: it serves until ctx ends (an interrupt, a terminate or a hangup, see stopSignals).
func runForeground(ctx context.Context, plan LaunchPlan, env workerEnv, out io.Writer) error {
	cfg, err := worker.LoadConfig(env.Dirs.ConfigPath())
	if err != nil {
		return subcommandError{err}
	}
	target, err := env.Locate(ctx, "")
	if err != nil {
		return subcommandError{err}
	}
	logger := func(format string, args ...any) {
		if env.Log != nil {
			fmt.Fprintf(env.Log, "tern-kata worker: "+format+"\n", args...)
		}
	}
	w, err := worker.New(worker.Options{
		Config: cfg, Dirs: env.Dirs, Kata: worker.NewKata(target.Client), Clock: env.Clock, Actor: plan.Actor,
		Takeover: plan.Takeover, Version: env.Version, Supervisor: env.Supervisor, Log: logger,
	})
	if err != nil {
		return subcommandError{err}
	}
	fmt.Fprintf(out, "Worker %q serving %d %s; state in %s. Stop it with ctrl+c.\n", cfg.Executor, len(cfg.Projects), map[bool]string{true: "Project", false: "Projects"}[len(cfg.Projects) == 1], env.Dirs.State)
	if err := w.Run(ctx); err != nil {
		return subcommandError{err}
	}
	return nil
}

// writeJSON writes v as indented JSON, in the shape the worker's control socket sends it.
func writeJSON(out io.Writer, v any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
