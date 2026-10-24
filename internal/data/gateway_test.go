package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	g "go.kenn.io/kata/pkg/client/generated"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/worker"
	"go.kenn.io/kata"
	"go.kenn.io/kata/pkg/client"
)

// kataServer is kata's own service, run in process over HTTP with a database of
// its own. The service reports no release version when built from a test, so
// ping answers for it with the version under test.
type kataServer struct {
	*httptest.Server
	handler http.Handler
	// intercept, when set, sees every request first; one that answers it returns true.
	intercept atomic.Pointer[func(http.ResponseWriter, *http.Request) bool]
}

func startKata(t *testing.T, version string) *kataServer {
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
	return serve(t, service.Handler(), version)
}

func serve(t *testing.T, service http.Handler, version string) *kataServer {
	t.Helper()
	srv := &kataServer{handler: service}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/ping" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "service": "kata", "version": version})
			return
		}
		if hook := srv.intercept.Load(); hook != nil && (*hook)(w, r) {
			return
		}
		service.ServeHTTP(w, r)
	})
	srv.Server = httptest.NewServer(handler)
	t.Cleanup(srv.Server.Close)
	return srv
}

func (s *kataServer) target() Target {
	c, err := client.New(s.URL)
	if err != nil {
		panic(err)
	}
	return Target{Client: c}
}

// locator counts its calls and answers with whichever Target is current.
type locator struct {
	calls  atomic.Int32
	target atomic.Pointer[Target]
	err    atomic.Pointer[error]
}

func newLocator(target Target) *locator {
	l := &locator{}
	l.target.Store(&target)
	return l
}

func (l *locator) locate(context.Context, string) (Target, error) {
	l.calls.Add(1)
	if err := l.err.Load(); err != nil {
		return Target{}, *err
	}
	return *l.target.Load(), nil
}

func newGateway(t *testing.T, l *locator) *Gateway {
	t.Helper()
	gateway := NewGateway(l.locate)
	state := t.TempDir()
	gateway.workerDirs = func() (worker.Dirs, error) { return worker.Dirs{Config: state, State: state}, nil }
	t.Cleanup(gateway.Close)
	return gateway
}

func run(t *testing.T, gateway *Gateway, cmd model.Cmd) model.Msg {
	t.Helper()
	messages := make(chan model.Msg, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	gateway.Run(ctx, cmd, messages)
	select {
	case msg := <-messages:
		return msg
	default:
		t.Fatalf("%T produced no message", cmd)
		return nil
	}
}

func as[T model.Msg](t *testing.T, msg model.Msg) T {
	t.Helper()
	got, ok := msg.(T)
	if !ok {
		t.Fatalf("got %T %+v, want %T", msg, msg, got)
	}
	return got
}

// project binds a fresh workspace to a Project on the daemon behind gateway.
func project(t *testing.T, gateway *Gateway, name string) (string, model.Project) {
	t.Helper()
	workspace := t.TempDir()
	saved := as[model.ProjectSaved](t, run(t, gateway, model.BindProject{Workspace: workspace, Actor: "tester", Name: name}))
	if saved.Err != nil {
		t.Fatal(saved.Err)
	}
	return workspace, saved.Project
}

func createIssue(t *testing.T, gateway *Gateway, workspace string, project model.Project, title string) string {
	t.Helper()
	created := as[model.IssueCreated](t, run(t, gateway, model.CreateIssueCmd{Workspace: workspace, Actor: "tester", ProjectID: project.ID, Title: title, Body: "body"}))
	if created.Err != nil || created.UID == "" {
		t.Fatalf("created %+v", created)
	}
	return created.UID
}

func TestGatewayBindsAndCreatesProjects(t *testing.T) {
	kata := startKata(t, "0.20.0")
	gateway := newGateway(t, newLocator(kata.target()))

	workspace, bound := project(t, gateway, "demo")
	binding, err := os.ReadFile(filepath.Join(workspace, ".kata.toml"))
	if err != nil || !strings.Contains(string(binding), `name = "demo"`) {
		t.Fatalf(".kata.toml %q, %v", binding, err)
	}
	loaded := as[model.ProjectLoaded](t, run(t, gateway, model.LoadProject{Workspace: workspace}))
	if loaded.Project.UID != bound.UID || loaded.Daemon.Kind != model.DaemonLocal || loaded.Daemon.Version != "v0.20.0" {
		t.Fatalf("workspace resolves to %+v, daemon %+v; want the bound Project %q on a local v0.20.0 daemon", loaded.Project, loaded.Daemon, bound.UID)
	}

	other := t.TempDir()
	created := as[model.ProjectSaved](t, run(t, gateway, model.CreateProject{Workspace: other, Actor: "tester", Name: "extra"}))
	if created.Err != nil || created.Project.Name != "extra" {
		t.Fatalf("created %+v", created)
	}
	if _, err := os.Stat(filepath.Join(other, ".kata.toml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("creating a Project must not bind the workspace: %v", err)
	}
	again := as[model.ProjectSaved](t, run(t, gateway, model.CreateProject{Workspace: other, Actor: "tester", Name: "extra"}))
	if again.Err == nil || !strings.Contains(again.Err.Error(), "already exists") {
		t.Fatalf("second create %+v, want an already-exists error", again)
	}

	rebound := as[model.ProjectSaved](t, run(t, gateway, model.BindProject{Workspace: workspace, Actor: "tester", Name: "demo"}))
	if rebound.Err != nil || rebound.Project.UID != bound.UID {
		t.Fatalf("binding the bound name again: %+v, want the same Project", rebound)
	}
	elsewhere := as[model.ProjectSaved](t, run(t, gateway, model.BindProject{Workspace: workspace, Actor: "tester", Name: "extra"}))
	if elsewhere.Err == nil || !strings.Contains(elsewhere.Err.Error(), "binds this directory to Project") {
		t.Fatalf("binding a bound directory to another name: %+v", elsewhere)
	}
}

// TestGatewayResolvesTheBindingTheSameForBothDaemonKinds runs every workspace
// shape against the in-process daemon, with the kind chosen by the Locator.
func TestGatewayResolvesTheBindingTheSameForBothDaemonKinds(t *testing.T) {
	git := func(t *testing.T, dir string, args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	repo := func(t *testing.T, remote string) string {
		dir := t.TempDir()
		git(t, dir, "init", "-q")
		git(t, dir, "remote", "add", "origin", remote)
		return dir
	}
	for _, remote := range []bool{false, true} {
		kind := map[bool]string{false: "local", true: "remote"}[remote]
		kata := startKata(t, "0.20.0")
		target := kata.target()
		target.Remote = func(context.Context) (bool, error) { return remote, nil }
		gateway := newGateway(t, newLocator(target))
		load := func(workspace string) (model.ProjectLoaded, error) {
			t.Helper()
			msg := run(t, gateway, model.LoadProject{Workspace: workspace})
			if failed, ok := msg.(model.Failed); ok {
				return model.ProjectLoaded{}, failed.Err
			}
			return as[model.ProjectLoaded](t, msg), nil
		}

		t.Run(kind+" bound workspace", func(t *testing.T) {
			workspace, bound := project(t, gateway, "demo-"+kind)
			if got, err := load(workspace); err != nil || got.Project.UID != bound.UID || got.Daemon.Kind != map[bool]model.DaemonKind{false: model.DaemonLocal, true: model.DaemonRemote}[remote] {
				t.Fatalf("got %+v, %v, want the bound Project on a %s daemon", got, err, kind)
			}
		})
		t.Run(kind+" alias-only repository the daemon knows", func(t *testing.T) {
			workspace := repo(t, "https://example.com/acme/known-"+kind+".git")
			bound := as[model.ProjectSaved](t, run(t, gateway, model.BindProject{Workspace: workspace, Actor: "tester", Name: "known-" + kind}))
			if bound.Err != nil {
				t.Fatal(bound.Err)
			}
			if err := os.Remove(filepath.Join(workspace, ".kata.toml")); err != nil {
				t.Fatal(err)
			}
			if got, err := load(workspace); err != nil || got.Project.UID != bound.Project.UID {
				t.Fatalf("got %+v, %v, want the Project the alias names", got, err)
			}
		})
		t.Run(kind+" alias-only repository the daemon does not know", func(t *testing.T) {
			got, err := load(repo(t, "https://example.com/acme/unknown-"+kind+".git"))
			if err != nil || got.Project.UID != "" {
				t.Fatalf("got %+v, %v, want the Inbox", got, err)
			}
		})
		t.Run(kind+" unbound directory", func(t *testing.T) {
			got, err := load(t.TempDir())
			if err != nil || got.Project.UID != "" {
				t.Fatalf("got %+v, %v, want the Inbox", got, err)
			}
		})
		t.Run(kind+" named workspace the daemon does not know", func(t *testing.T) {
			workspace := t.TempDir()
			if err := os.WriteFile(filepath.Join(workspace, ".kata.toml"), []byte("version = 1\n\n[project]\nname = \"ghost\"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := load(workspace)
			if err == nil || !strings.Contains(err.Error(), "kata init") {
				t.Fatalf("got %v, want kata's message and its kata init hint", err)
			}
		})
		t.Run(kind+" named workspace whose git remotes cannot be read", func(t *testing.T) {
			workspace, bound := project(t, gateway, "unreadable-"+kind)
			if err := os.Mkdir(filepath.Join(workspace, ".git"), 0o755); err != nil {
				t.Fatal(err)
			}
			if got, err := load(workspace); err != nil || got.Project.UID != bound.UID {
				t.Fatalf("got %+v, %v, want the Project resolved by name", got, err)
			}
		})
	}
}

func TestGatewayReadsAndWritesIssues(t *testing.T) {
	kata := startKata(t, "0.20.0")
	l := newLocator(kata.target())
	gateway := newGateway(t, l)
	workspace, demo := project(t, gateway, "demo")

	uid := createIssue(t, gateway, workspace, demo, "First")

	detail := as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: uid}))
	issue := detail.Selected.Issue
	if detail.UID != uid || issue == nil || issue.Title != "First" {
		t.Fatalf("detail %+v", detail)
	}

	collection := model.Collection{View: "project", ProjectUID: demo.UID, Title: demo.Name}
	query := model.CollectionQuery(collection, model.Filters{})
	loaded := as[model.SnapshotLoaded](t, run(t, gateway, model.ReadSnapshot{Workspace: workspace, Query: query, Request: 3}))
	if loaded.ETag == "" || loaded.Request != 3 || loaded.LocalDate == "" || len(loaded.Snapshot.Issues) != 1 || loaded.Snapshot.Issues[0].UID != uid {
		t.Fatalf("collection %+v", loaded)
	}
	unchanged := as[model.SnapshotLoaded](t, run(t, gateway, model.ReadSnapshot{Workspace: workspace, Query: query, ETag: loaded.ETag, Request: 4}))
	if !unchanged.NotModified || unchanged.Request != 4 || len(unchanged.Snapshot.Issues) != 0 {
		t.Fatalf("refresh with the current ETag %+v, want a 304", unchanged)
	}
	createIssue(t, gateway, workspace, demo, "Zebra migration")
	changed := as[model.SnapshotLoaded](t, run(t, gateway, model.ReadSnapshot{Workspace: workspace, Query: query, ETag: loaded.ETag, Request: 5}))
	if changed.NotModified || len(changed.Snapshot.Issues) != 2 || changed.ETag == "" || changed.ETag == loaded.ETag {
		t.Fatalf("refresh after a write %+v, want the new snapshot and ETag", changed)
	}

	opened := as[model.IssueOpened](t, run(t, gateway, model.OpenIssue{Workspace: workspace, Ref: "demo/" + issue.ShortID, ProjectID: demo.ID}))
	if opened.Selected == nil || opened.Selected.Issue.UID != uid {
		t.Fatalf("opened %+v", opened)
	}
	looked := as[model.SearchIDLooked](t, run(t, gateway, model.LookupSearchID{Workspace: workspace, ProjectID: demo.ID, ShortID: issue.ShortID, Request: 7}))
	if looked.UID != uid || looked.Request != 7 || looked.ShortID != issue.ShortID {
		t.Fatalf("looked %+v", looked)
	}

	for _, write := range []model.WriteField{
		{Action: model.AddLabelAction, Value: "bug"},
		{Action: model.AssignOwnerAction, Value: "alice"},
		{Action: model.PriorityAction, Value: "2"},
	} {
		write.Workspace, write.UID, write.ProjectID, write.Actor = workspace, uid, demo.ID, "tester"
		if written := as[model.FieldWritten](t, run(t, gateway, write)); written.UID != uid || written.Err != nil {
			t.Fatalf("%s: %+v", write.Action, written)
		}
	}
	labels := as[model.FieldLabelsLoaded](t, run(t, gateway, model.LoadFieldLabels{Workspace: workspace, UID: uid, ProjectID: demo.ID}))
	if labels.Err != nil || labels.UID != uid || !reflect.DeepEqual(labels.Labels, []string{"bug"}) {
		t.Fatalf("labels %+v", labels)
	}
	detail = as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: uid}))
	if got := detail.Selected.Issue; got.Owner == nil || *got.Owner != "alice" || got.Priority == nil || *got.Priority != 2 {
		t.Fatalf("written fields not stored: %+v", got)
	}

	closed := as[model.CloseResultMsg](t, run(t, gateway, model.CloseIssueCmd{Workspace: workspace, UID: uid, ProjectID: demo.ID, Actor: "tester", Quick: true, Key: "k-quick"}))
	if closed.UID != uid || closed.Err != nil {
		t.Fatalf("close %+v", closed)
	}
	reopened := as[model.CloseResultMsg](t, run(t, gateway, model.ReopenIssueCmd{Workspace: workspace, UID: uid, ProjectID: demo.ID, Actor: "tester"}))
	if reopened.UID != uid || reopened.Err != nil {
		t.Fatalf("reopen %+v", reopened)
	}

	if got := l.calls.Load(); got != 1 {
		t.Fatalf("daemon located %d times for a whole session of commands, want once", got)
	}
}

