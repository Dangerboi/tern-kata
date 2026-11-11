package data

import (
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"go.kenn.io/kata/pkg/client"
	g "go.kenn.io/kata/pkg/client/generated"
	"go.kenn.io/kata/pkg/katatest"
	kitdaemon "go.kenn.io/kit/daemon"
)

func unixConsumerHome(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Unix socket consumer contract")
	}
	home := t.TempDir()
	routeEnv(t, map[string]string{"KATA_HOME": home, "KATA_DB": filepath.Join(home, "kata.db")})
	t.Chdir(t.TempDir())
	return home
}

// These are authenticated, persisted operations, not just public ping or a
// transport echo: a missing/wrong credential or a different daemon must fail.
func unixConsumerCreateProject(t *testing.T, api *client.Client, name string) {
	t.Helper()
	actor := "tester"
	resp, err := api.InitProjectWithResponse(t.Context(), &g.InitProjectRequestOptions{
		Body: &g.InitProjectBody{Name: &name, Actor: &actor},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create project: status %d, body %s", resp.StatusCode, resp.Body)
	}
}

func unixConsumerWantProjects(t *testing.T, api *client.Client, want string) {
	t.Helper()
	resp, err := api.ListProjectsWithResponse(t.Context(), &g.ListProjectsRequestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || resp.JSON200 == nil {
		t.Fatalf("list projects: status %d, body %s", resp.StatusCode, resp.Body)
	}
	if len(resp.JSON200.Projects) != 1 || resp.JSON200.Projects[0].Name != want {
		t.Fatalf("projects = %+v, want only %q", resp.JSON200.Projects, want)
	}
}

func unixConsumerWorkspace(t *testing.T, endpoint string) string {
	t.Helper()
	dir := t.TempDir()
	for name, text := range map[string]string{
		".kata.toml":       "",
		".kata.local.toml": fmt.Sprintf("version = 1\n[server]\nurl = %q\n", endpoint),
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLocateWithKataExplicitUnixWithoutRuntime(t *testing.T) {
	for _, source := range []string{"environment", "workspace"} {
		t.Run(source, func(t *testing.T) {
			unixConsumerHome(t)
			server := katatest.New(t, katatest.WithUnixSocket(), katatest.WithToken("selected-secret"))
			t.Setenv("KATA_AUTH_TOKEN_FILE", tokenFile(t, "selected-secret\n"))
			workspace := t.TempDir()
			if source == "environment" {
				t.Setenv("KATA_SERVER", server.Endpoint)
			} else {
				workspace = unixConsumerWorkspace(t, server.Endpoint)
			}
			target, err := LocateWithKata(t.Context(), workspace)
			if err != nil {
				t.Fatal(err)
			}
			unixConsumerCreateProject(t, target.Client, "selected-project")
			unixConsumerWantProjects(t, server.Client(t), "selected-project")
			// Rejecting the wrong file proves this was an authenticated operation.
			t.Setenv("KATA_AUTH_TOKEN_FILE", tokenFile(t, "wrong-secret\n"))
			wrong, err := LocateWithKata(t.Context(), workspace)
			if err == nil {
				resp, readErr := wrong.Client.ListProjectsWithResponse(t.Context(), &g.ListProjectsRequestOptions{})
				if readErr == nil && resp.StatusCode == http.StatusOK {
					t.Fatal("wrong file credential could read the selected daemon")
				}
			}
		})
	}
}

// Publish the external SDK's on-disk runtime boundary, not a fake locator. The
// current process owns the record, just like Kata's own discovery fixtures.
func unixConsumerDefaultRuntime(t *testing.T, home, endpoint string) {
	t.Helper()
	db := filepath.Join(home, "kata.db")
	sum := sha256.Sum256([]byte(db))
	dir := filepath.Join(home, "runtime", fmt.Sprintf("%x", sum)[:12])
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := (kitdaemon.RuntimeStore{Dir: dir}).Write(kitdaemon.RuntimeRecord{
		PID: os.Getpid(), Address: endpoint, StartedAt: time.Now().UTC(),
		Metadata: map[string]string{"db_path": db},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLocateWithKataMissingSelectedUnixNeverFallsBackOrStarts(t *testing.T) {
	for _, source := range []string{"environment", "workspace"} {
		t.Run(source, func(t *testing.T) {
			home := unixConsumerHome(t)
			fallback := katatest.New(t, katatest.WithUnixSocket(), katatest.WithToken("selected-secret"))
			unixConsumerDefaultRuntime(t, home, fallback.Endpoint)
			t.Setenv("KATA_AUTH_TOKEN", "selected-secret")
			// Establish that the tempting default runtime is actually discoverable.
			local, err := LocateWithKata(t.Context(), "")
			if err != nil {
				t.Fatal(err)
			}
			unixConsumerCreateProject(t, local.Client, "default-project")
			selected := katatest.New(t, katatest.WithUnixSocket(), katatest.WithToken("selected-secret"))
			workspace := t.TempDir()
			if source == "environment" {
				t.Setenv("KATA_SERVER", selected.Endpoint)
			} else {
				workspace = unixConsumerWorkspace(t, selected.Endpoint)
			}
			target, err := LocateWithKata(t.Context(), workspace)
			if err != nil {
				t.Fatal(err)
			}
			unixConsumerCreateProject(t, target.Client, "selected-project")
			// Unlinking a live fixture's socket makes new dials fail without
			// changing either selection source or its server's persisted state.
			if err := os.Remove(strings.TrimPrefix(selected.Endpoint, "unix://")); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(t.TempDir(), "started")
			bin := t.TempDir()
			if err := os.WriteFile(filepath.Join(bin, "kata"), []byte("#!/bin/sh\nprintf started > '"+marker+"'\nexit 1\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin)
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			retry, err := LocateWithKata(ctx, workspace)
			if err == nil {
				resp, readErr := retry.Client.ListProjectsWithResponse(ctx, &g.ListProjectsRequestOptions{})
				if readErr == nil && resp.StatusCode == http.StatusOK {
					t.Fatalf("missing selected socket returned projects: %+v", resp.JSON200)
				}
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("discovery invoked kata: marker error %v", err)
			}
			unixConsumerWantProjects(t, fallback.Client(t), "default-project")
		})
	}
}

func unixConsumerApplyRoute(t *testing.T, route AgentRoute) {
	t.Helper()
	private, err := route.withSecretFiles(filepath.Join(t.TempDir(), "prompt"))
	if err != nil {
		t.Fatal(err)
	}
	set, unset, err := private.Env("")
	if err != nil {
		t.Fatal(err)
	}
	// A block starts in another environment; neither its endpoint nor token
	// may leak into the selected route.
	t.Setenv("KATA_SERVER", "unix:///unrelated-block.sock")
	t.Setenv("KATA_AUTH_TOKEN", "block-secret")
	t.Setenv("KATA_AUTH_TOKEN_FILE", "/unrelated-block-token")
	for _, name := range unset {
		t.Setenv(name, "")
	}
	for _, line := range set {
		name, value, _ := strings.Cut(line, "=")
		t.Setenv(name, value)
	}
}

func TestAgentRouteExplicitUnixPreservesFileCredentialPolicy(t *testing.T) {
	unixConsumerHome(t)
	server := katatest.New(t, katatest.WithUnixSocket(), katatest.WithToken("rotated-secret"))
	file := tokenFile(t, "original-secret\n")
	t.Setenv("KATA_SERVER", server.Endpoint)
	t.Setenv("KATA_AUTH_TOKEN_FILE", file)
	gateway := NewGateway(nil)
	t.Cleanup(gateway.Close)
	route, err := gateway.CurrentAgentRoute()
	if err != nil {
		t.Fatal(err)
	}
	unixConsumerApplyRoute(t, route)
	// The Pane's file is a reference, not a snapshot promoted to inline auth.
	if err := os.WriteFile(file, []byte("rotated-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	target, err := LocateWithKata(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	unixConsumerCreateProject(t, target.Client, "agent-project")
	unixConsumerWantProjects(t, server.Client(t), "agent-project")
	if os.Getenv("KATA_AUTH_TOKEN") != "" || os.Getenv("KATA_AUTH_TOKEN_FILE") != file {
		t.Fatal("agent route changed file credential precedence")
	}
}

func TestLocateNamedUnixAndAgentRouteIsolateCatalogToken(t *testing.T) {
	unixConsumerHome(t)
	server := katatest.New(t, katatest.WithUnixSocket(), katatest.WithToken("named-secret"))
	t.Setenv("WORK_TOKEN", "named-secret")
	t.Setenv("KATA_AUTH_TOKEN", "ambient-secret")
	t.Setenv("KATA_AUTH_TOKEN_FILE", tokenFile(t, "ambient-file-secret\n"))
	writeCatalog(t, fmt.Sprintf("[[daemon]]\nname = \"work\"\nurl = %q\ntoken_env = \"WORK_TOKEN\"\n", server.Endpoint))
	target, err := LocateNamed("work", "")(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	unixConsumerCreateProject(t, target.Client, "named-project")
	gateway := NewGateway(nil)
	t.Cleanup(gateway.Close)
	gateway.current = "work"
	route, err := gateway.CurrentAgentRoute()
	if err != nil {
		t.Fatal(err)
	}
	unixConsumerApplyRoute(t, route)
	// Named credentials are captured for this connection, unlike the Pane's
	// KATA_AUTH_TOKEN_FILE reference; later ambient changes must not replace it.
	t.Setenv("WORK_TOKEN", "changed-after-route")
	agent, err := LocateWithKata(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	unixConsumerWantProjects(t, agent.Client, "named-project")
	if os.Getenv("KATA_AUTH_TOKEN_FILE") != "" || os.Getenv("KATA_AUTH_TOKEN") != "named-secret" {
		t.Fatal("named agent route inherited ambient credentials")
	}
}

// Tern is the external boundary. Launch its emitted route in a separate
// process at the actual --cwd and perform an authenticated, persisted write.
func TestAgentLaunchWorkspaceUnixKeepsSelectedEndpoint(t *testing.T) {
	unixConsumerHome(t)
	server := katatest.New(t, katatest.WithUnixSocket(), katatest.WithToken("workspace-secret"))
	workspace := unixConsumerWorkspace(t, server.Endpoint)
	t.Setenv("KATA_AUTH_TOKEN_FILE", tokenFile(t, "workspace-secret\n"))
	gateway, dirs := agentGateway(t)
	writeAgentFile(t, filepath.Join(dirs.Config, "agent.toml"), "")
	gateway.runTern = func(ctx context.Context, args []string) ([]byte, error) {
		cwdIndex, agentIndex := slices.Index(args, "--cwd"), slices.Index(args, "agent")
		if cwdIndex < 0 || cwdIndex+1 >= len(args) || agentIndex < 0 {
			return nil, fmt.Errorf("unexpected Tern launch: %q", args)
		}
		executable, err := os.Executable()
		if err != nil {
			return nil, err
		}
		child := exec.CommandContext(ctx, executable, append([]string{"-test.run=^TestUnixAgentRouteProcess$", "--"}, args[agentIndex+1:]...)...)
		child.Dir = args[cwdIndex+1]
		child.Env = append(os.Environ(), "TERN_KATA_UNIX_AGENT_PROCESS=1")
		if out, err := child.CombinedOutput(); err != nil {
			return out, err
		}
		return []byte(`{"session":1,"tab":2,"block":31}`), nil
	}
	got := as[model.AgentStarted](t, run(t, gateway, model.StartAgent{
		Placement: model.AgentTab, Workspace: workspace, ProjectName: "workspace-project", IssueRef: "workspace-project#cd34",
	}))
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	unixConsumerWantProjects(t, server.Client(t), "spawned-agent-project")
}

func TestUnixAgentRouteProcess(t *testing.T) {
	if os.Getenv("TERN_KATA_UNIX_AGENT_PROCESS") != "1" {
		t.Skip("subprocess helper")
	}
	separator := slices.Index(os.Args, "--")
	if separator < 0 {
		t.Fatal("missing agent arguments")
	}
	var route AgentRoute
	flags := flag.NewFlagSet("agent-route", flag.ContinueOnError)
	flags.StringVar(&route.Home, "home", "", "")
	flags.StringVar(&route.Daemon, "daemon", "", "")
	flags.StringVar(&route.Server, "server", "", "")
	flags.StringVar(&route.TokenFile, "token-file", "", "")
	flags.StringVar(&route.AuthFile, "auth-file", "", "")
	flags.StringVar(&route.DB, "db", "", "")
	flags.StringVar(&route.Schema, "schema", "", "")
	flags.StringVar(&route.DSNFile, "dsn-file", "", "")
	flags.StringVar(&route.EnvFile, "env-file", "", "")
	flags.BoolVar(&route.AllowInsecure, "allow-insecure", false, "")
	flags.BoolVar(&route.TrustPrivate, "trust-private-network", false, "")
	flags.String("ref", "", "")
	if err := flags.Parse(os.Args[separator+1:]); err != nil {
		t.Fatal(err)
	}
	set, unset, err := route.Env("")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range unset {
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
	for _, line := range set {
		name, value, _ := strings.Cut(line, "=")
		t.Setenv(name, value)
	}
	target, err := LocateWithKata(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	unixConsumerCreateProject(t, target.Client, "spawned-agent-project")
}
