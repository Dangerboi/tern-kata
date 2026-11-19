package view_test

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

// hovercardOf is the JSON of a node's hovercard, empty when it has none.
func hovercardOf(t *testing.T, nodes shown, id string) string {
	t.Helper()
	n, ok := nodes[id]
	if !ok {
		t.Fatalf("no node %s", id)
	}
	card, ok := n.P["hovercard"]
	if !ok {
		return ""
	}
	raw, err := json.Marshal(card)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func wantCard(t *testing.T, where, card string, want ...string) {
	t.Helper()
	if card == "" {
		t.Errorf("%s has no hovercard", where)
		return
	}
	for _, w := range want {
		if !strings.Contains(card, w) {
			t.Errorf("%s hovercard lacks %q: %s", where, w, card)
		}
	}
}

// relating is a wide Pane on Issue "main" with a loaded open blocker "peer" owned by dana until an hour after testNow, an
// unloaded blocker "ghost" whose Link says it is open, and the parent "root" (loaded).
func relating() model.Model {
	dana := "dana"
	main := model.Issue{UID: "main", ShortID: "main", QualifiedID: "work#main", Title: "Main", Status: "open", ProjectName: "work"}
	peer := model.Issue{UID: "peer", ShortID: "peer", QualifiedID: "#peer", Title: "Peer title", Status: "open", ProjectName: "work", Owner: &dana, AssignmentExpiresOn: ptr(testNow.Add(time.Hour)), UpdatedAt: testNow.Add(-38 * time.Minute)}
	root := model.Issue{UID: "root", ShortID: "root", QualifiedID: "#root", Title: "Root title", Status: "open", ProjectName: "work", UpdatedAt: testNow.Add(-3 * time.Hour)}
	links := []model.Link{
		{ID: 1, Type: "blocks", FromIssueUID: "peer", FromQualifiedID: "#peer", FromStatus: "open", ToIssueUID: "main", ToQualifiedID: "work#main", ToStatus: "open"},
		{ID: 2, Type: "blocks", FromIssueUID: "ghost", FromQualifiedID: "other#ghst", FromStatus: "open", ToIssueUID: "main", ToQualifiedID: "work#main", ToStatus: "open"},
		{ID: 3, Type: "parent", FromIssueUID: "main", FromQualifiedID: "work#main", FromStatus: "open", ToIssueUID: "root", ToQualifiedID: "#root", ToStatus: "open"},
	}
	return apply(model.Model{},
		model.Start{Workspace: "/repo"},
		model.ResizeMsg{Cols: 160},
		model.ProjectLoaded{Project: model.Project{UID: "p", Name: "work"}}, snap{model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: []model.Issue{main, peer, root}, Links: links}}},
		model.JumpIssueMsg{UID: "main"},
		model.DetailLoaded{UID: "main", Selected: &model.IssueDetailData{Issue: &main, Links: links}},
		model.ActivityTabMsg{Tab: model.TabLinks},
	)
}

func TestLinkAndParentReferencesCarryTheIssuesCard(t *testing.T) {
	nodes := render(t, relating())
	parent := hovercardOf(t, nodes, view.DetailContentID+".properties.facts.right.parent.v")
	wantCard(t, "parent property", parent, "#root", "Root title", "open", model.PaneCalendar{}.Read(testNow.Add(-3*time.Hour), testNow))
	if strings.Contains(parent, "owner") {
		t.Errorf("unowned parent shows an owner: %s", parent)
	}
	peer := hovercardOf(t, nodes, view.DetailActivityID+".1.row.jump")
	wantCard(t, "loaded link", peer, "#peer", "Peer title", "open", "dana until", model.PaneCalendar{}.Read(testNow.Add(-38*time.Minute), testNow))
	ghost := hovercardOf(t, nodes, view.DetailActivityID+".2.row.jump")
	wantCard(t, "unloaded link", ghost, "other#ghst", "open")
	for _, invented := range []string{"title", "owner", "updated"} {
		if strings.Contains(ghost, `"`+invented+`"`) || strings.Contains(ghost, invented+" ") {
			t.Errorf("unloaded card invents %q: %s", invented, ghost)
		}
	}
	if click := nodes[view.DetailActivityID+".1.row.jump"].P["actions"].(map[string]any)["click"]; click != view.JumpClick("peer") {
		t.Errorf("click = %v", click)
	}
}

func TestEventReferenceCarriesTheIssuesCard(t *testing.T) {
	m, _ := referencing(model.TabEvents)
	nodes := render(t, m)
	wantCard(t, "event reference", hovercardOf(t, nodes, view.DetailActivityID+".linked.row.jump"), "#peer", "Peer")
}

