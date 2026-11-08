package data

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	g "go.kenn.io/kata/pkg/client/generated"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestSameURLIgnoresSchemeAndHostCaseDefaultPortsAndTrailingSlash(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		same bool
	}{
		{"HTTPS://Hub.Example.com/", "https://hub.example.com", true},
		{"https://hub.example.com:443", "https://hub.example.com", true},
		{"http://hub.example.com:80/", "http://hub.example.com", true},
		{"https://hub.example.com:7474", "https://hub.example.com", false},
		{"http://hub.example.com:443", "http://hub.example.com", false},
		{"https://hub.example.com/a", "https://hub.example.com", false},
		{"https://hub.example.com", "http://hub.example.com", false},
	} {
		if got := sameURL(tc.a, tc.b); got != tc.same {
			t.Errorf("sameURL(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.same)
		}
	}
}

func TestTheHubTokenIsFoundForACatalogUrlInAnotherSpelling(t *testing.T) {
	t.Setenv("KATA_HOME", t.TempDir())
	t.Setenv("HUB_TOKEN", "hub-token")
	// The entry has no allow_insecure, so a match refuses to send a token over http; no match sends the binding's insecure request.
	config := "[[daemon]]\nname = \"hub\"\nurl = \"HTTP://Hub.Example:80/\"\ntoken_env = \"HUB_TOKEN\"\n"
	if err := os.WriteFile(filepath.Join(os.Getenv("KATA_HOME"), "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := hubAdminClient(context.Background(), "http://hub.example", true); err == nil {
		t.Fatal("the hub's catalog entry was not matched, so its token would go unsent")
	}
}

func TestAnUnnamedConnectionKeepsItsPinnedUrlAsEndpointAndRecoveryHost(t *testing.T) {
	c := &Client{kind: model.DaemonRemote, pinURL: "https://pinned.example:7474"}
	t.Setenv("KATA_SERVER", "")
	name, endpoint, _ := c.activeDaemon(g.AuthInfoOut{})
	if endpoint != "https://pinned.example:7474" || name != "pinned.example:7474" {
		t.Errorf("name %q endpoint %q, want the pinned URL and its host", name, endpoint)
	}
	rec := c.recoveryBase(model.FederationEnrollDraft{SpokeProjectName: "p"}, CatalogEntry{Name: "hub"}, "https://hub", model.Project{ID: 1}, "tok", "alice")
	if rec.SpokeEndpoint != "https://pinned.example:7474" || strings.Contains(rec.Command.SpokeEndpoint, "remote") {
		t.Errorf("recovery endpoint %q, want the pinned URL", rec.SpokeEndpoint)
	}
}

func TestAnUnnamedConnectionIsTheCatalogEntryAtItsUrl(t *testing.T) {
	t.Setenv("KATA_SERVER", "")
	entry := CatalogEntry{Name: "hub", URL: "HTTPS://Pinned.Example:443/"}
	pinned := &Client{kind: model.DaemonRemote, pinURL: "https://pinned.example"}
	if !pinned.isActive(entry) {
		t.Error("a url-pinned connection did not match the entry at its URL")
	}
	if (&Client{kind: model.DaemonRemote, pinURL: "https://other.example"}).isActive(entry) {
		t.Error("a connection at another URL matched the entry")
	}
	if !(&Client{kind: model.DaemonRemote, name: "hub", entry: entry}).isActive(entry) {
		t.Error("a named connection did not match its own entry")
	}
	if (&Client{kind: model.DaemonRemote, name: "other", entry: CatalogEntry{Name: "other", URL: "https://other.example"}}).isActive(entry) {
		t.Error("a named connection matched another entry")
	}
	t.Setenv("KATA_SERVER", "https://pinned.example")
	if !(&Client{kind: model.DaemonRemote}).isActive(entry) {
		t.Error("a KATA_SERVER connection did not match the entry at its URL")
	}
	t.Setenv("KATA_HOME", t.TempDir())
	config := "[[daemon]]\nname = \"hub\"\nurl = \"https://pinned.example\"\n"
	if err := os.WriteFile(filepath.Join(os.Getenv("KATA_HOME"), "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Client{kind: model.DaemonRemote}).hubEntry("hub"); err == nil {
		t.Error("the connection's own daemon could be selected as hub")
	}
}

func TestAPinnedUrlReachesTheFederationHeader(t *testing.T) {
	kata := startKata(t, "0.21.0")
	t.Setenv("KATA_SERVER", "")
	workspace := t.TempDir()
	for name, body := range map[string]string{".kata.toml": "version = 1\n", ".kata.local.toml": "version = 1\n[server]\nurl = \"https://pinned.example:7474\"\n"} {
		if err := os.WriteFile(filepath.Join(workspace, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	target := kata.target()
	target.Remote = true
	gateway := newGateway(t, newLocator(target))
	loaded := as[model.FederationLoaded](t, run(t, gateway, model.LoadFederation{Workspace: workspace, Request: 1}))
	if loaded.Err != nil || loaded.Endpoint != "https://pinned.example:7474" {
		t.Fatalf("header %+v, want the pinned URL as endpoint", loaded)
	}
}

func TestASpokesEmbeddingStatusIsCarriedInKatasWords(t *testing.T) {
	reason, lastErr := "producer_unavailable", "hub unreachable"
	spoke := translateSpoke(g.FederationProjectStatus{Embedding: &g.FederationEmbeddingStatus{
		State: "degraded", Reason: &reason, LastError: &lastErr, ArtifactLimit: 10, Limited: true,
		Artifacts: []g.FederationEmbeddingArtifactStatus{{State: "generated"}, {State: "reused"}, {State: "generated"}},
	}})
	want := []string{
		"embedding: degraded", "embedding reason: producer_unavailable", "embedding last error: hub unreachable", "embedding producer: none",
		"embedding artifacts: generated=2 reused=1 incompatible=0 stored_unindexed=0", "embedding inventory: 3 retained / 10 limit (limited)",
	}
	if strings.Join(spoke.Embedding, "\n") != strings.Join(want, "\n") {
		t.Errorf("embedding %q, want %q", spoke.Embedding, want)
	}
	if got := translateSpoke(g.FederationProjectStatus{}).Embedding; got != nil {
		t.Errorf("a status without embedding carried %q", got)
	}
}
