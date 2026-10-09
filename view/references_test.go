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
	assertReference(t, nodes, view.DetailActivityID+".1.row.jump", "#peer", "peer")
	if style := spanStyles(t, nodes[view.DetailActivityID+".1.row.heading"].P)["blocks | "]; style != "muted" {
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

func TestPropertiesShowParentAndChildrenAsReferences(t *testing.T) {
	links := []g.UILink{
		{ID: 1, Type: "parent", FromIssueUID: "alpha", FromQualifiedID: "work#aaaa", ToIssueUID: "up", ToQualifiedID: "work#up"},
		{ID: 2, Type: "parent", FromIssueUID: "kid", FromQualifiedID: "work#kid", ToIssueUID: "alpha", ToQualifiedID: "work#aaaa"},
		{ID: 3, Type: "blocks", FromIssueUID: "alpha", FromQualifiedID: "work#aaaa", ToIssueUID: "other", ToQualifiedID: "work#other"},
	}
	m := apply(browsing(), model.DetailLoaded{UID: "alpha", Selected: &g.UISelectedAuthority{Issue: &alpha, Links: links}})
	nodes := render(t, m)
	relatives := view.DetailContentID + ".properties.relatives"
	assertReference(t, nodes, relatives+".1.row.jump", "#up", "up")
	assertReference(t, nodes, relatives+".2.row.jump", "#kid", "kid")
	if got := text(nodes[relatives+".1.row.heading"]); got != "parent | " {
		t.Errorf("parent label %q", got)
	}
	if got := text(nodes[relatives+".2.row.heading"]); got != "child | " {
		t.Errorf("child label %q", got)
	}
	if ids := nodes.under(relatives + ".3"); len(ids) != 0 {
		t.Errorf("a blocking link is in properties: %v", ids)
	}
}

func TestStatusBadgeIsShownOnceAfterTheTitle(t *testing.T) {
	nodes := render(t, apply(browsing(), detailOf(alpha)))
	if _, ok := nodes[view.DetailContentID+".properties.badges.status"]; ok {
		t.Error("properties repeat the status badge")
	}
	if _, ok := nodes[view.DetailContentID+".heading.status"]; !ok {
		t.Error("no status badge after the title")
	}
}

func TestLinksRowShowsThePeersTitleWhenLoaded(t *testing.T) {
	m, _ := referencing(model.TabLinks)
	nodes := render(t, m)
	if got := text(nodes[view.DetailActivityID+".1.row.title"]); got != "Peer" {
		t.Errorf("title after the reference = %q, want Peer", got)
	}

	ghost := g.UILink{ID: 2, Type: "related", FromIssueUID: "main", FromQualifiedID: "work#main", ToIssueUID: "ghost", ToQualifiedID: "work#ghost"}
	main := model.Issue{UID: "main", ShortID: "main", Title: "Main", Status: "open", ProjectName: "work"}
	nodes = render(t, apply(m, model.DetailLoaded{UID: "main", Selected: &g.UISelectedAuthority{Issue: &main, Links: []g.UILink{ghost}}}))
	assertReference(t, nodes, view.DetailActivityID+".2.row.jump", "#ghost", "ghost")
	if _, ok := nodes[view.DetailActivityID+".2.row.title"]; ok {
		t.Error("a peer that is not loaded has a title")
	}
}

func TestLinkRowReadsRelationReferenceDashTitle(t *testing.T) {
	m, _ := referencing(model.TabLinks)
	nodes := render(t, m)
	row := view.DetailActivityID + ".1.row"
	if got := text(nodes[row+".heading"]) + text(nodes[row+".jump"]) + text(nodes[row+".dash"]) + text(nodes[row+".title"]); got != "blocks | #peer - Peer" {
		t.Errorf("same-Project row = %q", got)
	}
	for _, id := range []string{row + ".dash", row + ".title"} {
		if click := nodes[id].P["actions"].(map[string]any)["click"]; click != "detail-item=2:1" {
			t.Errorf("%s click = %v", id, click)
		}
	}

	other := g.UILink{ID: 2, Type: "related", FromIssueUID: "main", FromQualifiedID: "work#main", ToIssueUID: "peer", ToQualifiedID: "elsewhere#peer"}
	main := model.Issue{UID: "main", ShortID: "main", Title: "Main", Status: "open", ProjectName: "work"}
	nodes = render(t, apply(m, model.DetailLoaded{UID: "main", Selected: &g.UISelectedAuthority{Issue: &main, Links: []g.UILink{other}}}))
	assertReference(t, nodes, view.DetailActivityID+".2.row.jump", "elsewhere#peer", "peer")
}

func TestPropertiesTitleJumpsToTheReferencedIssue(t *testing.T) {
	links := []g.UILink{{ID: 1, Type: "parent", FromIssueUID: "alpha", FromQualifiedID: "work#aaaa", ToIssueUID: "up", ToQualifiedID: "work#up"}}
	up := model.Issue{UID: "up", ShortID: "up", Title: "Up", ProjectName: "work"}
	m := apply(browsing(), model.ProjectLoaded{Project: model.Project{UID: "p", Name: "work"}, Snapshot: g.UISnapshotResponseBody{Collection: []model.Issue{alpha, up}}}, model.DetailLoaded{UID: "alpha", Selected: &g.UISelectedAuthority{Issue: &alpha, Links: links}})
	nodes := render(t, m)
	title := nodes[view.DetailContentID+".properties.relatives.1.row.title"]
	if text(title) != "Up" || title.P["actions"].(map[string]any)["click"] != "jump=up" {
		t.Errorf("title = %v", title)
	}
}
