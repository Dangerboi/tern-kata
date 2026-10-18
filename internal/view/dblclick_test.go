package view_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

// Every Issue row, a parent's and a nested child's included, names its own UID in the double-click action.
// A parent's single click stays the fold, and a leaf's stays Tern's select, so nothing else changes.
func TestEveryIssueRowCarriesTheDoubleClickAction(t *testing.T) {
	issues := []model.Issue{
		{UID: "parent", ShortID: "prnt", Title: "Parent", Status: "open", ProjectName: "work"},
		{UID: "child", ShortID: "chld", Title: "Child", Status: "open", ProjectName: "work"},
		{UID: "leaf", ShortID: "leaf", Title: "Leaf", Status: "closed", ProjectName: "work"},
	}
	links := []model.Link{{Type: "parent", FromIssueUID: "child", ToIssueUID: "parent"}}
	m := apply(model.Model{},
		model.Start{Workspace: "/repo"},
		model.ResizeMsg{Cols: 160},
		model.ProjectLoaded{Project: model.Project{UID: "p", Name: "work"}}, snap{model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: issues, Links: links}}},
		model.KeyMsg{Chord: "shift+e"},
	)
	nodes := render(t, m)
	for _, issue := range issues {
		row, ok := nodes[view.IssueID(issue.UID)]
		if !ok {
			t.Fatalf("no row for %s: %v", issue.UID, nodes.under(view.IssueListID))
		}
		actions, _ := row.P["actions"].(map[string]any)
		if got, want := actions["dblclick"], view.OpenRowClick(issue.UID); got != want {
			t.Errorf("%s: double click does %v, want %v", issue.UID, got, want)
		}
		wantClick := any(nil)
		if issue.UID == "parent" {
			wantClick = view.ToggleClick("parent")
		}
		if actions["click"] != wantClick {
			t.Errorf("%s: click does %v, want %v", issue.UID, actions["click"], wantClick)
		}
	}
}
