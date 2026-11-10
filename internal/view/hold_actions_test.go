package view_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

func TestIssueRowsAndBoardCardsHoldTheSameEntriesAsTheirMenu(t *testing.T) {
	closedAt := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	closed := model.Issue{UID: "gamma", ShortID: "gggg", Title: "Gamma", Status: "closed", ProjectName: "work", ClosedAt: &closedAt}
	wide := apply(model.Model{},
		model.Start{Workspace: "/repo"},
		model.ResizeMsg{Cols: 160},
		model.ProjectLoaded{Project: model.Project{UID: "p", Name: "work"}}, snap{model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: []model.Issue{alpha, closed}}}},
	)
	board := apply(wide, model.KeyMsg{Chord: "b"})
	board.Calendar.Date = "2026-10-20"
	lists, boards := render(t, wide), render(t, board)
	cases := []struct {
		name  string
		nodes shown
		id    string
	}{
		{"open list row", lists, view.IssueID(alpha.UID)},
		{"closed list row", lists, view.IssueID(closed.UID)},
		{"open Board card", boards, view.CardID(board, alpha.UID)},
		{"closed Board card", boards, view.CardID(board, closed.UID)},
	}
	for _, tc := range cases {
		node, ok := tc.nodes[tc.id]
		if !ok {
			t.Fatalf("%s: no node %s", tc.name, tc.id)
		}
		actions, _ := node.P["actions"].(map[string]any)
		menu, hold := actions["menu"], actions["hold"]
		if items, _ := menu.([]any); len(items) == 0 {
			t.Fatalf("%s: no menu entries", tc.name)
		}
		if !reflect.DeepEqual(hold, menu) {
			t.Errorf("%s: actions.hold\n got %v\nwant %v", tc.name, hold, menu)
		}
	}
	closedHold := holdEntry(t, lists[view.IssueID(closed.UID)], "Close")
	if closedHold["disabled"] != model.ClosedAlreadyRefusal {
		t.Errorf("closed Issue's hold Close disabled = %v, want %q", closedHold["disabled"], model.ClosedAlreadyRefusal)
	}
	if got := holdEntry(t, boards[view.CardID(board, closed.UID)], "Close")["disabled"]; got != model.ClosedAlreadyRefusal {
		t.Errorf("closed Board card's hold Close disabled = %v, want %q", got, model.ClosedAlreadyRefusal)
	}
	if got := holdEntry(t, lists[view.IssueID(alpha.UID)], "Close")["disabled"]; got != nil {
		t.Errorf("open Issue's hold Close disabled = %v, want enabled", got)
	}
}

func holdEntry(t *testing.T, node tern.WireNode, label string) map[string]any {
	t.Helper()
	actions, _ := node.P["actions"].(map[string]any)
	items, _ := actions["hold"].([]any)
	for _, item := range items {
		if fields := item.(map[string]any); fields["label"] == label {
			return fields
		}
	}
	t.Fatalf("no hold entry %q in %v", label, items)
	return nil
}
