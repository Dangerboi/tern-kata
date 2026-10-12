package data

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"go.kenn.io/kata/pkg/client"
)

func projectClient(t *testing.T, created bool) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if created {
			_, _ = w.Write([]byte(`{"created":true,"project":{"uid":"u","name":"work"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"created":false,"project":{"uid":"u","name":"work"}}`))
	}))
	t.Cleanup(server.Close)
	raw, err := client.NewWithHTTPClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	return &Client{Client: raw}
}

func TestCreateProjectRejectsATakenName(t *testing.T) {
	cmd := model.CreateProject{Workspace: t.TempDir(), Actor: "me", Name: "work"}
	project, err := projectClient(t, false).CreateProject(context.Background(), cmd)
	if err == nil || !strings.Contains(err.Error(), `"work"`) || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("taken name: project %+v, err %v", project, err)
	}
	if project, err = projectClient(t, true).CreateProject(context.Background(), cmd); err != nil || project.Name != "work" {
		t.Fatalf("new name: project %+v, err %v", project, err)
	}
}
