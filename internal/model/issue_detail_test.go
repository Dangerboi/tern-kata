package model_test

import (
	"slices"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

func shownIssueA() model.Model {
	return withDetail(model.Model{Workspace: "/repo", Cols: 160, Focus: model.ColumnDetail, SelectedUID: "a", Issues: []model.Issue{{UID: "a", ProjectID: 1, Status: "open"}}},
		&g.UISelectedAuthority{Issue: &model.Issue{UID: "a", ProjectID: 1, Status: "open"}, Comments: []g.Comment{{UID: "c1"}, {UID: "c2"}}})
}

func reloads(cmds []model.Cmd) bool {
	return slices.Contains(cmds, model.Cmd(model.LoadDetail{Workspace: "/repo", UID: "a"}))
}

func TestDetailReloadsAfterTheIssueIsWritten(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup string
		write model.Msg
	}{
		{"Draft save", "", model.DraftSaved{UID: "a"}},
		{"close", "x", model.CloseResultMsg{UID: "a"}},
		{"reopen", "r", model.CloseResultMsg{UID: "a"}},
		{"field write", "+", model.FieldWritten{UID: "a"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := shownIssueA()
			if tt.setup != "" {
				m, _ = model.Update(m, model.KeyMsg{Chord: tt.setup})
			}
			m, cmds := model.Update(m, tt.write)
			if !reloads(cmds) || !m.DetailLoading() || m.DetailAuthority() == nil {
				t.Fatalf("loading=%v authority=%v commands=%#v", m.DetailLoading(), m.DetailAuthority(), cmds)
			}
		})
	}
}

func TestDetailIgnoresAWriteToAnotherIssue(t *testing.T) {
	m, cmds := model.Update(shownIssueA(), model.DraftSaved{UID: "other"})
	if reloads(cmds) || m.DetailLoading() {
		t.Fatalf("reloaded for another Issue: %#v", cmds)
	}
}

func TestJumpHistoryEndsOnEveryWayOutOfTheDetail(t *testing.T) {
	for _, tt := range []struct {
		name string
		msg  model.Msg
	}{
		{"hide the detail", model.KeyMsg{Chord: "shift+l"}},
		{"click a list row", model.SelectMsg{UID: "b"}},
		{"click the list column", model.FocusColumnMsg{Column: model.ColumnList}},
		{"double click a row", model.ActionMsg{Action: model.OpenRow, Value: "c"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := shownIssueA()
			m.Issues = []model.Issue{{UID: "a"}, {UID: "b"}, {UID: "c"}}
			m, _ = model.Update(m, model.JumpIssueMsg{UID: "b"})
			m, _ = model.Update(m, tt.msg)
			m, _ = model.Update(m, model.FocusColumnMsg{Column: model.ColumnDetail})
			m, _ = model.Update(m, model.KeyMsg{Chord: "escape"})
			if m.SelectedUID == "a" {
				t.Fatalf("back replayed the jump after %s", tt.name)
			}
		})
	}
}

func TestIssueOpenedByReferenceShowsOutsideTheCollection(t *testing.T) {
	m := model.Model{Workspace: "/repo", Cols: 160, Issues: []model.Issue{{UID: "a"}}, SelectedUID: "a"}
	m, _ = model.Update(m, model.Start{Workspace: "/repo", IssueRef: "other/bbbb"})
	m, _ = loadFirst(m, model.ProjectLoaded{}, model.SnapshotLoaded{Snapshot: g.UISnapshotResponseBody{
		Catalog:    []g.UIProject{{Project: model.Project{ID: 7, Name: "other"}}},
		Collection: []model.Issue{{UID: "a"}},
	}})
	m, _ = model.Update(m, model.IssueOpened{Ref: "other/bbbb", Selected: &g.UISelectedAuthority{Issue: &model.Issue{UID: "far"}}})
	if m.SelectedUID != "far" || m.DetailIssueUID() != "far" || m.Focus != model.ColumnDetail || m.DetailLoading() {
		t.Fatalf("selected=%q shown=%q focus=%v loading=%v", m.SelectedUID, m.DetailIssueUID(), m.Focus, m.DetailLoading())
	}
	m, _ = model.Update(m, model.ResizeMsg{Cols: 160})
	if m.SelectedUID != "far" || m.DetailIssueUID() != "far" {
		t.Fatalf("a later step lost the Issue: selected=%q shown=%q", m.SelectedUID, m.DetailIssueUID())
	}
}
