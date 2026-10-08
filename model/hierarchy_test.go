package model_test

import (
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

func hierarchySnapshot() g.UISnapshotResponseBody {
	return g.UISnapshotResponseBody{
		Collection: []model.Issue{
			{UID: "parent", ShortID: "same", ProjectID: 1, Title: "Parent"},
			{UID: "child", ShortID: "same", ProjectID: 2, Title: "Child"},
			{UID: "grandchild", Title: "Grandchild"},
			{UID: "root", Title: "Outside parent"},
		},
		CollectionLinks: []g.UILink{
			{Type: "parent", FromIssueUID: "child", ToIssueUID: "parent"},
			{Type: "parent", FromIssueUID: "grandchild", ToIssueUID: "child"},
			{Type: "parent", FromIssueUID: "root", ToIssueUID: "outside"},
		},
	}
}

func TestHierarchyRows(t *testing.T) {
	m, cmds := model.Update(model.Model{}, model.ProjectLoaded{Snapshot: hierarchySnapshot()})
	rows := m.VisibleRows()
	var uids []string
	for _, row := range rows {
		uids = append(uids, row.UID)
	}
	if !reflect.DeepEqual(uids, []string{"parent", "root"}) || m.SelectedUID != "parent" || len(cmds) != 1 {
		t.Fatalf("collapsed rows=%v selected=%q commands=%#v", uids, m.SelectedUID, cmds)
	}
	if _, ok := cmds[0].(model.Subscribe); !ok {
		t.Fatalf("load commands=%#v", cmds)
	}
	if rows[0].Depth != 0 || !rows[0].HasChildren || rows[0].Expanded || rows[1].HasChildren {
		t.Fatalf("row hierarchy: %+v", rows)
	}
}

func TestHierarchyExpansion(t *testing.T) {
	for _, tt := range []struct {
		name string
		keys []string
		uids []string
		depths []int
		selected string
	}{
		{"space expands", []string{"space"}, []string{"parent", "child", "root"}, []int{0, 1, 0}, "parent"},
		{"space collapses", []string{"space", "space"}, []string{"parent", "root"}, []int{0, 0}, "parent"},
		{"right idempotent", []string{"right", "right"}, []string{"parent", "child", "root"}, []int{0, 1, 0}, "parent"},
		{"left idempotent", []string{"right", "left", "left"}, []string{"parent", "root"}, []int{0, 0}, "parent"},
		{"leaf no-op", []string{"shift+g", "space", "right", "left"}, []string{"parent", "root"}, []int{0, 0}, "root"},
		{"visible navigation", []string{"right", "j"}, []string{"parent", "child", "root"}, []int{0, 1, 0}, "child"},
		{"all expands recursively", []string{"shift+e", "j", "j"}, []string{"parent", "child", "grandchild", "root"}, []int{0, 1, 2, 0}, "grandchild"},
		{"all collapses to ancestor", []string{"shift+e", "j", "j", "shift+e"}, []string{"parent", "root"}, []int{0, 0}, "parent"},
		{"partially expanded all", []string{"right", "shift+e"}, []string{"parent", "child", "grandchild", "root"}, []int{0, 1, 2, 0}, "parent"},
		{"descendant expansion retained", []string{"shift+e", "left", "right"}, []string{"parent", "child", "grandchild", "root"}, []int{0, 1, 2, 0}, "parent"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := model.Update(model.Model{}, model.ProjectLoaded{Snapshot: hierarchySnapshot()})
			for _, chord := range tt.keys {
				var cmds []model.Cmd
				m, cmds = model.Update(m, model.KeyMsg{Chord: chord})
				if len(cmds) != 0 {
					t.Fatalf("%s requested commands %#v", chord, cmds)
				}
			}
			var uids []string
			var depths []int
			for _, row := range m.VisibleRows() {
				uids = append(uids, row.UID)
				depths = append(depths, row.Depth)
			}
			if !reflect.DeepEqual(uids, tt.uids) || !reflect.DeepEqual(depths, tt.depths) || m.SelectedUID != tt.selected {
				t.Fatalf("rows=%v depths=%v selected=%q", uids, depths, m.SelectedUID)
			}
		})
	}
}

