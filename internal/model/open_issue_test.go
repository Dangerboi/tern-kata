package model_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

func TestOpenAtIssueAndSelectAction(t *testing.T) {
	for _, startup := range []bool{true, false} {
		t.Run(map[bool]string{true: "open argument", false: "TSP select action"}[startup], func(t *testing.T) {
			m, _ := model.Update(model.Model{Workspace: "/repo", SelectedUID: "previous", Focus: model.ColumnList}, model.KeyMsg{Chord: "shift+l"})
			if startup {
				m, _ = model.Update(m, model.Start{Workspace: "/repo", IssueRef: "other/bbbb"})
			}
			snapshot := g.UISnapshotResponseBody{Catalog: []g.UIProject{{Project: model.Project{ID: 7, UID: "other-project", Name: "other"}}}, Collection: []model.Issue{{UID: "previous"}}}
			var cmds []model.Cmd
			m, cmds = loadFirst(m, model.ProjectLoaded{}, model.SnapshotLoaded{Snapshot: snapshot})
			if !startup {
				m, cmds = model.Update(m, model.ActionMsg{Action: model.SelectIssueAction, Value: "other/bbbb"})
			}
			found := false
			for _, cmd := range cmds {
				if load, ok := cmd.(model.OpenIssue); ok {
					found = load.Ref == "other/bbbb" && load.ProjectID == 7 && load.Workspace == "/repo"
				}
			}
			if !found {
				t.Fatalf("missing Issue resolution command: %#v", cmds)
			}
			selected := &model.IssueDetailData{Issue: &model.Issue{UID: "target", ShortID: "bbbb", Title: "Target"}}
			m, _ = model.Update(m, model.IssueOpened{Ref: "other/bbbb", Selected: selected})
			if m.SelectedUID != "target" || m.DetailAuthority() != selected || m.Focus != model.ColumnDetail || m.DetailHidden() || m.DetailLoading() {
				t.Fatalf("Issue not selected in detail: %+v", m)
			}
			m, _ = model.Update(m, model.SnapshotLoaded{Request: m.RefreshRequest, Query: m.SnapshotQuery(), Snapshot: snapshot, ETag: "next"})
			if m.SelectedUID != "target" {
				t.Fatalf("refresh lost external Issue: %q", m.SelectedUID)
			}
		})
	}
}
