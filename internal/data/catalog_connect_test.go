package data

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
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

func TestKataExecutableIgnoresAKataFoundThroughARelativePathEntry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stub kata is a shell script")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "kata"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	t.Setenv("PATH", ".")
	if got := kataExecutable(); got != "" {
		t.Fatalf("kataExecutable = %q, want none: kata would run a relative path without looking it up again", got)
	}
}

func TestLocateNamedConnectsToARemoteCatalogEntryWithItsToken(t *testing.T) {
	t.Setenv("KATA_HOME", t.TempDir()) // startTokenKata sets none, and the catalog is written into it
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

func TestSwitchingToARemoteEntrySendsItsOwnTokenAndNeverTheGlobalOne(t *testing.T) {
	t.Setenv("KATA_HOME", t.TempDir())
	t.Setenv("KATA_SERVER", "")
	t.Setenv("KATA_AUTH_TOKEN", "global-secret")
	t.Setenv("WORK_TOKEN", "work-secret")
	server := startKata(t, "0.21.0")
	log := watch(server, nil)
	writeCatalog(t, "[[daemon]]\nname = \"work\"\nurl = \""+server.URL+"\"\ntoken_env = \"WORK_TOKEN\"\nallow_insecure = true\n")

	gateway := newGateway(t, newLocator(server.target()))
	if got := switched(t, gateway, t.TempDir(), "work"); got.Err != nil {
		t.Fatalf("switch: %v", got.Err)
	}
	var reads []call
	for _, suffix := range []string{"/api/v1/instance", "/api/v1/projects"} {
		reads = append(reads, log.seen(suffix)...)
	}
	if len(reads) == 0 {
		t.Fatal("the daemon saw no read")
	}
	for _, c := range reads {
		if c.auth != "Bearer work-secret" {
			t.Errorf("%s %s carried %q, want the entry's own token", c.method, c.path, c.auth)
		}
	}
}

func TestLocateNamedReportsAnUnreadableConfigInsteadOfDiscovering(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KATA_HOME", home)
	t.Setenv("KATA_AUTH_TOKEN", "local-secret")
	config := "[[daemon]]\nname = \"far\"\nurl = \"https://far.example\"\ntoken = \"remote\"\n\n[[daemon]]\nname = \"far\"\nurl = \"https://other.example\"\n"
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LocateNamed("far", "")(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "config.toml") {
		t.Fatalf("LocateNamed error = %v, want one naming config.toml", err)
	}
}

func TestReadCatalogKeepsDaemonsWhenTUIIsMalformed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KATA_HOME", home)
	config := "[tui]\nconfirm_quit = \"yes\"\n\n[[daemon]]\nname = \"far\"\nurl = \"https://far.example\"\n"
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog, err := ReadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := catalog.Entry("far"); !ok || !catalog.ConfirmQuit {
		t.Fatalf("catalog = %+v, want the far entry and the default confirm_quit", catalog)
	}
}

func TestDialReportsAnUnreadableConfigOnlyWhereItIsUsed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KATA_HOME", home)
	t.Setenv("KATA_SERVER", "")
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("active_daemon = \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	located := false
	gateway := NewGateway(func(context.Context, string) (Target, error) {
		located = true
		return Target{}, errors.New("located")
	})
	t.Cleanup(gateway.Close)
	ctx := context.Background()
	_, err := gateway.dial(ctx, ctx, gateway.locate, "", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "config.toml") || located {
		t.Fatalf("dial error = %v, located = %v, want a config.toml error without locating", err, located)
	}
	t.Setenv("KATA_SERVER", "http://127.0.0.1:1")
	if _, err := gateway.dial(ctx, ctx, gateway.locate, "", t.TempDir()); err == nil || strings.Contains(err.Error(), "config.toml") || !located {
		t.Fatalf("with KATA_SERVER set, dial error = %v, located = %v, want the locator's error", err, located)
	}
}
