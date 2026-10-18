package model_test

import (
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

// childLink is "<child> is a child of parent".
func childLink(id int64, child string) model.Link {
	return model.Link{ID: id, Type: "parent", FromIssueUID: child, FromQualifiedID: "work#" + child, ToIssueUID: "parent", ToQualifiedID: "work#parent"}
}

// parentDetail is Issue "parent" with the keys in its detail: children "new"
// and "old" are loaded (in the list's order, "new" first), "gone" is not, and
// there are two comments.
func parentDetail(links ...model.Link) model.Model {
	if links == nil {
		links = []model.Link{childLink(1, "gone"), childLink(2, "old"), childLink(3, "new")}
	}
	return withDetail(model.Model{
		Workspace: "/repo", Cols: 160, Focus: model.ColumnDetail, SelectedUID: "parent",
		Issues: []model.Issue{
			{UID: "parent", ShortID: "par1", Title: "Parent", Status: "open", ProjectName: "work"},
			{UID: "new", ShortID: "new1", Title: "Newer", Status: "open", ProjectName: "work", UpdatedAt: ago(1)},
			{UID: "old", ShortID: "old1", Title: "Older", Status: "closed", ProjectName: "work", UpdatedAt: ago(5)},
		},
	}, &model.IssueDetailData{
		Issue:    &model.Issue{UID: "parent", ProjectName: "work"},
		Links:    links,
		Comments: []model.Comment{{UID: "c1"}, {UID: "c2"}},
	})
}

// enterOpens is the Issue enter opens with the cursor on detail row index.
func enterOpens(m model.Model, index int) string {
	m, _ = model.Update(cursorAt(m, index), model.KeyMsg{Chord: "enter"})
	return m.SelectedUID
}

func TestEnterOnChildRowsOpensTheLinkedChildrenInTheListsOrderUnloadedLast(t *testing.T) {
	for index, want := range []string{"new", "old", "gone"} {
		if got := enterOpens(parentDetail(), index); got != want {
			t.Errorf("enter on child row %d opened %q, want %q", index, got, want)
		}
	}
}

func TestNoChildrenWithoutChildLinks(t *testing.T) {
	m := parentDetail(model.Link{ID: 1, Type: "related", FromIssueUID: "parent", ToIssueUID: "new", ToQualifiedID: "work#new1"})
	if got := enterOpens(m, 0); got != "parent" {
		t.Fatalf("enter with no child rows opened %q", got)
	}
}

func TestJKWalkTheChildRowsBeforeTheActivityRows(t *testing.T) {
	m := parentDetail()
	// The 3 children ("new", "old", "gone"), then the 2 comments.
	for _, step := range []struct{ chord, row string }{
		{"j", "2"}, {"j", "1"}, {"j", "c1"}, {"j", "c2"}, {"j", "c2"}, {"k", "c1"}, {"k", "1"}, {"k", "2"}, {"k", "3"}, {"k", "3"}, {"shift+g", "c2"}, {"g", "3"},
		// Events is empty: the cursor rests below the children and j leaves it there.
		{"tab", ""}, {"j", ""},
	} {
		m, _ = model.Update(m, model.KeyMsg{Chord: step.chord})
		if got := cursorKey(m); got != step.row {
			t.Fatalf("after %s: cursor on %q, want %q", step.chord, got, step.row)
		}
	}
}

func TestEnterOnAChildRowJumpsToItAndEscComesBack(t *testing.T) {
	m, _ := clickPress(parentDetail(), "j") // "old", the second child
	m, cmds := model.Update(m, model.KeyMsg{Chord: "enter"})
	if m.SelectedUID != "old" || m.Focus != model.ColumnDetail || !reflect.DeepEqual(cmds, []model.Cmd{model.LoadDetail{Workspace: "/repo", UID: "old"}}) {
		t.Fatalf("enter on a child: selected=%s focus=%v commands=%#v", m.SelectedUID, m.Focus, cmds)
	}
	m, cmds = model.Update(m, model.KeyMsg{Chord: "escape"})
	if m.SelectedUID != "parent" || m.Focus != model.ColumnDetail || !reflect.DeepEqual(cmds, []model.Cmd{model.LoadDetail{Workspace: "/repo", UID: "parent"}}) {
		t.Fatalf("esc: selected=%s focus=%v commands=%#v", m.SelectedUID, m.Focus, cmds)
	}
}

func TestEnterOnAnUnloadedChildStillJumps(t *testing.T) {
	m, _ := clickPress(parentDetail(), "j", "j")
	m, _ = model.Update(m, model.KeyMsg{Chord: "enter"})
	if m.SelectedUID != "gone" {
		t.Fatalf("selected %s", m.SelectedUID)
	}
}

func TestEnterOnAnActivityRowBelowTheChildrenDoesNotJump(t *testing.T) {
	m, cmds := clickPress(parentDetail(), "j", "j", "j", "enter")
	if m.SelectedUID != "parent" || cursorKey(m) != "c1" || len(cmds) != 0 {
		t.Fatalf("selected=%s cursor on %q commands=%#v", m.SelectedUID, cursorKey(m), cmds)
	}
}

func TestClickOnAChildJumpsAndEscComesBack(t *testing.T) {
	m, _ := model.Update(parentDetail(), model.JumpIssueMsg{UID: "new"})
	if m.SelectedUID != "new" {
		t.Fatalf("selected %s", m.SelectedUID)
	}
	m, _ = model.Update(m, model.KeyMsg{Chord: "escape"})
	if m.SelectedUID != "parent" {
		t.Fatalf("esc: selected %s", m.SelectedUID)
	}
}

func TestTabSwitchLandsOnTheFirstActivityRow(t *testing.T) {
	for _, msg := range []model.Msg{model.ActivityTabMsg{Tab: model.TabComments}, model.ActivityTabMsg{Tab: model.TabLinks}} {
		m, _ := model.Update(parentDetail(), msg)
		rows := m.DetailRows()
		if i := m.DetailIndex(); i >= len(rows) || rows[i].Kind == model.RowChild || (i > 0 && rows[i-1].Kind != model.RowChild) {
			t.Fatalf("%T: cursor %d is not on the first Activity row", msg, i)
		}
	}
}

func TestClickOnAnActivityRowCursorsPastTheChildren(t *testing.T) {
	m, _ := model.Update(parentDetail(), model.DetailItemMsg{Tab: model.TabComments, Key: "c2"})
	if cursorKey(m) != "c2" {
		t.Fatalf("cursor on %q, want c2", cursorKey(m))
	}
}

func TestChildRowsFollowLiveRefreshes(t *testing.T) {
	m := parentDetail()
	m, cmds := model.Update(m, model.StreamEvent{ID: 1, Type: "issue.updated"})
	refresh := cmds[0].(model.ReadSnapshot)
	// "gone" is removed, "fresh" is added and "new" changed its title.
	parent := model.Issue{UID: "parent", ProjectName: "work"}
	snapshot := g.UISnapshotResponseBody{Cursor: 1,
		Collection: []model.Issue{
			parent,
			{UID: "fresh", ShortID: "fre1", Title: "Fresh", Status: "open", ProjectName: "work", UpdatedAt: ago(0)},
			{UID: "new", ShortID: "new1", Title: "Renamed", Status: "open", ProjectName: "work", UpdatedAt: ago(1)},
		},
	}
	selected := &model.IssueDetailData{Issue: &parent, Links: []model.Link{childLink(2, "fresh"), childLink(3, "new")}, Comments: []model.Comment{{UID: "c1"}, {UID: "c2"}}}
	m = cursorAt(m, 4) // on the last of the 2 comments, past all 3 children
	m, _ = model.Update(m, model.SnapshotLoaded{Request: refresh.Request, Query: refresh.Query, Snapshot: snapshot, Selected: selected})
	for index, want := range []string{"fresh", "new"} {
		if got := enterOpens(m, index); got != want {
			t.Errorf("enter on child row %d opened %q, want %q", index, got, want)
		}
	}
	if cursorKey(m) != "c2" {
		t.Errorf("cursor left its comment: on %q", cursorKey(m))
	}
}

func TestCursorStaysOnItsChildWhenARefreshReordersTheChildren(t *testing.T) {
	m := parentDetail()
	m, cmds := model.Update(m, model.StreamEvent{ID: 1, Type: "issue.updated"})
	refresh := cmds[0].(model.ReadSnapshot)
	parent := model.Issue{UID: "parent", ProjectName: "work"}
	// "old", under the cursor, was edited and moves above "new".
	snapshot := g.UISnapshotResponseBody{Cursor: 1,
		Collection: []model.Issue{
			parent,
			{UID: "old", ShortID: "old1", Title: "Older", Status: "closed", ProjectName: "work", UpdatedAt: ago(0)},
			{UID: "new", ShortID: "new1", Title: "Newer", Status: "open", ProjectName: "work", UpdatedAt: ago(1)},
		},
	}
	selected := &model.IssueDetailData{Issue: &parent, Links: []model.Link{childLink(1, "gone"), childLink(2, "old"), childLink(3, "new")}}
	m = cursorAt(m, 1) // "old"
	m, _ = model.Update(m, model.SnapshotLoaded{Request: refresh.Request, Query: refresh.Query, Snapshot: snapshot, Selected: selected})
	if got := enterOpens(m, m.DetailIndex()); got != "old" {
		t.Fatalf("enter after the refresh opened %q, want old", got)
	}
}

func TestCursorStaysOnItsCommentWhenARefreshAddsOneAbove(t *testing.T) {
	m := cursorAt(parentDetail(), 4) // "c2"
	m, cmds := model.Update(m, model.StreamEvent{ID: 1, Type: "issue.updated"})
	refresh := cmds[0].(model.ReadSnapshot)
	parent := model.Issue{UID: "parent", ProjectName: "work"}
	selected := &model.IssueDetailData{Issue: &parent, Links: []model.Link{childLink(3, "new")}, Comments: []model.Comment{{UID: "c0"}, {UID: "c1"}, {UID: "c2"}}}
	snapshot := g.UISnapshotResponseBody{Cursor: 1, Collection: []model.Issue{parent, {UID: "new", ShortID: "new1", Title: "Newer", Status: "open", ProjectName: "work"}}}
	m, _ = model.Update(m, model.SnapshotLoaded{Request: refresh.Request, Query: refresh.Query, Snapshot: snapshot, Selected: selected})
	if cursorKey(m) != "c2" {
		t.Fatalf("cursor on %q after the refresh, want c2", cursorKey(m))
	}
}
