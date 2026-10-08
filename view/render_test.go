package view_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/model"
	"github.com/Dangerboi/tern-kata/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
	g "go.kenn.io/kata/pkg/client/generated"
)

// shown is every node of a rendered view by the id Tern derives from keys.
// Each WireNode keeps its props (P) and ordered children (C).
type shown map[string]tern.WireNode

// render runs view.Render on m. It goes through the SDK's reconciler, so ids
// are the ones Tern sees and a duplicate sibling key fails the test.
func render(t testing.TB, m model.Model) shown {
	t.Helper()
	ops, err := tern.NewReconciler("test").Diff(view.Render(m))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	nodes := shown{}
	for _, op := range ops {
		if op[0] == "add" {
			nodes.add(op[4].(tern.WireNode))
		}
	}
	return nodes
}

func (s shown) add(n tern.WireNode) {
	s[n.ID] = n
	for _, child := range n.C {
		s.add(child)
	}
}

// under lists, sorted, the ids of id and everything below it.
func (s shown) under(id string) []string {
	var ids []string
	for other := range s {
		if other == id || strings.HasPrefix(other, id+".") {
			ids = append(ids, other)
		}
	}
	slices.Sort(ids)
	return ids
}

// sameIDs reports each id below root that one rendering has and the other lacks.
func sameIDs(t testing.TB, root string, want, got shown) {
	t.Helper()
	for _, id := range want.under(root) {
		if _, ok := got[id]; !ok {
			t.Errorf("%s is gone", id)
		}
	}
	for _, id := range got.under(root) {
		if _, ok := want[id]; !ok {
			t.Errorf("%s is new", id)
		}
	}
}

// text is what a node shows: its text, or its label or spans joined.
func text(n tern.WireNode) string {
	for _, prop := range []string{"text", "label", "spans"} {
		switch v := n.P[prop].(type) {
		case string:
			return v
		case []any:
			var b strings.Builder
			for _, span := range v {
				b.WriteString(span.(map[string]any)["t"].(string))
			}
			return b.String()
		}
	}
	return ""
}

// apply feeds msgs through model.Update, the way the app does.
func apply(m model.Model, msgs ...model.Msg) model.Model {
	for _, msg := range msgs {
		m, _ = model.Update(m, msg)
	}
	return m
}

var (
	alpha = model.Issue{UID: "alpha", ShortID: "aaaa", Title: "Alpha", Status: "open", ProjectName: "work"}
	beta  = model.Issue{UID: "beta", ShortID: "bbbb", Title: "Beta", Status: "open", ProjectName: "work"}
)

func detailOf(issue model.Issue) model.Msg {
	return model.DetailLoaded{UID: issue.UID, Selected: &g.UISelectedAuthority{Issue: &issue}}
}

// browsing is a wide Pane on a Project of alpha and beta, alpha's detail loaded.
func browsing() model.Model {
	return apply(model.Model{},
		model.Start{Workspace: "/repo"},
		model.ResizeMsg{Cols: 160},
		model.ProjectLoaded{Project: model.Project{UID: "p", Name: "work"}, Snapshot: g.UISnapshotResponseBody{Collection: []model.Issue{alpha, beta}}},
		detailOf(alpha),
	)
}

func TestFirstLoadShowsFullPaneMessage(t *testing.T) {
	nodes := render(t, apply(model.Model{}, model.Start{Workspace: "/repo"}))
	if _, ok := nodes["main.loading"]; !ok || len(nodes.under("main.columns")) != 0 {
		t.Fatalf("first load: %v", nodes.under("main"))
	}
}

func TestSwitchingCollectionKeepsListOnScreen(t *testing.T) {
	m := browsing()
	idle := render(t, m)
	allOpen := m.Collections[4]

	m = apply(m, model.CollectionSelected{Collection: allOpen})
	loading := render(t, m)
	if _, ok := loading["main.loading"]; ok {
		t.Fatal("full-Pane loading message replaced the columns")
	}
	sameIDs(t, "main.columns", idle, loading)
	if heading := text(loading["main.columns.list.heading"]); !strings.HasPrefix(heading, allOpen.Title) || heading == allOpen.Title {
		t.Errorf("list heading shows no loading marker: %q", heading)
	}

	m = apply(m, model.CollectionLoaded{Collection: allOpen, Snapshot: g.UISnapshotResponseBody{Collection: []model.Issue{alpha}}})
	loaded := render(t, m)
	if _, ok := loaded[view.IssueID(alpha.UID)]; !ok {
		t.Errorf("alpha's row changed id across loading: %v", loaded.under(view.IssueListID))
	}
	if heading := text(loaded["main.columns.list.heading"]); heading != allOpen.Title {
		t.Errorf("list heading keeps the loading marker: %q", heading)
	}
}

func TestSwitchingIssueReplacesDetailInPlace(t *testing.T) {
	m := browsing()
	idle := render(t, m)

	m = apply(m, model.SelectMsg{UID: beta.UID})
	loading := render(t, m)
	sameIDs(t, view.DetailID, idle, loading)
	title := text(loading[view.DetailID+".title"])
	if !strings.Contains(title, alpha.Title) || title == text(idle[view.DetailID+".title"]) {
		t.Errorf("detail heading keeps the previous Issue with a loading marker: %q", title)
	}

	m = apply(m, detailOf(beta))
	loaded := render(t, m)
	sameIDs(t, view.DetailID, idle, loaded)
	if title := text(loaded[view.DetailID+".title"]); !strings.Contains(title, beta.Title) || strings.Contains(title, alpha.Title) {
		t.Errorf("detail did not switch to the new Issue: %q", title)
	}
}

func TestSearchKeepsItsCaretWhileACollectionLoads(t *testing.T) {
	m := apply(browsing(), model.KeyMsg{Chord: "/"})
	if view.InputFocusID(m) != view.SearchID {
		t.Fatalf("fixture: search is not focused: %q", view.InputFocusID(m))
	}
	m = apply(m, model.CollectionSelected{Collection: m.Collections[4]})
	if !m.Loading || view.InputFocusID(m) != view.SearchID {
		t.Errorf("caret left the search box while loading: loading=%v focus=%q", m.Loading, view.InputFocusID(m))
	}
}

func TestStaleNoticeStaysWhileACollectionLoads(t *testing.T) {
	m := browsing()
	m = apply(m, model.StreamDropped{Generation: m.StreamGeneration, Err: errors.New("kata server not responding")})
	idle := render(t, m)
	loading := render(t, apply(m, model.CollectionSelected{Collection: m.Collections[4]}))
	if _, ok := idle["main.stale"]; !ok {
		t.Fatalf("fixture: no stale notice: %v", idle.under("main"))
	}
	sameIDs(t, "main", idle, loading)
}