func TestBoardFactsRowAndBlockedMarkerCarryTheIssuesCard(t *testing.T) {
	m := apply(relating(), model.FocusColumnMsg{Column: model.ColumnList}, model.KeyMsg{Chord: "b"})
	nodes := render(t, m)
	var line, marker int
	for id, n := range nodes {
		if _, ok := n.P["hovercard"]; !ok {
			continue
		}
		switch {
		case strings.HasSuffix(id, ".blocked"):
			marker++
			wantCard(t, id, hovercardOf(t, nodes, id), "#peer", "Peer title", "other#ghst")
		case strings.HasSuffix(id, ".facts.parent.root"):
			line++
			wantCard(t, id, hovercardOf(t, nodes, id), "#root", "Root title")
		}
	}
	// Blockers are named by the marker alone; the facts row carries the parent.
	if line != 1 || marker != 1 {
		t.Errorf("relationship refs with a card = %d, blocked markers = %d; nodes %v", line, marker, nodes.under("main.columns.list.board"))
	}
}

func TestChildTicketCellsCarryTheCardsFactsAsATooltip(t *testing.T) {
	nodes := render(t, parenting(model.TabComments))
	title := func(row string) string {
		cell, _ := tableCell(t, nodes, row, "ticket").(map[string]any)
		s, _ := cell["title"].(string)
		return s
	}
	for _, want := range []string{"#kidd", "open", "Open kid", "dana until"} {
		if !strings.Contains(title("kid"), want) {
			t.Errorf("loaded child tooltip %q lacks %q", title("kid"), want)
		}
	}
	if got := title("ghost"); got != "#ghost · closed" {
		t.Errorf("unloaded child tooltip = %q, want its qualified ref and status only", got)
	}
}

// commenting is relating's Pane on the Comments tab with the cursor on a comment that refutes a comment in the loaded Issue
// "peer" and is confirmed by one in the unloaded Issue "ghost".
func commenting() model.Model {
	main := model.Issue{UID: "main", ShortID: "main", QualifiedID: "work#main", Title: "Main", Status: "open", ProjectName: "work"}
	peer := model.Issue{UID: "peer", ShortID: "peer", QualifiedID: "#peer", Title: "Peer title", Status: "open", ProjectName: "work"}
	comment := model.Comment{UID: "c1", Author: "dana", CreatedAt: testNow.Add(-time.Hour), Body: "claim", ReplyKind: ptr("refute"),
		Reply:     &model.CommentLink{UID: ptr("p1"), Kind: "refute", Author: ptr("eli"), IssueUID: ptr("peer"), IssueShortID: ptr("peer")},
		Backlinks: []model.CommentLink{{UID: ptr("g1"), Kind: "confirm", Author: ptr("fay"), IssueUID: ptr("ghost"), IssueShortID: ptr("ghst")}}}
	return apply(model.Model{},
		model.Start{Workspace: "/repo"},
		model.ResizeMsg{Cols: 160},
		model.ProjectLoaded{Project: model.Project{UID: "p", Name: "work"}}, snap{model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: []model.Issue{main, peer}}}},
		model.JumpIssueMsg{UID: "main"},
		model.DetailLoaded{UID: "main", Selected: &model.IssueDetailData{Issue: &main, Comments: []model.Comment{comment}}},
		model.ActivityTabMsg{Tab: model.TabComments},
	)
}

func TestCommentLinksCarryTheIssuesCard(t *testing.T) {
	m := commenting()
	nodes := render(t, m)
	wantCard(t, "comment head", hovercardOf(t, nodes, view.DetailActivityID+".c1.head"), "#peer", "Peer title")
	wantCard(t, "chosen link", hovercardOf(t, nodes, view.DetailActivityID+".c1.link"), "#peer", "Peer title")
	nodes = render(t, apply(m, model.KeyMsg{Chord: "]"}))
	wantCard(t, "chosen link to an unloaded Issue", hovercardOf(t, nodes, view.DetailActivityID+".c1.link"), "#ghst")
}

func TestMachineStringsAreMono(t *testing.T) {
	spanStyles := func(nodes shown, id string) map[string]string {
		styles := map[string]string{}
		for _, span := range cellSpans(map[string]any{"text": nodes[id].P["spans"]}) {
			styles[span["t"].(string)], _ = span["s"].(string)
		}
		return styles
	}
	nodes := render(t, commenting())
	head := spanStyles(nodes, view.DetailActivityID+".c1.head")
	if !slices.Contains(slices.Collect(maps.Values(head)), "muted mono") {
		t.Errorf("no comment head span is muted mono (the time): %v", head)
	}
	m, _ := referencing(model.TabEvents)
	nodes = render(t, m)
	if got := spanStyles(nodes, view.DetailActivityID+".linked.row.jump")["peer"]; got != "accent link mono" {
		t.Errorf("event reference style = %q", got)
	}
	for _, id := range []string{".linked.row.time"} {
		var muted []string
		for text, style := range spanStyles(nodes, view.DetailActivityID+id) {
			if style == "muted mono" {
				muted = append(muted, text)
			}
		}
		if len(muted) != 1 {
			t.Errorf("event heading has %v in muted mono, want its time alone", muted)
		}
	}
	nodes = render(t, relating())
	if got := spanStyles(nodes, view.DetailActivityID+".1.row.jump")["#peer"]; got != "accent link mono" {
		t.Errorf("link reference style = %q", got)
	}
}
