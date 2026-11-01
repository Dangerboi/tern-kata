package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
	_ "time/tzdata" // zone database for hosts without one, such as Windows

	"github.com/Dangerboi/tern-kata/internal/data"
	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "tern-kata:", printable(err.Error()))
		os.Exit(exitStatus(err))
	}
}

// exitStatus is the status main exits with after err: 2 for a command line refused before anything ran, 128 plus the signal
// for a command a signal stopped, 1 for any other failure of a subcommand (which never opens a Pane), else 3.
// Pane contract with window.luau: the Pane title is `tern-kata` (set in run), the Pane's own failures exit 3 and a refused
// command line 2, as a Go panic does. The plugin reads 2 and 3 as "failed to start", 1 as either (Windows reports every
// failing launch as 1), and any other early failing status as "not installed", so a Pane failure must never exit 1.
func exitStatus(err error) int {
	if stopped, ok := errors.AsType[interrupted](err); ok {
		return stopped.status()
	}
	if _, ok := errors.AsType[usageError](err); ok {
		return 2
	}
	if _, ok := errors.AsType[subcommandError](err); ok {
		return 1
	}
	return 3
}

// stoppedBy is err, or interrupted when a signal ended the command: a command that wound down cleanly on it (a worker returns
// nil) was still stopped by it and exits 128 plus its number.
func stoppedBy(s os.Signal, err error) error {
	if s != nil {
		return interrupted{s, err}
	}
	return err
}

// printInfo prints what --version, --help and `completion` answer to out, and reports whether plan was one of them. A failed
// write is a subcommandError (exit 1) like every subcommand's, not the Pane's start failure.
func printInfo(plan LaunchPlan, out io.Writer) (bool, error) {
	switch {
	case plan.Version != "":
		_, err := fmt.Fprintln(out, "tern-kata", plan.Version)
		return true, err
	case plan.Help:
		_, err := fmt.Fprint(out, usage)
		return true, err
	case plan.Subcommand == "completion":
		return true, writeCompletion(plan.Operands[0], out)
	}
	return false, nil
}

