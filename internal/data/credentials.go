package data

import (
	"context"
	"errors"
	"time"

	g "go.kenn.io/kata/pkg/client/generated"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// ReadAuthCapabilities asks what the connection's account may do. Only the token audit right matters to the Pane: without it
// kata refuses the token list, and an empty list must never be mistaken for no tokens.
func (c *Client) ReadAuthCapabilities(ctx context.Context, cmd model.LoadAuthCapabilities) (model.AuthCapabilitiesLoaded, error) {
	response, err := c.InstanceWithResponse(ctx)
	if err != nil {
		return model.AuthCapabilitiesLoaded{}, daemonError(err)
	}
	if response.JSON200 == nil {
		return model.AuthCapabilitiesLoaded{}, errors.New("kata daemon returned no instance")
	}
	audit := response.JSON200.Auth.TokenAuditRead
	return model.AuthCapabilitiesLoaded{Request: cmd.Request, TokenAuditRead: audit != nil && *audit}, nil
}

// ListCredentials reads the daemon's redacted token inventory and the instant it classified each token's state.
func (c *Client) ListCredentials(ctx context.Context, cmd model.LoadCredentials) (model.CredentialsLoaded, error) {
	response, err := c.ListTokensWithResponse(ctx)
	if err != nil {
		return model.CredentialsLoaded{}, daemonError(err)
	}
	if response.JSON200 == nil {
		return model.CredentialsLoaded{}, errors.New("kata daemon returned no tokens")
	}
	tokens := make([]model.Credential, len(response.JSON200.Tokens))
	for i, wire := range response.JSON200.Tokens {
		tokens[i] = translateCredential(wire)
	}
	return model.CredentialsLoaded{Request: cmd.Request, Tokens: tokens, ObservedAt: response.JSON200.ObservedAt}, nil
}

func translateCredential(wire g.TokenOut) model.Credential {
	token := model.Credential{ID: wire.ID, State: string(wire.State), Actor: wire.Actor, CreatedAt: wire.CreatedAt,
		ExpiresAt: timeOrZero(wire.ExpiresAt), LastUsedAt: timeOrZero(wire.LastUsedAt), RevokedAt: timeOrZero(wire.RevokedAt)}
	if wire.Name != nil {
		token.Name = *wire.Name
	}
	if wire.Scope != nil {
		token.ScopeKind, token.ProjectUID, token.RootIssueUID = string(wire.Scope.Kind), wire.Scope.ProjectUID, wire.Scope.RootIssueUID
	}
	return token
}

func timeOrZero(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// pauseBeforeRefresh holds a LoadCredentials until its pause has passed on the wall clock. It checks the clock at least every
// wakePoll, so a machine that slept through the pause refreshes soon after resuming.
var pauseBeforeRefresh = beforeStep{"wait out the refresh pause", func(g *Gateway, ctx context.Context, cmd model.Cmd) {
	if load, ok := cmd.(model.LoadCredentials); ok && load.After > 0 {
		g.pause(ctx, load.After)
	}
}}

// pause returns once d has passed on the gateway's clock or ctx ends.
func (g *Gateway) pause(ctx context.Context, d time.Duration) {
	// Round(0) drops the monotonic reading, which does not advance while the machine sleeps.
	deadline := g.now().Round(0).Add(d)
	for {
		left := deadline.Sub(g.now().Round(0))
		if left <= 0 {
			return
		}
		timer := time.NewTimer(min(left, g.wakePoll))
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return
		}
	}
}
