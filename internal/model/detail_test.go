package model_test

import (
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestColumnDrillAndFolding(t *testing.T) {
	for _, tt := range []struct {
		name    string
		msgs    []model.Msg
		focus   model.Column
		columns []model.Column
		hidden  bool
	}{
		{"wide", nil, model.ColumnList, []model.Column{model.ColumnCollections, model.ColumnList, model.ColumnDetail}, false},
		{"drill in", []model.Msg{model.KeyMsg{Chord: "enter"}}, model.ColumnDetail, []model.Column{model.ColumnCollections, model.ColumnList, model.ColumnDetail}, false},
		{"drill out", []model.Msg{model.KeyMsg{Chord: "enter"}, model.KeyMsg{Chord: "escape"}}, model.ColumnList, []model.Column{model.ColumnCollections, model.ColumnList, model.ColumnDetail}, false},
		{"sidebar", []model.Msg{model.KeyMsg{Chord: "escape"}}, model.ColumnCollections, []model.Column{model.ColumnCollections, model.ColumnList, model.ColumnDetail}, false},
		{"sidebar to list", []model.Msg{model.KeyMsg{Chord: "escape"}, model.KeyMsg{Chord: "enter"}}, model.ColumnList, []model.Column{model.ColumnCollections, model.ColumnList, model.ColumnDetail}, false},
		{"hide detail", []model.Msg{model.KeyMsg{Chord: "enter"}, model.KeyMsg{Chord: "shift+l"}}, model.ColumnList, []model.Column{model.ColumnCollections, model.ColumnList}, true},
		{"restore detail", []model.Msg{model.KeyMsg{Chord: "shift+l"}, model.KeyMsg{Chord: "shift+l"}}, model.ColumnList, []model.Column{model.ColumnCollections, model.ColumnList, model.ColumnDetail}, false},
		{"open hidden detail", []model.Msg{model.KeyMsg{Chord: "shift+l"}, model.KeyMsg{Chord: "enter"}}, model.ColumnDetail, []model.Column{model.ColumnCollections, model.ColumnList, model.ColumnDetail}, false},
		{"single column list", []model.Msg{model.ResizeMsg{Cols: 100}}, model.ColumnList, []model.Column{model.ColumnList}, false},
		{"single column detail", []model.Msg{model.ResizeMsg{Cols: 100}, model.KeyMsg{Chord: "enter"}}, model.ColumnDetail, []model.Column{model.ColumnDetail}, false},
		{"single column sidebar", []model.Msg{model.ResizeMsg{Cols: 100}, model.KeyMsg{Chord: "escape"}}, model.ColumnCollections, []model.Column{model.ColumnCollections}, false},
		{"resize preserves focus", []model.Msg{model.KeyMsg{Chord: "enter"}, model.ResizeMsg{Cols: 60}, model.ResizeMsg{Cols: 160}}, model.ColumnDetail, []model.Column{model.ColumnCollections, model.ColumnList, model.ColumnDetail}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := withDetail(model.Model{Cols: 160, SelectedUID: "a", Issues: []model.Issue{{UID: "a"}}}, nil)
			for _, msg := range tt.msgs {
				m, _ = model.Update(m, msg)
			}
			if m.Focus != tt.focus || m.DetailHidden() != tt.hidden || !reflect.DeepEqual(m.VisibleColumns(), tt.columns) {
				t.Fatalf("focus=%v hidden=%v columns=%v", m.Focus, m.DetailHidden(), m.VisibleColumns())
			}
		})
	}
	m, cmds := model.Update(model.Model{}, model.KeyMsg{Chord: "enter"})
	if m.Focus != model.ColumnList || len(cmds) != 0 {
		t.Fatalf("empty list drilled into detail: %+v %#v", m, cmds)
	}
}

func TestSelectedIssueRequestsDetail(t *testing.T) {
	m, cmds := model.Update(model.Model{Workspace: "/repo", Issues: []model.Issue{{UID: "a"}, {UID: "b"}}, SelectedUID: "a"}, model.SelectMsg{UID: "b"})
	want := []model.Cmd{model.LoadDetail{Workspace: "/repo", UID: "b"}}
	if m.SelectedUID != "b" || !m.DetailLoading() || !reflect.DeepEqual(cmds, want) {
		t.Fatalf("selected detail: %+v %#v", m, cmds)
	}
	m, _ = model.Update(m, model.DetailLoaded{UID: "a", Selected: &model.IssueDetailData{Issue: &model.Issue{UID: "a"}}})
	if m.DetailAuthority() != nil || !m.DetailLoading() {
		t.Fatal("old request replaced current detail")
	}
	m, cmds = model.Update(m, model.DetailLoaded{UID: "b", Selected: &model.IssueDetailData{Issue: &model.Issue{UID: "b", Body: "Markdown"}}})
	if m.DetailAuthority() == nil || m.DetailAuthority().Issue.Body != "Markdown" || m.DetailLoading() || len(cmds) != 0 {
		t.Fatalf("loaded detail: %+v %#v", m, cmds)
	}
}

