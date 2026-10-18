package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strings"

	"go.kenn.io/kata/pkg/client"
	g "go.kenn.io/kata/pkg/client/generated"
	"golang.org/x/mod/semver"
)

type Client struct {
	*client.Client
	transport *http.Transport
	remote    bool
	version   string
}

// Target is where a located daemon listens, as `kata daemon locate --json` reports it.
type Target struct {
	Kind           string `json:"kind"`
	Network        string `json:"network"`
	Address        string `json:"address"`
	RequestBaseURL string `json:"request_base_url"`
}

// Locator finds the daemon that serves workspace.
type Locator func(ctx context.Context, workspace string) (Target, error)

// LocateWithKata is the production Locator: it asks the kata CLI.
func LocateWithKata(ctx context.Context, workspace string) (Target, error) {
	raw, err := exec.CommandContext(ctx, "kata", "--workspace", workspace, "daemon", "locate", "--json").Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && len(exit.Stderr) > 0 {
			return Target{}, errors.New(strings.TrimSpace(string(exit.Stderr)))
		}
		return Target{}, fmt.Errorf("kata daemon locate: %w", err)
	}
	var target Target
	if err := json.Unmarshal(raw, &target); err != nil {
		return Target{}, fmt.Errorf("kata daemon locate response: %w", err)
	}
	return target, nil
}

// connect builds a client for target and applies the version gate.
func connect(ctx context.Context, target Target) (result *Client, err error) {
	var c *client.Client
	var transport *http.Transport
	defer func() {
		if result == nil && transport != nil {
			transport.CloseIdleConnections()
		}
	}()
	if target.Network == "unix" {
		socket := strings.TrimPrefix(target.Address, "unix://")
		transport = &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		}}
		c, err = client.NewWithHTTPClient("http://kata.invalid", &http.Client{Transport: transport})
	} else {
		c, err = client.NewWithGlobalAuth(ctx, target.RequestBaseURL)
	}
	if err != nil {
		return nil, err
	}
	ping, err := c.PingWithResponse(ctx)
	if err != nil {
		return nil, daemonError(err)
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
	return &Client{Client: c, transport: transport, remote: target.Kind == "remote", version: normalized}, nil
}

// CloseIdleConnections releases the owned Unix transport when the client is dropped.
func (c *Client) CloseIdleConnections() {
	if c.transport != nil {
		c.transport.CloseIdleConnections()
	}
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
