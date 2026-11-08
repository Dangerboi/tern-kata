package model_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestUndoStillTakesBackTheLastIssueChangeAfterTheLaneListIsSharedWithTheProject(t *testing.T) {
	m, _ := loadFirst(model.Model{Cols: 160, Workspace: "/repo"}, model.ProjectLoaded{Project: model.Project{UID: "p", Name: "work"}}, model.SnapshotLoaded{})
	m, _ = clickPress(m, "b")
	m, _ = model.Update(m, model.LanesLoaded{ProjectUID: "p", Request: m.LanesRequest(), Lanes: []string{"triage"}, Shared: true, ProjectID: 7, Revision: 4})
	m = withHistory(m)
	m, _ = clickPress(m, "shift+b", "ctrl+u")
	m, _ = model.Update(m, model.KeyMsg{Chord: "paste", Text: "triage, review"})
	m, cmds := clickPress(m, "enter")
	var share model.ShareLanes
	for _, c := range cmds {
		if s, ok := c.(model.ShareLanes); ok {
			share = s
		}
	}
	if share.ProjectUID != "p" {
		t.Fatalf("saving sent %#v, want a write of the Project's Lane list", cmds)
	}
	m, _ = model.Update(m, model.LanesShared{Share: share, Revision: 5})
	if got := undoSays(m); got != "undoing" {
		t.Fatalf("after sharing the Lane list u says %q, want it to take back the last Issue change", got)
	}
}
