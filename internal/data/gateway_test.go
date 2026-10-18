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
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"go.kenn.io/kata"
	"go.kenn.io/kata/pkg/client"
)

// kataServer is kata's own service, run in process over HTTP with a database of
// its own. The service reports no release version when built from a test, so
// ping answers for it with the version under test.
type kataServer struct {
	*httptest.Server
	handler http.Handler
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
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/ping" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "service": "kata", "version": version})
			return
		}
		service.ServeHTTP(w, r)
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &kataServer{Server: server, handler: service}
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
	kata := startKata(t, "0.19.0")
	gateway := newGateway(t, newLocator(kata.target()))

	workspace, bound := project(t, gateway, "demo")
	binding, err := os.ReadFile(filepath.Join(workspace, ".kata.toml"))
	if err != nil || !strings.Contains(string(binding), `name = "demo"`) {
		t.Fatalf(".kata.toml %q, %v", binding, err)
	}
	loaded := as[model.ProjectLoaded](t, run(t, gateway, model.LoadProject{Workspace: workspace}))
	if loaded.Project.UID != bound.UID || loaded.Daemon.Remote || loaded.Daemon.Version != "v0.19.0" {
		t.Fatalf("workspace resolves to %+v, daemon %+v; want the bound Project %q on a local v0.19.0 daemon", loaded.Project, loaded.Daemon, bound.UID)
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

func TestGatewayReadsAndWritesIssues(t *testing.T) {
	kata := startKata(t, "0.19.0")
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

	closed := as[model.CloseResultMsg](t, run(t, gateway, model.CloseIssueCmd{Workspace: workspace, UID: uid, ProjectID: demo.ID, Actor: "tester", Quick: true}))
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
	kata := startKata(t, "0.19.0")
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
	kata := startKata(t, "0.19.0")
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
			tt.cmd.Workspace, tt.cmd.UID, tt.cmd.ProjectID, tt.cmd.Actor = workspace, uid, demo.ID, "tester"
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
	kata := startKata(t, "0.19.0")
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
	kata := startKata(t, "0.19.0")
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
		{"close", model.CloseIssueCmd{Workspace: workspace, UID: "nope", ProjectID: demo.ID}, func(err error) model.Msg { return model.CloseResultMsg{UID: "nope", Err: err} }},
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
			}), "0.19.0")
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
		{model.CloseIssueCmd{UID: "u"}, model.CloseResultMsg{UID: "u", Err: boom}},
		{model.ReopenIssueCmd{UID: "u"}, model.CloseResultMsg{UID: "u", Err: boom}},
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
	kata := startKata(t, "0.19.0")
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
	kata := startKata(t, "0.19.0")
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
	startKata(t, "0.19.0") // only for its isolated kata home; the server is stopped
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
		{"0.18.0", "kata daemon 0.18.0 is too old; tern-kata requires v0.19.0 or newer; upgrade kata"},
		{"dev", `cannot verify kata daemon version "dev"; tern-kata requires v0.19.0 or newer`},
		{"g1a2b3c", `cannot verify kata daemon version "g1a2b3c"; tern-kata requires v0.19.0 or newer`},
		{"v0.19.0", ""},
		{"1.2.0", ""},
	} {
		t.Run(tt.version, func(t *testing.T) {
			kata := startKata(t, tt.version)
			gateway := newGateway(t, newLocator(kata.target()))
			got := run(t, gateway, model.LoadDetail{UID: "u"})
			failed, isFailure := got.(model.DetailFailed)
			if tt.wantErr == "" {
				// The gate passed; the daemon then reports the Issue missing.
				if !isFailure || strings.Contains(failed.Err.Error(), "requires v0.19.0") {
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
	first := startKata(t, "0.19.0")
	l := newLocator(first.target())
	gateway := newGateway(t, l)
	workspace, demo := project(t, gateway, "demo")
	uid := createIssue(t, gateway, workspace, demo, "Survives")

	// The restarted daemon is a new process on a new address, with the same data.
	second := serve(t, first.handler, "0.19.0")
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
	first := startKata(t, "0.19.0")
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
	first := startKata(t, "0.19.0")
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

func stream(t *testing.T, gateway *Gateway, cmd model.Subscribe) (<-chan model.Msg, <-chan struct{}) {
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
	kata := startKata(t, "0.19.0")
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
	kata := startKata(t, "0.19.0")
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
	kata := startKata(t, "0.19.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, _ := project(t, gateway, "demo")

	oldMessages, oldDone := stream(t, gateway, model.Subscribe{Workspace: workspace, Generation: 1})
	receive(t, oldMessages)
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
	kata := startKata(t, "0.19.0")
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

func TestGatewaySnapshotCountsChildrenOnEveryRead(t *testing.T) {
	kata := startKata(t, "0.19.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")
	parent := createIssue(t, gateway, workspace, demo, "Parent")
	query := model.CollectionQuery(model.Collection{View: "project", ProjectUID: demo.UID, Title: demo.Name}, model.Filters{})

	before := as[model.SnapshotLoaded](t, run(t, gateway, model.ReadSnapshot{Workspace: workspace, Query: query}))
	if len(before.Snapshot.ChildCounts) != 0 {
		t.Fatalf("child counts %+v before any child", before.Snapshot.ChildCounts)
	}
	child := as[model.IssueCreated](t, run(t, gateway, model.CreateIssueCmd{Workspace: workspace, Actor: "tester", ProjectID: demo.ID, Title: "Child", ParentUID: parent}))
	if child.Err != nil {
		t.Fatal(child.Err)
	}
	after := as[model.SnapshotLoaded](t, run(t, gateway, model.ReadSnapshot{Workspace: workspace, Query: query, ETag: before.ETag}))
	if counts := after.Snapshot.ChildCounts[parent]; after.NotModified || counts.Total != 1 || counts.Open != 1 {
		t.Fatalf("after a child: %+v, want the parent to count its one open child", after)
	}
	links := after.Snapshot.Links
	if len(links) != 1 || links[0].Type != "parent" || links[0].FromIssueUID != child.UID || links[0].ToIssueUID != parent {
		t.Fatalf("links %+v, want the child's parent link to %s", links, parent)
	}
	if len(after.Snapshot.Issues) != 2 || after.Snapshot.Cursor == 0 {
		t.Fatalf("snapshot %+v, want both Issues and the event cursor", after.Snapshot)
	}
	if len(after.Snapshot.Catalog) != 1 || after.Snapshot.Catalog[0].Project.UID != demo.UID || after.Snapshot.Catalog[0].Stats == nil || after.Snapshot.Catalog[0].Stats.Open != 2 {
		t.Fatalf("catalog %+v, want the Project with two open Issues", after.Snapshot.Catalog)
	}
	unchanged := as[model.SnapshotLoaded](t, run(t, gateway, model.ReadSnapshot{Workspace: workspace, Query: query, ETag: after.ETag}))
	if !unchanged.NotModified {
		t.Fatalf("current ETag: %+v, want not modified", unchanged)
	}
}

func TestGatewayReadsASnapshotWhenTheTimeZoneIsBad(t *testing.T) {
	t.Setenv("TZ", "Nowhere/Land")
	kata := startKata(t, "0.19.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")
	query := model.CollectionQuery(model.Collection{View: "project", ProjectUID: demo.UID, Title: demo.Name}, model.Filters{})
	loaded := as[model.SnapshotLoaded](t, run(t, gateway, model.ReadSnapshot{Workspace: workspace, Query: query, Request: 1}))
	if loaded.TimeZone != "UTC" || !strings.Contains(loaded.TimeZoneNote, "Nowhere/Land") {
		t.Fatalf("zone %q note %q", loaded.TimeZone, loaded.TimeZoneNote)
	}
}

// changeOwner is another actor assigning the Issue: on kata v0.19 that moves the revision without touching the text.
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
	if closed := as[model.CloseResultMsg](t, run(t, gateway, model.CloseIssueCmd{Workspace: workspace, UID: uid, ProjectID: project.ID, Actor: "someone", Quick: true})); closed.Err != nil {
		t.Fatal(closed.Err)
	}
	if reopened := as[model.CloseResultMsg](t, run(t, gateway, model.ReopenIssueCmd{Workspace: workspace, UID: uid, ProjectID: project.ID, Actor: "someone"})); reopened.Err != nil {
		t.Fatal(reopened.Err)
	}
}

func TestGatewayGuardedWritesSurviveOwnerAndStatusChanges(t *testing.T) {
	kata := startKata(t, "0.19.0")
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
		closed := as[model.CloseResultMsg](t, run(t, gateway, model.CloseIssueCmd{Workspace: workspace, UID: uid, ProjectID: demo.ID, Actor: "tester", Quick: true}))
		if closed.Err != nil {
			t.Fatalf("close after an owner and status change from revision %d: %v", issue.Revision, closed.Err)
		}
	})
	t.Run("closed meanwhile", func(t *testing.T) {
		uid := createIssue(t, gateway, workspace, demo, "Closed twice")
		as[model.CloseResultMsg](t, run(t, gateway, model.CloseIssueCmd{Workspace: workspace, UID: uid, ProjectID: demo.ID, Actor: "someone", Quick: true}))
		before := as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: uid}))
		closed := as[model.CloseResultMsg](t, run(t, gateway, model.CloseIssueCmd{Workspace: workspace, UID: uid, ProjectID: demo.ID, Actor: "tester", Quick: true}))
		after := as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: uid}))
		if closed.Err == nil || !strings.Contains(closed.Err.Error(), "closed") || len(after.Selected.History) != len(before.Selected.History) {
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
	kata := startKata(t, "0.19.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")
	uid := createIssue(t, gateway, workspace, demo, "Discuss")
	parent := createIssue(t, gateway, workspace, demo, "Parent")
	detailOf := func() *model.IssueDetailData {
		return as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: uid})).Selected
	}

	as[model.DraftSaved](t, run(t, gateway, model.SaveDraft{Workspace: workspace, UID: uid, ProjectID: demo.ID, BaseText: detailOf().Issue.Body, Text: "the question", Comment: true, Actor: "asker"}))
	question := detailOf().Comments[0]
	// A reply is only reachable through kata's wire client; the Pane writes plain comments.
	wire, err := client.New(kata.URL)
	if err != nil {
		t.Fatal(err)
	}
	kind, reply, actor := g.Confirm, question.UID, "answerer"
	if _, err := wire.CreateCommentWithResponse(context.Background(), &g.CreateCommentRequestOptions{
		PathParams: &g.CreateCommentPath{ProjectID: fmt.Sprint(demo.ID), Ref: uid},
		Body:       &g.CreateCommentBody{Body: "confirmed: " + strings.Repeat("it reproduces. ", 4), Actor: &actor, Kind: &kind, ReplyTo: &reply},
	}); err != nil {
		t.Fatal(err)
	}
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
	kata := startKata(t, "0.19.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")
	uid := createIssue(t, gateway, workspace, demo, "Discuss")
	detailOf := func() *model.IssueDetailData {
		return as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: uid})).Selected
	}
	save := func(text, replyTo, kind string, force bool) model.Msg {
		return run(t, gateway, model.SaveDraft{Workspace: workspace, UID: uid, ProjectID: demo.ID, BaseText: detailOf().Issue.Body, Text: text, Comment: true, Actor: "answerer", ReplyTo: replyTo, Kind: kind, Force: force})
	}
	as[model.DraftSaved](t, save("the question", "", "", false))
	question := detailOf().Comments[0].UID
	evidence := strings.Repeat("it reproduces here. ", 3)

	for _, kind := range []string{"reply", "confirm", "refute", "supersede"} {
		as[model.DraftSaved](t, save(evidence, question, kind, false))
	}
	comments := detailOf().Comments
	if len(comments) != 5 {
		t.Fatalf("%d comments, want the question and four replies", len(comments))
	}
	for i, kind := range []string{"reply", "confirm", "refute", "supersede"} {
		reply := comments[i+1]
		if reply.ReplyKind == nil || *reply.ReplyKind != kind || reply.Reply == nil || reply.Reply.UID == nil || *reply.Reply.UID != question {
			t.Errorf("reply %d: %+v, want a %s answering %s", i, reply, kind, question)
		}
	}
}