func TestGatewayWritesAParentLink(t *testing.T) {
	kata := startKata(t, "0.20.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")
	parent := createIssue(t, gateway, workspace, demo, "Parent")
	child := createIssue(t, gateway, workspace, demo, "Child")
	shortID := as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: parent})).Selected.Issue.ShortID

	write := model.WriteField{Workspace: workspace, UID: child, ProjectID: demo.ID, Actor: "tester", Action: model.SetParentAction, Value: shortID}
	if written := as[model.FieldWritten](t, run(t, gateway, write)); written.UID != child || written.Err != nil {
		t.Fatalf("set parent: %+v", written)
	}
	links := as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: child})).Selected.Links
	if len(links) != 1 || links[0].Type != "parent" || links[0].FromIssueUID != child || links[0].ToIssueUID != parent {
		t.Fatalf("links %+v, want %s as the parent of %s", links, parent, child)
	}
}

func TestGatewayClosesWithEveryReason(t *testing.T) {
	kata := startKata(t, "0.20.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")
	target := createIssue(t, gateway, workspace, demo, "Target")
	message := "Implemented the requested change and verified the result."
	for _, tt := range []struct {
		name string
		cmd  model.CloseIssueCmd
	}{
		{"quick", model.CloseIssueCmd{Quick: true}},
		{"done", model.CloseIssueCmd{Reason: "done", Message: message, Evidence: []model.CloseEvidence{{Kind: model.EvidenceCommit, Value: "abcdef1"}, {Kind: model.EvidencePR, Value: "https://example.com/pr/1"}, {Kind: model.EvidenceTest, Value: "go test ./..."}}}},
		{"wontfix", model.CloseIssueCmd{Reason: "wontfix", Message: message + " It is out of scope for this project, so it will not be done."}},
		{"duplicate", model.CloseIssueCmd{Reason: "duplicate", Message: message, Evidence: []model.CloseEvidence{{Kind: model.EvidenceDuplicateOf, Value: target}}}},
		{"superseded", model.CloseIssueCmd{Reason: "superseded", Message: message, Evidence: []model.CloseEvidence{{Kind: model.EvidenceSupersededBy, Value: target}}}},
		{"audit-no-change", model.CloseIssueCmd{Reason: "audit-no-change", Message: message, Evidence: []model.CloseEvidence{{Kind: model.EvidenceAudit, Value: "Checked every caller; nothing needed to change."}}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			uid := createIssue(t, gateway, workspace, demo, tt.name)
			tt.cmd.Workspace, tt.cmd.UID, tt.cmd.ProjectID, tt.cmd.Actor, tt.cmd.Key = workspace, uid, demo.ID, "tester", "k-"+tt.name
			if closed := as[model.CloseResultMsg](t, run(t, gateway, tt.cmd)); closed.UID != uid || closed.Err != nil {
				t.Fatalf("close %+v", closed)
			}
			got := as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: uid})).Selected.Issue
			stored := closePayload(t, gateway, workspace, uid)
			wantReason := string(tt.cmd.Reason)
			if tt.cmd.Quick {
				wantReason = "done"
			}
			if stored["reason"] != wantReason {
				t.Fatalf("stored reason %v, want %q", stored["reason"], wantReason)
			}
			if got, _ := stored["message"].(string); got != tt.cmd.Message {
				t.Fatalf("stored message %q, want %q", got, tt.cmd.Message)
			}
			valueField := map[model.CloseEvidenceKind]string{model.EvidenceCommit: "sha", model.EvidencePR: "url", model.EvidenceTest: "command", model.EvidenceAudit: "rationale", model.EvidenceDuplicateOf: "issue_ref", model.EvidenceSupersededBy: "issue_ref"}
			var want []any
			for _, e := range tt.cmd.Evidence {
				want = append(want, map[string]any{"type": string(e.Kind), valueField[e.Kind]: e.Value})
			}
			if got, _ := stored["evidence"].([]any); !reflect.DeepEqual(got, want) {
				t.Fatalf("stored evidence %v, want %v", got, want)
			}
			if got.Status != "closed" {
				t.Fatalf("status %q, want closed", got.Status)
			}
		})
	}
}

func TestGatewayDraftTextConflicts(t *testing.T) {
	kata := startKata(t, "0.20.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")
	uid := createIssue(t, gateway, workspace, demo, "Draft me")
	detail := as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: uid}))
	baseText := detail.Selected.Issue.Body
	save := func(text string, comment bool) model.Msg {
		return run(t, gateway, model.SaveDraft{Workspace: workspace, UID: uid, ProjectID: demo.ID, BaseText: baseText, Text: text, Comment: comment, Actor: "tester"})
	}

	// Another writer changes the Issue's text after the Draft was started.
	if saved := as[model.DraftSaved](t, run(t, gateway, model.SaveDraft{Workspace: workspace, UID: uid, ProjectID: demo.ID, BaseText: baseText, Text: "first body", Actor: "someone"})); saved.UID != uid {
		t.Fatalf("saved %+v", saved)
	}
	conflict := as[model.DraftConflict](t, save("stale body", false))
	if conflict.UID != uid || conflict.Issue.Body != "first body" {
		t.Fatalf("stale edit %+v, want the stored Issue", conflict)
	}
	staleComment := as[model.DraftConflict](t, save("stale comment", true))
	if staleComment.Issue.Body != "first body" {
		t.Fatalf("stale comment %+v", staleComment)
	}
	baseText = conflict.Issue.Body
	if saved := as[model.DraftSaved](t, save("a comment", true)); saved.UID != uid {
		t.Fatalf("comment %+v", saved)
	}
	detail = as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: uid}))
	if len(detail.Selected.Comments) != 1 || detail.Selected.Comments[0].Body != "a comment" {
		t.Fatalf("comments %+v", detail.Selected.Comments)
	}
}

