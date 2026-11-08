package data

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// landedThen502 lets kata handle every write while on and answers 502 instead, so each write lands but reads as failed.
func landedThen502(kata *kataServer, on *atomic.Bool) {
	hook := func(w http.ResponseWriter, r *http.Request) bool {
		if on.Load() && r.Method != http.MethodGet {
			kata.handler.ServeHTTP(httptest.NewRecorder(), r)
			http.Error(w, "bad gateway", http.StatusBadGateway)
			return true
		}
		return false
	}
	kata.intercept.Store(&hook)
}

func TestAnArchiveThatLandedAnswersEvenWhenTheParentReadFails(t *testing.T) {
	kata := startKata(t, "0.21.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")
	f := &jobsFixture{gateway: gateway, client: kata.target().Client, workspace: workspace, demo: demo}
	parent := createIssue(t, gateway, workspace, demo, "Weekly report")
	job := f.created(t, "Report", parent)

	var landed atomic.Bool
	hook := func(w http.ResponseWriter, r *http.Request) bool {
		if landed.Load() && r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/issues/") {
			http.Error(w, `{"status":500,"error":{"code":"internal","message":"boom"}}`, http.StatusInternalServerError)
			return true
		}
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/cron/") {
			landed.Store(true)
		}
		return false
	}
	kata.intercept.Store(&hook)
	cmd := model.ArchiveJobCmd{Workspace: workspace, ProjectID: demo.ID, Actor: "tester", UID: job.UID, ExpectedEventUID: job.DefinitionEventUID}
	got := as[model.JobWritten](t, run(t, gateway, cmd))
	if got.Err != nil || got.Changed != nil || got.Job == nil || !got.Job.Archived() {
		t.Fatalf("archive that landed answered %+v, want the archived job and no error", got)
	}
	if got.Job.Parent != nil {
		t.Errorf("job carries parent %+v, want none: the read failed", got.Job.Parent)
	}
}

func TestATokenCreatedBehindAnAmbiguousAnswerSaysItMayExist(t *testing.T) {
	kata := startKata(t, "0.21.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, _ := project(t, gateway, "demo")
	var on atomic.Bool
	landedThen502(kata, &on)
	on.Store(true)
	file := filepath.Join(t.TempDir(), "tok")
	created := as[model.TokenCreated](t, run(t, gateway, model.CreateTokenCmd{Workspace: workspace, Actor: "ci-bot", Name: "ci", ExpiresInSeconds: 3600, File: file}))
	on.Store(false)
	if created.Err == nil || !strings.Contains(created.Err.Error(), "outcome of creating the token is unknown") || !strings.Contains(created.Err.Error(), "Credentials Page") {
		t.Fatalf("answer %v, want the outcome unknown and the Credentials Page named", created.Err)
	}
	if len(listed(t, gateway, workspace)) != 1 {
		t.Fatal("the daemon holds no token: the fixture did not land the write")
	}
}

func TestAHungDaemonWriteEndsAtTheCallBudgetAsUnknown(t *testing.T) {
	f := newUndoFixture(t)
	uid := f.issue("hang")
	f.gateway.callBudget = 200 * time.Millisecond
	var hang atomic.Bool
	hook := func(w http.ResponseWriter, r *http.Request) bool {
		if hang.Load() && r.Method != http.MethodGet {
			time.Sleep(2 * time.Second)
			return true
		}
		return false
	}
	f.kata.intercept.Store(&hook)
	hang.Store(true)
	start := time.Now()
	written := as[model.FieldWritten](t, run(t, f.gateway, model.WriteField{Workspace: f.workspace, UID: uid, ProjectID: f.project.ID, Actor: "tester", Action: model.AddLabelAction, Value: "x"}))
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("write took %v, want about the 200ms budget", elapsed)
	}
	if written.Err == nil || !written.Record.Unknown {
		t.Fatalf("hung write answered %+v, want an error with an unknown outcome", written)
	}
}

func TestAFailedDeleteReasonCommentLeavesTheUndoHistoryAlone(t *testing.T) {
	f := newUndoFixture(t)
	uid := f.issue("target")
	hook := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/comments") {
			http.Error(w, "bad gateway", http.StatusBadGateway)
			return true
		}
		return false
	}
	f.kata.intercept.Store(&hook)
	got := f.deleteIssue(uid, "because")
	if got.Err == nil {
		t.Fatal("the delete did not fail")
	}
	if got.Record.Unknown || got.Record.Entry != nil || got.Record.Boundary != "" {
		t.Errorf("record %+v, want none: nothing was deleted", got.Record)
	}
	f.kata.intercept.Store(nil)
	if !f.listed(uid) {
		t.Error("the Issue is gone")
	}
}

func TestACloseWhoseAnswerWasLostAndReplayedRecordsABoundary(t *testing.T) {
	f := newUndoFixture(t)
	uid := f.issue("to close")
	var dropped atomic.Int32
	hook := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/actions/close") && dropped.Add(1) == 1 {
			f.kata.handler.ServeHTTP(httptest.NewRecorder(), r)
			conn, _, _ := w.(http.Hijacker).Hijack()
			_ = conn.Close()
			return true
		}
		return false
	}
	f.kata.intercept.Store(&hook)
	f.field(uid, model.AddLabelAction, "earlier")
	cmd := model.CloseIssueCmd{Workspace: f.workspace, UID: uid, ProjectID: f.project.ID, Actor: "tester", Quick: true, Reason: model.ReasonDone, Key: "k1"}
	got := as[model.CloseResultMsg](t, run(t, f.gateway, cmd))
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	if got.Record.Entry != nil || got.Record.Boundary == "" {
		t.Fatalf("record %+v, want a boundary and no entry", got.Record)
	}
}
