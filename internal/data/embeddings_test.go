package data

import (
	"encoding/json"
	"net/http"
	"reflect"
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

func TestGatewayEmbeddingsStatusReportsKatasErrorForADaemonThatCannotReportIt(t *testing.T) {
	gateway := cannedEmbeddings(t, http.StatusNotFound, map[string]any{"status": 404, "error": map[string]any{"code": "not_found", "message": "local embeddings setup is unavailable"}})
	loaded := as[model.EmbeddingsStatusLoaded](t, run(t, gateway, model.LoadEmbeddingsStatus{Workspace: t.TempDir(), Request: 8}))
	if loaded.Err == nil || loaded.Err.Error() != "local embeddings setup is unavailable" || loaded.Request != 8 {
		t.Fatalf("answer %+v, want kata's error line for request 8", loaded)
	}
}
