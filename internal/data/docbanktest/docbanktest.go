// Package docbanktest is a small fake Docbank HTTP server for tests of kata's native attachments. It speaks the part of
// Docbank's API that kata's attachment handlers call: make folders, upload a file as a new version, and read a version's bytes.
// kata's own fake is internal to kata and cannot be imported.
package docbanktest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const Key = "test-key"

// Fake keeps uploaded bytes in memory.
type Fake struct {
	// URL is the fake's address, for a kata Docbank config's url.
	URL string

	server  *httptest.Server
	mu      sync.Mutex
	dirs    map[string]int64
	content map[string][]byte
	uploads int
}

func New(t testing.TB) *Fake {
	t.Helper()
	f := &Fake{dirs: map[string]int64{}, content: map[string][]byte{}}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	f.URL = f.server.URL
	t.Cleanup(f.server.Close)
	return f
}

func (f *Fake) Uploads() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.uploads
}

// Forget drops every stored version, as a Docbank that lost the content would.
func (f *Fake) Forget() {
	f.mu.Lock()
	defer f.mu.Unlock()
	clear(f.content)
}

func (f *Fake) Stop() { f.server.Close() }

func (f *Fake) serve(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Api-Key") != Key {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	send := func(v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/path" && r.URL.Query().Get("path") == "/":
		send(map[string]any{"id": 0, "kind": "dir"}) // the vault root: the read a key check uses
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/path":
		id, ok := f.dirs[r.URL.Query().Get("path")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		send(map[string]any{"id": id, "kind": "dir"})
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/path/mkdir":
		var in struct {
			Path string `json:"path"`
		}
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			http.Error(w, "path", http.StatusBadRequest)
			return
		}
		if _, exists := f.dirs[in.Path]; exists {
			http.Error(w, "exists", http.StatusConflict)
			return
		}
		f.dirs[in.Path] = int64(len(f.dirs) + 1)
		send(map[string]any{"id": f.dirs[in.Path], "kind": "dir"})
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/uploads":
		r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
		parts, err := r.MultipartReader()
		if err != nil {
			http.Error(w, "multipart", http.StatusBadRequest)
			return
		}
		var body []byte
		files := 0
		for {
			part, err := parts.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				http.Error(w, "multipart", http.StatusBadRequest)
				return
			}
			if part.FormName() == "file" {
				files++
				body, _ = io.ReadAll(part)
			}
		}
		if files != 1 {
			http.Error(w, "file count", http.StatusBadRequest)
			return
		}
		sum := sha256.Sum256(body)
		if hex.EncodeToString(sum[:]) != r.Header.Get("X-Docbank-Blob-Hash") || strconv.Itoa(len(body)) != r.Header.Get("X-Docbank-Blob-Size") {
			http.Error(w, "identity", http.StatusUnprocessableEntity)
			return
		}
		f.uploads++
		version := fmt.Sprintf("6f1c2e8a-2222-4222-8222-%012d", f.uploads)
		f.content[version] = body
		send(map[string]any{"node": map[string]string{"current_version_id": version}})
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/versions/") && strings.HasSuffix(r.URL.Path, "/content"):
		version := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/versions/"), "/content")
		body, ok := f.content[version]
		if !ok {
			http.NotFound(w, r)
			return
		}
		sum := sha256.Sum256(body)
		w.Header().Set("X-Docbank-Blob-Size", strconv.Itoa(len(body)))
		w.Header().Set("X-Docbank-Blob-Hash", hex.EncodeToString(sum[:]))
		_, _ = w.Write(body)
	default:
		http.NotFound(w, r)
	}
}