func TestGatewayReportsADuplicateReplyAndSendsItAnywayWithForce(t *testing.T) {
	kata := startKata(t, "0.19.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")
	uid := createIssue(t, gateway, workspace, demo, "Discuss")
	save := func(text, replyTo, kind string, force bool) model.Msg {
		detail := as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: uid})).Selected
		return run(t, gateway, model.SaveDraft{Workspace: workspace, UID: uid, ProjectID: demo.ID, BaseText: detail.Issue.Body, Text: text, Comment: true, Actor: "answerer", ReplyTo: replyTo, Kind: kind, Force: force})
	}
	as[model.DraftSaved](t, save("the question", "", "", false))
	question := as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: uid})).Selected.Comments[0].UID
	as[model.DraftSaved](t, save("first", question, "reply", false))

	dup := as[model.DraftFailed](t, save("second", question, "reply", false))
	if !dup.Duplicate || dup.UID != uid || !strings.Contains(dup.Err.Error(), "already exists") || strings.Contains(dup.Err.Error(), "--force") {
		t.Fatalf("duplicate result %+v", dup)
	}
	as[model.DraftSaved](t, save("second", question, "reply", true))
	if got := len(as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: uid})).Selected.Comments); got != 3 {
		t.Fatalf("%d comments after sending anyway, want 3", got)
	}
}