func TestHierarchyViewAndRefresh(t *testing.T) {
	refresh := hierarchySnapshot()
	refresh.Collection[0].ShortID = "renamed"
	refresh.Collection[0], refresh.Collection[3] = refresh.Collection[3], refresh.Collection[0]
	for _, tt := range []struct {
		name string
		msgs []model.Msg
		uids []string
		selected string
		flat bool
	}{
		{"flat shows all", []model.Msg{model.KeyMsg{Chord: "v"}}, []string{"parent", "child", "grandchild", "root"}, "parent", true},
		{"flat expansion no-op", []model.Msg{model.KeyMsg{Chord: "v"}, model.KeyMsg{Chord: "space"}, model.KeyMsg{Chord: "right"}, model.KeyMsg{Chord: "left"}, model.KeyMsg{Chord: "shift+e"}}, []string{"parent", "child", "grandchild", "root"}, "parent", true},
		{"nested resets expansion and selects ancestor", []model.Msg{model.KeyMsg{Chord: "shift+e"}, model.KeyMsg{Chord: "j"}, model.KeyMsg{Chord: "j"}, model.KeyMsg{Chord: "v"}, model.KeyMsg{Chord: "v"}}, []string{"parent", "root"}, "parent", false},
		{"refresh preserves UID expansion and selection", []model.Msg{model.KeyMsg{Chord: "shift+e"}, model.KeyMsg{Chord: "j"}, model.ProjectLoaded{Snapshot: refresh}}, []string{"root", "parent", "child", "grandchild"}, "child", false},
		{"pointer cannot select hidden row", []model.Msg{model.SelectMsg{UID: "grandchild"}}, []string{"parent", "root"}, "parent", false},
		{"pointer selects visible child", []model.Msg{model.KeyMsg{Chord: "right"}, model.SelectMsg{UID: "child"}}, []string{"parent", "child", "root"}, "child", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := model.Update(model.Model{}, model.ProjectLoaded{Snapshot: hierarchySnapshot()})
			for _, msg := range tt.msgs {
				var cmds []model.Cmd
				m, cmds = model.Update(m, msg)
				if _, loading := msg.(model.ProjectLoaded); loading {
					if len(cmds) != 1 {
						t.Fatalf("refresh commands=%#v", cmds)
					}
					if _, ok := cmds[0].(model.Subscribe); !ok {
						t.Fatalf("refresh commands=%#v", cmds)
					}
				} else if len(cmds) != 0 {
					t.Fatalf("%#v requested commands %#v", msg, cmds)
				}
			}
			var uids []string
			for _, row := range m.VisibleRows() {
				uids = append(uids, row.UID)
				if m.Flat && (row.Depth != 0 || row.HasChildren) {
					t.Fatalf("flat disclosure: %+v", row)
				}
			}
			if !reflect.DeepEqual(uids, tt.uids) || m.SelectedUID != tt.selected || m.Flat != tt.flat {
				t.Fatalf("rows=%v selected=%q flat=%v", uids, m.SelectedUID, m.Flat)
			}
		})
	}
}

func TestHierarchyCyclesAndEmptyLists(t *testing.T) {
	for _, tt := range []struct {
		name string
		snapshot g.UISnapshotResponseBody
		want []string
	}{
		{"empty", g.UISnapshotResponseBody{}, nil},
		{"cycle", g.UISnapshotResponseBody{
			Collection: []model.Issue{{UID: "a"}, {UID: "b"}},
			CollectionLinks: []g.UILink{
				{Type: "parent", FromIssueUID: "a", ToIssueUID: "b"},
				{Type: "parent", FromIssueUID: "b", ToIssueUID: "a"},
			},
		}, []string{"a", "b"}},
		{"unrelated links do not nest", g.UISnapshotResponseBody{
			Collection: []model.Issue{{UID: "a"}, {UID: "b"}},
			CollectionLinks: []g.UILink{{Type: "blocks", FromIssueUID: "a", ToIssueUID: "b"}},
		}, []string{"a", "b"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := model.Update(model.Model{}, model.ProjectLoaded{Snapshot: tt.snapshot})
			for _, chord := range []string{"space", "left", "right", "shift+e", "shift+e"} {
				var cmds []model.Cmd
				m, cmds = model.Update(m, model.KeyMsg{Chord: chord})
				if len(cmds) != 0 {
					t.Fatalf("commands=%#v", cmds)
				}
			}
			var uids []string
			for _, row := range m.VisibleRows() {
				uids = append(uids, row.UID)
			}
			if !reflect.DeepEqual(uids, tt.want) {
				t.Fatalf("rows=%v", uids)
			}
		})
	}
}

func TestHierarchyUpdatePreservesInput(t *testing.T) {
	m, _ := model.Update(model.Model{}, model.ProjectLoaded{Snapshot: hierarchySnapshot()})
	m, _ = model.Update(m, model.KeyMsg{Chord: "right"})
	collapsed, _ := model.Update(m, model.KeyMsg{Chord: "left"})
	if !m.ExpandedUIDs["parent"] || collapsed.ExpandedUIDs["parent"] {
		t.Fatalf("Update mutated previous expansion state: previous=%v next=%v", m.ExpandedUIDs, collapsed.ExpandedUIDs)
	}
}

func TestHierarchyLiveRefreshSelectsVisibleAncestor(t *testing.T) {
	m, _ := model.Update(model.Model{}, model.ProjectLoaded{Snapshot: hierarchySnapshot()})
	m, _ = model.Update(m, model.KeyMsg{Chord: "shift+e"})
	m, _ = model.Update(m, model.SelectMsg{UID: "grandchild"})
	m, cmds := model.Update(m, model.StreamEvent{Generation: m.StreamGeneration, ID: 1})
	if len(cmds) != 1 {
		t.Fatalf("event commands=%#v", cmds)
	}
	refresh, ok := cmds[0].(model.RefreshSnapshot)
	if !ok {
		t.Fatalf("event command=%#v", cmds[0])
	}
	snapshot := hierarchySnapshot()
	snapshot.Collection = append(snapshot.Collection[:2], snapshot.Collection[3:]...)
	snapshot.Cursor = 1
	m, cmds = model.Update(m, model.SnapshotRefreshed{Request: refresh.Request, Query: refresh.Query, Snapshot: snapshot})
	if m.SelectedUID != "parent" || !m.ExpandedUIDs["parent"] || !m.ExpandedUIDs["child"] || len(cmds) != 0 {
		t.Fatalf("refreshed selection=%q expansions=%v commands=%#v", m.SelectedUID, m.ExpandedUIDs, cmds)
	}
}
