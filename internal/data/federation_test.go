package data

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	g "go.kenn.io/kata/pkg/client/generated"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// joinReplica makes a spoke Project on the daemon the way `kata federation join` does: a replica of a hub Project, with a
// recorded enrollment token. Nothing contacts the hub.
func joinReplica(t *testing.T, kata *kataServer, name string) {
	t.Helper()
	actor, caps, push := "alice", "pull,push,claim", true
	body := g.CreateFederationReplicaBody{ProjectName: name, HubURL: "https://hub.example:7474", HubProjectID: 42, HubProjectUID: "01HZNQ7VFPK1XGD8R5MABCD4EX",
		ReplayHorizonEventID: 9, Actor: &actor, Token: new("hub-token"), Capabilities: &caps, PushEnabled: &push}
	if _, err := kata.target().Client.CreateFederationReplicaWithResponse(context.Background(), &g.CreateFederationReplicaRequestOptions{Body: &body}); err != nil {
		t.Fatal(err)
	}
}

func TestGatewayReadsAnUnfederatedDaemonsFederationAsNoSpokes(t *testing.T) {
	kata := startKata(t, "0.20.0")
	gateway := newGateway(t, newLocator(kata.target()))

	loaded := as[model.FederationLoaded](t, run(t, gateway, model.LoadFederation{Workspace: t.TempDir(), Request: 8}))
	if loaded.Err != nil || loaded.Request != 8 || len(loaded.Spokes) != 0 {
		t.Fatalf("loaded %+v, want no spokes for request 8", loaded)
	}
	if loaded.InstanceUID == "" || loaded.Daemon != "local" || loaded.Endpoint != "local" {
		t.Errorf("header %q %q %q, want the instance uid on the daemon called local", loaded.Daemon, loaded.Endpoint, loaded.InstanceUID)
	}
}

func TestGatewayReadsAReplicaAsASpokeRow(t *testing.T) {
	kata := startKata(t, "0.20.0")
	joinReplica(t, kata, "acme")
	gateway := newGateway(t, newLocator(kata.target()))

	loaded := as[model.FederationLoaded](t, run(t, gateway, model.LoadFederation{Workspace: t.TempDir(), Request: 1}))
	if loaded.Err != nil || len(loaded.Spokes) != 1 {
		t.Fatalf("loaded %+v, want one spoke", loaded)
	}
	spoke := loaded.Spokes[0]
	if spoke.ProjectName != "acme" || spoke.HubURL != "https://hub.example:7474" || spoke.HubProjectID != 42 || spoke.HubProjectUID != "01HZNQ7VFPK1XGD8R5MABCD4EX" {
		t.Errorf("spoke %+v, want acme bound to hub project 42", spoke)
	}
	if spoke.BoundActor != "alice" || spoke.Capabilities != "pull,push,lease" || !spoke.PushEnabled || spoke.AllowInsecure {
		t.Errorf("spoke grant %+v, want alice with pull,push,lease and push on", spoke)
	}
	if spoke.PullCursorEventID != 8 || spoke.PendingPushCount != 0 || spoke.Badges() != "-" || spoke.Sync(time.Now()) != "never" {
		t.Errorf("spoke sync state %+v, want a pull cursor one behind the replay horizon and nothing pending, synced or wrong", spoke)
	}
}

func TestGatewayLeavesHubRowsOutOfTheSpokeList(t *testing.T) {
	kata := startKata(t, "0.20.0")
	joinReplica(t, kata, "acme")
	roles := map[string]bool{}
	status, err := kata.target().Client.GetFederationStatusWithResponse(context.Background(), nil)
	if err != nil || status.JSON200 == nil {
		t.Fatalf("status %v %+v", err, status)
	}
	for _, row := range status.JSON200.Statuses {
		roles[row.Role] = true
	}
	if !roles["spoke"] {
		t.Fatalf("the daemon's status rows have roles %v, want a spoke", roles)
	}
	// An unfederated Project is not a spoke and stays out of the list.
	gateway := newGateway(t, newLocator(kata.target()))
	project(t, gateway, "plain")
	loaded := as[model.FederationLoaded](t, run(t, gateway, model.LoadFederation{Workspace: t.TempDir(), Request: 1}))
	if len(loaded.Spokes) != 1 || loaded.Spokes[0].ProjectName != "acme" {
		t.Fatalf("spokes %+v, want only acme", loaded.Spokes)
	}
}

