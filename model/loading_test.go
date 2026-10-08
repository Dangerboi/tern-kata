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
	m, _ := model.Update(model.Model{Cols: 160, Workspace: "/repo"}, model.ProjectLoaded{
		Project:  model.Project{UID: "project", Name: "work"},
		Snapshot: g.UISnapshotResponseBody{Collection: issues},
	})
	return m
}

func authority(uid string) *g.UISelectedAuthority {
	return &g.UISelectedAuthority{Issue: &model.Issue{UID: uid}}
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

// The detail of Issue a is on screen; msgs follow, and onScreen is the Issue
// whose detail is shown after them ("" for none).
func TestDetailStaysOnScreenUntilTheSelectedIssueArrives(t *testing.T) {
	failure := errors.New("kata server not responding")
	detailTop := model.Scroll{Column: model.ColumnDetail, Start: true}
	for _, tt := range []struct {
		name     string
		msgs     []model.Msg
		onScreen string
		loading  bool
		err      error
		wantCmds []model.Cmd // requested by the last Msg
	}{
		{"selecting b", []model.Msg{model.SelectMsg{UID: "b"}}, "a", true, nil, []model.Cmd{model.LoadDetail{Workspace: "/repo", UID: "b"}}},
		{"b arrives", []model.Msg{model.SelectMsg{UID: "b"}, model.DetailLoaded{UID: "b", Selected: authority("b")}}, "b", false, nil, []model.Cmd{detailTop}},
		{"c replaces b, b arrives late", []model.Msg{model.SelectMsg{UID: "b"}, model.SelectMsg{UID: "c"}, model.DetailLoaded{UID: "b", Selected: authority("b")}}, "a", true, nil, nil},
		{"c arrives after b", []model.Msg{model.SelectMsg{UID: "b"}, model.SelectMsg{UID: "c"}, model.DetailLoaded{UID: "b", Selected: authority("b")}, model.DetailLoaded{UID: "c", Selected: authority("c")}}, "c", false, nil, []model.Cmd{detailTop}},
		{"b fails", []model.Msg{model.SelectMsg{UID: "b"}, model.DetailFailed{UID: "b", Err: failure}}, "", false, failure, nil},
		{"a reloads after saving a Draft", []model.Msg{model.DraftSaved{UID: "a"}}, "a", true, nil, []model.Cmd{model.LoadDetail{Workspace: "/repo", UID: "a"}}},
		{"empty Collection", []model.Msg{model.CollectionLoaded{Collection: model.Collection{View: "project", ProjectUID: "project", Title: "work"}, Snapshot: g.UISnapshotResponseBody{}}}, "", false, nil, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := model.Update(projectOf("a", "b", "c"), model.DetailLoaded{UID: "a", Selected: authority("a")})
			var cmds []model.Cmd
			for _, msg := range tt.msgs {
				m, cmds = model.Update(m, msg)
			}
			if m.DetailIssueUID() != tt.onScreen || m.DetailLoading != tt.loading || m.DetailErr != tt.err || !reflect.DeepEqual(cmds, tt.wantCmds) {
				t.Fatalf("detail=%q loading=%v err=%v cmds=%#v", m.DetailIssueUID(), m.DetailLoading, m.DetailErr, cmds)
			}
		})
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
	m, _ = model.Update(m, model.DraftSaved{UID: "a"})
	m, _ = model.Update(m, model.DetailLoaded{UID: "a", Selected: a})
	if m.DetailIssueUID() != "a" || m.DetailSection != model.SectionComments || m.DetailIndex != 1 {
		t.Fatalf("reload moved the reader: issue=%q section=%v index=%d", m.DetailIssueUID(), m.DetailSection, m.DetailIndex)
	}

	// Another Issue starts at the top, while the previous detail stays until it arrives.
	m, _ = model.Update(m, model.SelectMsg{UID: "b"})
	if m.DetailIssueUID() != "a" || m.DetailSection != model.SectionProperties || m.DetailIndex != 0 {
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

// The detail column keeps its scroll offset from one Issue to the next, so the
// model asks for the top when another Issue's detail replaces the shown one,
// whatever column has the keys, and never when the same Issue refreshes.
func TestDetailScrollsToTopOnlyWhenADifferentIssueReplacesIt(t *testing.T) {
	top := []model.Cmd{model.Scroll{Column: model.ColumnDetail, Start: true}}
	m, _ := model.Update(projectOf("a", "b"), model.DetailLoaded{UID: "a", Selected: authority("a")})

	m, cmds := model.Update(m, model.DetailLoaded{UID: "a", Selected: authority("a")})
	if len(cmds) != 0 {
		t.Fatalf("refreshing the same Issue scrolled: %#v", cmds)
	}

	m, _ = model.Update(m, model.SelectMsg{UID: "b"})
	if m.Focus != model.ColumnList {
		t.Fatalf("fixture: focus %v", m.Focus)
	}
	_, cmds = model.Update(m, model.DetailLoaded{UID: "b", Selected: authority("b")})
	if !reflect.DeepEqual(cmds, top) {
		t.Fatalf("different Issue arriving with the keys in the list: cmds %#v, want %#v", cmds, top)
	}
}