func TestGatewayReportsDaemonErrorsInTheAskingMessage(t *testing.T) {
	kata := startKata(t, "0.20.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")
	snapshotView, nope := "project", "nope"

	for _, tt := range []struct {
		name string
		cmd  model.Cmd
		want func(error) model.Msg
	}{
		{"detail", model.LoadDetail{Workspace: workspace, UID: "nope"}, func(err error) model.Msg { return model.DetailFailed{UID: "nope", Err: err} }},
		{"create", model.CreateIssueCmd{Workspace: workspace, ProjectID: 9999, Title: "x", Actor: "tester"}, func(err error) model.Msg { return model.IssueCreated{Err: err} }},
		{"open", model.OpenIssue{Workspace: workspace, Ref: "demo/zzzz", ProjectID: demo.ID}, func(err error) model.Msg { return model.IssueOpenFailed{Ref: "demo/zzzz", Err: err} }},
		{"draft", model.SaveDraft{Workspace: workspace, UID: "nope", ProjectID: demo.ID, Actor: "tester"}, func(err error) model.Msg { return model.DraftFailed{UID: "nope", Err: err} }},
		{"close", model.CloseIssueCmd{Workspace: workspace, UID: "nope", ProjectID: demo.ID, Key: "k-nope"}, func(err error) model.Msg { return model.CloseResultMsg{UID: "nope", Err: err} }},
		{"reopen", model.ReopenIssueCmd{Workspace: workspace, UID: "nope", ProjectID: demo.ID, Actor: "tester"}, func(err error) model.Msg { return model.CloseResultMsg{UID: "nope", Err: err} }},
		{"recurrence", model.CreateRecurrenceCmd{Workspace: workspace, UID: "nope", ProjectID: demo.ID, Actor: "tester"}, func(err error) model.Msg { return model.RecurrenceResultMsg{UID: "nope", Err: err} }},
		{"field", model.WriteField{Workspace: workspace, UID: "nope", ProjectID: demo.ID, Action: model.AddLabelAction, Value: "x", Actor: "tester"}, func(err error) model.Msg { return model.FieldWritten{UID: "nope", Err: err} }},
		{"project", model.BindProject{Workspace: workspace, Actor: "tester", Name: "other"}, func(err error) model.Msg { return model.ProjectSaved{Err: err} }},
		{"new project", model.CreateProject{Workspace: t.TempDir(), Actor: "tester", Name: "demo"}, func(err error) model.Msg { return model.ProjectSaved{Err: err} }},
		{"labels", model.LoadFieldLabels{Workspace: workspace, UID: "nope", ProjectID: 9999}, func(err error) model.Msg { return model.FieldLabelsLoaded{UID: "nope", Err: err} }},
		{"snapshot", model.ReadSnapshot{Workspace: workspace, Request: 4, Query: model.SnapshotQuery{View: &snapshotView, ProjectUID: &nope}}, func(err error) model.Msg { return model.SnapshotFailed{Request: 4, Err: err} }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, gateway, tt.cmd)
			var failure error
			switch got := got.(type) {
			case model.DetailFailed:
				failure = got.Err
			case model.IssueCreated:
				failure = got.Err
			case model.IssueOpenFailed:
				failure = got.Err
			case model.DraftFailed:
				failure = got.Err
			case model.CloseResultMsg:
				failure = got.Err
			case model.RecurrenceResultMsg:
				failure = got.Err
			case model.FieldWritten:
				failure = got.Err
			case model.ProjectSaved:
				failure = got.Err
			case model.FieldLabelsLoaded:
				failure = got.Err
			case model.SnapshotFailed:
				failure = got.Err
			}
			if failure == nil || !reflect.DeepEqual(got, tt.want(failure)) {
				t.Fatalf("got %#v, want a %T carrying the daemon's error", got, tt.want(nil))
			}
			if statusAndCode.MatchString(failure.Error()) {
				t.Fatalf("failure text %q carries kata's status-and-code prefix", failure)
			}
		})
	}
}

// statusAndCode matches the prefix kata's error type puts before its message, such as "404 issue_not_found:".
var statusAndCode = regexp.MustCompile(`^\d{3} \w+:`)

func TestGatewayShowsKatasMessageThenItsHintWithoutStatusAndCode(t *testing.T) {
	for _, tt := range []struct {
		name, body, want string
		status           int
	}{
		{"message and hint", `{"status":404,"error":{"code":"issue_not_found","message":"issue not found","hint":"check the ref"}}`, "issue not found\ncheck the ref", http.StatusNotFound},
		{"no message", `{"status":404,"error":{"code":"issue_not_found","message":"","hint":"check the ref"}}`, "Not Found\ncheck the ref", http.StatusNotFound},
		{"no message and an unnamed status", `{"status":599,"error":{"code":"odd_failure","message":""}}`, "odd_failure", 599},
	} {
		t.Run(tt.name, func(t *testing.T) {
			failing := serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}), "0.20.0")
			gateway := newGateway(t, newLocator(failing.target()))
			got := as[model.IssueCreated](t, run(t, gateway, model.CreateIssueCmd{Workspace: t.TempDir(), ProjectID: 1, Title: "x", Actor: "tester"}))
			if got.Err == nil || got.Err.Error() != tt.want {
				t.Fatalf("got %v, want %q", got.Err, tt.want)
			}
		})
	}
}

func TestGatewayFailureShapePerCommand(t *testing.T) {
	boom := errors.New("boom")
	l := newLocator(Target{})
	l.err.Store(&boom)
	gateway := newGateway(t, l)
	for _, tt := range []struct {
		cmd  model.Cmd
		want model.Msg
	}{
		{model.CreateIssueCmd{}, model.IssueCreated{Err: boom}},
		{model.OpenIssue{Ref: "p/x"}, model.IssueOpenFailed{Ref: "p/x", Err: boom}},
		{model.LookupSearchID{Request: 3, ShortID: "abcd"}, model.SearchIDLooked{Request: 3, ShortID: "abcd"}},
		{model.SaveDraft{UID: "u"}, model.DraftFailed{UID: "u", Err: boom}},
		{model.CloseIssueCmd{UID: "u", Key: "k-u"}, model.CloseResultMsg{UID: "u", Err: boom}},
		{model.ReopenIssueCmd{UID: "u"}, model.CloseResultMsg{UID: "u", Err: boom}},
		{model.MoveIssueCmd{UID: "u"}, model.MoveResultMsg{UID: "u", Err: boom}},
		{model.CreateRecurrenceCmd{UID: "u"}, model.RecurrenceResultMsg{UID: "u", Err: boom}},
		{model.ChangeRecurrenceCmd{UID: "u"}, model.RecurrenceResultMsg{UID: "u", Err: boom}},
		{model.StopRecurrenceCmd{UID: "u"}, model.RecurrenceResultMsg{UID: "u", Err: boom}},
		{model.WriteField{UID: "u"}, model.FieldWritten{UID: "u", Err: boom}},
		{model.LoadFieldLabels{UID: "u"}, model.FieldLabelsLoaded{UID: "u", Err: boom}},
		{model.CreateProject{}, model.ProjectSaved{Err: boom}},
		{model.BindProject{}, model.ProjectSaved{Err: boom}},
		{model.LoadProject{}, model.Failed{Err: boom}},
		{model.LoadDetail{UID: "u"}, model.DetailFailed{UID: "u", Err: boom}},
		{model.ReadSnapshot{Request: 9}, model.SnapshotFailed{Request: 9, Err: boom}},
	} {
		if got := run(t, gateway, tt.cmd); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%T: got %#v, want %#v", tt.cmd, got, tt.want)
		}
	}
}

func TestGatewayLocatorFailureIsRetriedOnTheNextCommand(t *testing.T) {
	kata := startKata(t, "0.20.0")
	l := newLocator(kata.target())
	boom := errors.New("kata daemon locate: no daemon")
	l.err.Store(&boom)
	gateway := newGateway(t, l)

	if got := as[model.SnapshotFailed](t, run(t, gateway, model.ReadSnapshot{Request: 1})); got.Err != boom {
		t.Fatalf("got %v, want the locator's error", got.Err)
	}
	l.err.Store(nil)
	workspace, _ := project(t, gateway, "demo")
	if loaded := as[model.ProjectLoaded](t, run(t, gateway, model.LoadProject{Workspace: workspace})); loaded.Project.Name != "demo" {
		t.Fatalf("loaded %+v", loaded)
	}
	if got := l.calls.Load(); got != 2 {
		t.Fatalf("located %d times, want the failed attempt and one success reused", got)
	}
}

func TestGatewayDiscoversTheDaemonTheServerVariableNames(t *testing.T) {
	kata := startKata(t, "0.20.0")
	t.Setenv("KATA_SERVER", kata.URL)
	gateway := NewGateway(locateWith(func(context.Context, string) (bool, error) { return false, nil }))
	t.Cleanup(gateway.Close)
	workspace, demo := project(t, gateway, "demo")
	uid := createIssue(t, gateway, workspace, demo, "Found by discovery")
	if detail := as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: uid})); detail.Selected.Issue.Title != "Found by discovery" {
		t.Fatalf("detail %+v", detail)
	}
}

func TestGatewayWithNoDaemonShowsKatasMessageAndTheStartHint(t *testing.T) {
	startKata(t, "0.20.0") // only for its isolated kata home; the server is stopped
	gateway := NewGateway(LocateWithKata)
	t.Cleanup(gateway.Close)
	got := as[model.SnapshotFailed](t, run(t, gateway, model.ReadSnapshot{Request: 1}))
	if got.Err == nil || !strings.Contains(got.Err.Error(), "no reachable kata daemon") || !strings.Contains(got.Err.Error(), "kata doctor") {
		t.Fatalf("got %v, want kata's message and the hint to start the daemon or run kata doctor", got.Err)
	}
}

func TestGatewayVersionGate(t *testing.T) {
	for _, tt := range []struct {
		version string
		wantErr string
	}{
		{"0.19.0", "kata daemon 0.19.0 is too old; tern-kata requires v0.20.0 or newer; upgrade kata"},
		{"dev", `cannot verify kata daemon version "dev"; tern-kata requires v0.20.0 or newer`},
		{"g1a2b3c", `cannot verify kata daemon version "g1a2b3c"; tern-kata requires v0.20.0 or newer`},
		{"v0.20.0", ""},
		{"1.2.0", ""},
	} {
		t.Run(tt.version, func(t *testing.T) {
			kata := startKata(t, tt.version)
			gateway := newGateway(t, newLocator(kata.target()))
			got := run(t, gateway, model.LoadDetail{UID: "u"})
			failed, isFailure := got.(model.DetailFailed)
			if tt.wantErr == "" {
				// The gate passed; the daemon then reports the Issue missing.
				if !isFailure || strings.Contains(failed.Err.Error(), "requires v0.20.0") {
					t.Fatalf("got %#v, want the version accepted", got)
				}
				return
			}
			if !isFailure || failed.Err.Error() != tt.wantErr {
				t.Fatalf("got %#v, want error %q", got, tt.wantErr)
			}
		})
	}
}

func TestGatewayFindsARestartedDaemonAgain(t *testing.T) {
	first := startKata(t, "0.20.0")
	l := newLocator(first.target())
	gateway := newGateway(t, l)
	workspace, demo := project(t, gateway, "demo")
	uid := createIssue(t, gateway, workspace, demo, "Survives")

	// The restarted daemon is a new process on a new address, with the same data.
	second := serve(t, first.handler, "0.20.0")
	l.target.Store(new(second.target()))
	first.Close()

	detail := as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: uid}))
	if detail.Selected.Issue.Title != "Survives" {
		t.Fatalf("detail %+v", detail)
	}
	if got := l.calls.Load(); got != 2 {
		t.Fatalf("located %d times, want the first discovery and one after the daemon went away", got)
	}
	run(t, gateway, model.LoadDetail{Workspace: workspace, UID: uid})
	if got := l.calls.Load(); got != 2 {
		t.Fatalf("located %d times, want the new daemon reused", got)
	}
}

