package view_test

import (
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

// menuRows are the right-click menu rows of node id: label, id, keys and the disabled reason ("" when enabled).
type menuRow struct {
	Label, ID, Disabled string
	Keys                []string
}

func menuOf(t *testing.T, nodes shown, id string) []menuRow {
	t.Helper()
	node, ok := nodes[id]
	if !ok {
		t.Fatalf("no node %s", id)
	}
	actions, _ := node.P["actions"].(map[string]any)
	items, _ := actions["menu"].([]any)
	var rows []menuRow
	for _, item := range items {
		fields := item.(map[string]any)
		row := menuRow{Label: fields["label"].(string), ID: fields["id"].(string)}
		for _, key := range fields["keys"].([]any) {
			row.Keys = append(row.Keys, key.(string))
		}
		if reason, ok := fields["disabled"].(string); ok {
			row.Disabled = reason
		}
		rows = append(rows, row)
	}
	return rows
}

func TestIssueRowMenuListsTheKeysActionsAndDisablesWhatCannotRun(t *testing.T) {
	closed := model.Issue{UID: "gamma", ShortID: "gggg", Title: "Gamma", Status: "closed", ProjectName: "work"}
	m := apply(model.Model{},
		model.Start{Workspace: "/repo"},
		model.ResizeMsg{Cols: 160},
		model.ProjectLoaded{Project: model.Project{UID: "p", Name: "work"}}, snap{model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: []model.Issue{alpha, closed}}}},
	)
	nodes := render(t, m)
	if _, ok := nodes[view.IssueID(closed.UID)]; !ok {
		t.Fatalf("closed Issue not listed: %v", nodes.under(view.IssueListID))
	}
	cases := []struct {
		name string
		id   string
		uid  string
		want []menuRow
	}{
		{"open", view.IssueID(alpha.UID), alpha.UID, []menuRow{
			{"Open", view.ContextMenuPickClick(alpha.UID, model.ModeList, "enter"), "", []string{"↵"}},
			{"Agent…", view.ContextMenuPickClick(alpha.UID, model.ModeList, "w"), "", []string{"w"}},
			{"Close", view.ContextMenuPickClick(alpha.UID, model.ModeList, "x"), "", []string{"x"}},
			{"Priority…", view.ContextMenuPickClick(alpha.UID, model.ModeList, "!"), "", []string{"!"}},
			{"Dates…", view.ContextMenuPickClick(alpha.UID, model.ModeList, "d"), "", []string{"d"}},
			{"Move to Project", view.ContextMenuPickClick(alpha.UID, model.ModeList, "shift+m"), "", []string{"⇧m"}},
		}},
		{"closed", view.IssueID(closed.UID), closed.UID, []menuRow{
			{"Open", view.ContextMenuPickClick(closed.UID, model.ModeList, "enter"), "", []string{"↵"}},
			{"Agent…", view.ContextMenuPickClick(closed.UID, model.ModeList, "w"), model.ClosedIssueRefusal, []string{"w"}},
			{"Close", view.ContextMenuPickClick(closed.UID, model.ModeList, "x"), model.ClosedAlreadyRefusal, []string{"x"}},
			{"Reopen", view.ContextMenuPickClick(closed.UID, model.ModeList, "r"), "", []string{"r"}},
			{"Priority…", view.ContextMenuPickClick(closed.UID, model.ModeList, "!"), "", []string{"!"}},
			{"Dates…", view.ContextMenuPickClick(closed.UID, model.ModeList, "d"), "", []string{"d"}},
			{"Move to Project", view.ContextMenuPickClick(closed.UID, model.ModeList, "shift+m"), "", []string{"⇧m"}},
		}},
	}
	for _, tc := range cases {
		if got := menuOf(t, nodes, tc.id); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s Issue row menu:\n got %+v\nwant %+v", tc.name, got, tc.want)
		}
	}
}

func TestBoardCardAndLaneRowMenusPressTheirModesKeys(t *testing.T) {
	wide := apply(model.Model{},
		model.Start{Workspace: "/repo"},
		model.ResizeMsg{Cols: 160},
		model.ProjectLoaded{Project: model.Project{UID: "p", Name: "work"}}, snap{model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: []model.Issue{alpha}}}},
		model.KeyMsg{Chord: "b"},
	)
	board := render(t, wide)
	card := menuOf(t, board, "main.columns.list.board.lane0.cards."+alpha.UID)
	if len(card) != 6 || card[0].Label != "Open" || card[0].ID != view.ContextMenuPickClick(alpha.UID, model.ModeBoard, "enter") || card[5].ID != view.ContextMenuPickClick(alpha.UID, model.ModeBoard, "shift+m") {
		t.Errorf("Board card menu: %+v", card)
	}
	lane := apply(wide, model.KeyMsg{Chord: "enter"})
	rows := menuOf(t, render(t, lane), view.IssueID(alpha.UID))
	if len(rows) != 6 || rows[0].ID != view.ContextMenuPickClick(alpha.UID, model.ModeLane, "enter") || rows[2].Label != "Close" {
		t.Errorf("Lane row menu: %+v", rows)
	}
}