func TestGatewayNamesTheActiveDaemonFromKatasCatalog(t *testing.T) {
	kata := startKata(t, "0.20.0")
	config := "active_daemon = \"team\"\n\n[[daemon]]\nname = \"team\"\nurl = \"https://kata.team.example:7474\"\ntoken_env = \"TEAM_KATA_TOKEN\"\n"
	if err := os.WriteFile(filepath.Join(os.Getenv("KATA_HOME"), "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	target := kata.target()
	target.Remote = true
	gateway := newGateway(t, newLocator(target))

	loaded := as[model.FederationLoaded](t, run(t, gateway, model.LoadFederation{Workspace: t.TempDir(), Request: 1}))
	if loaded.Err != nil || loaded.Daemon != "team" || loaded.Endpoint != "https://kata.team.example:7474" || loaded.Auth != "token" {
		t.Fatalf("header %+v, want the catalog's team daemon at its URL with its token", loaded)
	}
}

func TestFederationAuthWordsKatasDisplay(t *testing.T) {
	actor := func(s string) *string { return &s }
	for _, tt := range []struct {
		name       string
		configured bool
		auth       g.AuthInfoOut
		want       string
	}{
		{"no token and no kind", false, g.AuthInfoOut{}, "no token"},
		{"a configured token", true, g.AuthInfoOut{}, "token"},
		{"a configured token the daemon calls none", true, g.AuthInfoOut{Kind: "none"}, "token"},
		{"a database token with an actor", false, g.AuthInfoOut{Kind: "db_token", Actor: actor("alice")}, "token actor alice"},
		{"a static token", false, g.AuthInfoOut{Kind: "static_token"}, "token static"},
		{"a bootstrap token", false, g.AuthInfoOut{Kind: "bootstrap"}, "token bootstrap"},
		{"a trusted proxy with an actor", false, g.AuthInfoOut{Kind: "trusted_proxy", Actor: actor(" bob ")}, "trusted-proxy actor bob"},
		{"a trusted proxy without an actor", false, g.AuthInfoOut{Kind: "trusted_proxy_absent"}, "trusted-proxy missing actor"},
		{"a blank actor is none", true, g.AuthInfoOut{Kind: "none", Actor: actor("  ")}, "token"},
	} {
		if got := federationAuth(tt.configured, tt.auth); got != tt.want {
			t.Errorf("%s: %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestDisplayCapabilitiesUsesKatasSpellingAndOrder(t *testing.T) {
	for stored, want := range map[string]string{"": "", "claim,push,pull": "pull,push,lease", "pull": "pull", " push , pull ": "pull,push", "pull,pull,bogus": "pull"} {
		if got := displayCapabilities(stored); got != want {
			t.Errorf("%q shows as %q, want %q", stored, got, want)
		}
	}
}

func TestGatewayReportsAFederationReadFailureInTheAskingMessage(t *testing.T) {
	l := newLocator(Target{})
	boom := errors.New("daemon down")
	l.err.Store(&boom)
	gateway := newGateway(t, l)
	got := as[model.FederationLoaded](t, run(t, gateway, model.LoadFederation{Request: 5}))
	if got.Request != 5 || !errors.Is(got.Err, boom) {
		t.Errorf("failure %#v, want request 5 and the daemon's error", got)
	}

	kata := startKata(t, "0.20.0")
	hook := func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != "/api/v1/federation/status" {
			return false
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"status":403,"error":{"code":"forbidden","message":"federation admin required"}}`))
		return true
	}
	kata.intercept.Store(&hook)
	refused := as[model.FederationLoaded](t, run(t, newGateway(t, newLocator(kata.target())), model.LoadFederation{Workspace: t.TempDir(), Request: 6}))
	if refused.Request != 6 || refused.Err == nil || refused.Err.Error() != "federation admin required" {
		t.Errorf("refusal %#v, want kata's message", refused)
	}
}
