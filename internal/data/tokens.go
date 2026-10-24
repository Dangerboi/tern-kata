package data

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	g "go.kenn.io/kata/pkg/client/generated"
	"go.kenn.io/kit/safefileio"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// CreateToken makes an API token as `kata tokens create` does and writes its secret to cmd.File, which it creates new with access
// for its owner alone. The file is reserved before anything is sent, so an existing path is refused without touching kata; a
// failure after the token exists revokes it, so no token is left whose secret nobody holds. The secret goes from kata's answer
// into the file and nowhere else: the answer the Pane gets names the token and the file.
func (c *Client) CreateToken(ctx context.Context, cmd model.CreateTokenCmd) (done model.TokenCreated, err error) {
	file, path, err := reserveTokenFile(cmd.File)
	if err != nil {
		return model.TokenCreated{}, err
	}
	defer func() {
		if err != nil {
			_ = file.Close()
			_ = os.Remove(path)
		}
	}()
	body := &g.CreateTokenBody{Actor: cmd.Actor}
	if cmd.Name != "" {
		body.Name = &cmd.Name
	}
	if cmd.ExpiresInSeconds > 0 {
		body.ExpiresInSeconds = &cmd.ExpiresInSeconds
	}
	var scope *g.TokenScopeIn
	if cmd.Subtree {
		if scope, err = c.tokenScope(ctx, cmd); err != nil {
			return model.TokenCreated{}, err
		}
		body.Scope = scope
	}
	if len(cmd.Teams) > 0 {
		if body.TeamUids, err = c.teamUIDs(ctx, cmd.Teams); err != nil {
			return model.TokenCreated{}, err
		}
	}
	response, err := c.CreateTokenWithResponse(ctx, &g.CreateTokenRequestOptions{Body: body})
	if err != nil {
		return model.TokenCreated{}, daemonError(err)
	}
	if response.JSON200 == nil {
		return model.TokenCreated{}, errors.New("kata daemon returned no token; the outcome is unknown, so check the token list")
	}
	id, secret := response.JSON200.Token.ID, response.JSON200.Plaintext
	if err = checkIssued(response.JSON200, cmd, scope); err == nil {
		err = writeSecret(file, path, secret)
	}
	if err != nil {
		return model.TokenCreated{}, c.withdraw(ctx, id, err)
	}
	return model.TokenCreated{ID: id, File: path}, nil
}

// reserveTokenFile creates the new, owner-only file for a secret. It refuses a path that exists.
func reserveTokenFile(path string) (*os.File, string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, "", errors.New("token file is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, "", fmt.Errorf("resolve token file path: %w", err)
	}
	file, err := safefileio.CreatePrivateFile(abs)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return nil, "", fmt.Errorf("token file %s already exists; name a new file", abs)
		}
		return nil, "", fmt.Errorf("create token file %s: %w", abs, err)
	}
	return file, abs, nil
}

