package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/data"
	"github.com/Dangerboi/tern-kata/internal/model"
	"go.kenn.io/kata"
	"go.kenn.io/kata/pkg/client"
)

const testHost = "TestBox"

// subcommandFixture is a git repository bound to a Project on an in-process kata daemon, holding one Issue, with the
// working directory inside it, as `tern-kata attach` and `detach` expect.
type subcommandFixture struct {
	repo, uid string
	locate    data.Locator
}

func newSubcommandFixture(t *testing.T) subcommandFixture {
	t.Helper()
	t.Setenv("KATA_HOME", t.TempDir())
	t.Setenv("KATA_SERVER", "")
	t.Setenv("KATA_AUTH_TOKEN", "")
	service, err := kata.New(context.Background(), kata.Config{
		DSN:  filepath.Join(t.TempDir(), "kata.db"),
		Auth: kata.AuthConfig{TrustCallerAuthentication: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	server := newPingServer(t, service.Handler())
	c, err := client.New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	f := subcommandFixture{repo: t.TempDir(), locate: func(context.Context, string) (data.Target, error) { return data.Target{Client: c}, nil }}
	for _, args := range [][]string{{"init", "-q"}, {"commit", "-q", "--allow-empty", "-m", "init"}} {
		if out, err := exec.Command("git", append([]string{"-C", f.repo, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	gateway := data.NewGateway(f.locate).WithHost(testHost)
	defer gateway.Close()
	saved := send(t, gateway, model.BindProject{Workspace: f.repo, Actor: "tester", Name: "demo"}).(model.ProjectSaved)
	if saved.Err != nil {
		t.Fatal(saved.Err)
	}
	created := send(t, gateway, model.CreateIssueCmd{Workspace: f.repo, Actor: "tester", ProjectID: saved.Project.ID, Title: "Screenshot", Body: "body"}).(model.IssueCreated)
	if created.Err != nil {
		t.Fatal(created.Err)
	}
	f.uid = created.UID
	t.Chdir(f.repo)
	return f
}

func send(t *testing.T, gateway *data.Gateway, cmd model.Cmd) model.Msg {
	t.Helper()
	results := make(chan model.Msg, 1)
	gateway.Run(context.Background(), cmd, results)
	return <-results
}

// runPlan runs one attach or detach through runSubcommand, returning what it wrote to stdout and its error.
func (f subcommandFixture) runPlan(t *testing.T, subcommand string, operands ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	plan := LaunchPlan{Subcommand: subcommand, TargetRef: f.uid, Actor: "tester", Operands: operands}
	err := runSubcommand(context.Background(), plan, data.NewGateway(f.locate).WithHost(testHost), &out)
	return out.String(), err
}

func writeSource(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAttachPrintsEachStoredPathAndStopsAtTheFirstFailure(t *testing.T) {
	f := newSubcommandFixture(t)
	a, b := writeSource(t, "a.png"), writeSource(t, "b.png")
	out, err := f.runPlan(t, "attach", a, filepath.Join(t.TempDir(), "missing.png"), b)
	store := filepath.Join(f.repo, ".kata-attachments", f.uid)
	if _, is := err.(subcommandError); !is || out != filepath.Join(store, "a.png")+"\n" {
		t.Fatalf("stdout %q, err %v; want a's stored path then the failure on the missing file", out, err)
	}
	if _, statErr := os.Stat(filepath.Join(store, "b.png")); statErr == nil {
		t.Fatal("b.png was attached after the failure")
	}
	out, err = f.runPlan(t, "attach", a, b)
	if err != nil || out != filepath.Join(store, "a-2.png")+"\n"+filepath.Join(store, "b.png")+"\n" {
		t.Fatalf("stdout %q, err %v", out, err)
	}
}

func TestDetachPrintsItsReport(t *testing.T) {
	f := newSubcommandFixture(t)
	stored, _ := f.runPlan(t, "attach", writeSource(t, "a.png"))
	stored = strings.TrimSpace(stored)
	out, err := f.runPlan(t, "detach", "a.png")
	if err != nil || out != "Removed a.png and deleted "+stored+"\n" {
		t.Fatalf("stdout %q, err %v", out, err)
	}
}

func TestAPartialDetachPrintsItsReportAndFails(t *testing.T) {
	f := newSubcommandFixture(t)
	stored, _ := f.runPlan(t, "attach", writeSource(t, "a.png"))
	stored = strings.TrimSpace(stored)
	dir := filepath.Dir(stored)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if probe, err := os.CreateTemp(dir, "probe"); err == nil {
		probe.Close()
		t.Skip("a read-only folder is not enforced here")
	}
	out, err := f.runPlan(t, "detach", "a.png")
	if _, is := err.(subcommandError); !is || !strings.HasPrefix(out, "Removed a.png; could not delete "+stored+": ") {
		t.Fatalf("stdout %q, err %v; want the partial report and an error", out, err)
	}
}

func TestARefusedDetachPrintsNothing(t *testing.T) {
	f := newSubcommandFixture(t)
	out, err := f.runPlan(t, "detach", "nope.png")
	if _, is := err.(subcommandError); !is || out != "" {
		t.Fatalf("stdout %q, err %v", out, err)
	}
}

// newPingServer serves service over HTTP, answering ping for the release version a service built from a test does not report.
func newPingServer(t *testing.T, service http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/ping" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "service": "kata", "version": "0.19.0"})
			return
		}
		service.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	return server
}
