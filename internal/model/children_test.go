package model_test

import (
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

// childLink is "<child> is a child of parent".
func childLink(id int64, child string) g.UILink {
	return g.UILink{ID: id, Type: "parent", FromIssueUID: child, FromQualifiedID: "work#" + child, ToIssueUID: "parent", ToQualifiedID: "work#parent"}
}

// parentDetail is Issue "parent" with the keys in its detail: children "new"
// and "old" are loaded (in the list's order, "new" first), "gone" is not, and
// there are two comments.
func parentDetail(links ...g.UILink) model.Model {
	if links == nil {
		links = []g.UILink{childLink(1, "gone"), childLink(2, "old"), childLink(3, "new")}
	}
	return withDetail(model.Model{
		Workspace: "/repo", Cols: 160, Focus: model.ColumnDetail, SelectedUID: "parent",
		Issues: []model.Issue{
			{UID: "parent", ShortID: "par1", Title: "Parent", Status: "open", ProjectName: "work"},
			{UID: "new", ShortID: "new1", Title: "Newer", Status: "open", ProjectName: "work", UpdatedAt: ago(1)},
			{UID: "old", ShortID: "old1", Title: "Older", Status: "closed", ProjectName: "work", UpdatedAt: ago(5)},
		},
	}, &g.UISelectedAuthority{
		Issue:    &model.Issue{UID: "parent", ProjectName: "work"},
		Links:    links,
		Comments: []g.Comment{{UID: "c1"}, {UID: "c2"}},
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
	m := parentDetail(g.UILink{ID: 1, Type: "related", FromIssueUID: "parent", ToIssueUID: "new", ToQualifiedID: "work#new1"})
	if got := enterOpens(m, 0); got != "parent" {
		t.Fatalf("enter with no child rows opened %q", got)
	}
}

func TestJKWalkTheChildRowsBeforeTheActivityRows(t *testing.T) {
	m := parentDetail()
	// 3 children (0-2), then the 2 comments (3-4).
	for _, step := range []struct {
		chord string
		index int
	}{
		{"j", 1}, {"j", 2}, {"j", 3}, {"j", 4}, {"j", 4}, {"k", 3}, {"k", 2}, {"k", 1}, {"k", 0}, {"k", 0}, {"shift+g", 4}, {"g", 0},
		// Events is empty: the cursor rests below the children and j leaves it there.
		{"tab", 3}, {"j", 3},
	} {
		m, _ = model.Update(m, model.KeyMsg{Chord: step.chord})
		if m.DetailIndex() != step.index {
			t.Fatalf("after %s: index %d, want %d", step.chord, m.DetailIndex(), step.index)
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
	if m.SelectedUID != "parent" || m.DetailIndex() != 3 || len(cmds) != 0 {
		t.Fatalf("selected=%s index=%d commands=%#v", m.SelectedUID, m.DetailIndex(), cmds)
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
	for _, msg := range []model.Msg{model.KeyMsg{Chord: "tab"}, model.ActivityTabMsg{Tab: model.TabLinks}} {
		m, _ := model.Update(parentDetail(), msg)
		if m.DetailIndex() != 3 {
			t.Fatalf("%T: index %d, want 3 (after the 3 children)", msg, m.DetailIndex())
		}
	}
}

func TestClickOnAnActivityRowCursorsPastTheChildren(t *testing.T) {
	m, _ := model.Update(parentDetail(), model.DetailItemMsg{Tab: model.TabComments, Key: "c2"})
	if m.DetailIndex() != 4 {
		t.Fatalf("index %d, want 4", m.DetailIndex())
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
		Selected: &g.UISelectedAuthority{Issue: &parent, Links: []g.UILink{childLink(2, "fresh"), childLink(3, "new")}, Comments: []g.Comment{{UID: "c1"}, {UID: "c2"}}},
	}
	m = cursorAt(m, 4) // on the last of the 2 comments, past all 3 children
	m, _ = model.Update(m, model.SnapshotLoaded{Request: refresh.Request, Query: refresh.Query, Snapshot: snapshot})
	for index, want := range []string{"fresh", "new"} {
		if got := enterOpens(m, index); got != want {
			t.Errorf("enter on child row %d opened %q, want %q", index, got, want)
		}
	}
	if m.DetailIndex() != 3 { // 2 children + 2 comments: the last row is 3
		t.Errorf("cursor not clamped to the rows left: %d", m.DetailIndex())
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
		Selected: &g.UISelectedAuthority{Issue: &parent, Links: []g.UILink{childLink(1, "gone"), childLink(2, "old"), childLink(3, "new")}},
	}
	m = cursorAt(m, 1) // "old"
	m, _ = model.Update(m, model.SnapshotLoaded{Request: refresh.Request, Query: refresh.Query, Snapshot: snapshot})
	if got := enterOpens(m, m.DetailIndex()); got != "old" {
		t.Fatalf("enter after the refresh opened %q, want old", got)
	}
}
