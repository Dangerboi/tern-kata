package data

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// cannedEmbeddings is a daemon answering method on /api/v1/embeddings/local with body in the given HTTP status, counting those requests.
// remote makes the connection read as a remote daemon.
func cannedEmbeddings(t *testing.T, method string, remote bool, code int, body map[string]any) (*Gateway, *atomic.Int32) {
	t.Helper()
	hits := new(atomic.Int32)
	server := startKata(t, "0.23.0")
	hook := func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != "/api/v1/embeddings/local" || r.Method != method {
			return false
		}
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(body)
		return true
	}
	server.intercept.Store(&hook)
	target := server.target()
	target.Remote = remote
	return newGateway(t, newLocator(target)), hits
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
	} {
		gateway, _ := cannedEmbeddings(t, http.MethodGet, false, http.StatusOK, tc.body)
		loaded := as[model.EmbeddingsStatusLoaded](t, run(t, gateway, model.LoadEmbeddingsStatus{Workspace: t.TempDir(), Request: 5}))
		if loaded.Err != nil || loaded.Request != 5 || !reflect.DeepEqual(loaded.Status, tc.want) {
			t.Errorf("%s: answer %+v, want %+v for request 5", name, loaded, tc.want)
		}
	}
}

func TestEmbeddingsStatusNamesTheRemoteDaemonsHostAndNoHostForALocalOne(t *testing.T) {
	body := map[string]any{"provider": "none", "download_bytes": 0, "ollama_available": false, "can_change": true}
	const catalog = "[[daemon]]\nname = \"hub\"\nurl = \"https://kata.example:7474/\"\n"
	const pin = "version = 1\n[server]\nurl = \"http://pinned.example:80\"\n"
	for name, tc := range map[string]struct {
		remote  bool
		server  string // KATA_SERVER
		current string // the catalog entry the Pane switched to
		pin     string // the workspace's .kata.local.toml
		want    string
	}{
		"catalog entry, which outranks KATA_SERVER": {true, "https://ignored.example", "hub", "", "kata.example:7474"},
		"KATA_SERVER":                       {true, "https://env.example", "", "", "env.example"},
		"pinned url":                        {true, "", "", pin, "pinned.example:80"},
		"KATA_SERVER outranks a pinned url": {true, "https://env.example", "", pin, "env.example"},
		"remote with no known url":          {true, "", "", "", ""},
		"local":                             {false, "https://env.example", "", pin, ""},
	} {
		t.Run(name, func(t *testing.T) {
			gateway, _ := cannedEmbeddings(t, http.MethodGet, tc.remote, http.StatusOK, body)
			t.Setenv("KATA_SERVER", tc.server)
			writeCatalog(t, catalog)
			gateway.current = tc.current
			workspace := t.TempDir()
			for file, text := range map[string]string{".kata.toml": "version = 1\n", ".kata.local.toml": tc.pin} {
				if text == "" {
					continue
				}
				if err := os.WriteFile(filepath.Join(workspace, file), []byte(text), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			loaded := as[model.EmbeddingsStatusLoaded](t, run(t, gateway, model.LoadEmbeddingsStatus{Workspace: workspace}))
			if loaded.Err != nil || loaded.Status.Remote != tc.remote || loaded.Status.Host != tc.want {
				t.Errorf("remote %v host %q (err %v), want remote %v host %q", loaded.Status.Remote, loaded.Status.Host, loaded.Err, tc.remote, tc.want)
			}
		})
	}
}

func TestGatewayHoldsAnEmbeddingsStatusReadUntilItsPauseHasPassed(t *testing.T) {
	gateway, _ := cannedEmbeddings(t, http.MethodGet, false, http.StatusOK, map[string]any{"provider": "none", "download_bytes": 0, "ollama_available": false, "can_change": true})
	const pause = 100 * time.Millisecond
	start := time.Now()
	loaded := as[model.EmbeddingsStatusLoaded](t, run(t, gateway, model.LoadEmbeddingsStatus{Workspace: t.TempDir(), Request: 9, After: pause}))
	if elapsed := time.Since(start); loaded.Err != nil || loaded.Request != 9 || elapsed < pause {
		t.Fatalf("read %+v after %v, want request 9 no sooner than %v", loaded, elapsed, pause)
	}
}

func TestGatewayEmbeddingsStatusReportsKatasErrorForADaemonThatCannotReportIt(t *testing.T) {
	gateway, _ := cannedEmbeddings(t, http.MethodGet, false, http.StatusNotFound, map[string]any{"status": 404, "error": map[string]any{"code": "not_found", "message": "local embeddings setup is unavailable"}})
	loaded := as[model.EmbeddingsStatusLoaded](t, run(t, gateway, model.LoadEmbeddingsStatus{Workspace: t.TempDir(), Request: 8}))
	if loaded.Err == nil || loaded.Err.Error() != "local embeddings setup is unavailable" || loaded.Request != 8 {
		t.Fatalf("answer %+v, want kata's error line for request 8", loaded)
	}
}

func TestGatewayTurnOnEmbeddingsAnswersWithTheStatusKataStartsWith(t *testing.T) {
	gateway, posts := cannedEmbeddings(t, http.MethodPost, false, http.StatusAccepted, map[string]any{
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
	const reason = "local embeddings setup requires daemon administration"
	gateway, posts := cannedEmbeddings(t, http.MethodPost, false, http.StatusForbidden, map[string]any{"status": 403, "error": map[string]any{"code": "forbidden", "message": reason}})
	turned := as[model.EmbeddingsTurnedOn](t, run(t, gateway, model.TurnOnEmbeddings{Workspace: t.TempDir()}))
	if turned.Err == nil || turned.Err.Error() != reason || posts.Load() != 1 {
		t.Errorf("answer %+v after %d POSTs, want kata's reason %q after one", turned, posts.Load(), reason)
	}
}
