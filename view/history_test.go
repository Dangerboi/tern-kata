package view_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/model"
	"github.com/Dangerboi/tern-kata/view"
	g "go.kenn.io/kata/pkg/client/generated"
)

// spansOf is the (text, style) runs of a node's spans.
func spansOf(n map[string]any) [][2]string {
	var runs [][2]string
	spans, _ := n["spans"].([]any)
	for _, span := range spans {
		fields := span.(map[string]any)
		style, _ := fields["s"].(string)
		runs = append(runs, [2]string{fields["t"].(string), style})
	}
	return runs
}

func TestHistoryRendersSentencesWithTonedVerbs(t *testing.T) {
	at := time.Date(2026, 10, 8, 18, 5, 5, 0, time.UTC)
	events := []g.Event{
		{UID: "e1", Actor: "dana", Type: "issue.created", Payload: `{"title":"Alpha"}`, CreatedAt: at},
		{UID: "e2", Actor: "dana", Type: "issue.closed", Payload: `{"reason":"done","message":"shipped"}`, CreatedAt: at},
		{UID: "e3", Actor: "dana", Type: "issue.reopened", Payload: `{}`, CreatedAt: at},
		{UID: "e4", Actor: "dana", Type: "issue.assigned", Payload: `{"owner":"alice"}`, CreatedAt: at},
	}
	m := apply(browsing(), model.DetailLoaded{UID: alpha.UID, Selected: &g.UISelectedAuthority{Issue: &alpha, History: events}}, model.ActivityTabMsg{Tab: model.TabEvents})
	nodes := render(t, m)
	section := view.DetailActivityID

	for id, n := range nodes {
		if strings.HasPrefix(id, section+".") && strings.Contains(text(n), "```") {
			t.Errorf("%s shows a code block: %q", id, text(n))
		}
	}
	for _, tt := range []struct {
		event, sentence string
		verb, style     string
	}{
		{"e1", "dana created this Issue", "created", "accent"},
		{"e2", "dana closed: done", "closed:", "success"},
		{"e3", "dana reopened", "reopened", "warning"},
		{"e4", "dana assigned to alice", "assigned", ""},
	} {
		heading := nodes[section+"."+tt.event+".heading"]
		if !strings.HasPrefix(text(heading), tt.sentence) {
			t.Errorf("%s reads %q, want it to start with %q", tt.event, text(heading), tt.sentence)
		}
		var got string
		found := false
		for _, run := range spansOf(heading.P) {
			if run[0] == tt.verb {
				got, found = run[1], true
			}
		}
		if !found || got != tt.style {
			t.Errorf("%s verb %q has style %q (found %v), want %q", tt.event, tt.verb, got, found, tt.style)
		}
	}
	if detail := text(nodes[section+".e2.body"]); detail != "shipped" {
		t.Errorf("close detail line = %q", detail)
	}
}
