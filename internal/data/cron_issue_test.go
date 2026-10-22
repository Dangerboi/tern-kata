package data

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"go.kenn.io/kata"
	"path/filepath"
)

// Native delivery runs in kata's daemon loop every 30 seconds from a job's creation, and no client can write the metadata it leaves,
// so a cron-made Issue is made here the way kata would send it: the real service answers, and the one Issue named in marked
// comes back with cron.occurrence metadata for the job in marked.job.
type marked struct{ issue, job atomic.Pointer[string] }

var occurredAt = time.Date(2026, 10, 22, 14, 0, 0, 0, time.UTC)

func (m *marked) mark(issue, job string) { m.issue.Store(&issue); m.job.Store(&job) }

func (m *marked) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Del("Accept-Encoding") // the body is read and rewritten, so it must not come compressed
		rec := httptest.NewRecorder()
		next.ServeHTTP(rec, r)
		body := rec.Body.Bytes()
		issue, job := m.issue.Load(), m.job.Load()
		var doc any
		if issue != nil && json.Unmarshal(body, &doc) == nil {
			m.inject(doc, *issue, *job)
			body, _ = json.Marshal(doc)
		}
		for k, v := range rec.Header() {
			if k != "Content-Length" {
				w.Header()[k] = v
			}
		}
		w.WriteHeader(rec.Code)
		_, _ = w.Write(body)
	})
}

func (m *marked) inject(doc any, issue, job string) {
	switch v := doc.(type) {
	case map[string]any:
		if v["uid"] == issue {
			metadata, _ := v["metadata"].(map[string]any)
			if metadata == nil {
				metadata = map[string]any{}
			}
			metadata["cron.occurrence"] = map[string]any{"version": 1, "job_uid": job, "definition_event_uid": "01EVENT", "scheduled_at": occurredAt.Format(time.RFC3339)}
			v["metadata"] = metadata
		}
		for _, child := range v {
			m.inject(child, issue, job)
		}
	case []any:
		for _, child := range v {
			m.inject(child, issue, job)
		}
	}
}

func newMarkedJobsFixture(t *testing.T) (*jobsFixture, *marked) {
	t.Helper()
	t.Setenv("KATA_HOME", t.TempDir())
	t.Setenv("KATA_SERVER", "")
	t.Setenv("KATA_AUTH_TOKEN", "")
	service, err := kata.New(context.Background(), kata.Config{DSN: filepath.Join(t.TempDir(), "kata.db"), Auth: kata.AuthConfig{TrustCallerAuthentication: true}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	marks := &marked{}
	server := serve(t, marks.wrap(service.Handler()), "0.19.0")
	gateway := newGateway(t, newLocator(server.target()))
	workspace, demo := project(t, gateway, "demo")
	return &jobsFixture{gateway: gateway, client: server.target().Client, workspace: workspace, demo: demo}, marks
}


func TestGatewayMarksACronMadeIssueInTheDetailAndTheSnapshotAndNoOtherIssue(t *testing.T) {
	f, marks := newMarkedJobsFixture(t)
	made := createIssue(t, f.gateway, f.workspace, f.demo, "Report for the 14:00 run")
	plain := createIssue(t, f.gateway, f.workspace, f.demo, "Plain")
	marks.mark(made, "01JOB")

	detail := as[model.DetailLoaded](t, run(t, f.gateway, model.LoadDetail{Workspace: f.workspace, UID: made})).Selected.Issue
	got := detail.CronOccurrence
	if got == nil || !detail.CronMade() || got.JobUID != "01JOB" || got.DefinitionEventUID != "01EVENT" || !got.ScheduledAt.Equal(occurredAt) {
		t.Fatalf("occurrence %+v", got)
	}
	if other := as[model.DetailLoaded](t, run(t, f.gateway, model.LoadDetail{Workspace: f.workspace, UID: plain})).Selected.Issue; other.CronMade() {
		t.Fatalf("an Issue without cron.occurrence is marked %+v", other.CronOccurrence)
	}

	view := "project"
	loaded := as[model.SnapshotLoaded](t, run(t, f.gateway, model.ReadSnapshot{Workspace: f.workspace, Request: 1, Query: model.SnapshotQuery{View: &view, ProjectUID: &f.demo.UID}}))
	marked, unmarked := 0, 0
	for _, issue := range loaded.Snapshot.Issues {
		switch {
		case issue.UID == made && issue.CronMade() && issue.CronOccurrence.JobUID == "01JOB":
			marked++
		case issue.UID == plain && !issue.CronMade():
			unmarked++
		}
	}
	if marked != 1 || unmarked != 1 {
		t.Fatalf("snapshot marks %d cron-made and %d plain Issues, want one of each: %+v", marked, unmarked, loaded.Snapshot.Issues)
	}
}

func TestGatewayJobsReadWithActivityCarryANativeJobsChildrenNewestFirst(t *testing.T) {
	f, marks := newMarkedJobsFixture(t)
	parent := createIssue(t, f.gateway, f.workspace, f.demo, "Weekly report")
	job := f.native(t, "Report", parent)
	child := createIssue(t, f.gateway, f.workspace, f.demo, "Report for the 14:00 run")
	stranger := createIssue(t, f.gateway, f.workspace, f.demo, "Unrelated work")
	shortID := as[model.DetailLoaded](t, run(t, f.gateway, model.LoadDetail{Workspace: f.workspace, UID: parent})).Selected.Issue.ShortID
	for _, uid := range []string{child, stranger} {
		write := model.WriteField{Workspace: f.workspace, UID: uid, ProjectID: f.demo.ID, Actor: "tester", Action: model.SetParentAction, Value: shortID}
		if written := as[model.FieldWritten](t, run(t, f.gateway, write)); written.Err != nil {
			t.Fatal(written.Err)
		}
	}
	marks.mark(child, job.UID)

	loaded := as[model.JobsLoaded](t, run(t, f.gateway, model.LoadJobs{Workspace: f.workspace, ProjectID: f.demo.ID, Request: 7, Activity: true}))
	children := loaded.Activity[job.UID].Children
	if loaded.Err != nil || len(children) != 1 || children[0].UID != child || children[0].Title != "Report for the 14:00 run" || children[0].Status != "open" || !children[0].ScheduledAt.Equal(occurredAt) {
		t.Fatalf("children %+v (err %v), want the Issue made for the job and not its other child", children, loaded.Err)
	}
}
