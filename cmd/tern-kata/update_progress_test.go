package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestDownloadTellsBytesAndAnnouncedSize(t *testing.T) {
	body := strings.Repeat("x", 4096)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	var out bytes.Buffer
	var lastDone, lastTotal int64
	err := download(context.Background(), server.Client(), server.URL, &out, 1<<20, func(done, total int64) { lastDone, lastTotal = done, total })
	if err != nil {
		t.Fatal(err)
	}
	if lastDone != int64(len(body)) || lastTotal != int64(len(body)) || out.String() != body {
		t.Errorf("done %d total %d, want %d of %d", lastDone, lastTotal, len(body), len(body))
	}
}