func TestGatewayAppliesTheVersionGateToEveryDiscovery(t *testing.T) {
	first := startKata(t, "0.20.0")
	l := newLocator(first.target())
	gateway := newGateway(t, l)
	workspace, demo := project(t, gateway, "demo")
	uid := createIssue(t, gateway, workspace, demo, "x")

	old := serve(t, first.handler, "0.17.0")
	l.target.Store(new(old.target()))
	first.Close()

	got := as[model.DetailFailed](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: uid}))
	if got.Err == nil || !strings.Contains(got.Err.Error(), "too old") {
		t.Fatalf("got %#v, want the too-old error from the second discovery", got)
	}
}

func TestGatewayReportsAnUnreachableDaemonAfterLocatingAgain(t *testing.T) {
	first := startKata(t, "0.20.0")
	l := newLocator(first.target())
	gateway := newGateway(t, l)
	workspace, _ := project(t, gateway, "demo")
	first.Close()

	got := as[model.SnapshotFailed](t, run(t, gateway, model.ReadSnapshot{Workspace: workspace, Query: model.CollectionQuery(model.Collection{View: "inbox", Title: "Inbox"}, model.Filters{})}))
	if got.Err == nil {
		t.Fatal("want an error for a daemon that is gone")
	}
	if calls := l.calls.Load(); calls != 2 {
		t.Fatalf("located %d times, want the first discovery plus a fresh locate once the daemon was gone", calls)
	}
}

func TestGatewayUnknownCommandFails(t *testing.T) {
	gateway := newGateway(t, newLocator(Target{}))
	got := as[model.Failed](t, run(t, gateway, model.Scroll{}))
	if got.Err == nil || !strings.Contains(got.Err.Error(), "model.Scroll") {
		t.Fatalf("got %#v, want a failure naming model.Scroll", got)
	}
}

func TestGatewayAnswersCommandsOfEveryKindInARowWithOneResultEach(t *testing.T) {
	boom := errors.New("boom")
	l := newLocator(Target{})
	l.err.Store(&boom)
	gateway := newGateway(t, l)
	messages := make(chan model.Msg, 8)
	for _, cmd := range []model.Cmd{
		model.LoadDetail{UID: "u"},
		model.LoadWorkerPickers{},
		model.Scroll{},
		model.WakeAt{Day: "d", At: time.Unix(100, 0), Generation: 1},
		model.ReadSnapshot{Request: 1},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		gateway.Run(ctx, cmd, messages)
		cancel()
		if got := len(messages); got != 1 {
			t.Fatalf("%T left %d results, want 1", cmd, got)
		}
		<-messages
	}
}

func receive(t *testing.T, messages <-chan model.Msg) model.Msg {
	t.Helper()
	select {
	case msg := <-messages:
		return msg
	case <-time.After(10 * time.Second):
		t.Fatal("no message")
		return nil
	}
}

func stream(t *testing.T, gateway *Gateway, cmd model.Cmd) (<-chan model.Msg, <-chan struct{}) {
	t.Helper()
	messages := make(chan model.Msg, 16)
	done := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() {
		defer close(done)
		gateway.Run(ctx, cmd, messages)
	}()
	return messages, done
}

func TestGatewayStreamsEventsAndReportsOneDrop(t *testing.T) {
	kata := startKata(t, "0.20.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")

	messages, done := stream(t, gateway, model.Subscribe{Workspace: workspace, Generation: 1})
	if opened := as[model.StreamOpened](t, receive(t, messages)); opened.Generation != 1 {
		t.Fatalf("opened %+v", opened)
	}
	createIssue(t, gateway, workspace, demo, "Live")
	event := as[model.StreamEvent](t, receive(t, messages))
	for event.Type != "issue.created" {
		event = as[model.StreamEvent](t, receive(t, messages))
	}
	if event.Generation != 1 || event.ID == 0 || event.Type != "issue.created" {
		t.Fatalf("event %+v", event)
	}

	kata.CloseClientConnections()
	if dropped := as[model.StreamDropped](t, receive(t, messages)); dropped.Generation != 1 || dropped.Err == nil {
		t.Fatalf("dropped %+v", dropped)
	}
	<-done
	select {
	case extra := <-messages:
		t.Fatalf("a drop is reported once, then the stream is silent; got %#v", extra)
	default:
	}
}

func TestGatewayStreamResumesAfterTheGivenEvent(t *testing.T) {
	kata := startKata(t, "0.20.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")
	createIssue(t, gateway, workspace, demo, "Alpha release")
	createIssue(t, gateway, workspace, demo, "Zebra migration")

	all, _ := stream(t, gateway, model.Subscribe{Workspace: workspace, Generation: 1})
	receive(t, all)
	first := as[model.StreamEvent](t, receive(t, all))

	resumed, _ := stream(t, gateway, model.Subscribe{Workspace: workspace, Generation: 2, AfterID: first.ID})
	receive(t, resumed)
	next := as[model.StreamEvent](t, receive(t, resumed))
	if next.ID <= first.ID {
		t.Fatalf("resumed at event %d, want one after %d", next.ID, first.ID)
	}
}

func TestGatewayNewerSubscriptionReplacesTheRunningStream(t *testing.T) {
	kata := startKata(t, "0.20.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, _ := project(t, gateway, "demo")

	oldMessages, oldDone := stream(t, gateway, model.Subscribe{Workspace: workspace, Generation: 1})
	receive(t, oldMessages)
	// The stream starts after event 0, so it replays the Project's creation; read it before the newer subscription.
	if replayed := as[model.StreamEvent](t, receive(t, oldMessages)); replayed.Type != "project.created" {
		t.Fatalf("replayed %+v", replayed)
	}
	newMessages, _ := stream(t, gateway, model.Subscribe{Workspace: workspace, Generation: 2})
	if opened := as[model.StreamOpened](t, receive(t, newMessages)); opened.Generation != 2 {
		t.Fatalf("opened %+v", opened)
	}
	select {
	case <-oldDone:
	case <-time.After(10 * time.Second):
		t.Fatal("the replaced stream is still running")
	}
	select {
	case msg := <-oldMessages:
		t.Fatalf("a replaced stream is cancelled, not dropped; got %#v", msg)
	default:
	}

	staleMessages, staleDone := stream(t, gateway, model.Subscribe{Workspace: workspace, Generation: 2})
	<-staleDone
	select {
	case msg := <-staleMessages:
		t.Fatalf("a subscription that is not newer is ignored; got %#v", msg)
	default:
	}
}

func TestGatewayStreamHonoursTheReconnectDelay(t *testing.T) {
	kata := startKata(t, "0.20.0")
	l := newLocator(kata.target())
	gateway := newGateway(t, l)
	delay := 300 * time.Millisecond

	start := time.Now()
	messages, _ := stream(t, gateway, model.Subscribe{Generation: 1, Delay: delay})
	if got := time.Since(start); l.calls.Load() != 0 || got > delay/2 {
		t.Fatalf("subscribing located the daemon (%d calls) before the delay", l.calls.Load())
	}
	receive(t, messages)
	if got := time.Since(start); got < delay {
		t.Fatalf("stream opened after %v, want at least the %v delay", got, delay)
	}
}

func TestGatewayStreamDropReportsAnUnreachableDaemon(t *testing.T) {
	boom := errors.New("no daemon")
	l := newLocator(Target{})
	l.err.Store(&boom)
	gateway := newGateway(t, l)

	messages, done := stream(t, gateway, model.Subscribe{Generation: 1})
	if dropped := as[model.StreamDropped](t, receive(t, messages)); dropped.Generation != 1 || dropped.Err != boom {
		t.Fatalf("dropped %+v", dropped)
	}
	<-done
}

// childCountFixture is a parent in one Project with an open child in the same Project, an open child and a closed child in
// another, and a soft-deleted child: kata counts three children, two open.
type childCountFixture struct {
	gateway          *Gateway
	requests         *atomic.Int32
	workspace        string
	demo, other      model.Project
	parent, otherKid string
}

func newChildCountFixture(t *testing.T) *childCountFixture {
	t.Helper()
	kata := startKata(t, "0.20.0")
	var listed atomic.Int32
	front := serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/issues" {
			listed.Add(1)
		}
		kata.handler.ServeHTTP(w, r)
	}), "0.20.0")
	gateway := newGateway(t, newLocator(front.target()))
	workspace, demo := project(t, gateway, "demo")
	_, other := project(t, gateway, "other")
	f := &childCountFixture{gateway: gateway, requests: &listed, workspace: workspace, demo: demo, other: other}
	f.parent = createIssue(t, gateway, workspace, demo, "Parent")
	child := func(project model.Project, title string) string {
		created := as[model.IssueCreated](t, run(t, gateway, model.CreateIssueCmd{Workspace: workspace, Actor: "tester", ProjectID: project.ID, Title: title, ParentUID: f.parent}))
		if created.Err != nil {
			t.Fatal(created.Err)
		}
		return created.UID
	}
	child(demo, "Same project child")
	f.otherKid = child(other, "Other project child")
	f.setClosed(t, child(other, "Closed child"), other, true)
	f.remove(t, child(demo, "Deleted child"), demo)
	return f
}

func (f *childCountFixture) setClosed(t *testing.T, uid string, project model.Project, closed bool) {
	t.Helper()
	var msg model.Msg
	if closed {
		msg = run(t, f.gateway, model.CloseIssueCmd{Workspace: f.workspace, UID: uid, ProjectID: project.ID, Actor: "tester", Quick: true, Key: fmt.Sprintf("k-%s-%d", uid, time.Now().UnixNano())})
	} else {
		msg = run(t, f.gateway, model.ReopenIssueCmd{Workspace: f.workspace, UID: uid, ProjectID: project.ID, Actor: "tester"})
	}
	if res := as[model.CloseResultMsg](t, msg); res.Err != nil {
		t.Fatal(res.Err)
	}
}

func (f *childCountFixture) remove(t *testing.T, uid string, project model.Project) {
	t.Helper()
	detail := as[model.DetailLoaded](t, run(t, f.gateway, model.LoadDetail{Workspace: f.workspace, UID: uid})).Selected
	c, err := f.gateway.client(context.Background(), f.workspace)
	if err != nil {
		t.Fatal(err)
	}
	confirm := "DELETE " + detail.Issue.QualifiedID
	response, err := c.DeleteIssueWithResponse(context.Background(), &g.DeleteIssueRequestOptions{
		PathParams: &g.DeleteIssuePath{ProjectID: project.ID, Ref: uid},
		Header:     &g.DeleteIssueHeaders{XKataConfirm: &confirm},
		Body:       &g.DeleteIssueBody{Actor: "tester"},
	})
	if err != nil || response.JSON200 == nil {
		t.Fatalf("delete %s: %v", uid, err)
	}
}

