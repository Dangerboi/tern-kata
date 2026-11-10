package data

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"sync/atomic"
	"testing"

	g "go.kenn.io/kata/pkg/client/generated"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestGatewaySearchByMeaningOnKataWithoutEmbeddingsAnswersByWordsInKatasOrder(t *testing.T) {
	kata := startKata(t, "0.23.0")
	gateway := newGateway(t, newLocator(kata.target()))
	workspace, demo := project(t, gateway, "demo")
	createIssue(t, gateway, workspace, demo, "Zebra migration plan")
	createIssue(t, gateway, workspace, demo, "Unrelated chore")
	createIssue(t, gateway, workspace, demo, "Zebra crossing alert for the zebra team")

	mode, limit := g.Auto, int64(model.SearchLimit)
	direct, err := kata.target().Client.SearchIssuesWithResponse(context.Background(), &g.SearchIssuesRequestOptions{
		PathParams: &g.SearchIssuesPath{ProjectID: demo.ID}, Query: &g.SearchIssuesQuery{Q: "zebra", Mode: &mode, Limit: &limit},
	})
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, hit := range direct.JSON200.Results {
		want = append(want, hit.Issue.UID)
	}
	if len(want) != 2 {
		t.Fatalf("kata found %d Issues for zebra, want 2", len(want))
	}

	done := as[model.SearchByMeaningDone](t, run(t, gateway, model.SearchByMeaning{Workspace: workspace, ProjectID: demo.ID, Query: "zebra", Request: 6}))
	if done.Err != nil || done.Request != 6 || done.Query != "zebra" || !done.Lexical || done.Truncated {
		t.Fatalf("answer %+v, want kata's words answer to request 6", done)
	}
	if !reflect.DeepEqual(done.UIDs, want) {
		t.Fatalf("hits %v, want kata's order %v", done.UIDs, want)
	}
}

// cannedSearch is a daemon answering a ranked search in mode with the hits' UIDs in the order given, degraded for reason when
// reason is not empty, and keeping the query it was asked.
func cannedSearch(t *testing.T, mode, reason string, uids []string, truncated bool) (*kataServer, *atomic.Pointer[url.Values]) {
	t.Helper()
	asked := new(atomic.Pointer[url.Values])
	server := startKata(t, "0.23.0")
	hook := func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != "/api/v1/projects/7/search" {
			return false
		}
		query := r.URL.Query()
		asked.Store(&query)
		results := make([]map[string]any, len(uids))
		for i, uid := range uids {
			results[i] = map[string]any{
				"issue": map[string]any{
					"id": i + 1, "uid": uid, "short_id": "abcd" + strconv.Itoa(i), "title": "Issue " + uid, "body": "", "author": "alice", "status": "open",
					"project_id": 7, "revision": 1, "metadata": map[string]any{},
					"created_at": "2026-11-01T10:00:00Z", "updated_at": "2026-11-01T10:00:00Z",
				},
				"matched_in": []string{"title"}, "score": 1.0 - float64(i)/10,
			}
		}
		w.Header().Set("Content-Type", "application/json")
		body := map[string]any{"mode": mode, "query": query.Get("q"), "complete": !truncated, "truncated": truncated, "results": results}
		if reason != "" {
			body["degraded"], body["degraded_reason"] = true, reason
		}
		_ = json.NewEncoder(w).Encode(body)
		return true
	}
	server.intercept.Store(&hook)
	return server, asked
}

func TestGatewaySearchByMeaningKeepsAHybridAnswersOrderAndIsNotCutBelowTheLimit(t *testing.T) {
	// kata v0.23 marks every ranked answer it cannot page as truncated, however few Issues matched.
	server, asked := cannedSearch(t, "hybrid", "", []string{"third", "first", "second"}, true)
	gateway := newGateway(t, newLocator(server.target()))

	done := as[model.SearchByMeaningDone](t, run(t, gateway, model.SearchByMeaning{
		Workspace: t.TempDir(), ProjectID: 7, Query: "slow login", Status: "open", Labels: []string{"bug", "ui"}, Request: 2,
	}))
	if done.Err != nil || done.Lexical || done.Reason != "" || done.Truncated || done.Request != 2 {
		t.Fatalf("answer %+v, want a hybrid answer to request 2 that is not cut: three hits are fewer than the limit", done)
	}
	if want := []string{"third", "first", "second"}; !reflect.DeepEqual(done.UIDs, want) {
		t.Fatalf("hits %v, want the daemon's order %v", done.UIDs, want)
	}
	sent := asked.Load()
	if sent == nil {
		t.Fatal("the daemon was not asked")
	}
	if sent.Get("mode") != "auto" || sent.Get("q") != "slow login" || sent.Get("status") != "open" || sent.Get("limit") != "50" || !reflect.DeepEqual((*sent)["label"], []string{"bug", "ui"}) {
		t.Fatalf("the daemon was asked %v, want mode auto, the query, the status, both labels and limit 50", *sent)
	}
}

func TestGatewaySearchByMeaningIsCutWhenKataFillsTheLimitAndSaysMoreMatched(t *testing.T) {
	uids := make([]string, model.SearchLimit)
	for i := range uids {
		uids[i] = "hit" + strconv.Itoa(i)
	}
	server, _ := cannedSearch(t, "hybrid", "", uids, true)
	gateway := newGateway(t, newLocator(server.target()))

	done := as[model.SearchByMeaningDone](t, run(t, gateway, model.SearchByMeaning{Workspace: t.TempDir(), ProjectID: 7, Query: "login", Request: 1}))
	if done.Err != nil || !done.Truncated {
		t.Fatalf("answer %+v, want it cut: kata filled the limit and said more matched", done)
	}
}

func TestGatewaySearchByMeaningCarriesKatasWordsReasonForAWordsAnswer(t *testing.T) {
	server, _ := cannedSearch(t, "lexical", "embedding model is loading", nil, false)
	gateway := newGateway(t, newLocator(server.target()))
	done := as[model.SearchByMeaningDone](t, run(t, gateway, model.SearchByMeaning{Workspace: t.TempDir(), ProjectID: 7, Query: "q"}))
	if done.Err != nil || !done.Lexical || done.Reason != "embedding model is loading" || len(done.UIDs) != 0 {
		t.Fatalf("answer %+v", done)
	}
}

func TestGatewaySearchByMeaningReportsKatasErrorForAProjectItDoesNotHave(t *testing.T) {
	server := startKata(t, "0.23.0")
	gateway := newGateway(t, newLocator(server.target()))
	done := as[model.SearchByMeaningDone](t, run(t, gateway, model.SearchByMeaning{Workspace: t.TempDir(), ProjectID: 9999, Query: "q", Request: 3}))
	if done.Err == nil || done.Request != 3 || done.Query != "q" {
		t.Fatalf("answer %+v, want kata's error to request 3", done)
	}
}
