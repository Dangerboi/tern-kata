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

// streamClient is a Client whose daemon answers every request with handler.
func streamClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	raw, err := client.NewWithHTTPClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	return &Client{Client: raw}
}

func readStream(t *testing.T, body string) (events []model.StreamEvent, err error) {
	t.Helper()
	c := streamClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(body))
	})
	err = c.ReadEvents(context.Background(), model.Subscribe{Generation: 3}, func(m model.Msg) bool {
		if e, ok := m.(model.StreamEvent); ok {
			events = append(events, e)
		}
		return true
	})
	return events, err
}

func TestReadEventsParsesAStreamOfEvents(t *testing.T) {
	events, err := readStream(t, ": hello\n\n"+
		"id: 7\nevent: issue.created\ndata: {\"event_id\":7,\"type\":\"issue.created\"}\n\n"+
		"data: {\"event_id\":8,\n"+"data: \"type\":\"issue.closed\"}\r\n\r\n"+
		"event: sync.reset_required\ndata: {\"event_id\":9}\n\n"+
		"data: {\"event_id\":10}\n\n")
	if err != nil {
		t.Fatal(err)
	}
	want := []model.StreamEvent{
		{Generation: 3, ID: 7, Type: "issue.created"},
		{Generation: 3, ID: 8, Type: "issue.closed"},
		{Generation: 3, ID: 9, Type: "sync.reset_required"},
	}
	if len(events) != len(want) {
		t.Fatalf("events %+v, want %+v", events, want)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Fatalf("event %d is %+v, want %+v", i, events[i], want[i])
		}
	}
}

func TestReadEventsFailsOnALineOverTheLimit(t *testing.T) {
	events, err := readStream(t, "data: "+strings.Repeat("x", 2<<20)+"\n\n")
	if err == nil || !strings.Contains(err.Error(), "line is longer") || len(events) != 0 {
		t.Fatalf("events %+v, err %v, want the stream dropped for its line", events, err)
	}
}

func TestReadEventsFailsOnAnEventWhoseDataIsOverTheLimit(t *testing.T) {
	line := "data: " + strings.Repeat("x", 100<<10) + "\n"
	events, err := readStream(t, strings.Repeat(line, 11)+"\n")
	if err == nil || !strings.Contains(err.Error(), "more than") || len(events) != 0 {
		t.Fatalf("events %+v, err %v, want the stream dropped for its data", events, err)
	}
}
