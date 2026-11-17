package model_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestAnIssueWithoutAUIDTakesAnEditInTheDoc(t *testing.T) {
	m := model.Model{Cols: 160, Workspace: "/repo", InPlaceBody: true}
	issue := model.Issue{Body: "body", ProjectID: 7, Revision: 3, QualifiedID: "work#aaaa"}
	m, _ = model.Update(m, model.DetailLoaded{Selected: &model.IssueDetailData{Issue: &issue}})
	if m.DetailAuthority() == nil {
		t.Fatal("the detail was not loaded")
	}
	m, _ = model.Update(m, model.BodyEdited{Text: "mine"})
	if doc := m.BodyDoc(issue); doc.Text != "body" {
		t.Fatalf("doc text %q", doc.Text)
	}
	if _, cmds := model.Update(m, model.BodyEditing{On: false}); len(cmds) == 0 {
		t.Fatal("leaving the doc sent nothing, want the edit saved")
	}
}
