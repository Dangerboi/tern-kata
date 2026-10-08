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

type Client struct{ *client.Client }

func Discover(ctx context.Context, workspace string) (*Client, error) {
	raw, err := exec.CommandContext(ctx, "kata", "--workspace", workspace, "daemon", "locate", "--json").Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && len(exit.Stderr) > 0 {
			return nil, ReadError(exit.Stderr, err)
		}
		return nil, fmt.Errorf("kata daemon locate: %w", err)
	}
	var target struct {
		Network        string `json:"network"`
		Address        string `json:"address"`
		RequestBaseURL string `json:"request_base_url"`
	}
	if err := json.Unmarshal(raw, &target); err != nil {
		return nil, fmt.Errorf("kata daemon locate response: %w", err)
	}
	var c *client.Client
	if target.Network == "unix" {
		socket := strings.TrimPrefix(target.Address, "unix://")
		transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
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
		if ping != nil {
			return nil, ReadError(ping.Body, err)
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
	return &Client{Client: c}, nil
}

func ReadError(body []byte, fallback error) error {
	var envelope g.ErrorEnvelope
	if json.Unmarshal(body, &envelope) == nil && envelope.ErrorData.Message != "" {
		message := envelope.ErrorData.Message
		if envelope.ErrorData.Hint != nil {
			message += "\n" + *envelope.ErrorData.Hint
		}
		return errors.New(message)
	}
	if len(body) > 0 {
		return errors.New(strings.TrimSpace(string(body)))
	}
	return fallback
}
