package data

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	g "go.kenn.io/kata/pkg/client/generated"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// secretIn reads the secret a token file holds.
func secretIn(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	secret := strings.TrimSpace(string(body))
	if !strings.HasPrefix(secret, "kata_") && !strings.HasPrefix(secret, "fixed-secret") {
		t.Fatalf("token file holds %q, want a kata secret", secret)
	}
	return secret
}

func wantOwnerOnly(t *testing.T, path string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Windows files carry no mode bits; their access is an ACL")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("token file mode %o, want 600", perm)
	}
}

func listed(t *testing.T, gateway *Gateway, workspace string) map[int64]model.Credential {
	t.Helper()
	loaded := as[model.CredentialsLoaded](t, run(t, gateway, model.LoadCredentials{Workspace: workspace, Request: 1}))
	if loaded.Err != nil {
		t.Fatal(loaded.Err)
	}
	byID := map[int64]model.Credential{}
	for _, token := range loaded.Tokens {
		byID[token.ID] = token
	}
	return byID
}

func TestGatewayCreatesAnAccountTokenIntoANewOwnerOnlyFileAndRevokesIt(t *testing.T) {
	kata := startKata(t, "0.20.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace := t.TempDir()
	file := filepath.Join(t.TempDir(), "ci.token")

	created := as[model.TokenCreated](t, run(t, gateway, model.CreateTokenCmd{Workspace: workspace, Actor: "ci-bot", Name: "ci", ExpiresInSeconds: 3600, File: file}))
	if created.Err != nil || created.ID == 0 || created.File != file {
		t.Fatalf("created %+v, want a token id and the file", created)
	}
	secret := secretIn(t, file)
	wantOwnerOnly(t, file)
	if strings.Contains(fmt.Sprintf("%+v", created), secret) {
		t.Error("the answer to the Pane holds the secret")
	}

	token, found := listed(t, gateway, workspace)[created.ID]
	if !found || token.State != "live" || token.Actor != "ci-bot" || token.Name != "ci" || token.ScopeKind != "" || token.ExpiresAt.IsZero() {
		t.Fatalf("listed token %+v, want a live account token for ci-bot named ci that expires", token)
	}

	revoked := as[model.TokenRevoked](t, run(t, gateway, model.RevokeTokenCmd{Workspace: workspace, ID: created.ID}))
	if revoked.Err != nil || revoked.ID != created.ID {
		t.Fatalf("revoked %+v", revoked)
	}
	if token := listed(t, gateway, workspace)[created.ID]; token.State != "revoked" || token.RevokedAt.IsZero() {
		t.Errorf("after the revoke the token is %+v, want revoked", token)
	}
}

func TestGatewayRefusesAnExistingTokenFileBeforeSendingAnything(t *testing.T) {
	kata := startKata(t, "0.20.0")
	var sent atomic.Int32
	count := func(http.ResponseWriter, *http.Request) bool { sent.Add(1); return false }
	kata.intercept.Store(&count)
	gateway := newGateway(t, newLocator(kata.target()))
	file := filepath.Join(t.TempDir(), "taken.token")
	if err := os.WriteFile(file, []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	created := as[model.TokenCreated](t, run(t, gateway, model.CreateTokenCmd{Workspace: t.TempDir(), Actor: "ci-bot", File: file}))
	if created.Err == nil || !strings.Contains(created.Err.Error(), "already exists") {
		t.Fatalf("created %+v, want a refusal naming the existing file", created)
	}
	if sent.Load() != 0 {
		t.Errorf("%d requests reached kata before the refusal", sent.Load())
	}
	if body, _ := os.ReadFile(file); string(body) != "mine\n" {
		t.Errorf("the existing file now holds %q", body)
	}
}

func TestGatewayRefusesALinkAtTheTokenFilePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating a symlink needs a privilege on Windows")
	}
	kata := startKata(t, "0.20.0")
	gateway := newGateway(t, newLocator(kata.target()))
	dir := t.TempDir()
	target, link := filepath.Join(dir, "elsewhere"), filepath.Join(dir, "link.token")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	created := as[model.TokenCreated](t, run(t, gateway, model.CreateTokenCmd{Workspace: t.TempDir(), Actor: "ci-bot", File: link}))
	if created.Err == nil {
		t.Fatal("a dangling link at the path was accepted")
	}
	if _, err := os.Lstat(target); err == nil {
		t.Error("the secret's file was made behind the link")
	}
}