// kataCounts is kata's own child_counts for uid, the figure the snapshot's link counts must match.
func (f *childCountFixture) kataCounts(t *testing.T, uid string) model.ChildCounts {
	t.Helper()
	c, err := f.gateway.client(context.Background(), f.workspace)
	if err != nil {
		t.Fatal(err)
	}
	response, err := c.ListAllIssuesWithResponse(context.Background(), &g.ListAllIssuesRequestOptions{Query: &g.ListAllIssuesQuery{}})
	if err != nil || response.JSON200 == nil {
		t.Fatalf("list Issues: %v", err)
	}
	for _, issue := range response.JSON200.Issues {
		if issue.UID == uid && issue.ChildCounts != nil {
			return model.ChildCounts{Open: int(issue.ChildCounts.Open), Total: int(issue.ChildCounts.Total)}
		}
	}
	return model.ChildCounts{}
}

func (f *childCountFixture) read(t *testing.T, filters model.Filters, project model.Project, etag string) model.SnapshotLoaded {
	t.Helper()
	query := model.CollectionQuery(model.Collection{View: "project", ProjectUID: project.UID, Title: project.Name}, filters)
	return as[model.SnapshotLoaded](t, run(t, f.gateway, model.ReadSnapshot{Workspace: f.workspace, Query: query, ETag: etag}))
}

func TestGatewaySnapshotLinksCountChildrenAsKataDoes(t *testing.T) {
	f := newChildCountFixture(t)
	want := model.ChildCounts{Open: 2, Total: 3}
	if got := f.kataCounts(t, f.parent); got != want {
		t.Fatalf("kata counts %+v, want %+v (the deleted child left out)", got, want)
	}
	f.requests.Store(0)
	for _, tt := range []struct {
		name    string
		filters model.Filters
		project model.Project
	}{
		{"filters hide the children", model.Filters{Search: "Parent"}, f.demo},
		{"an unfiltered Project", model.Filters{}, f.demo},
	} {
		t.Run(tt.name, func(t *testing.T) {
			loaded := f.read(t, tt.filters, tt.project, "")
			if got := model.CountChildren(loaded.Snapshot.Links, f.parent); got != want {
				t.Fatalf("counted %+v, want kata's %+v", got, want)
			}
		})
	}
	if f.requests.Load() != 0 {
		t.Fatalf("snapshot reads requested the Issue list %d times", f.requests.Load())
	}
}

func TestGatewaySnapshotLinksAreTheCollectionsOwn(t *testing.T) {
	f := newChildCountFixture(t)
	loaded := f.read(t, model.Filters{}, f.other, "")
	// The Collection holds only the other Project's children: their two Links count for the parent that is not in it, and the
	// same-Project children have no end in this Collection.
	if got := model.CountChildren(loaded.Snapshot.Links, f.parent); got != (model.ChildCounts{Open: 1, Total: 2}) {
		t.Fatalf("counted %+v, want the two children in the Collection", got)
	}
	if len(loaded.Snapshot.Catalog) != 2 || loaded.Snapshot.Cursor == 0 || loaded.ETag == "" {
		t.Fatalf("snapshot %+v, want the catalog of both Projects, an event cursor and an ETag", loaded)
	}
}

func TestGatewayChildOutsideTheCollectionMovesTheNextReadsCounts(t *testing.T) {
	f := newChildCountFixture(t)
	filters := model.Filters{Search: "Parent"}
	first := f.read(t, filters, f.demo, "")
	if got := model.CountChildren(first.Snapshot.Links, f.parent); got != (model.ChildCounts{Open: 2, Total: 3}) {
		t.Fatalf("counted %+v", got)
	}
	etag := first.ETag
	for _, closed := range []bool{true, false} {
		f.setClosed(t, f.otherKid, f.other, closed)
		next := f.read(t, filters, f.demo, etag)
		if next.NotModified {
			t.Fatalf("closed=%v: the previous ETag answered 304 over a changed count", closed)
		}
		want := f.kataCounts(t, f.parent)
		if got := model.CountChildren(next.Snapshot.Links, f.parent); got != want || (want.Open == 2) == closed {
			t.Fatalf("closed=%v: counted %+v, kata %+v", closed, got, want)
		}
		etag = next.ETag
	}
	if again := f.read(t, filters, f.demo, etag); !again.NotModified {
		t.Fatal("an unchanged Collection must still answer 304")
	}
}

func TestGatewayDetailLinksCarryTheChildStatuses(t *testing.T) {
	f := newChildCountFixture(t)
	detail := as[model.DetailLoaded](t, run(t, f.gateway, model.LoadDetail{Workspace: f.workspace, UID: f.parent})).Selected
	want := f.kataCounts(t, f.parent)
	if got := model.CountChildren(detail.Links, f.parent); got != want || want.Total != 3 {
		t.Fatalf("detail links count %+v, kata %+v", got, want)
	}
}

func TestGatewayReadsASnapshotWhenTheTimeZoneIsBad(t *testing.T) {
	t.Setenv("TZ", "Nowhere/Land")
	kata := startKata(t, "0.20.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")
	query := model.CollectionQuery(model.Collection{View: "project", ProjectUID: demo.UID, Title: demo.Name}, model.Filters{})
	loaded := as[model.SnapshotLoaded](t, run(t, gateway, model.ReadSnapshot{Workspace: workspace, Query: query, Request: 1}))
	if loaded.TimeZone != "UTC" || !strings.Contains(loaded.TimeZoneNote, "Nowhere/Land") {
		t.Fatalf("zone %q note %q", loaded.TimeZone, loaded.TimeZoneNote)
	}
}

// changeOwner is another actor assigning the Issue: on kata v0.20 that moves the revision without touching the text.
func changeOwner(t *testing.T, gateway *Gateway, workspace string, project model.Project, uid, owner string) {
	t.Helper()
	written := as[model.FieldWritten](t, run(t, gateway, model.WriteField{Workspace: workspace, UID: uid, ProjectID: project.ID, Actor: "someone", Action: model.AssignOwnerAction, Value: owner}))
	if written.Err != nil {
		t.Fatal(written.Err)
	}
}

// bounceStatus is another actor closing and reopening the Issue: the revision moves twice and the status ends where it began.
func bounceStatus(t *testing.T, gateway *Gateway, workspace string, project model.Project, uid string) {
	t.Helper()
	if closed := as[model.CloseResultMsg](t, run(t, gateway, model.CloseIssueCmd{Workspace: workspace, UID: uid, ProjectID: project.ID, Actor: "someone", Quick: true, Key: fmt.Sprintf("k-bounce-%s-%d", uid, time.Now().UnixNano())})); closed.Err != nil {
		t.Fatal(closed.Err)
	}
	if reopened := as[model.CloseResultMsg](t, run(t, gateway, model.ReopenIssueCmd{Workspace: workspace, UID: uid, ProjectID: project.ID, Actor: "someone"})); reopened.Err != nil {
		t.Fatal(reopened.Err)
	}
}

func TestGatewayGuardedWritesSurviveOwnerAndStatusChanges(t *testing.T) {
	kata := startKata(t, "0.20.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")
	seen := func(uid string) model.Issue {
		detail := as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: uid}))
		return *detail.Selected.Issue
	}

	t.Run("Draft", func(t *testing.T) {
		uid := createIssue(t, gateway, workspace, demo, "Draft me")
		issue := seen(uid)
		changeOwner(t, gateway, workspace, demo, uid, "alice")
		bounceStatus(t, gateway, workspace, demo, uid)
		as[model.DraftSaved](t, run(t, gateway, model.SaveDraft{Workspace: workspace, UID: uid, ProjectID: demo.ID, BaseText: issue.Body, Text: "new body", Actor: "tester"}))
		if body := seen(uid).Body; body != "new body" {
			t.Fatalf("body %q", body)
		}
	})
	t.Run("comment", func(t *testing.T) {
		uid := createIssue(t, gateway, workspace, demo, "Comment on me")
		issue := seen(uid)
		changeOwner(t, gateway, workspace, demo, uid, "alice")
		bounceStatus(t, gateway, workspace, demo, uid)
		as[model.DraftSaved](t, run(t, gateway, model.SaveDraft{Workspace: workspace, UID: uid, ProjectID: demo.ID, BaseText: issue.Body, Text: "hello", Comment: true, Actor: "tester"}))
		detail := as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: uid}))
		if len(detail.Selected.Comments) != 1 {
			t.Fatalf("comments %+v", detail.Selected.Comments)
		}
	})
	t.Run("text changed", func(t *testing.T) {
		uid := createIssue(t, gateway, workspace, demo, "Real conflict")
		issue := seen(uid)
		as[model.DraftSaved](t, run(t, gateway, model.SaveDraft{Workspace: workspace, UID: uid, ProjectID: demo.ID, BaseText: issue.Body, Text: "their body", Actor: "someone"}))
		for _, comment := range []bool{false, true} {
			conflict := as[model.DraftConflict](t, run(t, gateway, model.SaveDraft{Workspace: workspace, UID: uid, ProjectID: demo.ID, BaseText: issue.Body, Text: "mine", Comment: comment, Actor: "tester"}))
			if conflict.Issue.Body != "their body" {
				t.Fatalf("conflict %+v", conflict)
			}
		}
	})
	t.Run("close", func(t *testing.T) {
		uid := createIssue(t, gateway, workspace, demo, "Close me")
		issue := seen(uid)
		changeOwner(t, gateway, workspace, demo, uid, "alice")
		bounceStatus(t, gateway, workspace, demo, uid)
		closed := as[model.CloseResultMsg](t, run(t, gateway, model.CloseIssueCmd{Workspace: workspace, UID: uid, ProjectID: demo.ID, Actor: "tester", Quick: true, Key: "k-moved"}))
		if closed.Err != nil {
			t.Fatalf("close after an owner and status change from revision %d: %v", issue.Revision, closed.Err)
		}
	})
	t.Run("closed meanwhile", func(t *testing.T) {
		uid := createIssue(t, gateway, workspace, demo, "Closed twice")
		as[model.CloseResultMsg](t, run(t, gateway, model.CloseIssueCmd{Workspace: workspace, UID: uid, ProjectID: demo.ID, Actor: "someone", Quick: true, Key: "k-theirs"}))
		before := as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: uid}))
		closed := as[model.CloseResultMsg](t, run(t, gateway, model.CloseIssueCmd{Workspace: workspace, UID: uid, ProjectID: demo.ID, Actor: "tester", Quick: true, Key: "k-mine"}))
		after := as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: uid}))
		if closed.Err == nil || !strings.Contains(closed.Err.Error(), "closed meanwhile; nothing was written") || len(after.Selected.History) != len(before.Selected.History) {
			t.Fatalf("second close %+v wrote events %d -> %d", closed, len(before.Selected.History), len(after.Selected.History))
		}
	})
}

