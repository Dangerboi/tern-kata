package data

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"go.kenn.io/kata"
	g "go.kenn.io/kata/pkg/client/generated"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// allowAll is a host access controller that admits every request.
type allowAll struct{}

func (allowAll) Authorize(context.Context, kata.AccessRequest) (kata.AccessDecision, error) {
	return kata.AccessDecision{TransactionFence: func(context.Context, kata.Transaction) error { return nil }}, nil
}

// startKataAsHost is kata's own service run in process for an account a host authenticated: kata lists no tokens to it.
func startKataAsHost(t *testing.T, version string) *kataServer {
	t.Helper()
	t.Setenv("KATA_HOME", t.TempDir())
	t.Setenv("KATA_SERVER", "")
	t.Setenv("KATA_AUTH_TOKEN", "")
	service, err := kata.New(context.Background(), kata.Config{DSN: filepath.Join(t.TempDir(), "kata.db"), Access: allowAll{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	return serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal := kata.Principal{Subject: "host-user", Actor: "host-user"}
		service.Handler().ServeHTTP(w, r.WithContext(kata.WithPrincipal(r.Context(), principal)))
	}), version)
}

func createToken(t *testing.T, kata *kataServer, actor, name string) {
	t.Helper()
	body := g.CreateTokenBody{Actor: actor}
	if name != "" {
		body.Name = &name
	}
	if _, err := kata.target().Client.CreateTokenWithResponse(context.Background(), &g.CreateTokenRequestOptions{Body: &body}); err != nil {
		t.Fatal(err)
	}
}

func TestGatewayListsTheTokensOfAnAccountWithTokenAuditRead(t *testing.T) {
	kata := startKata(t, "0.21.0")
	createToken(t, kata, "alice", "laptop")
	createToken(t, kata, "build-bot", "")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace := t.TempDir()

	capabilities := as[model.AuthCapabilitiesLoaded](t, run(t, gateway, model.LoadAuthCapabilities{Workspace: workspace, Request: 4}))
	if capabilities.Err != nil || !capabilities.TokenAuditRead || capabilities.Request != 4 {
		t.Fatalf("capabilities %+v, want token audit read for request 4", capabilities)
	}
	loaded := as[model.CredentialsLoaded](t, run(t, gateway, model.LoadCredentials{Workspace: workspace, Request: 5}))
	if loaded.Err != nil || loaded.Request != 5 || len(loaded.Tokens) != 2 || loaded.ObservedAt.IsZero() {
		t.Fatalf("loaded %+v, want two tokens observed at a time", loaded)
	}
	byActor := map[string]model.Credential{}
	for _, token := range loaded.Tokens {
		byActor[token.Actor] = token
	}
	alice, bot := byActor["alice"], byActor["build-bot"]
	if alice.Name != "laptop" || alice.State != "live" || alice.CreatedAt.IsZero() || !alice.RevokedAt.IsZero() || alice.ScopeKind != "" {
		t.Fatalf("alice's token %+v, want a live global token named laptop", alice)
	}
	if bot.Name != "" || bot.State != "live" || bot.ID == alice.ID {
		t.Fatalf("build-bot's token %+v, want a live unnamed token of its own", bot)
	}
}