func TestGatewayShowsKatasRefusalOfAnIssueSubtreeTokenWithoutIdentityMode(t *testing.T) {
	kata := startKata(t, "0.20.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, bound := project(t, gateway, "demo")
	issue := createIssue(t, gateway, workspace, bound, "subtree root")
	file := filepath.Join(t.TempDir(), "sub.token")

	created := as[model.TokenCreated](t, run(t, gateway, model.CreateTokenCmd{Workspace: workspace, Actor: "ci-bot", ExpiresInSeconds: 3600,
		Subtree: true, ProjectID: bound.ID, ShortID: issue, File: file}))
	if created.Err == nil || !strings.Contains(created.Err.Error(), "require_token_identity") {
		t.Fatalf("created %+v, want kata's identity-mode refusal", created)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Errorf("a refused token left its file behind: %v", err)
	}
	if got := listed(t, gateway, workspace); len(got) != 0 {
		t.Errorf("a refused token was listed: %+v", got)
	}
}

func TestGatewayRefusesATeamKataDoesNotKnowAndLeavesNoFile(t *testing.T) {
	kata := startKata(t, "0.20.0")
	gateway := newGateway(t, newLocator(kata.target()))
	file := filepath.Join(t.TempDir(), "team.token")
	created := as[model.TokenCreated](t, run(t, gateway, model.CreateTokenCmd{Workspace: t.TempDir(), Actor: "ci-bot", Teams: []string{"core"}, File: file}))
	if created.Err == nil || !strings.Contains(created.Err.Error(), `team "core" not found`) {
		t.Fatalf("created %+v, want the unknown team named", created)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Errorf("a refused token left its file behind: %v", err)
	}
}

// The in-process service cannot turn token identity mode on (kata.Config has no switch for it), so an Issue-subtree token can
// only be refused there. This answers the create the way a daemon in identity mode does, to pin what the gateway sends and what
// it does with the answer.
func TestGatewayCreatesAnIssueSubtreeTokenFromKatasAnswerInIdentityMode(t *testing.T) {
	kata := startKata(t, "0.20.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, bound := project(t, gateway, "demo")
	issue := createIssue(t, gateway, workspace, bound, "subtree root")
	file := filepath.Join(t.TempDir(), "sub.token")

	var asked g.CreateTokenBody
	created := time.Now().UTC()
	answer := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/tokens" {
			return false
		}
		if err := json.NewDecoder(r.Body).Decode(&asked); err != nil {
			t.Errorf("decode the create: %v", err)
		}
		expires := created.Add(time.Duration(*asked.ExpiresInSeconds) * time.Second)
		token := g.TokenOut{ID: 41, Actor: asked.Actor, CreatedAt: created, ExpiresAt: &expires, State: "live",
			Scope: &g.TokenScopeOut{Kind: "issue_subtree", ProjectUID: asked.Scope.ProjectUID, RootIssueUID: asked.Scope.RootIssueUID}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(g.CreateTokenResponseBody{Plaintext: "fixed-secret-41", Token: token})
		return true
	}
	kata.intercept.Store(&answer)

	done := as[model.TokenCreated](t, run(t, gateway, model.CreateTokenCmd{Workspace: workspace, Actor: "ci-bot", Name: "sub", ExpiresInSeconds: 7200,
		Subtree: true, ProjectID: bound.ID, ShortID: issue, File: file}))
	if done.Err != nil || done.ID != 41 || done.File != file {
		t.Fatalf("created %+v, want token 41 in the file", done)
	}
	if secretIn(t, file) != "fixed-secret-41" {
		t.Error("the file does not hold the secret kata returned")
	}
	wantOwnerOnly(t, file)
	if asked.Scope == nil || asked.Scope.RootIssueUID != issue || asked.Scope.ProjectUID != bound.UID ||
		asked.ExpiresInSeconds == nil || *asked.ExpiresInSeconds != 7200 || asked.Actor != "ci-bot" || asked.Name == nil || *asked.Name != "sub" {
		t.Errorf("the gateway asked for %+v, want the Issue's Project uid and root uid, 7200 seconds, ci-bot named sub", asked)
	}
}

func TestGatewayRevokesATokenWhoseSecretCouldNotBeKept(t *testing.T) {
	kata := startKata(t, "0.20.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace := t.TempDir()
	// An answer with an empty secret cannot be delivered: the token it names must not stay live.
	var revoked atomic.Int64
	hook := func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/tokens":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(g.CreateTokenResponseBody{Plaintext: " ", Token: g.TokenOut{ID: 77, Actor: "ci-bot", CreatedAt: time.Now(), State: "live"}})
			return true
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/tokens/77/actions/revoke":
			revoked.Store(77)
			w.WriteHeader(http.StatusInternalServerError)
			return true
		}
		return false
	}
	kata.intercept.Store(&hook)
	file := filepath.Join(t.TempDir(), "lost.token")
	created := as[model.TokenCreated](t, run(t, gateway, model.CreateTokenCmd{Workspace: workspace, Actor: "ci-bot", File: file}))
	if created.Err == nil || revoked.Load() != 77 {
		t.Fatalf("created %+v, revoke of token %d asked; want a failure and a revoke of token 77", created, revoked.Load())
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Errorf("the failed delivery left its file behind: %v", err)
	}
}
