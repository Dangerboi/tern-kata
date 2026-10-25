package data

import (
	"context"
	"errors"
	"net/http"
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
	kata := startKata(t, "0.20.0")
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
	kata := startKataAsHost(t, "0.20.0")
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
	kata := startKata(t, "0.20.0")
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