func writeSecret(file *os.File, path, secret string) error {
	if strings.TrimSpace(secret) == "" {
		return errors.New("kata daemon returned an empty token secret")
	}
	if _, err := file.WriteString(secret + "\n"); err != nil {
		return fmt.Errorf("write token file %s: %w", path, err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync token file %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close token file %s: %w", path, err)
	}
	return nil
}

// checkIssued confirms kata did not omit or widen what was asked for, as kata's own CLI does for an Issue-subtree token.
func checkIssued(issued *g.CreateTokenResponseBody, cmd model.CreateTokenCmd, scope *g.TokenScopeIn) error {
	token := issued.Token
	if token.ID <= 0 || strings.TrimSpace(issued.Plaintext) == "" {
		return errors.New("kata daemon returned an incomplete token")
	}
	if token.Actor != cmd.Actor {
		return errors.New("kata daemon returned a token for a different actor")
	}
	if scope == nil {
		return nil
	}
	if token.Scope == nil || string(token.Scope.Kind) != string(scope.Kind) || token.Scope.ProjectUID != scope.ProjectUID || token.Scope.RootIssueUID != scope.RootIssueUID {
		return errors.New("kata daemon returned a missing or different token scope")
	}
	if token.ExpiresAt == nil || token.CreatedAt.IsZero() {
		return errors.New("kata daemon returned a token without its expiry")
	}
	want := token.CreatedAt.Add(time.Duration(cmd.ExpiresInSeconds) * time.Second)
	if delta := token.ExpiresAt.Sub(want); delta < -2*time.Second || delta > 2*time.Second {
		return errors.New("kata daemon returned a different token expiry")
	}
	return nil
}

// withdraw revokes a token whose secret could not be delivered, and says so.
func (c *Client) withdraw(ctx context.Context, id int64, cause error) error {
	if id <= 0 {
		return fmt.Errorf("token delivery failed; the outcome of creating it is unknown: %w", cause)
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if _, err := c.RevokeToken(ctx, id); err != nil {
		return fmt.Errorf("token delivery failed and token %d could not be revoked (%v): %w", id, err, cause)
	}
	return fmt.Errorf("token delivery failed; token %d was revoked: %w", id, cause)
}

// tokenScope resolves the Issue of a subtree token to the Project uid and root Issue uid kata wants. A daemon that cannot issue
// scoped tokens is refused first, as kata's CLI does.
func (c *Client) tokenScope(ctx context.Context, cmd model.CreateTokenCmd) (*g.TokenScopeIn, error) {
	instance, err := c.InstanceWithResponse(ctx)
	if err != nil {
		return nil, daemonError(err)
	}
	if instance.JSON200 == nil || !instance.JSON200.IssueSubtreeTokens {
		return nil, errors.New("the selected daemon does not support issue-scoped tokens (issue_subtree_tokens_unsupported)")
	}
	shown, err := c.ShowIssueWithResponse(ctx, &g.ShowIssueRequestOptions{PathParams: &g.ShowIssuePath{ProjectID: cmd.ProjectID, Ref: cmd.ShortID}})
	if err != nil {
		return nil, daemonError(err)
	}
	if shown.JSON200 == nil || shown.JSON200.Issue.UID == "" || shown.JSON200.Issue.ProjectUID == nil || *shown.JSON200.Issue.ProjectUID == "" {
		return nil, errors.New("kata daemon returned incomplete Issue identity for the token's scope")
	}
	return &g.TokenScopeIn{Kind: g.IssueSubtree, ProjectUID: *shown.JSON200.Issue.ProjectUID, RootIssueUID: shown.JSON200.Issue.UID}, nil
}

// teamUIDs resolves team names or uids to uids, in order. A selector that names no team, or two, is refused.
func (c *Client) teamUIDs(ctx context.Context, selectors []string) ([]string, error) {
	listed, err := c.ListTeamsWithResponse(ctx)
	if err != nil {
		return nil, daemonError(err)
	}
	if listed.JSON200 == nil {
		return nil, errors.New("kata daemon returned no team list")
	}
	uids := make([]string, 0, len(selectors))
	for _, selector := range selectors {
		resolved := ""
		for _, team := range listed.JSON200.Teams {
			if team.Name != selector && team.UID != selector {
				continue
			}
			if resolved != "" && resolved != team.UID {
				return nil, fmt.Errorf("team %q is ambiguous; use its uid", selector)
			}
			resolved = team.UID
		}
		if resolved == "" {
			return nil, fmt.Errorf("team %q not found", selector)
		}
		uids = append(uids, resolved)
	}
	return uids, nil
}

// RevokeToken revokes the token with id as `kata tokens revoke` does and returns kata's record of it.
func (c *Client) RevokeToken(ctx context.Context, id int64) (model.Credential, error) {
	response, err := c.RevokeTokenWithResponse(ctx, &g.RevokeTokenRequestOptions{PathParams: &g.RevokeTokenPath{ID: id}})
	if err != nil {
		return model.Credential{}, daemonError(err)
	}
	if response.JSON200 == nil {
		return model.Credential{}, errors.New("kata daemon returned no token")
	}
	return translateCredential(response.JSON200.Token), nil
}
