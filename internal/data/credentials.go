package data

import (
	"context"
	"errors"
	"time"

	g "go.kenn.io/kata/pkg/client/generated"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// ReadAuthCapabilities asks what the connection's account may do. The token audit right matters to the Credentials Page: without
// it kata refuses the token list, and an empty list must never be mistaken for no tokens. A credential limited to one Issue
// (a scope) matters to the Pane, which does not offer what kata refuses to it. Whether the daemon verifies the actor of a write
// matters to messaging.
func (c *Client) ReadAuthCapabilities(ctx context.Context, cmd model.LoadAuthCapabilities) (model.AuthCapabilitiesLoaded, error) {
	auth, err := c.readInstanceAuth(ctx)
	if err != nil {
		return model.AuthCapabilitiesLoaded{}, err
	}
	return model.AuthCapabilitiesLoaded{Request: cmd.Request, Account: cmd.Account, TokenAuditRead: auth.TokenAuditRead != nil && *auth.TokenAuditRead, IssueScoped: auth.Scope != nil,
		VerifiedSenders: VerifiedAuthKind(auth.Kind)}, nil
}

func (c *Client) readInstanceAuth(ctx context.Context) (g.AuthInfoOut, error) {
	response, err := c.InstanceWithResponse(ctx)
	if err != nil {
		return g.AuthInfoOut{}, daemonError(err)
	}
	if response.JSON200 == nil {
		return g.AuthInfoOut{}, errors.New("kata daemon returned no instance")
	}
	return response.JSON200.Auth, nil
}

// VerifiedAuthKind is whether a daemon whose /api/v1/instance reports auth kind verifies the actor of every write: it derives the
// actor from a database token, the bootstrap token or a trusted proxy, and ignores one a body claims (kata's identity mode, [auth]
// require_token_identity). Any other kind takes a writer's word for its actor.
func VerifiedAuthKind(kind string) bool {
	switch kind {
	case "db_token", "bootstrap", "trusted_proxy":
		return true
	}
	return false
}

// ReadVerifiedSenders asks the daemon locate finds for workspace whether it verifies the actor of every write. The wrapper
// `tern-kata agent` asks it on the agent's route at start.
func ReadVerifiedSenders(ctx context.Context, locate Locator, workspace string) (bool, error) {
	c, err := connectLocated(ctx, locate, workspace)
	if err != nil {
		return false, err
	}
	auth, err := c.readInstanceAuth(ctx)
	return err == nil && VerifiedAuthKind(auth.Kind), err
}

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

func (g *Gateway) pause(ctx context.Context, d time.Duration) {
	g.waitUntil(ctx, g.now().Add(d))
}
