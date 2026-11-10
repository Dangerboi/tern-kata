package data

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"go.kenn.io/kit/consolewindow"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// daemonRestartTimeout bounds `kata daemon restart`, which returns once the replacement daemon is ready (migrations included). A
// var so tests can shorten it.
var daemonRestartTimeout = 2 * time.Minute

// daemonRestartWaitDelay is how long a killed kata's output pipes may stay open before the run gives up on them.
const daemonRestartWaitDelay = 2 * time.Second

// RestartDaemon restarts the local kata daemon the Pane is connected to for workspace: `kata daemon restart` with that daemon's
// KATA_HOME, the Pane's environment otherwise. kata keeps the replacement's listen address and flags (it re-executes the daemon
// with its own arguments), restarts only the daemon of that home, and refuses --daemon, so a catalog profile is reached through its
// home. A remote daemon is refused and nothing runs. Like the Docbank switch the run does not follow ctx's cancellation and Close
// waits for it.
func (g *Gateway) RestartDaemon(ctx context.Context, workspace string) error {
	c, err := g.client(ctx, workspace)
	if err != nil {
		return err
	}
	if c.kind == model.DaemonRemote {
		return errors.New("a remote daemon is restarted on its own machine: run `kata daemon restart` there")
	}
	home := c.entry.Home
	if home == "" {
		if home, err = kataHome(); err != nil {
			return err
		}
	}
	g.mu.Lock()
	if g.closed {
		g.mu.Unlock()
		return errors.New("the Pane is closing, so the daemon was not restarted")
	}
	g.switches.Add(1)
	g.mu.Unlock()
	defer g.switches.Done()
	return runDaemonRestart(ctx, home)
}

// runDaemonRestart runs `kata daemon restart` for the daemon whose home is kataHome, on a context that ignores ctx's cancellation
// and ends after daemonRestartTimeout. Its error is kata's last non-empty stderr line, or the run's own error when kata printed
// none; a kata killed at the timeout is reported as not having answered in time.
func runDaemonRestart(ctx context.Context, kataHome string) error {
	exe := kataExecutable()
	if exe == "" {
		return errors.New("kata is not on PATH")
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), daemonRestartTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "daemon", "restart")
	cmd.Env = append(os.Environ(), "KATA_HOME="+kataHome)
	cmd.WaitDelay = daemonRestartWaitDelay
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	consolewindow.Hide(cmd)
	err := cmd.Run()
	if err == nil {
		return nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("kata did not answer within %s", daemonRestartTimeout)
	}
	if line := lastLine(stderr.String()); line != "" {
		return errors.New(line)
	}
	return err
}