// closePayload is what the daemon recorded when the Issue uid was closed.
func closePayload(t *testing.T, gateway *Gateway, workspace, uid string) map[string]any {
	t.Helper()
	ctx := context.Background()
	client, err := gateway.client(ctx, workspace)
	if err != nil {
		t.Fatal(err)
	}
	limit := int64(1000)
	events, err := client.PollEventsWithResponse(ctx, &g.PollEventsRequestOptions{Query: &g.PollEventsQuery{Limit: &limit}})
	if err != nil || events.JSON200 == nil {
		t.Fatalf("poll events: %v", err)
	}
	for _, e := range events.JSON200.Events {
		if e.IssueUID != nil && *e.IssueUID == uid && e.Type == "issue.closed" {
			var payload map[string]any
			if err := json.Unmarshal(e.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			return payload
		}
	}
	t.Fatalf("no close event for %s", uid)
	return nil
}

func TestGatewayTranslatesTheIssueDetailFromRealPayloads(t *testing.T) {
	kata := startKata(t, "0.20.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")
	uid := createIssue(t, gateway, workspace, demo, "Discuss")
	parent := createIssue(t, gateway, workspace, demo, "Parent")
	detailOf := func() *model.IssueDetailData {
		return as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: uid})).Selected
	}

	as[model.DraftSaved](t, run(t, gateway, model.SaveDraft{Workspace: workspace, UID: uid, ProjectID: demo.ID, BaseText: detailOf().Issue.Body, Text: "the question", Comment: true, Actor: "asker"}))
	question := detailOf().Comments[0]
	as[model.DraftSaved](t, run(t, gateway, model.SaveDraft{Workspace: workspace, UID: uid, ProjectID: demo.ID, BaseText: detailOf().Issue.Body, Text: "confirmed: " + strings.Repeat("it reproduces. ", 4),
		Comment: true, Actor: "answerer", ReplyTo: question.UID, ReplyKind: model.ReplyConfirm}))
	shortID := as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: parent})).Selected.Issue.ShortID
	as[model.FieldWritten](t, run(t, gateway, model.WriteField{Workspace: workspace, UID: uid, ProjectID: demo.ID, Actor: "tester", Action: model.SetParentAction, Value: shortID}))

	check := func(name string, d *model.IssueDetailData) {
		t.Helper()
		if d.Issue == nil || d.Issue.UID != uid || len(d.Comments) != 2 {
			t.Fatalf("%s: detail %+v", name, d)
		}
		asked, answer := d.Comments[0], d.Comments[1]
		if asked.Author != "asker" || asked.Body != "the question" || asked.ReplyKind != nil || asked.Reply != nil ||
			len(asked.Backlinks) != 1 || asked.Backlinks[0].Kind != "confirm" || asked.Backlinks[0].UID == nil || *asked.Backlinks[0].UID != answer.UID {
			t.Errorf("%s: the answered comment %+v", name, asked)
		}
		if answer.Author != "answerer" || answer.ReplyKind == nil || *answer.ReplyKind != "confirm" ||
			answer.Reply == nil || answer.Reply.UID == nil || *answer.Reply.UID != asked.UID || answer.Reply.IssueUID == nil || *answer.Reply.IssueUID != uid {
			t.Errorf("%s: the reply %+v", name, answer)
		}
		if len(d.Links) != 1 || d.Links[0].Type != "parent" || d.Links[0].FromIssueUID != uid || d.Links[0].ToIssueUID != parent || d.Links[0].ToQualifiedID == "" {
			t.Errorf("%s: links %+v", name, d.Links)
		}
		types := make([]string, len(d.History))
		for i, e := range d.History {
			types[i] = e.Type
			if e.UID == "" || e.Actor == "" || e.Payload == "" || e.CreatedAt.IsZero() || e.ProjectName != "demo" {
				t.Errorf("%s: event %+v", name, e)
			}
		}
		// Newest first, as kata lists them.
		if len(types) < 4 || types[0] != "issue.linked" && types[0] != "issue.links_changed" || types[len(types)-1] != "issue.created" {
			t.Errorf("%s: history %v", name, types)
		}
	}

	check("detail load", detailOf())
	collection := model.Collection{View: "project", ProjectUID: demo.UID, Title: demo.Name}
	query := model.CollectionQuery(collection, model.Filters{})
	query.SelectedIssueUID, query.IncludeHistory = &uid, new(true)
	check("snapshot", as[model.SnapshotLoaded](t, run(t, gateway, model.ReadSnapshot{Workspace: workspace, Query: query, Request: 1})).Selected)
	check("opened reference", as[model.IssueOpened](t, run(t, gateway, model.OpenIssue{Workspace: workspace, Ref: "demo/" + detailOf().Issue.ShortID, ProjectID: demo.ID})).Selected)
}

func TestGatewayPostsRepliesOfEachKindWithTheirTarget(t *testing.T) {
	kata := startKata(t, "0.20.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")
	uid := createIssue(t, gateway, workspace, demo, "Discuss")
	detailOf := func() *model.IssueDetailData {
		return as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: uid})).Selected
	}
	save := func(text, replyTo string, kind model.ReplyKind, force bool) model.Msg {
		return run(t, gateway, model.SaveDraft{Workspace: workspace, UID: uid, ProjectID: demo.ID, BaseText: detailOf().Issue.Body, Text: text, Comment: true, Actor: "answerer", ReplyTo: replyTo, ReplyKind: kind, Force: force})
	}
	as[model.DraftSaved](t, save("the question", "", "", false))
	question := detailOf().Comments[0].UID
	evidence := strings.Repeat("it reproduces here. ", 3)

	for _, kind := range model.ReplyKinds {
		as[model.DraftSaved](t, save(evidence, question, kind, false))
	}
	comments := detailOf().Comments
	if len(comments) != 5 {
		t.Fatalf("%d comments, want the question and four replies", len(comments))
	}
	for i, kind := range model.ReplyKinds {
		reply := comments[i+1]
		if reply.ReplyKind == nil || *reply.ReplyKind != string(kind) || reply.Reply == nil || reply.Reply.UID == nil || *reply.Reply.UID != question {
			t.Errorf("reply %d: %+v, want a %s answering %s", i, reply, kind, question)
		}
	}
}

func TestGatewayReportsADuplicateReplyAndSendsItAnywayWithForce(t *testing.T) {
	kata := startKata(t, "0.20.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")
	uid := createIssue(t, gateway, workspace, demo, "Discuss")
	save := func(text, replyTo string, kind model.ReplyKind, force bool) model.Msg {
		detail := as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: uid})).Selected
		return run(t, gateway, model.SaveDraft{Workspace: workspace, UID: uid, ProjectID: demo.ID, BaseText: detail.Issue.Body, Text: text, Comment: true, Actor: "answerer", ReplyTo: replyTo, ReplyKind: kind, Force: force})
	}
	as[model.DraftSaved](t, save("the question", "", "", false))
	question := as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: uid})).Selected.Comments[0].UID
	as[model.DraftSaved](t, save("first", question, model.ReplyPlain, false))

	dup := as[model.DraftFailed](t, save("second", question, model.ReplyPlain, false))
	if _, ok := errors.AsType[model.DuplicateReply](dup.Err); !ok || dup.UID != uid || !strings.Contains(dup.Err.Error(), "already exists") || strings.Contains(dup.Err.Error(), "--force") {
		t.Fatalf("duplicate result %+v", dup)
	}
	as[model.DraftSaved](t, save("second", question, model.ReplyPlain, true))
	if got := len(as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: uid})).Selected.Comments); got != 3 {
		t.Fatalf("%d comments after sending anyway, want 3", got)
	}
}

func TestGatewayTranslatesTheRecurrenceFromTheSnapshotIncludingAStoppedOne(t *testing.T) {
	kata := startKata(t, "0.20.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")
	live, liveRec := recurringIssue(t, gateway, workspace, demo, "Live", "FREQ=WEEKLY;INTERVAL=1;BYDAY=MO,TH")
	stopped, stoppedRec := recurringIssue(t, gateway, workspace, demo, "Stopped", "FREQ=DAILY;INTERVAL=1")
	if got := as[model.RecurrenceResultMsg](t, run(t, gateway, model.StopRecurrenceCmd{Workspace: workspace, UID: stopped, ProjectID: demo.ID, Actor: "tester", RecurrenceUID: stoppedRec.UID})); got.Err != nil {
		t.Fatal(got.Err)
	}
	plain := createIssue(t, gateway, workspace, demo, "Plain")

	selected := func(uid string) *model.IssueDetailData {
		query := model.CollectionQuery(model.Collection{View: "project", ProjectUID: demo.UID, Title: demo.Name}, model.Filters{})
		query.SelectedIssueUID = &uid
		return as[model.SnapshotLoaded](t, run(t, gateway, model.ReadSnapshot{Workspace: workspace, Query: query, Request: 1})).Selected
	}
	want := model.Recurrence{UID: liveRec.UID, RecurrenceTerms: model.RecurrenceTerms{Rule: "FREQ=WEEKLY;INTERVAL=1;BYDAY=MO,TH", Start: "2026-10-19", TimeZone: "Europe/Zurich"}, Next: "2026-10-22", Revision: liveRec.Revision}
	if got := selected(live).Recurrence; got == nil || *got != want {
		t.Errorf("live recurrence %+v, want %+v", got, want)
	}
	if got := as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: live})).Selected.Recurrence; got == nil || *got != want {
		t.Errorf("detail load recurrence %+v, want %+v", got, want)
	}
	if got := selected(stopped).Recurrence; got == nil || !got.Stopped {
		t.Errorf("stopped recurrence %+v, want stopped", got)
	}
	if got := selected(plain).Recurrence; got != nil {
		t.Errorf("plain Issue has recurrence %+v", got)
	}
}

