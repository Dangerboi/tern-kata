package model_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

const manyChildren = 40

// parentBase is a Project list of a leaf, a parent with manyChildren children
// and another leaf, collapsed, with the first leaf selected and its detail
// loaded. Rows sort by update time: "A leaf", "B parent", then "C child NN" under it,
// then "Z leaf".
func parentBase() model.Model {
	issues := []model.Issue{
		{UID: "first", ShortID: "fir1", Title: "A leaf", Status: "open", UpdatedAt: ago(1)},
		{UID: "parent", ShortID: "par1", Title: "B parent", Status: "open", UpdatedAt: ago(2)},
		{UID: "last", ShortID: "las1", Title: "Z leaf", Status: "open", UpdatedAt: ago(3)},
	}
	var links []model.Link
	for i := range manyChildren {
		uid := fmt.Sprintf("child%02d", i)
		issues = append(issues, model.Issue{UID: uid, ShortID: uid, Title: "C " + uid, Status: "open", UpdatedAt: ago(4)})
		links = append(links, model.Link{ID: int64(i + 1), Type: "parent", FromIssueUID: uid, ToIssueUID: "parent", FromQualifiedID: "work#" + uid})
	}
	m, _ := loadFirst(model.Model{Cols: 160, Workspace: "/repo"}, model.ProjectLoaded{}, model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: issues, Links: links}})
	m, _ = model.Update(m, model.DetailLoaded{UID: "first", Selected: &model.IssueDetailData{Issue: &issues[0]}})
	return m
}

// parentAt applies keys from parentBase, loading the detail of whichever Issue the keys select.
func parentAt(keys ...string) model.Model {
	m, _ := clickPress(parentBase(), keys...)
	parent := model.Issue{UID: m.SelectedUID}
	var links []model.Link
	if m.SelectedUID == "parent" {
		for i := range manyChildren {
			links = append(links, model.Link{ID: int64(i + 1), Type: "parent", FromIssueUID: fmt.Sprintf("child%02d", i), ToIssueUID: "parent"})
		}
	}
	m, _ = model.Update(m, model.DetailLoaded{UID: m.SelectedUID, Selected: &model.IssueDetailData{Issue: &parent, Links: links}})
	return m
}

func TestParentRowSelectedByKeyAndByClickEndsInTheSameState(t *testing.T) {
	toggle := model.ActionMsg{Action: model.ToggleExpansion, Value: "parent"}
	selectParent := model.SelectMsg{UID: "parent"}
	for _, tt := range []struct {
		name string
		from []string // keys to the starting state
		keys []string
		// A click on the row; a parent row's click both selects and folds.
		click    []model.Msg
		expanded bool
	}{
		{"collapsed parent from above", nil, []string{"j"}, []model.Msg{selectParent}, false},
		{"collapsed parent from below", []string{"shift+g"}, []string{"k"}, []model.Msg{selectParent}, false},
		{"expanded parent from its child", []string{"j", "space", "j"}, []string{"k"}, []model.Msg{selectParent}, true},
		{"expanded parent from the leaf above", []string{"j", "space", "k"}, []string{"j"}, []model.Msg{selectParent}, true},
		{"expanding click, selecting first", nil, []string{"j", "space"}, []model.Msg{selectParent, toggle}, true},
		{"expanding click, folding only", nil, []string{"j", "space"}, []model.Msg{toggle}, true},
		{"folding click", []string{"j", "space"}, []string{"space"}, []model.Msg{selectParent, toggle}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			keyed, keyCmds := clickPress(parentAt(tt.from...), tt.keys...)
			clicked := parentAt(tt.from...)
			var clickCmds []model.Cmd
			for _, msg := range tt.click {
				var cmds []model.Cmd
				clicked, cmds = model.Update(clicked, msg)
				clickCmds = append(clickCmds, cmds...)
			}
			if !reflect.DeepEqual(clicked, keyed) || !reflect.DeepEqual(clickCmds, keyCmds) {
				t.Fatalf("click and keys diverge (click != keys):%s\ncommands: %#v != %#v", clickDiff(clicked, keyed), clickCmds, keyCmds)
			}
			rows := 3
			if tt.expanded {
				rows += manyChildren
			}
			if keyed.SelectedUID != "parent" || keyed.Focus != model.ColumnList || len(keyed.VisibleRows()) != rows {
				t.Fatalf("selected=%q focus=%v rows=%d, want the parent in the list with %d rows", keyed.SelectedUID, keyed.Focus, len(keyed.VisibleRows()), rows)
			}
		})
	}
}