func TestGatewayAnswersNoTokenAuditReadToAnAccountWithoutIt(t *testing.T) {
	kata := startKataAsHost(t, "0.21.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace := t.TempDir()

	capabilities := as[model.AuthCapabilitiesLoaded](t, run(t, gateway, model.LoadAuthCapabilities{Workspace: workspace, Request: 1}))
	if capabilities.Err != nil || capabilities.TokenAuditRead {
		t.Fatalf("capabilities %+v, want an answer without token audit read", capabilities)
	}
	loaded := as[model.CredentialsLoaded](t, run(t, gateway, model.LoadCredentials{Workspace: workspace, Request: 2}))
	if loaded.Err == nil || len(loaded.Tokens) != 0 {
		t.Fatalf("loaded %+v, want kata's refusal and no tokens", loaded)
	}
}

func TestGatewayReportsAFailedCapabilityCheckAndTokenReadInTheAskingMessage(t *testing.T) {
	l := newLocator(Target{})
	boom := context.DeadlineExceeded
	l.err.Store(&boom)
	gateway := newGateway(t, l)
	if got := run(t, gateway, model.LoadAuthCapabilities{Request: 3}); got != (model.AuthCapabilitiesLoaded{Request: 3, Err: boom}) {
		t.Errorf("capabilities failure %#v", got)
	}
	if got := run(t, gateway, model.LoadCredentials{Request: 4}); got.(model.CredentialsLoaded).Request != 4 || !errors.Is(got.(model.CredentialsLoaded).Err, boom) {
		t.Errorf("token list failure %#v", got)
	}
}

func TestGatewayHoldsARefreshUntilItsPauseHasPassedOnTheWallClock(t *testing.T) {
	kata := startKata(t, "0.21.0")
	createToken(t, kata, "alice", "")
	gateway := newGateway(t, newLocator(kata.target()))
	gateway.wakePoll = 5 * time.Millisecond
	start := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	var wall atomic.Int64
	wall.Store(start.UnixNano())
	gateway.now = func() time.Time { return time.Unix(0, wall.Load()) }

	messages := make(chan model.Msg, 1)
	go gateway.Run(context.Background(), model.LoadCredentials{Workspace: t.TempDir(), Request: 7, After: 30 * time.Second}, messages)
	select {
	case msg := <-messages:
		t.Fatalf("refreshed before the pause: %+v", msg)
	case <-time.After(60 * time.Millisecond):
	}
	wall.Store(start.Add(29 * time.Second).UnixNano())
	select {
	case msg := <-messages:
		t.Fatalf("refreshed a second early: %+v", msg)
	case <-time.After(60 * time.Millisecond):
	}
	wall.Store(start.Add(30 * time.Second).UnixNano())
	select {
	case msg := <-messages:
		if loaded := as[model.CredentialsLoaded](t, msg); loaded.Err != nil || loaded.Request != 7 || len(loaded.Tokens) != 1 {
			t.Fatalf("refresh %+v", loaded)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no refresh after the pause")
	}
}

// scopeTheInstanceAnswer makes the daemon's instance answer report an account whose credential is limited to one Issue, as kata
// does for such a token: the real answer with a scope in its auth block. (The in-process service authenticates callers itself and
// has no token to hold, so the scope is added on the wire.)
func scopeTheInstanceAnswer(server *kataServer) {
	hook := func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != "/api/v1/instance" {
			return false
		}
		recorded := httptest.NewRecorder()
		server.handler.ServeHTTP(recorded, r)
		var body map[string]any
		if err := json.Unmarshal(recorded.Body.Bytes(), &body); err != nil {
			panic(err)
		}
		auth, _ := body["auth"].(map[string]any)
		auth["scope"] = map[string]any{"kind": "issue_subtree", "project_uid": "p", "root_issue_uid": "i"}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
		return true
	}
	server.intercept.Store(&hook)
}

func TestGatewayReportsAnIssueScopedAccountAndEchoesWhoAsked(t *testing.T) {
	kata := startKata(t, "0.22.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace := t.TempDir()

	global := as[model.AuthCapabilitiesLoaded](t, run(t, gateway, model.LoadAuthCapabilities{Workspace: workspace, Request: 1, Account: true}))
	if global.Err != nil || global.IssueScoped || !global.Account || global.Request != 1 {
		t.Fatalf("global account %+v, want not Issue-scoped, answering the Pane's ask", global)
	}

	scopeTheInstanceAnswer(kata)
	scoped := as[model.AuthCapabilitiesLoaded](t, run(t, gateway, model.LoadAuthCapabilities{Workspace: workspace, Request: 2, Account: true}))
	if scoped.Err != nil || !scoped.IssueScoped || !scoped.Account || scoped.Request != 2 {
		t.Fatalf("scoped account %+v, want Issue-scoped", scoped)
	}
	page := as[model.AuthCapabilitiesLoaded](t, run(t, gateway, model.LoadAuthCapabilities{Workspace: workspace, Request: 3}))
	if page.Account || !page.IssueScoped {
		t.Fatalf("the Credentials Page's ask came back %+v, want Account false", page)
	}
}

func TestGatewayEchoesWhoAskedWhenTheCapabilityReadFails(t *testing.T) {
	l := newLocator(Target{})
	boom := context.DeadlineExceeded
	l.err.Store(&boom)
	gateway := newGateway(t, l)
	if got := run(t, gateway, model.LoadAuthCapabilities{Request: 5, Account: true}); got != (model.AuthCapabilitiesLoaded{Request: 5, Account: true, Err: boom}) {
		t.Errorf("failure %#v", got)
	}
}

// instanceOf is a daemon whose /api/v1/instance reports auth kind kind.
func instanceOf(t *testing.T, kind string) *kataServer {
	t.Helper()
	return serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/instance" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"instance_uid": "u", "version": "0.22.0", "auth": map[string]any{"kind": kind}})
	}), "0.22.0")
}

func TestADaemonIsVerifiedWhenItDerivesTheActorFromTheCredential(t *testing.T) {
	for kind, want := range map[string]bool{"db_token": true, "bootstrap": true, "trusted_proxy": true, "none": false, "local": false, "static_token": false, "": false} {
		daemon := instanceOf(t, kind)
		locate := newLocator(daemon.target()).locate
		verified, err := ReadVerifiedSenders(context.Background(), locate, t.TempDir())
		if err != nil || verified != want {
			t.Errorf("auth kind %q: verified %v, %v; want %v", kind, verified, err, want)
		}
		capabilities := as[model.AuthCapabilitiesLoaded](t, run(t, newGateway(t, newLocator(daemon.target())), model.LoadAuthCapabilities{Workspace: t.TempDir(), Account: true}))
		if capabilities.Err != nil || capabilities.VerifiedSenders != want {
			t.Errorf("auth kind %q: capabilities %+v, want VerifiedSenders %v", kind, capabilities, want)
		}
	}
}

func TestReadingVerifiedSendersFailsWhenTheDaemonCannotBeReached(t *testing.T) {
	l := newLocator(Target{})
	boom := context.DeadlineExceeded
	l.err.Store(&boom)
	if verified, err := ReadVerifiedSenders(context.Background(), l.locate, t.TempDir()); verified || err == nil {
		t.Errorf("verified %v, %v; want not verified and the error", verified, err)
	}
}