func TestGatewayMakesAnIssueTheFirstOccurrenceAndClosingItCreatesTheNext(t *testing.T) {
	kata := startKata(t, "0.20.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")
	priority := int64(2)
	created := as[model.IssueCreated](t, run(t, gateway, model.CreateIssueCmd{Workspace: workspace, Actor: "tester", ProjectID: demo.ID, Title: "Water plants", Body: "Every pot", Owner: "alice", Labels: []string{"home"}, Priority: &priority}))
	if created.Err != nil {
		t.Fatal(created.Err)
	}
	uid := created.UID

	saved := as[model.RecurrenceResultMsg](t, run(t, gateway, model.CreateRecurrenceCmd{Workspace: workspace, UID: uid, ProjectID: demo.ID, Actor: "tester", RecurrenceTerms: weeklyTerms("FREQ=WEEKLY;INTERVAL=1;BYDAY=MO")}))
	if saved.UID != uid || saved.Err != nil {
		t.Fatalf("create recurrence %+v", saved)
	}
	first := as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: uid})).Selected
	if first.Issue.RecurrenceID == nil || first.Recurrence == nil || first.Recurrence.Stopped || first.Recurrence.Revision != 2 || first.Recurrence.Next != "2026-10-26" {
		t.Fatalf("first occurrence %+v, recurrence %+v", first.Issue, first.Recurrence)
	}
	if owner := first.Issue.Owner; owner == nil || *owner != "alice" {
		t.Fatalf("the first occurrence kept owner %v, want alice", owner)
	}

	if closed := as[model.CloseResultMsg](t, run(t, gateway, model.CloseIssueCmd{Workspace: workspace, UID: uid, ProjectID: demo.ID, Actor: "tester", Quick: true, Key: "k-snapshot"})); closed.Err != nil {
		t.Fatal(closed.Err)
	}
	query := model.CollectionQuery(model.Collection{View: "project", ProjectUID: demo.UID, Title: demo.Name}, model.Filters{})
	var next *model.Issue
	for _, issue := range as[model.SnapshotLoaded](t, run(t, gateway, model.ReadSnapshot{Workspace: workspace, Query: query, Request: 1})).Snapshot.Issues {
		if issue.UID != uid && issue.Status != "closed" {
			next = &issue
		}
	}
	if next == nil || next.Title != "Water plants" || next.Body != "Every pot" || next.Priority == nil || *next.Priority != 2 || !slices.Equal(next.Labels, []string{"home"}) || next.Owner != nil {
		t.Fatalf("next occurrence %+v, want the template copied without the owner", next)
	}
	if next.RecurrenceID == nil || *next.RecurrenceID != *first.Issue.RecurrenceID {
		t.Fatalf("next occurrence belongs to recurrence %v, want %v", next.RecurrenceID, first.Issue.RecurrenceID)
	}
}

// weeklyTerms are the terms of a schedule that starts on Monday 2026-10-19 in Zurich.
func weeklyTerms(rule string) model.RecurrenceTerms {
	return model.RecurrenceTerms{Rule: rule, Start: "2026-10-19", TimeZone: "Europe/Zurich"}
}

// recurringIssue makes a new Issue the first occurrence of a recurrence with the rule and returns its UID and the recurrence as the detail shows it.
func recurringIssue(t *testing.T, gateway *Gateway, workspace string, demo model.Project, title, rule string) (string, model.Recurrence) {
	t.Helper()
	uid := createIssue(t, gateway, workspace, demo, title)
	saved := as[model.RecurrenceResultMsg](t, run(t, gateway, model.CreateRecurrenceCmd{Workspace: workspace, UID: uid, ProjectID: demo.ID, Actor: "tester", RecurrenceTerms: weeklyTerms(rule)}))
	if saved.Err != nil {
		t.Fatal(saved.Err)
	}
	return uid, recurrenceOf(t, gateway, workspace, uid)
}

func recurrenceOf(t *testing.T, gateway *Gateway, workspace, uid string) model.Recurrence {
	t.Helper()
	rec := as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: uid})).Selected.Recurrence
	if rec == nil {
		t.Fatal("the Issue has no recurrence")
	}
	return *rec
}

func changeTo(demo model.Project, uid string, opened model.Recurrence, workspace, rule string) model.ChangeRecurrenceCmd {
	return model.ChangeRecurrenceCmd{Workspace: workspace, UID: uid, ProjectID: demo.ID, Actor: "tester", RecurrenceUID: opened.UID,
		RecurrenceTerms: model.RecurrenceTerms{Rule: rule, Start: "2026-10-26", TimeZone: "UTC"}, Opened: opened.RecurrenceTerms}
}

func TestGatewayChangesTheScheduleAfterOnlyBookkeepingMoved(t *testing.T) {
	kata := startKata(t, "0.20.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")
	uid, opened := recurringIssue(t, gateway, workspace, demo, "Water plants", "FREQ=WEEKLY;INTERVAL=1;BYDAY=MO")

	// Closing the occurrence as done creates the next one and moves the recurrence's revision, not its schedule.
	if closed := as[model.CloseResultMsg](t, run(t, gateway, model.CloseIssueCmd{Workspace: workspace, UID: uid, ProjectID: demo.ID, Actor: "tester", Quick: true, Key: "k-occurrence"})); closed.Err != nil {
		t.Fatal(closed.Err)
	}
	if now := recurrenceOf(t, gateway, workspace, uid); now.Revision == opened.Revision || now.Rule != opened.Rule {
		t.Fatalf("closing moved the recurrence from %+v to %+v, want a new revision and the same rule", opened, now)
	}

	got := as[model.RecurrenceResultMsg](t, run(t, gateway, changeTo(demo, uid, opened, workspace, "FREQ=DAILY;INTERVAL=2")))
	if got.Err != nil || got.Changed != nil {
		t.Fatalf("change at the stale revision: %+v", got)
	}
	now := recurrenceOf(t, gateway, workspace, uid)
	if now.Rule != "FREQ=DAILY;INTERVAL=2" || now.Start != "2026-10-26" || now.TimeZone != "UTC" {
		t.Fatalf("schedule after the change: %+v", now)
	}
}

func TestGatewayChangeAfterTheRuleMovedWritesNothingAndReturnsTheCurrentSchedule(t *testing.T) {
	kata := startKata(t, "0.20.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")
	uid, opened := recurringIssue(t, gateway, workspace, demo, "Water plants", "FREQ=WEEKLY;INTERVAL=1;BYDAY=MO")

	elsewhere := as[model.RecurrenceResultMsg](t, run(t, gateway, changeTo(demo, uid, opened, workspace, "FREQ=MONTHLY;INTERVAL=1")))
	if elsewhere.Err != nil || elsewhere.Changed != nil {
		t.Fatal(elsewhere)
	}
	moved := recurrenceOf(t, gateway, workspace, uid)

	got := as[model.RecurrenceResultMsg](t, run(t, gateway, changeTo(demo, uid, opened, workspace, "FREQ=DAILY;INTERVAL=1")))
	if got.Err != nil || got.Changed == nil || *got.Changed != moved {
		t.Fatalf("conflicting change: %+v, want the current recurrence %+v", got, moved)
	}
	if now := recurrenceOf(t, gateway, workspace, uid); now != moved {
		t.Fatalf("the conflicting change wrote: %+v, was %+v", now, moved)
	}
}

func TestGatewayStopsTheRecurrenceAndReportsOneAlreadyStopped(t *testing.T) {
	kata := startKata(t, "0.20.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")
	uid, opened := recurringIssue(t, gateway, workspace, demo, "Water plants", "FREQ=WEEKLY;INTERVAL=1;BYDAY=MO")
	stop := model.StopRecurrenceCmd{Workspace: workspace, UID: uid, ProjectID: demo.ID, Actor: "tester", RecurrenceUID: opened.UID}

	if got := as[model.RecurrenceResultMsg](t, run(t, gateway, stop)); got.UID != uid || got.Err != nil {
		t.Fatalf("stop: %+v", got)
	}
	if now := recurrenceOf(t, gateway, workspace, uid); !now.Stopped {
		t.Fatalf("after stopping the recurrence reads %+v, want stopped", now)
	}

	again := as[model.RecurrenceResultMsg](t, run(t, gateway, stop))
	if again.Err == nil || !strings.Contains(again.Err.Error(), "stopped") {
		t.Fatalf("stopping a stopped recurrence: %+v, want an error saying it was stopped", again)
	}
	change := as[model.RecurrenceResultMsg](t, run(t, gateway, changeTo(demo, uid, opened, workspace, "FREQ=DAILY;INTERVAL=1")))
	if change.Err == nil || change.Changed != nil {
		t.Fatalf("changing a stopped recurrence: %+v, want an error", change)
	}
}

func TestGatewayKindCheckFailureLeavesTheKindUnknown(t *testing.T) {
	kata := startKata(t, "0.20.0")
	target := kata.target()
	target.Remote = func(context.Context) (bool, error) { return false, errors.New("kata: not found") }
	gateway := newGateway(t, newLocator(target))
	workspace, _ := project(t, gateway, "demo")
	loaded := as[model.ProjectLoaded](t, run(t, gateway, model.LoadProject{Workspace: workspace}))
	if loaded.Daemon.Kind != model.DaemonKindUnknown || loaded.Daemon.Version != "v0.20.0" {
		t.Fatalf("daemon %+v, want the version with no kind", loaded.Daemon)
	}
}

func TestGatewayWithoutTheKataBinaryStillLoadsWithoutAKind(t *testing.T) {
	kata := startKata(t, "0.20.0")
	t.Setenv("KATA_SERVER", kata.URL)
	t.Setenv("PATH", t.TempDir())
	gateway := NewGateway(LocateWithKata)
	t.Cleanup(gateway.Close)
	workspace, _ := project(t, gateway, "demo")
	loaded := as[model.ProjectLoaded](t, run(t, gateway, model.LoadProject{Workspace: workspace}))
	if loaded.Daemon.Kind != model.DaemonKindUnknown || loaded.Daemon.Version != "v0.20.0" {
		t.Fatalf("daemon %+v, want the version with no kind", loaded.Daemon)
	}
}

func TestGatewayHungKindCheckStillConnects(t *testing.T) {
	kata := startKata(t, "0.20.0")
	target := kata.target()
	target.Remote = func(ctx context.Context) (bool, error) { <-ctx.Done(); return false, ctx.Err() }
	l := newLocator(target)
	gateway := newGateway(t, l)
	gateway.deadline = 50 * time.Millisecond
	workspace := t.TempDir()
	loaded := as[model.ProjectLoaded](t, run(t, gateway, model.LoadProject{Workspace: workspace}))
	if loaded.Daemon.Kind != model.DaemonKindUnknown || loaded.Daemon.Version != "v0.20.0" {
		t.Fatalf("daemon %+v, want the version with no kind", loaded.Daemon)
	}
}

