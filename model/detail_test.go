package model_test

import (
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

func TestColumnDrillAndFolding(t *testing.T) {
	for _, tt := range []struct {
		name string
		msgs []model.Msg
		focus model.Column
		columns []model.Column
		hidden bool
	}{
		{"wide", nil, model.ColumnList, []model.Column{model.ColumnCollections, model.ColumnList, model.ColumnDetail}, false},
		{"drill in", []model.Msg{model.KeyMsg{Chord: "enter"}}, model.ColumnDetail, []model.Column{model.ColumnCollections, model.ColumnList, model.ColumnDetail}, false},
		{"drill out", []model.Msg{model.KeyMsg{Chord: "enter"}, model.KeyMsg{Chord: "escape"}}, model.ColumnList, []model.Column{model.ColumnCollections, model.ColumnList, model.ColumnDetail}, false},
		{"sidebar", []model.Msg{model.KeyMsg{Chord: "escape"}}, model.ColumnCollections, []model.Column{model.ColumnCollections, model.ColumnList, model.ColumnDetail}, false},
		{"sidebar to list", []model.Msg{model.KeyMsg{Chord: "escape"}, model.KeyMsg{Chord: "enter"}}, model.ColumnList, []model.Column{model.ColumnCollections, model.ColumnList, model.ColumnDetail}, false},
		{"hide detail", []model.Msg{model.KeyMsg{Chord: "enter"}, model.KeyMsg{Chord: "shift+l"}}, model.ColumnList, []model.Column{model.ColumnCollections, model.ColumnList}, true},
		{"restore detail", []model.Msg{model.KeyMsg{Chord: "shift+l"}, model.KeyMsg{Chord: "shift+l"}}, model.ColumnList, []model.Column{model.ColumnCollections, model.ColumnList, model.ColumnDetail}, false},
		{"open hidden detail", []model.Msg{model.KeyMsg{Chord: "shift+l"}, model.KeyMsg{Chord: "enter"}}, model.ColumnDetail, []model.Column{model.ColumnCollections, model.ColumnList, model.ColumnDetail}, false},
		{"narrow list", []model.Msg{model.ResizeMsg{Cols: 60}}, model.ColumnList, []model.Column{model.ColumnList}, false},
		{"narrow detail", []model.Msg{model.ResizeMsg{Cols: 60}, model.KeyMsg{Chord: "enter"}}, model.ColumnDetail, []model.Column{model.ColumnDetail}, false},
		{"narrow sidebar", []model.Msg{model.ResizeMsg{Cols: 60}, model.KeyMsg{Chord: "escape"}}, model.ColumnCollections, []model.Column{model.ColumnCollections}, false},
		{"resize preserves focus", []model.Msg{model.KeyMsg{Chord: "enter"}, model.ResizeMsg{Cols: 60}, model.ResizeMsg{Cols: 120}}, model.ColumnDetail, []model.Column{model.ColumnCollections, model.ColumnList, model.ColumnDetail}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := model.Model{Cols: 120, SelectedUID: "a", Issues: []model.Issue{{UID: "a"}}, DetailUID: "a"}
			for _, msg := range tt.msgs { m, _ = model.Update(m, msg) }
			if m.Focus != tt.focus || m.DetailHidden != tt.hidden || !reflect.DeepEqual(m.VisibleColumns(), tt.columns) {
				t.Fatalf("focus=%v hidden=%v columns=%v", m.Focus, m.DetailHidden, m.VisibleColumns())
			}
		})
	}
	m, cmds := model.Update(model.Model{}, model.KeyMsg{Chord: "enter"})
	if m.Focus != model.ColumnList || len(cmds) != 0 { t.Fatalf("empty list drilled into detail: %+v %#v", m, cmds) }
}

func TestSelectedIssueRequestsDetail(t *testing.T) {
	m, cmds := model.Update(model.Model{Workspace: "/repo", Issues: []model.Issue{{UID: "a"}, {UID: "b"}}, SelectedUID: "a"}, model.SelectMsg{UID: "b"})
	want := []model.Cmd{model.LoadDetail{Workspace: "/repo", UID: "b"}}
	if m.SelectedUID != "b" || !m.DetailLoading || !reflect.DeepEqual(cmds, want) { t.Fatalf("selected detail: %+v %#v", m, cmds) }
	m, _ = model.Update(m, model.DetailLoaded{UID: "a", Selected: &g.UISelectedAuthority{Issue: &model.Issue{UID: "a"}}})
	if m.Detail != nil || !m.DetailLoading { t.Fatal("old request replaced current detail") }
	m, cmds = model.Update(m, model.DetailLoaded{UID: "b", Selected: &g.UISelectedAuthority{Issue: &model.Issue{UID: "b", Body: "Markdown"}}})
	if m.Detail == nil || m.Detail.Issue.Body != "Markdown" || m.DetailLoading || len(cmds) != 0 { t.Fatalf("loaded detail: %+v %#v", m, cmds) }
}

