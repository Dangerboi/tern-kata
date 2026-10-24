package data

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.kenn.io/kata/pkg/client"
	g "go.kenn.io/kata/pkg/client/generated"
	"golang.org/x/mod/semver"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/worker"
)

type Client struct {
	*client.Client
	kind    model.DaemonKind
	version string
}

// Target is a client for the daemon that serves a workspace.
type Target struct {
	Client *client.Client
	// Remote reports whether the daemon is remote; a client built from an explicit endpoint reads as local.
	Remote bool
}

// Locator finds the daemon that serves workspace.
type Locator func(ctx context.Context, workspace string) (Target, error)

// connectDeadline is how long locating and connecting to the daemon may take.
const connectDeadline = 10 * time.Second

// noDaemonHint is what tern-kata adds when no daemon answers.
const noDaemonHint = "start the kata daemon (kata daemon start) or run `kata doctor`"

// LocateWithKata is the production Locator: kata's own discovery for the workspace, which also says whether the daemon is remote.
func LocateWithKata(ctx context.Context, workspace string) (Target, error) {
	c, err := client.Discover(ctx, client.DiscoverOptions{Workspace: workspace})
	if err != nil {
		if errors.Is(err, client.ErrDaemonUnavailable) {
			err = fmt.Errorf("%w\n%s", err, noDaemonHint)
		}
		return Target{}, err
	}
	discovered, _ := c.Discovered()
	return Target{Client: c, Remote: discovered.Remote}, nil
}

// connect applies the version gate to target's daemon.
func connect(ctx context.Context, target Target) (*Client, error) {
	c := target.Client
	ping, err := c.PingWithResponse(ctx)
	if err != nil {
		noStatus := client.StatusCode(err) == 0
		err = daemonError(err)
		if noStatus {
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
		return nil, fmt.Errorf("cannot verify kata daemon version %q; tern-kata requires %s or newer", version, worker.MinKataVersion)
	}
	if semver.Compare(normalized, worker.MinKataVersion) < 0 {
		return nil, fmt.Errorf("kata daemon %s is too old; tern-kata requires %s or newer; upgrade kata", version, worker.MinKataVersion)
	}
	kind := model.DaemonLocal
	if target.Remote {
		kind = model.DaemonRemote
	}
	return &Client{Client: c, kind: kind, version: normalized}, nil
}

// daemonError is the text the Pane shows for any daemon failure: kata's message, then its recovery hint on its
// own line, without the status-and-code prefix kata's error type carries. A kata failure with no message shows the
// status text, or kata's code when there is no status.
func daemonError(err error) error {
	envelope, ok := errors.AsType[g.ErrorEnvelope](err)
	if !ok {
		return err
	}
	message := envelope.ErrorData.Message
	if message == "" {
		status := client.StatusCode(err)
		if status == 0 {
			status = int(envelope.Status)
		}
		if message = http.StatusText(status); message == "" {
			message = envelope.ErrorData.Code
		}
	}
	if message == "" {
		return err
	}
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
