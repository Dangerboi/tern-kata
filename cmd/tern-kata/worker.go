package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

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
}

// newWorkerEnv is the production environment: tern-kata's user folders and kata's own discovery.
func newWorkerEnv() (workerEnv, error) {
	dirs, err := worker.DefaultDirs()
	if err != nil {
		return workerEnv{}, subcommandError{err}
	}
	return workerEnv{Dirs: dirs, Locate: data.LocateWithKata, Log: os.Stderr, Version: buildVersion()}, nil
}

// isWorkerCommand reports whether the plan is `run` or one of the `worker` forms, which need no workspace or gateway.
func isWorkerCommand(plan LaunchPlan) bool { return plan.Subcommand == "run" || plan.Subcommand == "worker" }

// runWorkerCommand runs `run <job-uid>`, `worker status`, `worker doctor` or the foreground worker.
func runWorkerCommand(ctx context.Context, plan LaunchPlan, env workerEnv, out io.Writer) error {
	switch {
	case plan.Subcommand == "run":
		return runJobNow(ctx, plan, env, out)
	case len(plan.Operands) == 1 && plan.Operands[0] == "status":
		resp, err := worker.Ask(ctx, env.Dirs, worker.Request{Op: "status"})
		if err != nil {
			return subcommandError{err}
		}
		if !resp.OK {
			return subcommandError{errors.New(resp.Error)}
		}
		_, err = fmt.Fprint(out, resp.Status.String())
		return err
	case len(plan.Operands) == 1:
		return runDoctor(ctx, plan, env, out)
	}
	return runForeground(ctx, plan, env, out)
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

// runForeground is `tern-kata worker`: it serves until ctx ends (an interrupt or a terminate signal in main).
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
		Takeover: plan.Takeover, Version: env.Version, Log: logger,
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

// workerContext ends on an interrupt or a terminate signal.
func workerContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
}
