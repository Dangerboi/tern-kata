package data

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// issueFixture is a git repository bound to a Project on an in-process daemon, holding one Issue.
type issueFixture struct {
	kata      *kataServer
	gateway   *Gateway
	repo, uid string
	project   model.Project
	host      string
}

func newIssueFixture(t *testing.T) issueFixture {
	t.Helper()
	return fixtureOn(t, startKata(t, "0.21.0"))
}

// fixtureOn is a fixture whose Project lives on the daemon kata.
func fixtureOn(t *testing.T, kata *kataServer) issueFixture {
	t.Helper()
	const host = "TestBox"
	gateway := newGateway(t, newLocator(kata.target())).WithHost(host)
	repo := t.TempDir()
	git(t, repo, "init", "-q")
	git(t, repo, "commit", "-q", "--allow-empty", "-m", "init")
	saved := as[model.ProjectSaved](t, run(t, gateway, model.BindProject{Workspace: repo, Actor: "tester", Name: "demo"}))
	if saved.Err != nil {
		t.Fatal(saved.Err)
	}
	return issueFixture{kata, gateway, repo, createIssue(t, gateway, repo, saved.Project, "Screenshot"), saved.Project, host}
}

func (f issueFixture) issue(t *testing.T, uid string) g.ShowIssueOut {
	t.Helper()
	response, err := f.kata.target().Client.ShowIssueByUIDWithResponse(context.Background(), &g.ShowIssueByUIDRequestOptions{PathParams: &g.ShowIssueByUIDPath{UID: uid}})
	if err != nil {
		t.Fatal(err)
	}
	return response.JSON200.Issue
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
