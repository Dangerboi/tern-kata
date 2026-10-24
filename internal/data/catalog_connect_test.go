package data

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go.kenn.io/kata"

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
	return serve(t, service.Handler(), "0.20.0")
}

// loadsProject binds a Project through a Gateway that finds its daemon with loc, and returns the daemon the Project loads from.
func loadsProject(t *testing.T, loc Locator) model.Daemon {
	t.Helper()
	gateway := NewGateway(loc)
	t.Cleanup(gateway.Close)
	workspace, _ := project(t, gateway, "demo")
	return as[model.ProjectLoaded](t, run(t, gateway, model.LoadProject{Workspace: workspace})).Daemon
}

func TestGatewayConnectsToARemoteCatalogEntryWithItsToken(t *testing.T) {
	t.Setenv("KATA_HOME", t.TempDir())
	t.Setenv("KATA_SERVER", "")
	t.Setenv("KATA_AUTH_TOKEN", "")
	t.Setenv("WORK_TOKEN", "s3cret")
	server := startTokenKata(t, "s3cret")
	entry := CatalogEntry{Name: "work", URL: server.URL, TokenEnv: "WORK_TOKEN", AllowInsecure: true}

	if d := loadsProject(t, LocateEntry(entry)); d.Kind != model.DaemonRemote {
		t.Fatalf("daemon %+v, want a remote one", d)
	}

	t.Setenv("WORK_TOKEN", "")
	if _, err := LocateEntry(entry)(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "WORK_TOKEN") {
		t.Fatalf("err = %v, want it to name the unset variable", err)
	}
}

func TestGatewayConnectsToALocalCatalogEntryThroughKataLocate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stub kata is a shell script")
	}
	t.Setenv("KATA_HOME", t.TempDir())
	t.Setenv("KATA_SERVER", "")
	t.Setenv("KATA_AUTH_TOKEN", "")
	kataD := startKata(t, "0.20.0")
	bin := t.TempDir()
	args := filepath.Join(bin, "args")
	stub := "#!/bin/sh\necho \"$@\" > '" + args + "'\n" +
		"echo '{\"source\":\"daemon_flag\",\"kind\":\"local\",\"network\":\"tcp\",\"scheme\":\"http\",\"address\":\"" +
		strings.TrimPrefix(kataD.URL, "http://") + "\",\"request_base_url\":\"" + kataD.URL + "\"}'\n"
	if err := os.WriteFile(filepath.Join(bin, "kata"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	if d := loadsProject(t, LocateEntry(CatalogEntry{Name: "mine", Local: true})); d.Kind != model.DaemonLocal {
		t.Fatalf("daemon %+v, want a local one", d)
	}
	if got, _ := os.ReadFile(args); strings.TrimSpace(string(got)) != "--daemon mine daemon locate --json" {
		t.Fatalf("kata ran with %q", got)
	}
}

func TestLocalCatalogEntryThatKataCannotLocateSaysWhy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stub kata is a shell script")
	}
	bin := t.TempDir()
	stub := "#!/bin/sh\necho '{\"error\":{\"message\":\"stopped_local_profile: run kata daemon recover\"}}'\nexit 7\n"
	if err := os.WriteFile(filepath.Join(bin, "kata"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	_, err := LocateEntry(CatalogEntry{Name: "mine", Local: true})(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), `daemon "mine": stopped_local_profile`) {
		t.Fatalf("err = %v", err)
	}
}

func TestGatewayConnectsToATokenProtectedLocalCatalogEntryWithItsToken(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stub kata is a shell script")
	}
	t.Setenv("KATA_HOME", t.TempDir())
	t.Setenv("KATA_SERVER", "")
	t.Setenv("KATA_AUTH_TOKEN", "")
	t.Setenv("MINE_TOKEN", "s3cret")
	protected := startTokenKata(t, "s3cret")
	installStubKata(t, protected.URL, false, "")

	entry := CatalogEntry{Name: "mine", Local: true, TokenEnv: "MINE_TOKEN"}
	if d := loadsProject(t, LocateEntry(entry)); d.Kind != model.DaemonLocal {
		t.Fatalf("daemon %+v, want a local one reached with its token", d)
	}
	entry = CatalogEntry{Name: "mine", Local: true, InlineToken: "s3cret"}
	if d := loadsProject(t, LocateEntry(entry)); d.Kind != model.DaemonLocal {
		t.Fatalf("daemon %+v, want a local one reached with its inline token", d)
	}
	t.Setenv("MINE_TOKEN", "")
	entry = CatalogEntry{Name: "mine", Local: true, TokenEnv: "MINE_TOKEN"}
	if _, err := LocateEntry(entry)(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "MINE_TOKEN") {
		t.Fatalf("err = %v, want it to name the unset variable", err)
	}
}

func TestALocalCatalogEntryWithNoTokenConnectsWithTheImplicitCredentialsKataWouldUse(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stub kata is a shell script")
	}
	profileHome := t.TempDir()
	for name, c := range map[string]struct {
		setup func(kataHome string)
		entry CatalogEntry
	}{
		"the environment token":                   {func(string) { t.Setenv("KATA_AUTH_TOKEN", "s3cret") }, CatalogEntry{Name: "mine", Local: true}},
		"the config auth token":                   {func(home string) { writeCredential(t, filepath.Join(home, "config.toml"), "[auth]\ntoken = \"s3cret\"\n") }, CatalogEntry{Name: "mine", Local: true}},
		"the persisted owner token":               {func(home string) { writeCredential(t, filepath.Join(home, "auth-token"), "s3cret\n") }, CatalogEntry{Name: "mine", Local: true}},
		"the persisted owner token of its profile": {func(string) { writeCredential(t, filepath.Join(profileHome, "auth-token"), "s3cret\n") }, CatalogEntry{Name: "mine", Local: true, Home: profileHome}},
		"its own token over the environment":      {func(string) { t.Setenv("KATA_AUTH_TOKEN", "wrong") }, CatalogEntry{Name: "mine", Local: true, InlineToken: "s3cret"}},
	} {
		home := t.TempDir()
		t.Setenv("KATA_HOME", home)
		t.Setenv("KATA_SERVER", "")
		t.Setenv("KATA_AUTH_TOKEN", "")
		c.setup(home)
		protected := startTokenKata(t, "s3cret")
		installStubKata(t, protected.URL, false, "")
		if d := loadsProject(t, LocateEntry(c.entry)); d.Kind != model.DaemonLocal {
			t.Errorf("%s: daemon %+v, want a local one", name, d)
		}
	}
}

func writeCredential(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