func TestActivityTabsAndReferenceJump(t *testing.T) {
	reveal := []model.Cmd{model.RevealActivityTab{}}
	for _, tt := range []struct {
		name     string
		msgs     []model.Msg
		tab      model.ActivityTab
		selected string
		focus    model.Column
		wantCmds []model.Cmd
	}{
		{"next tab", []model.Msg{model.KeyMsg{Chord: "tab"}}, model.TabEvents, "a", model.ColumnDetail, reveal},
		{"previous wraps", []model.Msg{model.KeyMsg{Chord: "shift+tab"}}, model.TabAttachments, "a", model.ColumnDetail, reveal},
		{"next wraps", []model.Msg{model.KeyMsg{Chord: "shift+tab"}, model.KeyMsg{Chord: "tab"}}, model.TabComments, "a", model.ColumnDetail, reveal},
		{"click selects a tab", []model.Msg{model.ActivityTabMsg{Tab: model.TabLinks}}, model.TabLinks, "a", model.ColumnDetail, reveal},
		{"link reference, tab kept across Issues", []model.Msg{model.KeyMsg{Chord: "tab"}, model.KeyMsg{Chord: "tab"}, model.KeyMsg{Chord: "enter"}}, model.TabLinks, "closed-b", model.ColumnDetail, []model.Cmd{model.LoadDetail{Workspace: "/repo", UID: "closed-b"}}},
		{"event reference", []model.Msg{model.KeyMsg{Chord: "tab"}, model.KeyMsg{Chord: "enter"}}, model.TabEvents, "closed-b", model.ColumnDetail, []model.Cmd{model.LoadDetail{Workspace: "/repo", UID: "closed-b"}}},
		{"reference back stays in detail", []model.Msg{model.KeyMsg{Chord: "tab"}, model.KeyMsg{Chord: "tab"}, model.KeyMsg{Chord: "enter"}, model.KeyMsg{Chord: "escape"}}, model.TabLinks, "a", model.ColumnDetail, []model.Cmd{model.LoadDetail{Workspace: "/repo", UID: "a"}}},
		{"pointer reference jump", []model.Msg{model.JumpIssueMsg{UID: "closed-b"}}, model.TabComments, "closed-b", model.ColumnDetail, []model.Cmd{model.LoadDetail{Workspace: "/repo", UID: "closed-b"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			peer := "closed-b"
			m := withDetail(model.Model{Workspace: "/repo", Cols: 160, Focus: model.ColumnDetail, SelectedUID: "a", Issues: []model.Issue{{UID: "a"}}}, &model.IssueDetailData{
				Issue:   &model.Issue{UID: "a"},
				Links:   []model.Link{{ID: 1, FromIssueUID: "a", ToIssueUID: peer, ToQualifiedID: "other#bbbb", Type: "related"}},
				History: []model.Event{{UID: "event-1", RelatedIssueUID: &peer}},
			})
			var cmds []model.Cmd
			for _, msg := range tt.msgs {
				m, cmds = model.Update(m, msg)
			}
			if m.Focus != tt.focus || m.SelectedUID != tt.selected || m.ActiveTab() != tt.tab || !reflect.DeepEqual(cmds, tt.wantCmds) {
				t.Fatalf("focus=%v selected=%s tab=%v commands=%#v", m.Focus, m.SelectedUID, m.ActiveTab(), cmds)
			}
		})
	}
}

func TestLiveDetailAuthority(t *testing.T) {
	for _, tt := range []struct {
		name         string
		selected     string
		back         []string
		authority    string
		wantSelected string
		wantBody     string
		wantCmds     []model.Cmd
	}{
		{"current authority", "a", nil, "a", "a", "Live body", nil},
		{"old selection authority", "b", nil, "a", "b", "", []model.Cmd{model.LoadDetail{Workspace: "/repo", UID: "b"}}},
		{"referenced Issue outside list", "closed", []string{"a"}, "closed", "closed", "Live body", nil},
		{"removed selection", "removed", nil, "removed", "a", "", []model.Cmd{model.LoadDetail{Workspace: "/repo", UID: "a"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			start := tt.selected
			if len(tt.back) > 0 {
				start = tt.back[0]
			}
			m := withDetail(model.Model{LiveState: model.LiveState{StreamGeneration: 1}, Workspace: "/repo", Project: model.Project{UID: "project"}, SelectedUID: start, Focus: model.ColumnDetail}, nil)
			if len(tt.back) > 0 {
				m, _ = model.Update(m, model.JumpIssueMsg{UID: tt.selected})
			}
			m, cmds := model.Update(m, model.StreamEvent{Generation: 1, ID: 1, Type: "issue.updated"})
			refresh := cmds[0].(model.ReadSnapshot)
			m, cmds = model.Update(m, model.SnapshotLoaded{Request: refresh.Request, Query: refresh.Query, Snapshot: model.Snapshot{
				Cursor: 1, Issues: []model.Issue{{UID: "a"}, {UID: "b"}},
			}, Selected: &model.IssueDetailData{Issue: &model.Issue{UID: tt.authority, Body: "Live body"}}})
			body := ""
			if m.DetailAuthority() != nil && m.DetailAuthority().Issue != nil {
				body = m.DetailAuthority().Issue.Body
			}
			if m.SelectedUID != tt.wantSelected || body != tt.wantBody || !reflect.DeepEqual(cmds, tt.wantCmds) {
				t.Fatalf("selection=%s body=%q commands=%#v", m.SelectedUID, body, cmds)
			}
		})
	}
}

func TestLinkRelationsRelativeToSelectedIssue(t *testing.T) {
	for _, tt := range []struct{ relation, selected, shown, ref, peer string }{
		{"parent", "child", "parent", "project#parent", "parent"},
		{"parent", "parent", "child", "project#child", "child"},
		{"blocks", "child", "blocks", "project#parent", "parent"},
		{"blocks", "parent", "blocked_by", "project#child", "child"},
		{"related", "parent", "related", "project#child", "child"},
	} {
		t.Run(tt.relation+"/"+tt.selected, func(t *testing.T) {
			m, _ := model.Update(withDetail(model.Model{SelectedUID: tt.selected}, nil), model.DetailLoaded{
				UID: tt.selected, Selected: &model.IssueDetailData{Issue: &model.Issue{UID: tt.selected}, Links: []model.Link{{
					ID: 1, Type: tt.relation, FromIssueUID: "child", ToIssueUID: "parent", FromQualifiedID: "project#child", ToQualifiedID: "project#parent",
				}}},
			})
			row, ok := activityRow(m, model.TabLinks, 0)
			if !ok || row.Relation != tt.shown || row.Ref != tt.ref || row.ReferenceUID != tt.peer {
				t.Fatalf("relation: %+v, want %q %q -> %q", row, tt.shown, tt.ref, tt.peer)
			}
		})
	}
}

func TestJKMoveTheRowCursorInsideTheActiveTab(t *testing.T) {
	m := withDetail(model.Model{Workspace: "/repo", Cols: 160, Focus: model.ColumnDetail, SelectedUID: "a", Issues: []model.Issue{{UID: "a"}}}, &model.IssueDetailData{
		Issue:    &model.Issue{UID: "a"},
		Comments: []model.Comment{{UID: "c1"}, {UID: "c2"}, {UID: "c3"}},
		Links:    []model.Link{{ID: 1, FromIssueUID: "a", ToIssueUID: "b", ToQualifiedID: "w#b", Type: "related"}},
	})
	for _, step := range []struct {
		chord    string
		tab      model.ActivityTab
		index    int
		selected string
		wantCmds int
	}{
		{"j", model.TabComments, 1, "a", 0},
		{"j", model.TabComments, 2, "a", 0},
		{"j", model.TabComments, 2, "a", 0}, // stops at the last row
		{"k", model.TabComments, 1, "a", 0},
		{"enter", model.TabComments, 1, "a", 0}, // a comment has no reference
		{"tab", model.TabEvents, 0, "a", 1},     // a new tab starts at its first row; the reveal is the one command
		{"tab", model.TabLinks, 0, "a", 1},
		{"enter", model.TabLinks, 0, "b", 1},
	} {
		var cmds []model.Cmd
		m, cmds = model.Update(m, model.KeyMsg{Chord: step.chord})
		if m.ActiveTab() != step.tab || m.DetailIndex() != step.index || m.SelectedUID != step.selected || len(cmds) != step.wantCmds {
			t.Fatalf("after %s: tab=%v index=%d selected=%s cmds=%#v", step.chord, m.ActiveTab(), m.DetailIndex(), m.SelectedUID, cmds)
		}
	}
}

func TestLinkRefIsShortInTheShownIssuesProject(t *testing.T) {
	link := func(id int64, peer string) model.Link {
		return model.Link{ID: id, Type: "related", FromIssueUID: "a", FromQualifiedID: "work#a", ToIssueUID: peer, ToQualifiedID: peer + "#x"}
	}
	m, _ := model.Update(withDetail(model.Model{SelectedUID: "a"}, nil), model.DetailLoaded{UID: "a", Selected: &model.IssueDetailData{
		Issue: &model.Issue{UID: "a", ProjectName: "work"}, Links: []model.Link{link(1, "work"), link(2, "other")},
	}})
	for i, want := range []string{"#x", "other#x"} {
		if row, _ := activityRow(m, model.TabLinks, i); row.Ref != want {
			t.Errorf("row %d ref = %q, want %q", i, row.Ref, want)
		}
	}
}
