package view_test

import (
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
	g "go.kenn.io/kata/pkg/client/generated"
)

// drawnRows are the Issue rows of the list in the order they are drawn.
func drawnRows(nodes shown) []tern.WireNode {
	var rows []tern.WireNode
	for _, n := range nodes[view.IssueListID].C {
		if !strings.Contains(n.ID, ".group.") {
			rows = append(rows, n)
		}
	}
	return rows
}

// striped is which drawn rows carry the shade role, top to bottom.
func striped(nodes shown) []bool {
	var out []bool
	for _, row := range drawnRows(nodes) {
		role, _ := row.P["role"].(string)
		out = append(out, role == view.StripeRole)
	}
	return out
}

func alternates(t testing.TB, nodes shown, rows int) {
	t.Helper()
	got := striped(nodes)
	if len(got) != rows {
		t.Fatalf("%d rows drawn, want %d", len(got), rows)
	}
	for i, shaded := range got {
		if shaded != (i%2 == 1) {
			t.Errorf("row %d shaded = %v, want alternating from an unshaded first row: %v", i, shaded, got)
			return
		}
	}
}

func stripedProject() model.Model {
	issues := []model.Issue{
		{UID: "p", ShortID: "pppp", Title: "Parent", Status: "open", ProjectName: "work", Labels: []string{"label-in-row"}},
		{UID: "c1", ShortID: "cc01", Title: "Child one", Status: "open", ProjectName: "work"},
		{UID: "c2", ShortID: "cc02", Title: "Child two", Status: "open", ProjectName: "work"},
		{UID: "x", ShortID: "xxxx", Title: "Open", Status: "open", ProjectName: "work"},
		{UID: "y", ShortID: "yyyy", Title: "Closed", Status: "closed", ProjectName: "work"},
		{UID: "z", ShortID: "zzzz", Title: "Last", Status: "open", ProjectName: "work"},
	}
	links := []model.Link{
		{ID: 1, Type: "parent", FromIssueUID: "c1", FromQualifiedID: "work#cc01", ToIssueUID: "p", ToQualifiedID: "work#pppp"},
		{ID: 2, Type: "parent", FromIssueUID: "c2", FromQualifiedID: "work#cc02", ToIssueUID: "p", ToQualifiedID: "work#pppp"},
	}
	return apply(model.Model{},
		model.Start{Workspace: "/repo"},
		model.ResizeMsg{Cols: 160},
		model.ProjectLoaded{Project: model.Project{UID: "p", Name: "work"}}, snap{model.SnapshotLoaded{Snapshot: g.UISnapshotResponseBody{Collection: issues, CollectionLinks: collectionLinks(links)}}},
	)
}

func TestEveryOtherDrawnRowIsShadedAndStaysSoAfterFoldAndFilter(t *testing.T) {
	m := stripedProject()
	nodes := render(t, m)
	rows := drawnRows(nodes)
	alternates(t, nodes, len(rows))
	if len(rows) < 4 {
		t.Fatalf("fixture: %d rows drawn", len(rows))
	}

	m = apply(m, model.KeyMsg{Chord: "space"}) // unfold the selected parent
	unfolded := render(t, m)
	if got := len(drawnRows(unfolded)); got != len(rows)+2 {
		t.Fatalf("fixture: unfolding left %d rows, want %d", got, len(rows)+2)
	}
	alternates(t, unfolded, len(rows)+2)

	m, cmds := model.Update(m, model.KeyMsg{Chord: "s"}) // status=open: the closed Issue goes
	refresh := cmds[0].(model.ReadSnapshot)
	var open []model.Issue
	for _, issue := range m.Issues {
		if issue.Status == "open" {
			open = append(open, issue)
		}
	}
	m = apply(m, model.SnapshotLoaded{Request: refresh.Request, Query: refresh.Query, Snapshot: g.UISnapshotResponseBody{Collection: open, CollectionLinks: m.Snapshot.CollectionLinks}})
	filtered := render(t, m)
	if got := len(drawnRows(filtered)); got != len(rows)+1 {
		t.Fatalf("fixture: the filter left %d rows, want %d", got, len(rows)+1)
	}
	alternates(t, filtered, len(rows)+1)
}

func TestHeaderIsNotShadedAndRowsShowNoLabels(t *testing.T) {
	m := stripedProject()
	nodes := render(t, m)
	if role := nodes["main.columns.list.header"].P["role"]; role != nil {
		t.Errorf("header carries role %v", role)
	}
	for _, row := range drawnRows(nodes) {
		if strings.Contains(text(row), "label-in-row") {
			t.Errorf("row shows a label: %q", text(row))
		}
	}
}

func TestListMarksOnlyTheSelectedIssueRow(t *testing.T) {
	m := stripedProject()
	nodes := render(t, m)
	list := nodes[view.IssueListID]
	want := view.IssueID(m.SelectedUID)
	if got, _ := list.P["selected"].(string); got != want {
		t.Errorf("list selected = %q, want %q", got, want)
	}
}
