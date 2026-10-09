package view_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/model"
	"github.com/Dangerboi/tern-kata/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
	g "go.kenn.io/kata/pkg/client/generated"
)

const childrenID = view.DetailContentID + ".children"

// A child row's id ends in its Link's id, never in the child's UID (that is the list row's suffix).
const (
	kidRow   = childrenID + ".3"
	doneRow  = childrenID + ".2"
	ghostRow = childrenID + ".1"
)

// parenting is a wide Pane on a parent "alpha" with three children: "kid" (open,
// updated 38 minutes before testNow), "done" (closed, 3 hours before) and "ghost",
// which is not in the Collection. The daemon counts 1 open of 3.
func parenting(tab model.ActivityTab) model.Model {
	parent := alpha
	parent.Body = "The parent's body."
	kid := model.Issue{UID: "kid", ShortID: "kidd", Title: "Open kid", Status: "open", ProjectName: "work", UpdatedAt: testNow.Add(-38 * time.Minute)}
	done := model.Issue{UID: "done", ShortID: "done", Title: "Closed kid", Status: "closed", ProjectName: "work", UpdatedAt: testNow.Add(-3 * time.Hour)}
	child := func(id int64, uid string) g.UILink {
		return g.UILink{ID: id, Type: "parent", FromIssueUID: uid, FromQualifiedID: "work#" + uid, ToIssueUID: "alpha", ToQualifiedID: "work#aaaa"}
	}
	links := []g.UILink{child(1, "ghost"), child(2, "done"), child(3, "kid")}
	return apply(model.Model{},
		model.Start{Workspace: "/repo"},
		model.ResizeMsg{Cols: 160},
		model.ProjectLoaded{Project: model.Project{UID: "p", Name: "work"}, ChildCounts: map[string]g.ChildCounts{"alpha": {Open: 1, Total: 3}},
			Snapshot: g.UISnapshotResponseBody{Collection: []model.Issue{parent, kid, done}, CollectionLinks: links}},
		model.DetailLoaded{UID: "alpha", Selected: &g.UISelectedAuthority{Issue: &parent, Links: links, Comments: []g.Comment{{UID: "c1", Body: "hi"}}}},
		model.ActivityTabMsg{Tab: tab},
	)
}

func TestPropertiesShowTheDaemonsChildCountsAndNoChildLines(t *testing.T) {
	nodes := render(t, parenting(model.TabLinks))
	if got := text(nodes[view.DetailContentID+".properties.children"]); got != "Children: 1 open / 3 total" {
		t.Errorf("counts line = %q", got)
	}
	if ids := nodes.under(view.DetailContentID + ".properties.relatives"); len(ids) != 0 {
		t.Errorf("properties still list relatives with no parent: %v", ids)
	}
}

func TestChildrenTableSitsBetweenTheBodyAndActivity(t *testing.T) {
	nodes := render(t, parenting(model.TabComments))
	var order []string
	for _, c := range nodes[view.DetailContentID].C {
		order = append(order, c.ID)
	}
	body, table, activity := slices.Index(order, view.DetailContentID+".body"), slices.Index(order, childrenID), slices.Index(order, view.DetailActivityID)
	if body < 0 || table <= body || activity <= table {
		t.Fatalf("content order = %v, want body, children, activity", order)
	}
	if got := text(nodes[childrenID+".heading"]); got != "Children" {
		t.Errorf("heading = %q", got)
	}
}

func TestChildRowsShowReferenceStatusTitleAndUpdatedInTheListsOrder(t *testing.T) {
	nodes := render(t, parenting(model.TabComments))
	var rows []string
	for _, c := range nodes[childrenID].C {
		if c.ID != childrenID+".heading" {
			rows = append(rows, c.ID)
		}
	}
	if want := []string{kidRow, doneRow, ghostRow}; !slices.Equal(rows, want) {
		t.Fatalf("rows = %v, want %v", rows, want)
	}
	for row, want := range map[string][4]string{
		kidRow:  {"#kid", "open", "Open kid", "38m ago"},
		doneRow: {"#done", "closed", "Closed kid", "3h ago"},
	} {
		for i, cell := range []string{"jump", "status", "title", "updated"} {
			if got := text(nodes[row+"."+cell]); got != want[i] {
				t.Errorf("%s %s = %q, want %q", row, cell, got, want[i])
			}
		}
	}
}

func TestEveryChildCellJumpsToTheChild(t *testing.T) {
	nodes := render(t, parenting(model.TabComments))
	for _, cell := range []string{"jump", "status", "title", "updated"} {
		n := nodes[kidRow+"."+cell]
		if click := n.P["actions"].(map[string]any)["click"]; click != "jump=kid" {
			t.Errorf("%s click = %v", cell, click)
		}
	}
	assertReference(t, nodes, kidRow+".jump", "#kid", "kid")
}

func TestClosedChildIsMutedAndOpenChildIsNot(t *testing.T) {
	nodes := render(t, parenting(model.TabComments))
	for _, tt := range []struct {
		row, cell, want string
	}{
		{doneRow, "status", "muted"}, {doneRow, "title", "muted"}, {doneRow, "updated", "muted"},
		{kidRow, "status", "accent"}, {kidRow, "title", ""}, {kidRow, "updated", "muted"},
	} {
		got := ""
		for _, style := range spanStyles(t, nodes[tt.row+"."+tt.cell].P) {
			got = style
		}
		if got != tt.want {
			t.Errorf("%s %s style = %q, want %q", tt.row, tt.cell, got, tt.want)
		}
	}
}

func TestUnloadedChildShowsItsReferenceOnly(t *testing.T) {
	nodes := render(t, parenting(model.TabComments))
	assertReference(t, nodes, ghostRow+".jump", "#ghost", "ghost")
	if ids := nodes.under(ghostRow); len(ids) != 2 { // the row and its reference
		t.Errorf("unloaded child shows %v", ids)
	}
}

func TestNoChildrenTableNorCountsLineWithoutChildren(t *testing.T) {
	nodes := render(t, browsing())
	for _, id := range []string{childrenID, view.DetailContentID + ".properties.children"} {
		if ids := nodes.under(id); len(ids) != 0 {
			t.Errorf("%s shown without children: %v", id, ids)
		}
	}
}

func TestChildRowUnderTheCursorIsMarkedAndTheActivityRowIsNotWhileItIsOnAChild(t *testing.T) {
	mark := func(n tern.WireNode) any { return n.P["mark"] }
	m := apply(parenting(model.TabComments), model.KeyMsg{Chord: "k"}, model.KeyMsg{Chord: "k"}) // from the comment up to the 2nd child; focus is the detail
	nodes := render(t, m)
	if mark(nodes[doneRow]) != "pick" || mark(nodes[kidRow]) != nil || mark(nodes[view.DetailActivityID+".c1"]) != nil {
		t.Errorf("marks: kid=%v done=%v comment=%v", mark(nodes[kidRow]), mark(nodes[doneRow]), mark(nodes[view.DetailActivityID+".c1"]))
	}
	m = apply(m, model.KeyMsg{Chord: "j"}, model.KeyMsg{Chord: "j"}) // down past the 3 children: the comment
	nodes = render(t, m)
	if mark(nodes[view.DetailActivityID+".c1"]) != "pick" || strings.Contains(strings.Join(markedIDs(nodes), ","), "children") {
		t.Errorf("marked rows = %v", markedIDs(nodes))
	}
}

func markedIDs(nodes shown) []string {
	var ids []string
	for id, n := range nodes {
		if n.P["mark"] == "pick" {
			ids = append(ids, id)
		}
	}
	return ids
}