func run() error {
	plan, err := PlanLaunch(os.Args[1:], lookupEnv, buildVersion())
	if err != nil {
		return err
	}
	if handled, err := printInfo(plan, printableWriter{os.Stdout}); handled {
		return err
	}
	if plan.Subcommand == "agent" {
		return execAgent(plan.Actor, plan.Operands, os.Environ())
	}
	if plan.Subcommand != "" {
		ctx, received, stop := withSignals(context.Background(), stopSignals...)
		defer stop()
		err := runCommand(ctx, plan)
		return stoppedBy(received(), err)
	}
	// A terminate or a hangup ends the Pane cleanly: the loop returns, so the surface and the gateway close.
	ctx, _, stop := withSignals(context.Background(), paneStopSignals...)
	defer stop()
	session, err := tern.Connect(ctx, editorOptions())
	if errors.Is(err, tern.ErrUnsupported) {
		kata, err := exec.LookPath("kata")
		if err != nil {
			return err
		}
		return execKata(kata, plan.FallbackArgs, os.Environ(), plan.fallbackAuthor)
	}
	if err != nil {
		if ctx.Err() != nil { // a stop signal during the connect ends the Pane cleanly
			return nil
		}
		return err
	}
	if len(plan.TUIArgs) > 0 {
		session.Close()
		return usageError{fmt.Errorf("%s: only kata tui takes these, and inside Tern tern-kata opens a Pane instead", strings.Join(plan.TUIArgs, " "))}
	}
	// Pane contract with window.luau: this title identifies a kata Pane.
	if _, err := fmt.Fprint(os.Stdout, "\x1b]0;tern-kata\x07"); err != nil {
		session.Close()
		return err
	}
	quitAtOnce := quitsAtOnce(data.ReadCatalog())
	surface, err := openSurface(session, quitAtOnce)
	if err != nil {
		session.Close()
		return err
	}
	term := newTerminal(ctx, session, surface, connectPane(quitAtOnce))
	defer term.Close()
	workspace, err := os.Getwd()
	if err != nil {
		return err
	}
	host := data.HostName()
	size := paneSize(term.Caps().Cols)
	state := driver.State{Model: model.Model{Cols: size.Cols, Rows: size.Rows, Actor: plan.Actor, InboxUser: plan.InboxUser, Host: host, PaneBlock: os.Getenv("TERN_PANE"), QuitAtOnce: quitAtOnce}}
	results := make(chan model.Msg)
	gateway := data.NewGateway(data.LocateWithKata).WithHost(host)
	defer gateway.Close()
	input := any(model.Start{Workspace: workspace, IssueRef: plan.IssueRef, Nonce: rand.Text()})
	loopErr := func() error {
		for {
			if ctx.Err() != nil { // a stop signal that arrived while an editor ran
				return nil //nolint:nilerr // a stop signal ends the Pane cleanly
			}
			out := driver.Step(state, input, time.Now())
			state = out.State
			if out.Err != nil {
				return out.Err
			}
			if out.Quit {
				return nil
			}
			if out.View != nil {
				if err := term.Render(*out.View); err != nil {
					return err
				}
			}
			for _, op := range out.Ops {
				if err := term.Send(op); err != nil {
					return err
				}
			}
			for _, cmd := range out.Cmds {
				go gateway.Run(ctx, cmd, results)
			}
			if out.Editor != nil {
				closed, err := editInTerminal(term, *out.Editor, editorCommand)
				if err != nil {
					return err
				}
				input, state = closed, state.Fresh()
				continue
			}
			select {
			case input = <-term.Inputs():
			case input = <-results:
			case err := <-term.InputErrors():
				if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
					return nil
				}
				return err
			case <-ctx.Done():
				return nil
			}
		}
	}()
	quitAgentAttention(state.Model, gateway)
	return loopErr
}

// quitAgentAttention clears the Attention requests the Pane raised for waiting agents. The loop drops the Cmds of a quitting Step
// and a stop signal has cancelled the Pane's context, so this runs the clears itself, on a context of its own, before the gateway
// closes; one that does not finish in time is left, as after a kill.
func quitAgentAttention(m model.Model, gateway *data.Gateway) {
	cmds := m.QuitAgentAttention()
	if len(cmds) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), quitClearDeadline)
	defer cancel()
	answers := make(chan model.Msg, len(cmds))
	var done sync.WaitGroup
	for _, cmd := range cmds {
		done.Go(func() { gateway.Run(ctx, cmd, answers) })
	}
	done.Wait()
}

// quitClearDeadline bounds how long a quitting Pane waits for the daemon to take its clears.
const quitClearDeadline = 3 * time.Second

// lookupEnv reads the environment and answers gitUserNameKey from `git config user.name`.
func lookupEnv(key string) (string, bool) {
	if key != gitUserNameKey {
		return os.LookupEnv(key)
	}
	out, err := exec.Command("git", "config", "user.name").Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

// runCommand runs one of the subcommands, which need no Tern. What they print goes through printable: it holds text kata
// answered.
func runCommand(ctx context.Context, plan LaunchPlan) error {
	out := printableWriter{os.Stdout}
	if isWorkerCommand(plan) {
		env, err := newWorkerEnv(plan)
		if err != nil {
			return err
		}
		return runWorkerCommand(ctx, plan, env, out)
	}
	return runSubcommand(ctx, plan, data.NewGateway(data.LocateWithKata).WithHost(data.HostName()), out)
}

// quitsAtOnce is kata's [tui].confirm_quit = false. A config kata itself would refuse is not this Pane's to report: it
// keeps asking before it quits.
func quitsAtOnce(catalog data.Catalog, err error) bool {
	return err == nil && !catalog.ConfirmQuit
}
