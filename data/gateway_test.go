package data

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/model"
	"go.kenn.io/kata"
	g "go.kenn.io/kata/pkg/client/generated"
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
	return Target{Kind: "local", Network: "tcp", RequestBaseURL: s.URL}
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
	kata := startKata(t, "0.18.0")
	gateway := newGateway(t, newLocator(kata.target()))

	workspace, bound := project(t, gateway, "demo")
	binding, err := os.ReadFile(filepath.Join(workspace, ".kata.toml"))
	if err != nil || !strings.Contains(string(binding), `name = "demo"`) {
		t.Fatalf(".kata.toml %q, %v", binding, err)
	}
	loaded := as[model.ProjectLoaded](t, run(t, gateway, model.LoadProject{Workspace: workspace}))
	if loaded.Project.UID != bound.UID || loaded.Daemon.Remote || loaded.Daemon.Version != "v0.18.0" {
		t.Fatalf("workspace resolves to %+v, daemon %+v; want the bound Project %q on a local v0.18.0 daemon", loaded.Project, loaded.Daemon, bound.UID)
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
	kata := startKata(t, "0.18.0")
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
	loaded := as[model.CollectionLoaded](t, run(t, gateway, model.LoadCollection{Workspace: workspace, Collection: collection}))
	if loaded.ETag == "" || len(loaded.Snapshot.Collection) != 1 || loaded.Snapshot.Collection[0].UID != uid {
		t.Fatalf("collection %+v", loaded)
	}
	query := model.Model{Collection: collection, LocalDate: loaded.LocalDate, TimeZone: loaded.TimeZone}.SnapshotQuery()
	unchanged := as[model.SnapshotRefreshed](t, run(t, gateway, model.RefreshSnapshot{Workspace: workspace, Query: query, ETag: loaded.ETag, Request: 4}))
	if !unchanged.NotModified || unchanged.Request != 4 || len(unchanged.Snapshot.Collection) != 0 {
		t.Fatalf("refresh with the current ETag %+v, want a 304", unchanged)
	}
	createIssue(t, gateway, workspace, demo, "Zebra migration")
	changed := as[model.SnapshotRefreshed](t, run(t, gateway, model.RefreshSnapshot{Workspace: workspace, Query: query, ETag: loaded.ETag, Request: 5}))
	if changed.NotModified || len(changed.Snapshot.Collection) != 2 || changed.ETag == "" || changed.ETag == loaded.ETag {
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

	closed := as[model.CloseResultMsg](t, run(t, gateway, model.CloseIssueCmd{Workspace: workspace, UID: uid, ProjectID: demo.ID, Revision: detail.Selected.Issue.Revision, Body: quickClose("tester")}))
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

func quickClose(actor string) g.CloseIssueBody {
	reason, protocol, source := g.CloseActionRequestBodyReason("done"), g.CloseActionRequestBodyRetryProtocol("close-v1"), g.CloseActionRequestBodySource("tui")
	return g.CloseIssueBody{Actor: &actor, Reason: &reason, RetryProtocol: &protocol, Source: &source}
}

func TestGatewayDraftRevisionConflicts(t *testing.T) {
	kata := startKata(t, "0.18.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")
	uid := createIssue(t, gateway, workspace, demo, "Draft me")
	detail := as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: uid}))
	base := detail.Selected.Issue.Revision
	save := func(text string, revision int64, comment bool) model.Msg {
		return run(t, gateway, model.SaveDraft{Workspace: workspace, UID: uid, ProjectID: demo.ID, BaseRevision: revision, Text: text, Comment: comment, Actor: "tester"})
	}

	if saved := as[model.DraftSaved](t, save("first body", base, false)); saved.UID != uid {
		t.Fatalf("saved %+v", saved)
	}
	// Another writer changes the Issue after the Draft was started.
	other, err := connect(context.Background(), kata.target())
	if err != nil {
		t.Fatal(err)
	}
	actor := "someone"
	patched, err := other.PatchIssueMetadataWithResponse(context.Background(), &g.PatchIssueMetadataRequestOptions{
		PathParams: &g.PatchIssueMetadataPath{ProjectID: demo.ID, Ref: uid},
		Body:       &g.PatchIssueMetadataBody{Actor: &actor, Patch: map[string]any{"work.attention": "needs-human"}},
	})
	if err != nil {
		t.Fatalf("%v: %s", err, patched.Body)
	}
	conflict := as[model.DraftConflict](t, save("stale body", base, false))
	if conflict.UID != uid || conflict.Issue.Body != "first body" || conflict.Issue.Revision <= base {
		t.Fatalf("stale edit %+v, want the stored Issue", conflict)
	}
	staleComment := as[model.DraftConflict](t, save("stale comment", base, true))
	if staleComment.Issue.Body != "first body" {
		t.Fatalf("stale comment %+v", staleComment)
	}
	if saved := as[model.DraftSaved](t, save("a comment", conflict.Issue.Revision, true)); saved.UID != uid {
		t.Fatalf("comment %+v", saved)
	}
	detail = as[model.DetailLoaded](t, run(t, gateway, model.LoadDetail{Workspace: workspace, UID: uid}))
	if len(detail.Selected.Comments) != 1 || detail.Selected.Comments[0].Body != "a comment" {
		t.Fatalf("comments %+v", detail.Selected.Comments)
	}
}