func TestGatewayTranslatesTheRecurrenceFromTheSnapshotIncludingAStoppedOne(t *testing.T) {
	kata := startKata(t, "0.19.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")
	wire, err := client.New(kata.URL)
	if err != nil {
		t.Fatal(err)
	}
	ctx, actor := context.Background(), "tester"
	recur := func(title, rule string) (uid string, rec g.Recurrence) {
		uid = createIssue(t, gateway, workspace, demo, title)
		created, err := wire.CreateRecurrenceWithResponse(ctx, &g.CreateRecurrenceRequestOptions{
			PathParams: &g.CreateRecurrencePath{ProjectID: demo.ID},
			Body: &g.CreateRecurrenceBody{Actor: &actor, Rrule: rule, Dtstart: "2026-10-19", Timezone: "Europe/Zurich", InitialIssueRef: &uid,
				Template: g.RecurrenceTemplateInput{Title: title}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return uid, created.JSON201.Recurrence
	}
	live, liveRec := recur("Live", "FREQ=WEEKLY;INTERVAL=1;BYDAY=MO,TH")
	stopped, stoppedRec := recur("Stopped", "FREQ=DAILY;INTERVAL=1")
	if _, err := wire.DeleteRecurrence(ctx, &g.DeleteRecurrenceRequestOptions{
		PathParams: &g.DeleteRecurrencePath{ProjectID: demo.ID, RecurrenceUID: stoppedRec.UID},
		Query:      &g.DeleteRecurrenceQuery{Actor: &actor},
		Header:     &g.DeleteRecurrenceHeaders{IfMatch: fmt.Sprintf("\"rev-%d\"", stoppedRec.Revision)},
	}); err != nil {
		t.Fatal(err)
	}
	plain := createIssue(t, gateway, workspace, demo, "Plain")

	selected := func(uid string) *model.IssueDetailData {
		query := model.CollectionQuery(model.Collection{View: "project", ProjectUID: demo.UID, Title: demo.Name}, model.Filters{})
		query.SelectedIssueUID = &uid
		return as[model.SnapshotLoaded](t, run(t, gateway, model.ReadSnapshot{Workspace: workspace, Query: query, Request: 1})).Selected
	}
	want := model.Recurrence{UID: liveRec.UID, Rule: "FREQ=WEEKLY;INTERVAL=1;BYDAY=MO,TH", Start: "2026-10-19", TimeZone: "Europe/Zurich", Next: "2026-10-22", Revision: liveRec.Revision}
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
	kata := startKata(t, "0.19.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")
	priority := int64(2)
	created := as[model.IssueCreated](t, run(t, gateway, model.CreateIssueCmd{Workspace: workspace, Actor: "tester", ProjectID: demo.ID, Title: "Water plants", Body: "Every pot", Owner: "alice", Labels: []string{"home"}, Priority: &priority}))
	if created.Err != nil {
		t.Fatal(created.Err)
	}
	uid := created.UID

	saved := as[model.RecurrenceResultMsg](t, run(t, gateway, model.CreateRecurrenceCmd{Workspace: workspace, UID: uid, ProjectID: demo.ID, Actor: "tester", Rule: "FREQ=WEEKLY;INTERVAL=1;BYDAY=MO", Start: "2026-10-19", TimeZone: "Europe/Zurich"}))
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

	if closed := as[model.CloseResultMsg](t, run(t, gateway, model.CloseIssueCmd{Workspace: workspace, UID: uid, ProjectID: demo.ID, Actor: "tester", Quick: true})); closed.Err != nil {
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

// recurringIssue makes a new Issue the first occurrence of a weekly recurrence and returns its UID and the recurrence as the detail shows it.
func recurringIssue(t *testing.T, gateway *Gateway, workspace string, demo model.Project) (string, model.Recurrence) {
	t.Helper()
	uid := createIssue(t, gateway, workspace, demo, "Water plants")
	saved := as[model.RecurrenceResultMsg](t, run(t, gateway, model.CreateRecurrenceCmd{Workspace: workspace, UID: uid, ProjectID: demo.ID, Actor: "tester", Rule: "FREQ=WEEKLY;INTERVAL=1;BYDAY=MO", Start: "2026-10-19", TimeZone: "Europe/Zurich"}))
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
		Rule: rule, Start: "2026-10-26", TimeZone: "UTC", OpenedRule: opened.Rule, OpenedStart: opened.Start, OpenedZone: opened.TimeZone}
}

func TestGatewayChangesTheScheduleAfterOnlyBookkeepingMoved(t *testing.T) {
	kata := startKata(t, "0.19.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")
	uid, opened := recurringIssue(t, gateway, workspace, demo)

	// Closing the occurrence as done creates the next one and moves the recurrence's revision, not its schedule.
	if closed := as[model.CloseResultMsg](t, run(t, gateway, model.CloseIssueCmd{Workspace: workspace, UID: uid, ProjectID: demo.ID, Actor: "tester", Quick: true})); closed.Err != nil {
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
	kata := startKata(t, "0.19.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")
	uid, opened := recurringIssue(t, gateway, workspace, demo)

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
	kata := startKata(t, "0.19.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")
	uid, opened := recurringIssue(t, gateway, workspace, demo)
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
