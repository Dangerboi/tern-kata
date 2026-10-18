package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"go.kenn.io/kata/pkg/client"
	g "go.kenn.io/kata/pkg/client/generated"
	"golang.org/x/mod/semver"
)

type Client struct {
	*client.Client
	remote  bool
	version string
}

// Target is a client for the daemon that serves a workspace.
type Target struct {
	Client *client.Client
	// Remote reports whether the daemon is remote; nil means local.
	Remote func(ctx context.Context) (bool, error)
}

// Locator finds the daemon that serves workspace.
type Locator func(ctx context.Context, workspace string) (Target, error)

// noDaemonHint is what tern-kata adds when no daemon answers.
const noDaemonHint = "start the kata daemon (kata daemon start) or run `kata doctor`"

// LocateWithKata is the production Locator: it uses kata's own discovery for the workspace.
func LocateWithKata(ctx context.Context, workspace string) (Target, error) {
	c, err := client.Discover(ctx, client.DiscoverOptions{Workspace: workspace})
	if err != nil {
		if errors.Is(err, client.ErrDaemonUnavailable) {
			err = fmt.Errorf("%w\n%s", err, noDaemonHint)
		}
		return Target{}, err
	}
	return Target{Client: c, Remote: func(ctx context.Context) (bool, error) { return daemonIsRemote(ctx, workspace) }}, nil
}

// daemonIsRemote asks the kata CLI which kind of daemon serves workspace.
// shortcut: kata's client does not expose the daemon kind; delete this once it does.
func daemonIsRemote(ctx context.Context, workspace string) (bool, error) {
	raw, err := exec.CommandContext(ctx, "kata", "--workspace", workspace, "daemon", "locate", "--json").Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && len(exit.Stderr) > 0 {
			return false, errors.New(strings.TrimSpace(string(exit.Stderr)))
		}
		return false, fmt.Errorf("kata daemon locate: %w", err)
	}
	var located struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &located); err != nil {
		return false, fmt.Errorf("kata daemon locate response: %w", err)
	}
	return located.Kind == "remote", nil
}

// connect applies the version gate to target's daemon.
func connect(ctx context.Context, target Target) (*Client, error) {
	c := target.Client
	ping, err := c.PingWithResponse(ctx)
	if err != nil {
		err = daemonError(err)
		if client.StatusCode(err) == 0 {
			err = fmt.Errorf("%w\n%s", err, noDaemonHint)
		}
		return nil, err
	}
	if ping.JSON200 == nil {
		return nil, fmt.Errorf("kata daemon returned no version")
	}
	version := ping.JSON200.Version
	normalized := "v" + strings.TrimPrefix(version, "v")
	if !semver.IsValid(normalized) {
		return nil, fmt.Errorf("cannot verify kata daemon version %q; tern-kata requires v0.18.0 or newer", version)
	}
	if semver.Compare(normalized, "v0.18.0") < 0 {
		return nil, fmt.Errorf("kata daemon %s is too old; tern-kata requires v0.18.0 or newer; upgrade kata", version)
	}
	var remote bool
	if target.Remote != nil {
		if remote, err = target.Remote(ctx); err != nil {
			return nil, err
		}
	}
	return &Client{Client: c, remote: remote, version: normalized}, nil
}

// daemonError is the text the Pane shows for any daemon failure: kata's message, then its recovery hint on its
// own line, without the status-and-code prefix kata's error type carries. A failure with no kata message keeps its own text.
func daemonError(err error) error {
	envelope, ok := errors.AsType[g.ErrorEnvelope](err)
	if !ok || envelope.ErrorData.Message == "" {
		return err
	}
	message := envelope.ErrorData.Message
	if hint := envelope.ErrorData.Hint; hint != nil && *hint != "" {
		message += "\n" + *hint
	}
	return errors.New(message)
}

// daemonCode is the error code kata gave the failure, empty when it gave none.
func daemonCode(err error) string {
	envelope, _ := errors.AsType[g.ErrorEnvelope](err)
	return envelope.ErrorData.Code
}