func TestGatewayReportsDaemonErrorsInTheAskingMessage(t *testing.T) {
	kata := startKata(t, "0.18.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")

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
		{"field", model.WriteField{Workspace: workspace, UID: "nope", ProjectID: demo.ID, Action: model.AddLabelAction, Value: "x", Actor: "tester"}, func(err error) model.Msg { return model.FieldWritten{UID: "nope", Err: err} }},
		{"project", model.BindProject{Workspace: workspace, Actor: "tester", Name: "other"}, func(err error) model.Msg { return model.ProjectSaved{Err: err} }},
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
			case model.FieldWritten:
				failure = got.Err
			case model.ProjectSaved:
				failure = got.Err
			}
			if failure == nil || !reflect.DeepEqual(got, tt.want(failure)) {
				t.Fatalf("got %#v, want a %T carrying the daemon's error", got, tt.want(nil))
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
		{model.WriteField{UID: "u"}, model.FieldWritten{UID: "u", Err: boom}},
		{model.LoadFieldLabels{UID: "u"}, model.FieldLabelsLoaded{UID: "u", Err: boom}},
		{model.CreateProject{}, model.ProjectSaved{Err: boom}},
		{model.BindProject{}, model.ProjectSaved{Err: boom}},
		{model.LoadProject{}, model.Failed{Err: boom}},
		{model.LoadDetail{UID: "u"}, model.DetailFailed{UID: "u", Err: boom}},
		{model.LoadCollection{}, model.Failed{Err: boom}},
		{model.RefreshSnapshot{Request: 9}, model.SnapshotFailed{Request: 9, Err: boom}},
	} {
		if got := run(t, gateway, tt.cmd); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%T: got %#v, want %#v", tt.cmd, got, tt.want)
		}
	}
}

func TestGatewayLocatorFailureIsRetriedOnTheNextCommand(t *testing.T) {
	kata := startKata(t, "0.18.0")
	l := newLocator(kata.target())
	boom := errors.New("kata daemon locate: no daemon")
	l.err.Store(&boom)
	gateway := newGateway(t, l)

	if got := as[model.Failed](t, run(t, gateway, model.LoadCollection{})); got.Err != boom {
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

func TestGatewayVersionGate(t *testing.T) {
	for _, tt := range []struct {
		version string
		wantErr string
	}{
		{"0.17.9", "kata daemon 0.17.9 is too old; tern-kata requires v0.18.0 or newer; upgrade kata"},
		{"dev", `cannot verify kata daemon version "dev"; tern-kata requires v0.18.0 or newer`},
		{"g1a2b3c", `cannot verify kata daemon version "g1a2b3c"; tern-kata requires v0.18.0 or newer`},
		{"v0.18.0", ""},
		{"1.2.0", ""},
	} {
		t.Run(tt.version, func(t *testing.T) {
			kata := startKata(t, tt.version)
			gateway := newGateway(t, newLocator(kata.target()))
			got := run(t, gateway, model.LoadDetail{UID: "u"})
			failed, isFailure := got.(model.DetailFailed)
			if tt.wantErr == "" {
				// The gate passed; the daemon then reports the Issue missing.
				if !isFailure || strings.Contains(failed.Err.Error(), "requires v0.18.0") {
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
	first := startKata(t, "0.18.0")
	l := newLocator(first.target())
	gateway := newGateway(t, l)
	workspace, demo := project(t, gateway, "demo")
	uid := createIssue(t, gateway, workspace, demo, "Survives")

	// The restarted daemon is a new process on a new address, with the same data.
	second := serve(t, first.handler, "0.18.0")
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
	first := startKata(t, "0.18.0")
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
	first := startKata(t, "0.18.0")
	l := newLocator(first.target())
	gateway := newGateway(t, l)
	workspace, _ := project(t, gateway, "demo")
	first.Close()

	got := as[model.Failed](t, run(t, gateway, model.LoadCollection{Workspace: workspace, Collection: model.Collection{View: "inbox", Title: "Inbox"}}))
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
	kata := startKata(t, "0.18.0")
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
	kata := startKata(t, "0.18.0")
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
	kata := startKata(t, "0.18.0")
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
	kata := startKata(t, "0.18.0")
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
