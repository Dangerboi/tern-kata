package view_test

import (
	"encoding/json"
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
	peer := model.Issue{UID: "peer", ShortID: "peer", QualifiedID: "work#peer", Title: "Peer title", Status: "open", ProjectName: "work", Owner: &dana, AssignmentExpiresOn: ptr(testNow.Add(time.Hour)), UpdatedAt: testNow.Add(-38 * time.Minute)}
	root := model.Issue{UID: "root", ShortID: "root", QualifiedID: "work#root", Title: "Root title", Status: "open", ProjectName: "work", UpdatedAt: testNow.Add(-3 * time.Hour)}
	links := []model.Link{
		{ID: 1, Type: "blocks", FromIssueUID: "peer", FromQualifiedID: "work#peer", FromStatus: "open", ToIssueUID: "main", ToQualifiedID: "work#main", ToStatus: "open"},
		{ID: 2, Type: "blocks", FromIssueUID: "ghost", FromQualifiedID: "other#ghst", FromStatus: "open", ToIssueUID: "main", ToQualifiedID: "work#main", ToStatus: "open"},
		{ID: 3, Type: "parent", FromIssueUID: "main", FromQualifiedID: "work#main", FromStatus: "open", ToIssueUID: "root", ToQualifiedID: "work#root", ToStatus: "open"},
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
	parent := hovercardOf(t, nodes, view.DetailContentID+".properties.parent.v")
	wantCard(t, "parent property", parent, "work#root", "Root title", "open", "updated", "3h ago")
	if strings.Contains(parent, "owner") {
		t.Errorf("unowned parent shows an owner: %s", parent)
	}
	peer := hovercardOf(t, nodes, view.DetailActivityID+".1.row.jump")
	wantCard(t, "loaded link", peer, "work#peer", "Peer title", "open", "dana until", "38m ago")
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
	wantCard(t, "event reference", hovercardOf(t, nodes, view.DetailActivityID+".linked.row.jump"), "work#peer", "Peer")
}

func TestBoardRelationshipsLineAndBlockedMarkerCarryTheIssuesCard(t *testing.T) {
	m := apply(relating(), model.FocusColumnMsg{Column: model.ColumnList}, model.KeyMsg{Chord: "b"})
	nodes := render(t, m)
	var line, marker int
	for id, n := range nodes {
		if _, ok := n.P["hovercard"]; !ok {
			continue
		}
		switch {
		case strings.HasSuffix(id, ".relationships.blocked-by.peer"):
			line++
			wantCard(t, id, hovercardOf(t, nodes, id), "work#peer", "Peer title", "dana until")
		case strings.HasSuffix(id, ".relationships.blocked-by.ghost"):
			line++
			wantCard(t, id, hovercardOf(t, nodes, id), "other#ghst", "open")
		case strings.HasSuffix(id, ".blocked"):
			marker++
			wantCard(t, id, hovercardOf(t, nodes, id), "work#peer", "Peer title", "other#ghst")
		case strings.HasSuffix(id, ".relationships.parent.root"):
			line++
			wantCard(t, id, hovercardOf(t, nodes, id), "work#root", "Root title")
		}
	}
	if line != 3 || marker != 1 {
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
	for _, want := range []string{"work#kid", "open", "Open kid", "owner dana until", "updated 38m ago"} {
		if !strings.Contains(title("kid"), want) {
			t.Errorf("loaded child tooltip %q lacks %q", title("kid"), want)
		}
	}
	if got := title("ghost"); got != "work#ghost - closed" {
		t.Errorf("unloaded child tooltip = %q, want its qualified ref and status only", got)
	}
}
