package view_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// referencing is a wide Pane on Issue "main" with one Link to "peer", a linked
// event naming "peer", a long close message with evidence, and a plain event.
func referencing(tab model.ActivityTab) (model.Model, string) {
	long := strings.Repeat("a long close message ", 30)
	main := model.Issue{UID: "main", ShortID: "main", Title: "Main", Status: "open", ProjectName: "work"}
	peer := model.Issue{UID: "peer", ShortID: "peer", Title: "Peer", Status: "open", ProjectName: "work"}
	links := []model.Link{{ID: 1, Type: "blocks", FromIssueUID: "main", FromQualifiedID: "work#main", ToIssueUID: "peer", ToQualifiedID: "work#peer"}}
	peerUID, peerShort := "peer", "peer"
	at := time.Date(2026, 10, 8, 18, 5, 5, 0, time.UTC)
	events := []model.Event{
		{UID: "linked", Actor: "dana", Type: "issue.linked", Payload: `{"type":"blocks","to_short_id":"peer"}`, RelatedIssueUID: &peerUID, RelatedIssueShortID: &peerShort, CreatedAt: at},
		{UID: "closed", Actor: "dana", Type: "issue.closed", Payload: `{"reason":"done","message":"` + strings.TrimSpace(long) + `","evidence":[{"type":"commit","sha":"abc123def4"},{"type":"test","command":"go test ./..."}]}`, CreatedAt: at},
	}
	m := apply(model.Model{},
		model.Start{Workspace: "/repo"},
		model.ResizeMsg{Cols: 160},
		model.ProjectLoaded{Project: model.Project{UID: "p", Name: "work"}}, snap{model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: []model.Issue{main, peer}, Links: links}}},
		model.DetailLoaded{UID: "main", Selected: &model.IssueDetailData{Issue: &main, Links: links, History: events}},
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
	if click := jump.P["actions"].(map[string]any)["click"]; click != view.JumpClick(uid) {
		t.Errorf("reference click = %v", click)
	}
}

