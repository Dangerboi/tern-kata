package data

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"go.kenn.io/kata/pkg/client"
)

func TestChildCountsAreTheDaemonsWithOneRequest(t *testing.T) {
	var queries []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.Path+"?"+r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"issues":[
			{"uid":"parent","child_counts":{"open":2,"total":6}},
			{"uid":"leaf"}]}`))
	}))
	t.Cleanup(server.Close)
	raw, err := client.NewWithHTTPClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	c := &Client{Client: raw}
	snapshot := model.Snapshot{Catalog: []model.CatalogProject{{Project: model.Project{ID: 7, UID: "p"}}}}
	for _, tt := range []struct {
		name       string
		projectUID *string
		wantQuery  string
	}{
		{"a Project Collection asks only that Project", new("p"), "/api/v1/issues?project_id=7"},
		{"any other Collection asks every Project", nil, "/api/v1/issues?"},
	} {
		queries = nil
		counts, err := c.readChildCounts(context.Background(), snapshot, tt.projectUID)
		if err != nil || len(queries) != 1 || queries[0] != tt.wantQuery {
			t.Fatalf("%s: err %v, requests %q, want %q", tt.name, err, queries, tt.wantQuery)
		}
		if counts["parent"] != (model.ChildCounts{Open: 2, Total: 6}) || len(counts) != 1 {
			t.Fatalf("%s: counts %+v: only an Issue with children has an entry", tt.name, counts)
		}
	}
}
