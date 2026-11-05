package data

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"go.kenn.io/kata"
	"go.kenn.io/kata/pkg/client"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// startTokenKata is startKata behind a bearer token.
func startTokenKata(t *testing.T, token string) *kataServer {
	t.Helper()
	service, err := kata.New(context.Background(), kata.Config{
		DSN:  filepath.Join(t.TempDir(), "kata.db"),
		Auth: kata.AuthConfig{Token: token},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	return serve(t, service.Handler(), "0.21.0")
}

// loadsProject binds a Project through a Gateway that finds its daemon with loc, and returns the daemon the Project loads from.
func loadsProject(t *testing.T, loc Locator) model.Daemon {
	t.Helper()
	gateway := NewGateway(loc)
	t.Cleanup(gateway.Close)
	workspace, _ := project(t, gateway, "demo")
	return as[model.ProjectLoaded](t, run(t, gateway, model.LoadProject{Workspace: workspace})).Daemon
}

func TestLocateNamedConnectsToARemoteCatalogEntryWithItsToken(t *testing.T) {
	t.Setenv("KATA_SERVER", "")
	t.Setenv("KATA_AUTH_TOKEN", "")
	t.Setenv("WORK_TOKEN", "s3cret")
	server := startTokenKata(t, "s3cret")
	writeCatalog(t, "[[daemon]]\nname = \"work\"\nurl = \""+server.URL+"\"\ntoken_env = \"WORK_TOKEN\"\nallow_insecure = true\n")

	if d := loadsProject(t, LocateNamed("work", "")); d.Kind != model.DaemonRemote {
		t.Fatalf("daemon %+v, want a remote one", d)
	}

	t.Setenv("WORK_TOKEN", "")
	if _, err := LocateNamed("work", "")(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "WORK_TOKEN") {
		t.Fatalf("err = %v, want it to name the unset variable", err)
	}
}

func TestLocateNamedRefusesANameTheCatalogLacks(t *testing.T) {
	t.Setenv("KATA_HOME", t.TempDir())
	if _, err := LocateNamed("nowhere", "")(context.Background(), ""); !errors.Is(err, client.ErrDaemonNotInCatalog) {
		t.Fatalf("err = %v, want kata's not-in-catalog error", err)
	}
}