func TestLinksReferenceIsStyledAsALinkAndRelationIsMuted(t *testing.T) {
	m, _ := referencing(model.TabLinks)
	nodes := render(t, m)
	assertReference(t, nodes, view.DetailActivityID+".1.row.jump", "#peer", "peer")
	styles := spanStyles(t, nodes[view.DetailActivityID+".1.row.heading"].P)
	if len(styles) != 1 {
		t.Fatalf("relation spans = %v, want one", styles)
	}
	for _, style := range styles {
		if style != "muted" {
			t.Errorf("relation style = %q", style)
		}
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
	if _, ok := nodes[view.DetailActivityID+".closed.heading"]; !ok {
		t.Fatalf("no plain heading: %v", nodes.under(view.DetailActivityID))
	}
	for _, id := range nodes.under(view.DetailActivityID + ".closed") {
		if strings.HasSuffix(id, ".jump") {
			t.Errorf("event without a reference has %s", id)
		}
	}
	if click := nodes[view.DetailActivityID+".closed.heading"].P["actions"].(map[string]any)["click"]; strings.HasPrefix(click.(string), view.JumpClick("")) {
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

func TestPropertiesShowTheParentAsAReferenceAndNoChildLines(t *testing.T) {
	links := []model.Link{
		{ID: 1, Type: "parent", FromIssueUID: "alpha", FromQualifiedID: "work#aaaa", ToIssueUID: "up", ToQualifiedID: "work#up"},
		{ID: 2, Type: "parent", FromIssueUID: "kid", FromQualifiedID: "work#kid", ToIssueUID: "alpha", ToQualifiedID: "work#aaaa"},
		{ID: 3, Type: "blocks", FromIssueUID: "alpha", FromQualifiedID: "work#aaaa", ToIssueUID: "other", ToQualifiedID: "work#other"},
	}
	m := apply(browsing(), model.DetailLoaded{UID: "alpha", Selected: &model.IssueDetailData{Issue: &alpha, Links: links}})
	nodes := render(t, m)
	parent := view.DetailContentID + ".properties.parent"
	assertReference(t, nodes, parent+".v", "#up", "up")
	if got := text(nodes[parent+".v"]); got != "#up" {
		t.Errorf("parent value draws %q", got)
	}
	if got := strings.TrimRight(text(nodes[parent+".k"]), "\u00a0"); got != "Parent:" {
		t.Errorf("parent key %q", got)
	}
	for _, other := range []string{"kid", "other", "blocks"} {
		if _, ok := nodes[view.DetailContentID+".properties."+other]; ok {
			t.Errorf("properties has a %s row", other)
		}
	}
	if got := text(nodes[view.DetailContentID+".properties.children.v"]); got != "0 open / 1 total" {
		t.Errorf("counts line = %q", got)
	}
}

func TestStatusBadgeIsShownOnceUnderTheTitle(t *testing.T) {
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
	if got := text(nodes[view.DetailActivityID+".1.row.jump"]); got != "#peer - Peer" {
		t.Errorf("reference and title = %q, want %q", got, "#peer - Peer")
	}

	ghost := model.Link{ID: 2, Type: "related", FromIssueUID: "main", FromQualifiedID: "work#main", ToIssueUID: "ghost", ToQualifiedID: "work#ghost"}
	main := model.Issue{UID: "main", ShortID: "main", Title: "Main", Status: "open", ProjectName: "work"}
	nodes = render(t, apply(m, model.DetailLoaded{UID: "main", Selected: &model.IssueDetailData{Issue: &main, Links: []model.Link{ghost}}}))
	assertReference(t, nodes, view.DetailActivityID+".2.row.jump", "#ghost", "ghost")
	if got := text(nodes[view.DetailActivityID+".2.row.jump"]); got != "#ghost" {
		t.Errorf("a peer that is not loaded draws %q", got)
	}
}

// drawn is a row's text after Tern's layout: each child loses its edge spaces and tabs (a no-break space survives),
// and the children join without a gap.
func drawn(row tern.WireNode) string {
	var b strings.Builder
	for _, child := range row.C {
		b.WriteString(strings.Trim(text(child), " \t\n"))
	}
	return strings.ReplaceAll(b.String(), "\u00a0", " ")
}

func TestLinkRowReadsRelationReferenceDashTitle(t *testing.T) {
	m, _ := referencing(model.TabLinks)
	nodes := render(t, m)
	row := view.DetailActivityID + ".1.row"
	if got := drawn(nodes[row]); got != "blocks | #peer - Peer" {
		t.Errorf("same-Project row draws %q", got)
	}

	main := model.Issue{UID: "main", ShortID: "main", Title: "Main", Status: "open", ProjectName: "work"}
	other := model.Link{ID: 2, Type: "related", FromIssueUID: "main", FromQualifiedID: "work#main", ToIssueUID: "peer", ToQualifiedID: "elsewhere#peer"}
	nodes = render(t, apply(m, model.DetailLoaded{UID: "main", Selected: &model.IssueDetailData{Issue: &main, Links: []model.Link{other}}}))
	assertReference(t, nodes, view.DetailActivityID+".2.row.jump", "elsewhere#peer", "peer")
	if got := drawn(nodes[view.DetailActivityID+".2.row"]); got != "related | elsewhere#peer - Peer" {
		t.Errorf("cross-Project row draws %q", got)
	}

	ghost := model.Link{ID: 3, Type: "related", FromIssueUID: "main", FromQualifiedID: "work#main", ToIssueUID: "ghost", ToQualifiedID: "work#ghost"}
	nodes = render(t, apply(m, model.DetailLoaded{UID: "main", Selected: &model.IssueDetailData{Issue: &main, Links: []model.Link{ghost}}}))
	if got := drawn(nodes[view.DetailActivityID+".3.row"]); got != "related | #ghost" {
		t.Errorf("row without a title draws %q", got)
	}
}

func TestPropertiesTitleJumpsToTheReferencedIssue(t *testing.T) {
	links := []model.Link{{ID: 1, Type: "parent", FromIssueUID: "alpha", FromQualifiedID: "work#aaaa", ToIssueUID: "up", ToQualifiedID: "work#up"}}
	up := model.Issue{UID: "up", ShortID: "up", Title: "Up", ProjectName: "work"}
	m := apply(browsing(), model.ProjectLoaded{Project: model.Project{UID: "p", Name: "work"}}, snap{model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: []model.Issue{alpha, up}}}}, model.DetailLoaded{UID: "alpha", Selected: &model.IssueDetailData{Issue: &alpha, Links: links}})
	nodes := render(t, m)
	value := nodes[view.DetailContentID+".properties.parent.v"]
	if text(value) != "#up - Up" || value.P["actions"].(map[string]any)["click"] != view.JumpClick("up") {
		t.Errorf("parent value = %v", value)
	}
}
