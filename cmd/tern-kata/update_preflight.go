package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/data"
	"github.com/Dangerboi/tern-kata/internal/worker"
)

// Pre-flight is the check one release makes of another: `tern-kata update --preflight`, run by the old binary on the unpacked
// new one before the swap. It is a contract between releases, which later ones may only add to: exit 0 when nothing is
// refused and 1 when something is, and stdout is JSON, {"refusals":[{"kind":"kata"|"config","message":"…"}],"warnings":["…"]}.

// Refusal kinds.
const (
	refusalKata   = "kata"
	refusalConfig = "config"
)

// preflightTimeout is how long the old binary lets the new one check this machine.
const preflightTimeout = 30 * time.Second

type preflightRefusal struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

type preflightAnswer struct {
	Refusals []preflightRefusal `json:"refusals"`
	Warnings []string           `json:"warnings"`
}

// preflightEnv is what the new binary's side reads from the machine.
type preflightEnv struct {
	Locate data.Locator
	Dirs   worker.Dirs
	// Version is this binary's version, which the daemon must satisfy.
	Version string
}

// preflightAnswerFor checks this machine against this binary's requirements: the kata daemon's version, and worker.toml when
// there is one. Nothing it reads is changed.
func preflightAnswerFor(ctx context.Context, env preflightEnv) preflightAnswer {
	answer := preflightAnswer{Refusals: []preflightRefusal{}, Warnings: []string{}}
	target, err := env.Locate(ctx, "")
	if err == nil {
		_, err = worker.NewKata(target.Client).Ping(ctx)
	}
	if tooOld, ok := errors.AsType[*worker.KataTooOldError](err); ok {
		answer.Refusals = append(answer.Refusals, preflightRefusal{Kind: refusalKata, Message: fmt.Sprintf(
			"tern-kata %s needs kata %s or newer; this machine's daemon is %s: upgrade kata, then run tern-kata update again",
			withV(env.Version), worker.MinKataVersion, withV(tooOld.Version))})
	} else if err != nil {
		answer.Warnings = append(answer.Warnings, "cannot check this machine's kata daemon: "+firstLine(err.Error()))
	}
	path := env.Dirs.ConfigPath()
	if _, statErr := os.Stat(path); statErr == nil {
		if _, err := worker.LoadConfig(path); err != nil {
			answer.Refusals = append(answer.Refusals, preflightRefusal{Kind: refusalConfig, Message: fmt.Sprintf(
				"%s; after updating, fix it with `tern-kata worker upgrade` or `tern-kata worker edit`", err)})
		}
	}
	return answer
}

// runPreflight is `tern-kata update --preflight`: it writes the answer's JSON and returns a subcommandError (exit 1) when
// there is a refusal.
func runPreflight(ctx context.Context, env preflightEnv, out io.Writer) error {
	answer := preflightAnswerFor(ctx, env)
	if err := json.NewEncoder(out).Encode(answer); err != nil {
		return err
	}
	if len(answer.Refusals) > 0 {
		return subcommandError{errors.New("this machine cannot run this release")}
	}
	return nil
}

func newPreflightEnv() (preflightEnv, error) {
	dirs, err := worker.DefaultDirs()
	if err != nil {
		return preflightEnv{}, subcommandError{err}
	}
	return preflightEnv{Locate: data.LocateWithKata, Dirs: dirs, Version: buildVersion()}, nil
}

// runPreflightOf runs the unpacked release's binary with `update --preflight` and applies its answer. Warnings always print.
// A refusal stops the update (an error, before anything is swapped) unless force is set, which prints it as a warning too.
func runPreflightOf(ctx context.Context, env updateEnv, release unpackedRelease, force bool, out io.Writer) (updateOutcome, error) {
	timeout := env.PreflightTimeout
	if timeout == 0 {
		timeout = preflightTimeout
	}
	answer, refusals := askPreflight(ctx, release.Binary, timeout)
	for _, w := range answer.Warnings {
		fmt.Fprintln(out, "warning: "+w)
	}
	var outcome updateOutcome
	messages := make([]string, 0, len(refusals))
	for _, r := range refusals {
		messages = append(messages, r.Message)
		if r.Kind == refusalConfig {
			outcome.ForcedConfig = force
		}
	}
	if len(messages) == 0 {
		return outcome, nil
	}
	if !force {
		return updateOutcome{}, errors.New(strings.Join(messages, "\n") + "\nnothing was installed; update --force installs anyway")
	}
	for _, m := range messages {
		fmt.Fprintln(out, "warning: "+m)
	}
	return outcome, nil
}

// askPreflight runs the binary and returns its warnings and the refusals to act on: the ones it named, or one saying the
// check itself failed (any exit status but 0 or 1, unreadable JSON, an unknown kind, a timeout, or an exit status that
// disagrees with the refusals).
func askPreflight(ctx context.Context, binary string, timeout time.Duration) (preflightAnswer, []preflightRefusal) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, binary, "update", "--preflight")
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = time.Second
	runErr := cmd.Run()
	failed := func(reason string) (preflightAnswer, []preflightRefusal) {
		if last := lastLine(stderr.String()); last != "" {
			reason = last
		}
		return preflightAnswer{}, []preflightRefusal{{Kind: refusalKata, Message: "the new tern-kata could not check this machine: " + reason}}
	}
	status := 0
	if runErr != nil {
		exitErr, ok := errors.AsType[*exec.ExitError](runErr)
		switch {
		case ctx.Err() != nil:
			return failed(fmt.Sprintf("it took longer than %s", timeout))
		case !ok:
			return failed(runErr.Error())
		}
		status = exitErr.ExitCode()
	}
	if status != 0 && status != 1 {
		return failed(fmt.Sprintf("it exited with status %d", status))
	}
	var answer preflightAnswer
	if err := json.Unmarshal(stdout.Bytes(), &answer); err != nil {
		return failed("its answer was not readable")
	}
	for _, r := range answer.Refusals {
		if r.Kind != refusalKata && r.Kind != refusalConfig {
			return failed(fmt.Sprintf("its answer has a refusal of unknown kind %q", r.Kind))
		}
	}
	if (status == 1) != (len(answer.Refusals) > 0) {
		return failed("its exit status and its answer disagree")
	}
	return answer, answer.Refusals
}

func withV(version string) string { return "v" + strings.TrimPrefix(version, "v") }

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
