package model_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

// Loads leave what is on screen in place until the new content arrives.

func projectOf(uids ...string) model.Model {
	issues := make([]model.Issue, len(uids))
	for i, uid := range uids {
		issues[i] = model.Issue{UID: uid, Status: "open"}
	}
	m, _ := model.Update(model.Model{Workspace: "/repo"}, model.ProjectLoaded{
		Project:  model.Project{UID: "project", Name: "work"},
		Snapshot: g.UISnapshotResponseBody{Collection: issues},
	})
	return m
}

func TestFirstLoadIsTheOnlyFullPaneLoad(t *testing.T) {
	m, _ := model.Update(model.Model{}, model.Start{Workspace: "/repo"})
	if !m.FirstLoad() {
		t.Fatal("first load not recognised")
	}
	m = projectOf()
	m, _ = model.Update(m, model.CollectionSelected{Collection: m.Collections[1]})
	if !m.Loading || m.FirstLoad() {
		t.Fatalf("switching Collection with an empty list counts as a first load: loading=%v first=%v", m.Loading, m.FirstLoad())
	}
}

func TestSwitchingCollectionKeepsPreviousIssues(t *testing.T) {
	m := projectOf("a", "b")
	m, _ = model.Update(m, model.SelectMsg{UID: "b"})
	allOpen := m.Collections[4]
	m, cmds := model.Update(m, model.CollectionSelected{Collection: allOpen})
	if !m.Loading || len(m.Issues) != 2 || m.SelectedUID != "b" || !reflect.DeepEqual(cmds, []model.Cmd{model.LoadCollection{Workspace: "/repo", Collection: allOpen}}) {
		t.Fatalf("switching Collection: loading=%v issues=%d selected=%q cmds=%#v", m.Loading, len(m.Issues), m.SelectedUID, cmds)
	}
	m, _ = model.Update(m, model.CollectionLoaded{Collection: allOpen, Snapshot: g.UISnapshotResponseBody{Collection: []model.Issue{{UID: "c", Status: "open"}}}})
	if m.Loading || len(m.Issues) != 1 || m.Issues[0].UID != "c" {
		t.Fatalf("new Collection did not replace the Issues: loading=%v issues=%+v", m.Loading, m.Issues)
	}
}

func TestSelectingAnotherIssueKeepsPreviousDetailUntilItArrives(t *testing.T) {
	a := &g.UISelectedAuthority{Issue: &model.Issue{UID: "a", Title: "Alpha"}}
	b := &g.UISelectedAuthority{Issue: &model.Issue{UID: "b", Title: "Beta"}}
	m, _ := model.Update(projectOf("a", "b"), model.DetailLoaded{UID: "a", Selected: a})

	m, cmds := model.Update(m, model.SelectMsg{UID: "b"})
	if m.Detail != a || m.DetailIssueUID() != "a" || !m.DetailLoading || !reflect.DeepEqual(cmds, []model.Cmd{model.LoadDetail{Workspace: "/repo", UID: "b"}}) {
		t.Fatalf("selecting b: detail=%q loading=%v cmds=%#v", m.DetailIssueUID(), m.DetailLoading, cmds)
	}
	m, cmds = model.Update(m, model.DetailLoaded{UID: "b", Selected: b})
	if m.Detail != b || m.DetailIssueUID() != "b" || m.DetailLoading || len(cmds) != 0 {
		t.Fatalf("b arrived: detail=%q loading=%v cmds=%#v", m.DetailIssueUID(), m.DetailLoading, cmds)
	}

	m, _ = model.Update(m, model.CollectionLoaded{Collection: m.Collection, Snapshot: g.UISnapshotResponseBody{}})
	if m.SelectedUID != "" || m.Detail != nil {
		t.Fatalf("an empty list left a detail on screen: selected=%q detail=%q", m.SelectedUID, m.DetailIssueUID())
	}
}

func TestDetailReturnsToFirstSectionOnlyForADifferentIssue(t *testing.T) {
	a := &g.UISelectedAuthority{Issue: &model.Issue{UID: "a"}, Comments: []g.Comment{{UID: "one"}, {UID: "two"}}}
	m, _ := model.Update(projectOf("a", "b"), model.DetailLoaded{UID: "a", Selected: a})
	m, _ = model.Update(m, model.DetailItemMsg{Section: model.SectionComments, Key: "two"})
	if m.DetailSection != model.SectionComments || m.DetailIndex != 1 {
		t.Fatalf("fixture: section=%v index=%d", m.DetailSection, m.DetailIndex)
	}

	// The same Issue reloads (after saving a Draft) where the reader was.
	m, cmds := model.Update(m, model.DraftSaved{UID: "a"})
	if !reflect.DeepEqual(cmds, []model.Cmd{model.LoadDetail{Workspace: "/repo", UID: "a"}}) {
		t.Fatalf("saved Draft: %#v", cmds)
	}
	m, _ = model.Update(m, model.DetailLoaded{UID: "a", Selected: a})
	if m.DetailIssueUID() != "a" || m.DetailSection != model.SectionComments || m.DetailIndex != 1 {
		t.Fatalf("reload moved the reader: issue=%q section=%v index=%d", m.DetailIssueUID(), m.DetailSection, m.DetailIndex)
	}

	// Another Issue starts at the top, while the previous detail stays until it arrives.
	m, _ = model.Update(m, model.SelectMsg{UID: "b"})
	if m.DetailIssueUID() != "a" || m.DetailSection != model.SectionBody || m.DetailIndex != 0 {
		t.Fatalf("selecting b: issue=%q section=%v index=%d", m.DetailIssueUID(), m.DetailSection, m.DetailIndex)
	}
}

func TestPreviousDetailLinksStayRelativeToTheirOwnIssue(t *testing.T) {
	link := g.UILink{ID: 1, Type: "parent", FromIssueUID: "child", ToIssueUID: "a", FromQualifiedID: "work#child", ToQualifiedID: "work#a"}
	m, _ := model.Update(projectOf("a", "b"), model.DetailLoaded{UID: "a", Selected: &g.UISelectedAuthority{Issue: &model.Issue{UID: "a"}, Links: []g.UILink{link}}})
	m, _ = model.Update(m, model.SelectMsg{UID: "b"})

	row, ok := m.DetailRow(model.SectionLinks, 0)
	if !ok || row.Title != "child work#child" || row.ReferenceUID != "child" {
		t.Fatalf("link row while another Issue loads: %+v", row)
	}
}

func TestFailedDetailLoadDoesNotKeepThePreviousIssue(t *testing.T) {
	failure := errors.New("kata server not responding")
	m, _ := model.Update(projectOf("a", "b"), model.DetailLoaded{UID: "a", Selected: &g.UISelectedAuthority{Issue: &model.Issue{UID: "a"}}})
	m, _ = model.Update(m, model.SelectMsg{UID: "b"})
	m, _ = model.Update(m, model.DetailFailed{UID: "b", Err: failure})
	if m.DetailLoading || m.DetailErr != failure || m.Detail != nil {
		t.Fatalf("b failed: loading=%v err=%v detail=%q", m.DetailLoading, m.DetailErr, m.DetailIssueUID())
	}
}
