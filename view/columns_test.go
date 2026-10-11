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

// columnsFixture is a parent with one open and one closed child, plus a leaf
// with no owner, listed on a Project at 160 columns.
func columnsFixture(t testing.TB) model.Model {
	t.Helper()
	noon := testNow
	parent := model.Issue{UID: "parent", ShortID: "pppp", Title: "Parent", Status: "open", ProjectName: "work", Owner: ptr("dana"), UpdatedAt: noon.Add(-2 * time.Hour)}
	open := model.Issue{UID: "open", ShortID: "oooo", Title: "Open child", Status: "open", ProjectName: "work", UpdatedAt: noon}
	done := model.Issue{UID: "done", ShortID: "dddd", Title: "Done child", Status: "closed", ProjectName: "work", UpdatedAt: noon}
	leaf := model.Issue{UID: "leaf", ShortID: "llll", Title: "Leaf", Status: "open", ProjectName: "work", UpdatedAt: noon.Add(-3 * 24 * time.Hour)}
	links := []g.UILink{
		{Type: "parent", FromIssueUID: "open", ToIssueUID: "parent"},
		{Type: "parent", FromIssueUID: "done", ToIssueUID: "parent"},
	}
	return apply(model.Model{},
		model.Start{Workspace: "/repo"},
		model.ResizeMsg{Cols: 160},
		model.ProjectLoaded{Project: model.Project{UID: "p", Name: "work"}}, snap{model.SnapshotLoaded{Snapshot: g.UISnapshotResponseBody{Collection: []model.Issue{parent, open, done, leaf}, CollectionLinks: links}, ChildCounts: map[string]g.ChildCounts{"parent": {Open: 1, Total: 2}}}},
	)
}

func labels(n [][2]string) []string {
	var out []string
	for _, span := range n {
		out = append(out, strings.TrimSpace(span[0]))
	}
	return out
}

func TestHeaderRowLabelsTheColumns(t *testing.T) {
	m := columnsFixture(t)
	side := render(t, m)["main.columns.list.header"]
	if got, want := labels(styled(side)), []string{"#", "prio", "status", "title", "kids", "updated"}; !slices.Equal(got, want) {
		t.Errorf("side by side header %q, want %q", got, want)
	}
	m, _ = model.Update(m, model.KeyMsg{Chord: "shift+l"})
	full := render(t, m)["main.columns.list.header"]
	if got, want := labels(styled(full)), []string{"#", "prio", "status", "title", "kids", "owner", "updated"}; !slices.Equal(got, want) {
		t.Errorf("full width header %q, want %q", got, want)
	}
}

func TestRowShowsKidsOwnerAndUpdated(t *testing.T) {
	m := columnsFixture(t)
	m, _ = model.Update(m, model.KeyMsg{Chord: "shift+l"})
	nodes := render(t, m)
	parent, leaf := text(nodes[view.IssueID("parent")]), text(nodes[view.IssueID("leaf")])
	for _, want := range []string{"pppp", "Parent", "1/2", "dana", "2h ago"} {
		if !strings.Contains(parent, want) {
			t.Errorf("parent row %q lacks %q", parent, want)
		}
	}
	if strings.Contains(leaf, "/") || !strings.Contains(leaf, "3d ago") {
		t.Errorf("leaf row %q: want no kids and 3d ago", leaf)
	}
	if got := style(t, nodes[view.IssueID("parent")], "dana"); got != "muted" {
		t.Errorf("owner style = %q, want muted", got)
	}
}

func TestOwnerDroppedWhenSharingThePaneWithDetail(t *testing.T) {
	m := columnsFixture(t)
	if row := text(render(t, m)[view.IssueID("parent")]); strings.Contains(row, "dana") || !strings.Contains(row, "1/2") || !strings.Contains(row, "2h ago") {
		t.Errorf("side by side parent row %q: want kids and updated, no owner", row)
	}
}

// Only the Issue rows scroll: the list node is height-capped (Tern scrolls a
// capped list, and an uncapped one grows until the whole Pane scrolls), and
// the top bar and header are its siblings, not inside it.
func TestTopBarAndHeaderStayOutsideTheScrollingList(t *testing.T) {
	nodes := render(t, columnsFixture(t))
	list := nodes["main.columns.list.issues"]
	if list.P["max"] == nil {
		t.Errorf("Issue list has no max height, so it never scrolls on its own: %v", list.P)
	}
	inside := map[string]bool{}
	var walk func(n tern.WireNode)
	walk = func(n tern.WireNode) {
		inside[n.ID] = true
		for _, c := range n.C {
			walk(c)
		}
	}
	walk(list)
	for _, id := range []string{"main.topbar", "main.topbar.left.heading", "main.columns.list.header"} {
		if _, ok := nodes[id]; !ok || inside[id] {
			t.Errorf("%s present=%v inside the scrolling list=%v, want outside", id, ok, inside[id])
		}
	}
	if !inside[view.IssueID("leaf")] {
		t.Error("Issue rows are not inside the scrolling list")
	}
}
