package view_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/model"
	"github.com/Dangerboi/tern-kata/view"
	g "go.kenn.io/kata/pkg/client/generated"
)

// referencing is a wide Pane on Issue "main" with one Link to "peer", a linked
// event naming "peer", a long close message with evidence, and a plain event.
func referencing(tab model.ActivityTab) (model.Model, string) {
	long := strings.Repeat("a long close message ", 30)
	main := model.Issue{UID: "main", ShortID: "main", Title: "Main", Status: "open", ProjectName: "work"}
	peer := model.Issue{UID: "peer", ShortID: "peer", Title: "Peer", Status: "open", ProjectName: "work"}
	links := []g.UILink{{ID: 1, Type: "blocks", FromIssueUID: "main", FromQualifiedID: "work#main", ToIssueUID: "peer", ToQualifiedID: "work#peer"}}
	peerUID, peerShort := "peer", "peer"
	at := time.Date(2026, 10, 8, 18, 5, 5, 0, time.UTC)
	events := []g.Event{
		{UID: "linked", Actor: "dana", Type: "issue.linked", Payload: `{"type":"blocks","to_short_id":"peer"}`, RelatedIssueUID: &peerUID, RelatedIssueShortID: &peerShort, CreatedAt: at},
		{UID: "closed", Actor: "dana", Type: "issue.closed", Payload: `{"reason":"done","message":"` + strings.TrimSpace(long) + `","evidence":[{"type":"commit","sha":"abc123def4"},{"type":"test","command":"go test ./..."}]}`, CreatedAt: at},
	}
	m := apply(model.Model{},
		model.Start{Workspace: "/repo"},
		model.ResizeMsg{Cols: 160},
		model.ProjectLoaded{Project: model.Project{UID: "p", Name: "work"}, Snapshot: g.UISnapshotResponseBody{Collection: []model.Issue{main, peer}, CollectionLinks: links}},
		model.DetailLoaded{UID: "main", Selected: &g.UISelectedAuthority{Issue: &main, Links: links, History: events}},
		model.ActivityTabMsg{Tab: tab},
	)
	return m, strings.TrimSpace(long)
}

// spanStyles is the style of each span of a node, keyed by span text.
func spanStyles(t *testing.T, n map[string]any) map[string]string {
	t.Helper()
	styles := map[string]string{}
	spans, _ := n["spans"].([]any)
	for _, span := range spans {
		s := span.(map[string]any)
		style, _ := s["s"].(string)
		styles[s["t"].(string)] = style
	}
	return styles
}

func assertReference(t *testing.T, nodes shown, id, short, uid string) {
	t.Helper()
	jump, ok := nodes[id]
	if !ok {
		t.Fatalf("no reference at %s: %v", id, nodes.under(view.DetailActivityID))
	}
	style := spanStyles(t, jump.P)[short]
	if !strings.Contains(style, "accent") || !strings.Contains(style, "link") {
		t.Errorf("reference %q style = %q, want accent and underlined link", short, style)
	}
	if click := jump.P["actions"].(map[string]any)["click"]; click != "jump="+uid {
		t.Errorf("reference click = %v", click)
	}
}

func TestLinksReferenceIsStyledAsALinkAndRelationIsMuted(t *testing.T) {
	m, _ := referencing(model.TabLinks)
	nodes := render(t, m)
	assertReference(t, nodes, view.DetailActivityID+".1.row.jump", "work#peer", "peer")
	if style := spanStyles(t, nodes[view.DetailActivityID+".1.row.heading"].P)["blocks"]; style != "muted" {
		t.Errorf("relation style = %q", style)
	}
	for id, n := range nodes {
		if strings.Contains(text(n), "Open referenced Issue") {
			t.Errorf("%s still has the button", id)
		}
	}
}

func TestEventsReferenceIsStyledAndPlainEventsAreNot(t *testing.T) {
	m, _ := referencing(model.TabEvents)
	nodes := render(t, m)
	assertReference(t, nodes, view.DetailActivityID+".linked.row.jump", "peer", "peer")
	if style := spanStyles(t, nodes[view.DetailActivityID+".linked.row.heading"].P)["dana"]; style != "muted" {
		t.Errorf("actor style = %q", style)
	}
	if _, ok := nodes[view.DetailActivityID+".closed.heading"]; !ok {
		t.Fatalf("no plain heading: %v", nodes.under(view.DetailActivityID))
	}
	for _, id := range nodes.under(view.DetailActivityID + ".closed") {
		if strings.HasSuffix(id, ".jump") {
			t.Errorf("event without a reference has %s", id)
		}
	}
	if click := nodes[view.DetailActivityID+".closed.heading"].P["actions"].(map[string]any)["click"]; strings.HasPrefix(click.(string), "jump=") {
		t.Errorf("plain row jumps: %v", click)
	}
}

func TestLongCloseMessageAndEvidenceRenderWhole(t *testing.T) {
	m, long := referencing(model.TabEvents)
	body := text(render(t, m)[view.DetailActivityID+".closed.body"])
	want := long + " (commit abc123d, test go test ./...)"
	if body != want {
		t.Errorf("close detail = %q, want %q", body, want)
	}
}
