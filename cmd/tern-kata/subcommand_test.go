package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/data"
	"github.com/Dangerboi/tern-kata/internal/data/docbanktest"
	"github.com/Dangerboi/tern-kata/internal/model"
	"go.kenn.io/kata"
	"go.kenn.io/kata/pkg/client"
	g "go.kenn.io/kata/pkg/client/generated"
)

const testHost = "TestBox"

// subcommandFixture is a git repository bound to a Project on an in-process kata daemon, holding one Issue, with the
// working directory inside it, as the subcommands expect.
type subcommandFixture struct {
	repo, uid string
	projectID int64
	locate    data.Locator
}

func newSubcommandFixture(t *testing.T) subcommandFixture {
	t.Helper()
	return newFixtureWithDocbank(t, kata.DocbankConfig{})
}

// newDocbankSubcommandFixture is newSubcommandFixture on a daemon that stores attachments in a fake Docbank.
func newDocbankSubcommandFixture(t *testing.T) subcommandFixture {
	t.Helper()
	fake := docbanktest.New(t)
	return newFixtureWithDocbank(t, kata.DocbankConfig{URL: fake.URL, APIKey: docbanktest.Key})
}

func newFixtureWithDocbank(t *testing.T, docbank kata.DocbankConfig) subcommandFixture {
	t.Helper()
	t.Setenv("KATA_HOME", t.TempDir())
	t.Setenv("KATA_SERVER", "")
	t.Setenv("KATA_AUTH_TOKEN", "")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	service, err := kata.New(context.Background(), kata.Config{
		DSN:     filepath.Join(t.TempDir(), "kata.db"),
		Auth:    kata.AuthConfig{TrustCallerAuthentication: true},
		Docbank: docbank,
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
		if out, err := exec.Command("git", append([]string{"-C", f.repo, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...).CombinedOutput(); err != nil {
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
	f.uid, f.projectID = created.UID, saved.Project.ID
	t.Chdir(f.repo)
	return f
}

func send(t *testing.T, gateway *data.Gateway, cmd model.Cmd) model.Msg {
	t.Helper()
	results := make(chan model.Msg, 1)
	gateway.Run(context.Background(), cmd, results)
	return <-results
}

// runPlan runs one subcommand through runSubcommand, returning what it wrote to stdout and its error.
func (f subcommandFixture) runPlan(t *testing.T, subcommand string, operands ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	plan := LaunchPlan{Subcommand: subcommand, TargetRef: f.uid, Actor: "tester", Operands: operands}
	err := runSubcommand(context.Background(), plan, data.NewGateway(f.locate).WithHost(testHost), &out)
	return out.String(), err
}

// newPingServer serves service over HTTP, answering ping for the release version a service built from a test does not report.
func newPingServer(t *testing.T, service http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/ping" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "service": "kata", "version": "0.21.0"})
			return
		}
		service.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestMigrateAttachmentsMovesTheLocalCopiesAndPrintsTheRest(t *testing.T) {
	f := newDocbankSubcommandFixture(t)
	target, err := f.locate(context.Background(), f.repo)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(f.repo, ".kata-attachments", f.uid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(dir, "shot.png")
	if err := os.WriteFile(local, []byte("pixels"), 0o644); err != nil {
		t.Fatal(err)
	}
	patch := map[string]any{
		"tern-kata.attachment.shot.png": map[string]any{"host": testHost, "path": local},
		"tern-kata.attachment.far.log":  map[string]any{"host": "another-machine", "path": "/x/far.log"},
	}
	if _, err := target.Client.PatchIssueMetadataWithResponse(context.Background(), &g.PatchIssueMetadataRequestOptions{
		PathParams: &g.PatchIssueMetadataPath{ProjectID: f.projectID, Ref: f.uid},
		Body:       &g.PatchIssueMetadataBody{Actor: new("tester"), Patch: patch},
	}); err != nil {
		t.Fatal(err)
	}

	out, err := f.runPlan(t, "migrate-attachments")
	if err != nil || !strings.Contains(out, "Moved 1 attachment(s)") || !strings.Contains(out, "shot.png") || !strings.Contains(out, "another-machine") {
		t.Fatalf("stdout %q, err %v", out, err)
	}
	if _, statErr := os.Stat(local); !os.IsNotExist(statErr) {
		t.Fatalf("the local copy remains: %v", statErr)
	}
	shown, err := target.Client.ShowIssueByUIDWithResponse(context.Background(), &g.ShowIssueByUIDRequestOptions{PathParams: &g.ShowIssueByUIDPath{UID: f.uid}})
	if err != nil {
		t.Fatal(err)
	}
	list := model.Attachments(shown.JSON200.Issue.Metadata)
	if len(list) != 1 || list[0].Name != "shot.png" || list[0].Size != 6 {
		t.Fatalf("the Issue lists %+v", list)
	}
	if _, kept := shown.JSON200.Issue.Metadata["tern-kata.attachment.far.log"]; !kept {
		t.Fatal("the key of the file on another machine was cleared")
	}
}

func TestMigrateAttachmentsRefusesPlainlyWithoutDocbank(t *testing.T) {
	f := newSubcommandFixture(t)
	out, err := f.runPlan(t, "migrate-attachments")
	if !errors.As(err, new(subcommandError)) || out != "" || err.Error() != model.DocbankOffMessage {
		t.Fatalf("stdout %q, err %v", out, err)
	}
}
