package data

import (
	"encoding/json"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// cannedEmbeddings is a daemon answering GET /api/v1/embeddings/local with status in the given HTTP status.
func cannedEmbeddings(t *testing.T, code int, body map[string]any) *Gateway {
	t.Helper()
	server := startKata(t, "0.23.0")
	hook := func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != "/api/v1/embeddings/local" || r.Method != http.MethodGet {
			return false
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(body)
		return true
	}
	server.intercept.Store(&hook)
	return newGateway(t, newLocator(server.target()))
}

func TestGatewayEmbeddingsStatusMapsEachProviderJobAndIndexingShape(t *testing.T) {
	for name, tc := range map[string]struct {
		body map[string]any
		want model.EmbeddingsStatus
	}{
		"off with Ollama": {
			map[string]any{"provider": "none", "download_bytes": 290_400_000, "ollama_available": true, "can_change": true},
			model.EmbeddingsStatus{Provider: model.EmbeddingsOff, DownloadBytes: 290_400_000, OllamaAvailable: true, CanChange: true},
		},
		"downloading": {
			map[string]any{"provider": "none", "download_bytes": 290_400_000, "ollama_available": false, "can_change": true,
				"job": map[string]any{"state": "downloading", "downloaded": 100, "total": 290_400_000}},
			model.EmbeddingsStatus{Provider: model.EmbeddingsOff, DownloadBytes: 290_400_000, CanChange: true,
				Job: &model.EmbeddingsJob{State: model.EmbeddingsDownloading, Downloaded: 100, Total: 290_400_000}},
		},
		"restart owed": {
			map[string]any{"provider": "none", "download_bytes": 0, "ollama_available": false, "can_change": true, "job": map[string]any{"state": "restart", "downloaded": 0, "total": 0}},
			model.EmbeddingsStatus{Provider: model.EmbeddingsOff, CanChange: true, Job: &model.EmbeddingsJob{State: model.EmbeddingsRestart}},
		},
		"failed job": {
			map[string]any{"provider": "none", "download_bytes": 0, "ollama_available": false, "can_change": true, "job": map[string]any{"state": "failed", "downloaded": 0, "total": 0, "error": "disk full"}},
			model.EmbeddingsStatus{Provider: model.EmbeddingsOff, CanChange: true, Job: &model.EmbeddingsJob{State: model.EmbeddingsFailed, Error: "disk full"}},
		},
		"built-in indexing with an ETA": {
			map[string]any{"provider": "built_in", "download_bytes": 0, "ollama_available": false, "can_change": true,
				"indexing": map[string]any{"done": 40, "total": 200, "eta_seconds": 150, "failed": false}},
			model.EmbeddingsStatus{Provider: model.EmbeddingsBuiltIn, CanChange: true,
				Indexing: &model.EmbeddingsIndexing{Done: 40, Total: 200, ETA: 150 * time.Second}},
		},
		"Ollama with indexing failed": {
			map[string]any{"provider": "ollama", "download_bytes": 0, "ollama_available": true, "can_change": false,
				"indexing": map[string]any{"done": 1, "total": 2, "failed": true}},
			model.EmbeddingsStatus{Provider: model.EmbeddingsOllama, OllamaAvailable: true, Indexing: &model.EmbeddingsIndexing{Done: 1, Total: 2, Failed: true}},
		},
		"another provider": {
			map[string]any{"provider": "other", "download_bytes": 0, "ollama_available": false, "can_change": false},
			model.EmbeddingsStatus{Provider: model.EmbeddingsOther},
		},
		"redacted for a non-administrator": {
			map[string]any{"provider": "built_in", "download_bytes": 0, "ollama_available": false, "can_change": false,
				"indexing": map[string]any{"done": 0, "total": 0, "failed": false}},
			model.EmbeddingsStatus{Provider: model.EmbeddingsBuiltIn, Indexing: &model.EmbeddingsIndexing{}},
		},
	} {
		loaded := as[model.EmbeddingsStatusLoaded](t, run(t, cannedEmbeddings(t, http.StatusOK, tc.body), model.LoadEmbeddingsStatus{Workspace: t.TempDir(), Request: 5}))
		if loaded.Err != nil || loaded.Request != 5 || !reflect.DeepEqual(loaded.Status, tc.want) {
			t.Errorf("%s: answer %+v, want %+v for request 5", name, loaded, tc.want)
		}
	}
}

func TestEmbeddingsStatusHostIsTheRemoteDaemonsHostAndEmptyForALocalOne(t *testing.T) {
	for name, tc := range map[string]struct {
		c      Client
		server string
		want   string
	}{
		"catalog entry":   {Client{kind: model.DaemonRemote, entry: CatalogEntry{URL: "https://kata.example:7474/"}}, "https://ignored.example", "kata.example:7474"},
		"KATA_SERVER":     {Client{kind: model.DaemonRemote}, "https://env.example", "env.example"},
		"pinned url":      {Client{kind: model.DaemonRemote, pinURL: "http://pinned.example:80"}, "", "pinned.example:80"},
		"remote, unknown": {Client{kind: model.DaemonRemote}, "", ""},
		"local":           {Client{kind: model.DaemonLocal, pinURL: "https://pinned.example"}, "https://env.example", ""},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("KATA_SERVER", tc.server)
			if got := tc.c.remoteHost(); got != tc.want {
				t.Errorf("host %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGatewayEmbeddingsStatusReportsKatasErrorForADaemonThatCannotReportIt(t *testing.T) {
	gateway := cannedEmbeddings(t, http.StatusNotFound, map[string]any{"status": 404, "error": map[string]any{"code": "not_found", "message": "local embeddings setup is unavailable"}})
	loaded := as[model.EmbeddingsStatusLoaded](t, run(t, gateway, model.LoadEmbeddingsStatus{Workspace: t.TempDir(), Request: 8}))
	if loaded.Err == nil || loaded.Err.Error() != "local embeddings setup is unavailable" || loaded.Request != 8 {
		t.Fatalf("answer %+v, want kata's error line for request 8", loaded)
	}
}

// cannedTurnOn is a daemon answering POST /api/v1/embeddings/local with body in the given HTTP status, and counting the POSTs.
func cannedTurnOn(t *testing.T, code int, body map[string]any) (*Gateway, *atomic.Int32) {
	t.Helper()
	posts := new(atomic.Int32)
	server := startKata(t, "0.23.0")
	hook := func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != "/api/v1/embeddings/local" || r.Method != http.MethodPost {
			return false
		}
		posts.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(body)
		return true
	}
	server.intercept.Store(&hook)
	return newGateway(t, newLocator(server.target())), posts
}

func TestGatewayTurnOnEmbeddingsAnswersWithTheStatusKataStartsWith(t *testing.T) {
	gateway, posts := cannedTurnOn(t, http.StatusAccepted, map[string]any{
		"provider": "none", "download_bytes": 290_400_000, "ollama_available": false, "can_change": true,
		"job": map[string]any{"state": "downloading", "downloaded": 0, "total": 290_400_000},
	})
	turned := as[model.EmbeddingsTurnedOn](t, run(t, gateway, model.TurnOnEmbeddings{Workspace: t.TempDir()}))
	want := model.EmbeddingsStatus{Provider: model.EmbeddingsOff, DownloadBytes: 290_400_000, CanChange: true,
		Job: &model.EmbeddingsJob{State: model.EmbeddingsDownloading, Total: 290_400_000}}
	if turned.Err != nil || !reflect.DeepEqual(turned.Status, want) || posts.Load() != 1 {
		t.Fatalf("answer %+v after %d POSTs, want status %+v after one", turned, posts.Load(), want)
	}
}

func TestGatewayTurnOnEmbeddingsReportsKatasReasonForARefusal(t *testing.T) {
	for name, tc := range map[string]struct {
		code   int
		status string
		reason string
	}{
		"no daemon administration": {http.StatusForbidden, "forbidden", "local embeddings setup requires daemon administration"},
		"busy or configured":       {http.StatusConflict, "conflict", "local embeddings setup is already running"},
	} {
		gateway, posts := cannedTurnOn(t, tc.code, map[string]any{"status": tc.code, "error": map[string]any{"code": tc.status, "message": tc.reason}})
		turned := as[model.EmbeddingsTurnedOn](t, run(t, gateway, model.TurnOnEmbeddings{Workspace: t.TempDir()}))
		if turned.Err == nil || turned.Err.Error() != tc.reason || posts.Load() != 1 {
			t.Errorf("%s: answer %+v after %d POSTs, want kata's reason %q after one", name, turned, posts.Load(), tc.reason)
		}
	}
}