func TestDetailSectionsAndReferenceJump(t *testing.T) {
	for _, tt := range []struct {
		name string
		msgs []model.Msg
		section model.DetailSection
		selected string
		focus model.Column
		wantCmds []model.Cmd
	}{
		{"next section", []model.Msg{model.KeyMsg{Chord: "tab"}}, model.SectionProperties, "a", model.ColumnDetail, nil},
		{"previous wraps", []model.Msg{model.KeyMsg{Chord: "shift+tab"}}, model.SectionHistory, "a", model.ColumnDetail, nil},
		{"next wraps", []model.Msg{model.KeyMsg{Chord: "shift+tab"}, model.KeyMsg{Chord: "tab"}}, model.SectionBody, "a", model.ColumnDetail, nil},
		{"reference outside list", []model.Msg{model.KeyMsg{Chord: "tab"}, model.KeyMsg{Chord: "tab"}, model.KeyMsg{Chord: "tab"}, model.KeyMsg{Chord: "enter"}}, model.SectionBody, "closed-b", model.ColumnDetail, []model.Cmd{model.LoadDetail{Workspace: "/repo", UID: "closed-b"}}},
		{"history reference", []model.Msg{model.KeyMsg{Chord: "shift+tab"}, model.KeyMsg{Chord: "enter"}}, model.SectionBody, "closed-b", model.ColumnDetail, []model.Cmd{model.LoadDetail{Workspace: "/repo", UID: "closed-b"}}},
		{"reference back stays in detail", []model.Msg{model.KeyMsg{Chord: "tab"}, model.KeyMsg{Chord: "tab"}, model.KeyMsg{Chord: "tab"}, model.KeyMsg{Chord: "enter"}, model.KeyMsg{Chord: "escape"}}, model.SectionBody, "a", model.ColumnDetail, []model.Cmd{model.LoadDetail{Workspace: "/repo", UID: "a"}}},
		{"pointer reference jump", []model.Msg{model.JumpIssueMsg{UID: "closed-b"}}, model.SectionBody, "closed-b", model.ColumnDetail, []model.Cmd{model.LoadDetail{Workspace: "/repo", UID: "closed-b"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			peer := "closed-b"
			m := model.Model{Workspace: "/repo", Focus: model.ColumnDetail, SelectedUID: "a", DetailUID: "a", Issues: []model.Issue{{UID: "a"}}, Detail: &g.UISelectedAuthority{
				Issue: &model.Issue{UID: "a"},
				Links: []g.UILink{{ID: 1, FromIssueUID: "a", ToIssueUID: peer, ToQualifiedID: "other#bbbb", Type: "related"}},
				History: []g.Event{{UID: "event-1", RelatedIssueUID: &peer}},
			}}
			var cmds []model.Cmd
			for _, msg := range tt.msgs { m, cmds = model.Update(m, msg) }
			if m.Focus != tt.focus || m.SelectedUID != tt.selected || m.DetailSection != tt.section || !reflect.DeepEqual(cmds, tt.wantCmds) {
				t.Fatalf("focus=%v selected=%s section=%v commands=%#v", m.Focus, m.SelectedUID, m.DetailSection, cmds)
			}
		})
	}
}

func TestLiveDetailAuthority(t *testing.T) {
	for _, tt := range []struct {
		name string
		selected string
		back []string
		authority string
		wantSelected string
		wantBody string
		wantCmds []model.Cmd
	}{
		{"current authority", "a", nil, "a", "a", "Live body", nil},
		{"old selection authority", "b", nil, "a", "b", "", []model.Cmd{model.LoadDetail{Workspace: "/repo", UID: "b"}}},
		{"referenced Issue outside list", "closed", []string{"a"}, "closed", "closed", "Live body", nil},
		{"removed selection", "removed", nil, "removed", "a", "", []model.Cmd{model.LoadDetail{Workspace: "/repo", UID: "a"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := model.Model{Workspace: "/repo", Project: model.Project{UID: "project"}, SelectedUID: tt.selected, DetailUID: tt.selected, Focus: model.ColumnDetail, DetailBack: tt.back}
			m, cmds := model.Update(m, model.StreamEvent{ID: 1, Type: "issue.updated"})
			refresh := cmds[0].(model.RefreshSnapshot)
			m, cmds = model.Update(m, model.SnapshotRefreshed{Request: refresh.Request, Query: refresh.Query, Snapshot: g.UISnapshotResponseBody{
				Cursor: 1, Collection: []model.Issue{{UID: "a"}, {UID: "b"}},
				Selected: &g.UISelectedAuthority{Issue: &model.Issue{UID: tt.authority, Body: "Live body"}},
			}})
			body := ""
			if m.Detail != nil && m.Detail.Issue != nil { body = m.Detail.Issue.Body }
			if m.SelectedUID != tt.wantSelected || body != tt.wantBody || !reflect.DeepEqual(cmds, tt.wantCmds) {
				t.Fatalf("selection=%s body=%q commands=%#v", m.SelectedUID, body, cmds)
			}
		})
	}
}
