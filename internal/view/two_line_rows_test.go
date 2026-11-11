package view_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

func headerDrawn(nodes shown) bool {
	for id := range nodes {
		if strings.HasPrefix(id, "main.columns.list.") && strings.HasSuffix(id, ".header") {
			return true
		}
	}
	return false
}

func TestIssueRowsTakeTwoLinesUnderTwoLineRowCols(t *testing.T) {
	nodes := render(t, apply(browsing(), model.ResizeMsg{Cols: 47, Rows: 42}))
	row, ok := nodes[view.IssueID(alpha.UID)]
	if !ok {
		t.Fatalf("no row node %s", view.IssueID(alpha.UID))
	}
	if row.K != "col" {
		t.Fatalf("row kind = %s, want one col holding both lines", row.K)
	}
	head, title := nodes[view.IssueID(alpha.UID)+".line.head"], nodes[view.IssueID(alpha.UID)+".title"]
	if got := text(head); !strings.Contains(got, alpha.ShortID) || !strings.Contains(got, alpha.Status) || strings.Contains(got, alpha.Title) {
		t.Errorf("line 1 = %q, want id and status without the title", got)
	}
	if got := text(title); got != alpha.Title {
		t.Errorf("line 2 = %q, want the title %q", got, alpha.Title)
	}
	if lines := fmt.Sprint(title.P["lines"]); lines != "2" {
		t.Errorf("title lines = %v, want at most 2", lines)
	}
	if headerDrawn(nodes) {
		t.Error("the column header is drawn under TwoLineRowCols")
	}
}

func TestTwoLineRowKeepsTheRowActionsOnTheOneNode(t *testing.T) {
	nodes := render(t, apply(browsing(), model.ResizeMsg{Cols: 47, Rows: 42}))
	actions, _ := nodes[view.IssueID(alpha.UID)].P["actions"].(map[string]any)
	if actions["click"] != view.CardClick(alpha.UID) || actions["dblclick"] != view.OpenRowClick(alpha.UID) {
		t.Errorf("actions = %v, want click and double click on the row", actions)
	}
	menu, hold := actions["menu"].([]any), actions["hold"].([]any)
	if len(menu) == 0 || len(hold) != len(menu) {
		t.Errorf("menu %d entries, hold %d, want the same non-empty entries", len(menu), len(hold))
	}
	selectedRole := func(uid string) bool {
		return strings.Contains(fmt.Sprint(nodes[view.IssueID(uid)].P["role"]), view.TwoLineSelectedRole)
	}
	if selectedRole(beta.UID) || !selectedRole(alpha.UID) {
		t.Errorf("selected role: alpha %v, beta %v, want only alpha", selectedRole(alpha.UID), selectedRole(beta.UID))
	}
}

func TestIssueRowsAreOneLineFromTwoLineRowCols(t *testing.T) {
	nodes := render(t, apply(browsing(), model.ResizeMsg{Cols: model.TwoLineRowCols}))
	row := nodes[view.IssueID(alpha.UID)]
	if row.K != "item" || !strings.Contains(text(row), alpha.Title) {
		t.Errorf("row = %s %q, want the one-line item", row.K, text(row))
	}
	if !headerDrawn(nodes) {
		t.Error("the column header is not drawn from TwoLineRowCols")
	}
}

func TestParentRowFoldsOnClickAtTwoLines(t *testing.T) {
	parent := model.Issue{UID: "parent", ShortID: "pppp", Title: "Parent", Status: "open", ProjectName: "work"}
	child := model.Issue{UID: "child", ShortID: "cccc", Title: "Child", Status: "open", ProjectName: "work"}
	m := apply(model.Model{},
		model.Start{Workspace: "/repo"},
		model.ResizeMsg{Cols: 47, Rows: 42},
		model.ProjectLoaded{Project: model.Project{UID: "p", Name: "work"}}, snap{model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: []model.Issue{parent, child}, Links: []model.Link{{ID: 1, Type: "parent", FromIssueUID: "child", FromQualifiedID: "work#child", ToIssueUID: "parent", ToQualifiedID: "work#parent"}}}}},
	)
	nodes := render(t, m)
	row := nodes[view.IssueID(parent.UID)]
	actions, _ := row.P["actions"].(map[string]any)
	if actions["click"] != view.ToggleClick(parent.UID) {
		t.Fatalf("parent click = %v, want the fold", actions["click"])
	}
	if got := text(nodes[view.IssueID(parent.UID)+".line.head"]) + text(nodes[view.IssueID(parent.UID)+".line.meta"]); !strings.Contains(got, "▸") || !strings.Contains(got, "1") {
		t.Errorf("parent line 1 = %q, want the fold marker and the kids", got)
	}
}

func TestBelowMinimumColsStillShowsTheNoticeNamingFortyColumns(t *testing.T) {
	nodes := render(t, apply(browsing(), model.ResizeMsg{Cols: model.MinCols - 1}))
	for _, node := range nodes {
		if strings.Contains(text(node), "40 columns") {
			return
		}
	}
	t.Fatal("no notice naming 40 columns")
}