func TestGatewayHungLocatorOrPingReadsAsAnUnreachableDaemonAndIsLocatedAgain(t *testing.T) {
	hungPing := &kataServer{Server: httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))}
	t.Cleanup(hungPing.Close)
	for name, tt := range map[string]struct {
		locate func(context.Context, string) (Target, error)
	}{
		"locator": {func(ctx context.Context, _ string) (Target, error) { <-ctx.Done(); return Target{}, ctx.Err() }},
		"ping":    {func(context.Context, string) (Target, error) { return hungPing.target(), nil }},
	} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			gateway := NewGateway(func(ctx context.Context, w string) (Target, error) {
				calls.Add(1)
				return tt.locate(ctx, w)
			})
			t.Cleanup(gateway.Close)
			gateway.deadline = 50 * time.Millisecond
			for range 2 {
				got := as[model.SnapshotFailed](t, run(t, gateway, model.ReadSnapshot{Request: 1}))
				if got.Err == nil || !strings.Contains(got.Err.Error(), "no reachable kata daemon") || !strings.Contains(got.Err.Error(), "kata doctor") {
					t.Fatalf("got %v, want kata's message and the start hint", got.Err)
				}
			}
			if got := calls.Load(); got != 2 {
				t.Fatalf("located %d times, want once per command", got)
			}
		})
	}
}

func TestGatewayWakeFiresOnTheWallClockEvenIfTheMonotonicClockLagged(t *testing.T) {
	gateway := newGateway(t, newLocator(Target{}))
	gateway.wakePoll = 5 * time.Millisecond
	deadline := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	var wall atomic.Int64
	wall.Store(deadline.Add(-time.Hour).UnixNano())
	gateway.now = func() time.Time { return time.Unix(0, wall.Load()) }

	messages, _ := stream(t, gateway, model.WakeAt{Day: "2026-10-08", At: deadline, Generation: 1})
	select {
	case msg := <-messages:
		t.Fatalf("woke an hour early: %+v", msg)
	case <-time.After(50 * time.Millisecond):
	}
	wall.Store(deadline.UnixNano()) // the machine slept: only the wall clock moved
	if got := receive(t, messages); got != (model.DayEnded{Day: "2026-10-08"}) {
		t.Fatalf("wake = %+v", got)
	}
}

func TestGatewayWakeEndsWhenThePaneCloses(t *testing.T) {
	gateway := newGateway(t, newLocator(Target{}))
	gateway.now = func() time.Time { return time.Unix(0, 0) }
	messages := make(chan model.Msg, 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		gateway.Run(ctx, model.WakeAt{Day: "d", At: time.Unix(1<<40, 0), Generation: 1}, messages)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("wake outlived the Pane")
	}
	if len(messages) != 0 {
		t.Fatal("a closed Pane got a wake")
	}
}

func TestGatewayNewestWakeSurvivesAnOlderOneArrivingLate(t *testing.T) {
	gateway := newGateway(t, newLocator(Target{}))
	gateway.wakePoll = 5 * time.Millisecond
	var wall atomic.Int64
	gateway.now = func() time.Time { return time.Unix(0, wall.Load()) }

	newest, _ := stream(t, gateway, model.WakeAt{Day: "new", At: time.Unix(100, 0), Generation: 2})
	older, olderDone := stream(t, gateway, model.WakeAt{Day: "old", At: time.Unix(100, 0), Generation: 1})
	select {
	case <-olderDone:
	case <-time.After(5 * time.Second):
		t.Fatal("an older wake kept running")
	}
	wall.Store(time.Unix(100, 0).UnixNano())
	if got := receive(t, newest); got != (model.DayEnded{Day: "new"}) {
		t.Fatalf("the newest wake answered %+v", got)
	}
	if len(older) != 0 {
		t.Fatal("the older wake answered")
	}
}

func TestGatewayCloseWithoutAKeyIsRefusedAndWritesNothing(t *testing.T) {
	kata := startKata(t, "0.20.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")
	uid := createIssue(t, gateway, workspace, demo, "Keyless")
	got := as[model.CloseResultMsg](t, run(t, gateway, model.CloseIssueCmd{Workspace: workspace, UID: uid, ProjectID: demo.ID, Actor: "tester", Quick: true}))
	if got.Err == nil || !strings.Contains(got.Err.Error(), "idempotency key") {
		t.Fatalf("keyless close %+v", got)
	}
	if issue := as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: uid})).Selected.Issue; issue.Status != "open" {
		t.Fatalf("a refused close left the Issue %q", issue.Status)
	}
}

func TestGatewayACommandQueuedBehindAHungConnectGivesUpWithinOneDeadline(t *testing.T) {
	gateway := NewGateway(func(ctx context.Context, _ string) (Target, error) {
		<-ctx.Done()
		return Target{}, ctx.Err()
	})
	t.Cleanup(gateway.Close)
	gateway.deadline = 150 * time.Millisecond
	start := time.Now()
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() { run(t, gateway, model.ReadSnapshot{Request: 1}) })
	}
	wg.Wait()
	if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
		t.Fatalf("two queued commands took %v, want about one deadline of 150ms", elapsed)
	}
}

func TestGatewaySendsTheIdempotencyKeyOnCommentsRepliesAndCreates(t *testing.T) {
	kata := startKata(t, "0.20.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")
	uid := createIssue(t, gateway, workspace, demo, "Discuss")
	detailOf := func() *model.IssueDetailData {
		return as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: uid})).Selected
	}
	comment := func(text, replyTo, key string) {
		as[model.DraftSaved](t, run(t, gateway, model.SaveDraft{Workspace: workspace, UID: uid, ProjectID: demo.ID, BaseText: detailOf().Issue.Body, Text: text, Comment: true, Actor: "tester", ReplyTo: replyTo, ReplyKind: model.ReplyPlain, Key: key}))
	}

	comment("once", "", "k-comment")
	comment("once", "", "k-comment")
	if got := len(detailOf().Comments); got != 1 {
		t.Fatalf("%d comments after one key sent twice, want 1", got)
	}
	// Two deliberate comments with the same text carry different keys and both post.
	comment("once", "", "k-other")
	question := detailOf().Comments[0].UID
	comment("answer", question, "k-reply")
	comment("answer", question, "k-reply")
	if got := len(detailOf().Comments); got != 3 {
		t.Fatalf("%d comments, want the two deliberate comments and one reply", got)
	}

	create := func(key string) model.IssueCreated {
		return as[model.IssueCreated](t, run(t, gateway, model.CreateIssueCmd{Workspace: workspace, Actor: "tester", ProjectID: demo.ID, Title: "Keyed", Key: key}))
	}
	first, second := create("k-create"), create("k-create")
	if first.Err != nil || first.UID == "" || second != first {
		t.Fatalf("create twice with one key: %+v then %+v", first, second)
	}
	snapshot := as[model.SnapshotLoaded](t, run(t, gateway, model.ReadSnapshot{Workspace: workspace, Query: model.CollectionQuery(model.Collection{View: "project", ProjectUID: demo.UID, Title: demo.Name}, model.Filters{}), Request: 1}))
	keyed := 0
	for _, issue := range snapshot.Snapshot.Issues {
		if issue.Title == "Keyed" {
			keyed++
		}
	}
	if keyed != 1 {
		t.Fatalf("%d Issues titled Keyed, want 1", keyed)
	}
}

func TestGatewayClosingTwiceWithOneKeyClosesOnce(t *testing.T) {
	kata := startKata(t, "0.20.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")
	uid := createIssue(t, gateway, workspace, demo, "Close once")
	closeIt := func() model.CloseResultMsg {
		return as[model.CloseResultMsg](t, run(t, gateway, model.CloseIssueCmd{Workspace: workspace, UID: uid, ProjectID: demo.ID, Actor: "tester", Quick: true, Key: "k-once"}))
	}
	if first, second := closeIt(), closeIt(); first.Err != nil || second.Err != nil {
		t.Fatalf("close twice with one key: %v then %v", first.Err, second.Err)
	}
	closes := 0
	for _, e := range as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: uid})).Selected.History {
		if e.Type == "issue.closed" {
			closes++
		}
	}
	if closes != 1 {
		t.Fatalf("%d close events, want 1", closes)
	}
}

func TestRecurrenceConflictingThroughAllAttemptsSaysSoPlainly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"recurrence":{"uid":"r","revision":3,"rrule":"FREQ=DAILY","dtstart":"2026-10-19","timezone":"UTC"}}`))
			return
		}
		w.WriteHeader(http.StatusPreconditionFailed)
		_, _ = w.Write([]byte(`{"status":412,"error":{"code":"revision_conflict","message":"issue revision is 3"}}`))
	}))
	t.Cleanup(server.Close)
	raw, err := client.NewWithHTTPClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	c := &Client{Client: raw}
	const want = "The Recurrence kept changing while saving; nothing was written. Try again."
	terms := model.RecurrenceTerms{Rule: "FREQ=DAILY", Start: "2026-10-19", TimeZone: "UTC"}
	if _, err := c.ChangeRecurrence(context.Background(), model.ChangeRecurrenceCmd{RecurrenceUID: "r", RecurrenceTerms: terms, Opened: terms}); err == nil || err.Error() != want {
		t.Fatalf("change: %v", err)
	}
	if err := c.StopRecurrence(context.Background(), model.StopRecurrenceCmd{RecurrenceUID: "r"}); err == nil || err.Error() != want {
		t.Fatalf("stop: %v", err)
	}
}

func TestGatewayCreatesAnIssueWithDateMetadataOnlyForTheDatesSet(t *testing.T) {
	kata := startKata(t, "0.20.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")
	for _, tt := range []struct {
		title, scheduled, deadline string
		want                       map[string]any
	}{
		{"Both", "2026-10-30", "2026-10-28", map[string]any{"scheduled_on": "2026-10-30", "deadline_on": "2026-10-28"}},
		{"Deadline only", "", "2026-11-01", map[string]any{"deadline_on": "2026-11-01"}},
		{"Neither", "", "", map[string]any{}},
	} {
		created := as[model.IssueCreated](t, run(t, gateway, model.CreateIssueCmd{Workspace: workspace, Actor: "tester", ProjectID: demo.ID, Title: tt.title, ScheduledOn: tt.scheduled, DeadlineOn: tt.deadline}))
		if created.Err != nil {
			t.Fatalf("%s: %v", tt.title, created.Err)
		}
		got := as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: created.UID})).Selected.Issue.Metadata
		if len(got) != len(tt.want) {
			t.Errorf("%s: metadata %v, want %v", tt.title, got, tt.want)
		}
		for k, v := range tt.want {
			if got[k] != v {
				t.Errorf("%s: %s = %v, want %v", tt.title, k, got[k], v)
			}
		}
	}
}
