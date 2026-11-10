package data

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"go.kenn.io/kit/consolewindow"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// kataRunTimeout bounds a kata run (`daemon restart` returns once the replacement daemon is ready, migrations included; `docbank
// setup` is bounded alike). The runs do not follow the Pane's cancellation: a Pane Tern closes would otherwise kill kata
// mid-run. A var so tests can shorten it.
var kataRunTimeout = 2 * time.Minute

// kataRunWaitDelay is how long a killed kata's output pipes may stay open before the run gives up on them.
const kataRunWaitDelay = 2 * time.Second

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
	home, profile := c.entry.Home, c.entry.Home != ""
	if !profile {
		if home, err = kataHome(); err != nil {
			return err
		}
	}
	done, ok := g.track()
	if !ok {
		return errors.New("the Pane is closing, so the daemon was not restarted")
	}
	defer done()
	env := os.Environ()
	if profile {
		env = profileEnv()
	}
	return runKata(ctx, home, env, "daemon", "restart")
}

// restartKataDaemon answers the Pane's RestartKataDaemon with the restart's outcome.
func (g *Gateway) restartKataDaemon(ctx context.Context, cmd model.RestartKataDaemon) model.Msg {
	return model.KataDaemonRestarted{Err: g.RestartDaemon(ctx, cmd.Workspace), Connection: cmd.Connection}
}

// profileEnv is the Pane's environment without the route and storage variables (KATA_DB, KATA_DSN and the rest of routeNames):
// kata reads them before a catalog profile's own storage, so they would name another runtime namespace than the profile's.
func profileEnv() []string {
	return slices.DeleteFunc(os.Environ(), func(kv string) bool {
		name, _, _ := strings.Cut(kv, "=")
		return slices.ContainsFunc(routeNames, func(r string) bool { return strings.EqualFold(name, r) })
	})
}

// track registers a run that must outlive ctx's cancellation so Close waits for it. It reports false once the Gateway is
// closing; otherwise done must be called when the run ends.
func (g *Gateway) track() (done func(), ok bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return nil, false
	}
	g.switches.Add(1)
	return g.switches.Done, true
}

// runKata runs kata with args and env plus KATA_HOME=kataHome, on a context that ignores ctx's cancellation and ends after
// kataRunTimeout. Its error is kata's last non-empty stderr line, or the run's own error when kata printed none; a kata killed at
// the timeout is reported as not having answered in time.
func runKata(ctx context.Context, kataHome string, env []string, args ...string) error {
	exe := kataExecutable()
	if exe == "" {
		return errors.New("kata is not on PATH")
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), kataRunTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Env = append(env, "KATA_HOME="+kataHome)
	cmd.WaitDelay = kataRunWaitDelay
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	consolewindow.Hide(cmd)
	err := cmd.Run()
	if err == nil {
		return nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("kata did not answer within %s", kataRunTimeout)
	}
	if line := lastLine(stderr.String()); line != "" {
		return errors.New(line)
	}
	return err
}
