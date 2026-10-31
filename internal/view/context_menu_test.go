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

func pick(uid, chord string) string { return view.MenuPickClick(uid, chord) }

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
			{"Open", pick(alpha.UID, "enter"), "", []string{"↵"}},
			{"Agent…", pick(alpha.UID, "w"), "", []string{"w"}},
			{"Close", pick(alpha.UID, "x"), "", []string{"x"}},
			{"Priority…", pick(alpha.UID, "!"), "", []string{"!"}},
			{"Dates…", pick(alpha.UID, "d"), "", []string{"d"}},
			{"Move to Project", pick(alpha.UID, "shift+m"), "", []string{"M"}},
		}},
		{"closed", view.IssueID(closed.UID), closed.UID, []menuRow{
			{"Open", pick(closed.UID, "enter"), "", []string{"↵"}},
			{"Agent…", pick(closed.UID, "w"), "The Issue is closed; reopen it first", []string{"w"}},
			{"Close", pick(closed.UID, "x"), "The Issue is already closed", []string{"x"}},
			{"Reopen", pick(closed.UID, "r"), "", []string{"r"}},
			{"Priority…", pick(closed.UID, "!"), "", []string{"!"}},
			{"Dates…", pick(closed.UID, "d"), "", []string{"d"}},
			{"Move to Project", pick(closed.UID, "shift+m"), "", []string{"M"}},
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
	if len(card) != 6 || card[0].Label != "Open" || card[0].ID != pick(alpha.UID, "enter") || card[5].ID != pick(alpha.UID, "shift+m") {
		t.Errorf("Board card menu: %+v", card)
	}
	lane := apply(wide, model.KeyMsg{Chord: "enter"})
	rows := menuOf(t, render(t, lane), view.IssueID(alpha.UID))
	if len(rows) != 6 || rows[0].ID != pick(alpha.UID, "enter") || rows[2].Label != "Close" {
		t.Errorf("Lane row menu: %+v", rows)
	}
}
