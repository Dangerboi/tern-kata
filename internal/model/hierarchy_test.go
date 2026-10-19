package model_test

import (
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func hierarchySnapshot() model.Snapshot {
	return model.Snapshot{
		Issues: []model.Issue{
			{UID: "parent", ShortID: "same", ProjectID: 1, Title: "Parent", UpdatedAt: ago(1)},
			{UID: "child", ShortID: "same", ProjectID: 2, Title: "Child", UpdatedAt: ago(2)},
			{UID: "grandchild", Title: "Grandchild", UpdatedAt: ago(3)},
			{UID: "root", Title: "Outside parent", UpdatedAt: ago(4)},
		},
		Links: []model.Link{
			{Type: "parent", FromIssueUID: "child", ToIssueUID: "parent"},
			{Type: "parent", FromIssueUID: "grandchild", ToIssueUID: "child"},
			{Type: "parent", FromIssueUID: "root", ToIssueUID: "outside"},
		},
	}
}

func TestHierarchyRows(t *testing.T) {
	m, cmds := loadFirst(model.Model{}, model.ProjectLoaded{}, model.SnapshotLoaded{Snapshot: hierarchySnapshot()})
	rows := m.VisibleRows()
	var uids []string
	for _, row := range rows {
		uids = append(uids, row.UID)
	}
	if !reflect.DeepEqual(uids, []string{"parent", "root"}) || m.SelectedUID != "parent" || !reflect.DeepEqual(cmds, []model.Cmd{model.Subscribe{Generation: 1}, model.LoadDetail{UID: "parent"}}) {
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
		name     string
		keys     []string
		uids     []string
		depths   []int
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
			m, _ := loadFirst(model.Model{}, model.ProjectLoaded{}, model.SnapshotLoaded{Snapshot: hierarchySnapshot()})
			for _, chord := range tt.keys {
				var cmds []model.Cmd
				previousSelection := m.SelectedUID
				m, cmds = model.Update(m, model.KeyMsg{Chord: chord})
				var want []model.Cmd
				if m.SelectedUID != previousSelection {
					want = []model.Cmd{model.LoadDetail{UID: m.SelectedUID}}
				}
				if !reflect.DeepEqual(cmds, want) {
					t.Fatalf("%s requested commands %#v, want %#v", chord, cmds, want)
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

// A middle child continues its ancestor's line; the last child of each level ends it.
func TestHierarchyGuides(t *testing.T) {
	snap := model.Snapshot{
		Issues: []model.Issue{
			{UID: "p", Title: "P", UpdatedAt: ago(1)},
			{UID: "a", Title: "A", UpdatedAt: ago(2)},
			{UID: "a1", Title: "A1", UpdatedAt: ago(3)},
			{UID: "b", Title: "B", UpdatedAt: ago(4)},
			{UID: "b1", Title: "B1", UpdatedAt: ago(5)},
		},
		Links: []model.Link{
			{Type: "parent", FromIssueUID: "a", ToIssueUID: "p"},
			{Type: "parent", FromIssueUID: "a1", ToIssueUID: "a"},
			{Type: "parent", FromIssueUID: "b", ToIssueUID: "p"},
			{Type: "parent", FromIssueUID: "b1", ToIssueUID: "b"},
		},
	}
	m, _ := loadFirst(model.Model{}, model.ProjectLoaded{}, model.SnapshotLoaded{Snapshot: snap})
	m, _ = model.Update(m, model.KeyMsg{Chord: "shift+e"})
	got := map[string]string{}
	for _, row := range m.VisibleRows() {
		got[row.UID] = row.Guide
	}
	want := map[string]string{"p": "", "a": "├─\u00a0", "a1": "│\u00a0\u00a0└─\u00a0", "b": "└─\u00a0", "b1": "\u00a0\u00a0\u00a0└─\u00a0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("guides = %q, want %q", got, want)
	}
}

func TestHierarchyViewAndRefresh(t *testing.T) {
	refresh := hierarchySnapshot()
	refresh.Issues[0].ShortID = "renamed"
	refresh.Issues[3].UpdatedAt = ago(0)
	refresh.Issues[0], refresh.Issues[3] = refresh.Issues[3], refresh.Issues[0]
	for _, tt := range []struct {
		name     string
		msgs     []model.Msg
		uids     []string
		selected string
		flat     bool
	}{
		{"flat shows all", []model.Msg{model.KeyMsg{Chord: "v"}}, []string{"parent", "child", "grandchild", "root"}, "parent", true},
		{"flat expansion no-op", []model.Msg{model.KeyMsg{Chord: "v"}, model.KeyMsg{Chord: "space"}, model.KeyMsg{Chord: "right"}, model.KeyMsg{Chord: "left"}, model.KeyMsg{Chord: "shift+e"}}, []string{"parent", "child", "grandchild", "root"}, "parent", true},
		{"nested resets expansion and selects ancestor", []model.Msg{model.KeyMsg{Chord: "shift+e"}, model.KeyMsg{Chord: "j"}, model.KeyMsg{Chord: "j"}, model.KeyMsg{Chord: "v"}, model.KeyMsg{Chord: "v"}}, []string{"parent", "root"}, "parent", false},
		{"refresh preserves UID expansion and selection", []model.Msg{model.KeyMsg{Chord: "shift+e"}, model.KeyMsg{Chord: "j"}, snap{model.SnapshotLoaded{Snapshot: refresh}}}, []string{"root", "parent", "child", "grandchild"}, "child", false},
		{"pointer cannot select hidden row", []model.Msg{model.SelectMsg{UID: "grandchild"}}, []string{"parent", "root"}, "parent", false},
		{"pointer selects visible child", []model.Msg{model.KeyMsg{Chord: "right"}, model.SelectMsg{UID: "child"}}, []string{"parent", "child", "root"}, "child", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := loadFirst(model.Model{}, model.ProjectLoaded{}, model.SnapshotLoaded{Snapshot: hierarchySnapshot()})
			for _, msg := range tt.msgs {
				var cmds []model.Cmd
				previousSelection := m.SelectedUID
				m, cmds = step(m, msg)
				var want []model.Cmd
				if _, refreshed := msg.(snap); m.SelectedUID != previousSelection || refreshed {
					// A refresh without the selected authority reloads the shown Issue.
					want = []model.Cmd{model.LoadDetail{UID: m.SelectedUID}}
				}
				if !reflect.DeepEqual(cmds, want) {
					t.Fatalf("%#v requested commands %#v, want %#v", msg, cmds, want)
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
		name     string
		snapshot model.Snapshot
		want     []string
	}{
		{"empty", model.Snapshot{}, nil},
		{"cycle", model.Snapshot{
			Issues: []model.Issue{{UID: "a"}, {UID: "b"}},
			Links: []model.Link{
				{Type: "parent", FromIssueUID: "a", ToIssueUID: "b"},
				{Type: "parent", FromIssueUID: "b", ToIssueUID: "a"},
			},
		}, []string{"a", "b"}},
		{"unrelated links do not nest", model.Snapshot{
			Issues: []model.Issue{{UID: "a"}, {UID: "b"}},
			Links:  []model.Link{{Type: "blocks", FromIssueUID: "a", ToIssueUID: "b"}},
		}, []string{"a", "b"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := loadFirst(model.Model{}, model.ProjectLoaded{}, model.SnapshotLoaded{Snapshot: tt.snapshot})
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
	m, _ := loadFirst(model.Model{}, model.ProjectLoaded{}, model.SnapshotLoaded{Snapshot: hierarchySnapshot()})
	m, _ = model.Update(m, model.KeyMsg{Chord: "right"})
	collapsed, _ := model.Update(m, model.KeyMsg{Chord: "left"})
	if !m.ExpandedUIDs["parent"] || collapsed.ExpandedUIDs["parent"] {
		t.Fatalf("Update mutated previous expansion state: previous=%v next=%v", m.ExpandedUIDs, collapsed.ExpandedUIDs)
	}
}

func TestHierarchyLiveRefreshSelectsVisibleAncestor(t *testing.T) {
	m, _ := loadFirst(model.Model{}, model.ProjectLoaded{}, model.SnapshotLoaded{Snapshot: hierarchySnapshot()})
	m, _ = model.Update(m, model.KeyMsg{Chord: "shift+e"})
	m, _ = model.Update(m, model.SelectMsg{UID: "grandchild"})
	m, cmds := model.Update(m, model.StreamEvent{Generation: m.StreamGeneration, ID: 1})
	if len(cmds) != 1 {
		t.Fatalf("event commands=%#v", cmds)
	}
	refresh, ok := cmds[0].(model.ReadSnapshot)
	if !ok {
		t.Fatalf("event command=%#v", cmds[0])
	}
	snapshot := hierarchySnapshot()
	snapshot.Issues = append(snapshot.Issues[:2], snapshot.Issues[3:]...)
	snapshot.Cursor = 1
	m, cmds = model.Update(m, model.SnapshotLoaded{Request: refresh.Request, Query: refresh.Query, Snapshot: snapshot})
	if m.SelectedUID != "parent" || !m.ExpandedUIDs["parent"] || !m.ExpandedUIDs["child"] || !reflect.DeepEqual(cmds, []model.Cmd{model.LoadDetail{UID: "parent"}}) {
		t.Fatalf("refreshed selection=%q expansions=%v commands=%#v", m.SelectedUID, m.ExpandedUIDs, cmds)
	}
}

// The kids cell is the daemon's count of direct children, not a count over the
// Issues the filters left in the list.
func TestKidsAreTheDaemonsCountsWhateverTheFilters(t *testing.T) {
	snapshot := model.Snapshot{Issues: []model.Issue{{UID: "parent", Status: "open"}, {UID: "leaf", Status: "open"}}, ChildCounts: map[string]model.ChildCounts{"parent": {Open: 2, Total: 6}}}
	kids := func(m model.Model) [2]int {
		var got [2]int
		for _, row := range m.VisibleRows() {
			if row.HasChildren || (row.UID == "leaf" && row.TotalKids != 0) {
				t.Fatalf("row %+v: no child is loaded, so nothing expands", row)
			}
			if row.UID == "parent" {
				got = [2]int{row.OpenKids, row.TotalKids}
			}
		}
		return got
	}
	m, _ := loadFirst(model.Model{}, model.ProjectLoaded{}, model.SnapshotLoaded{Snapshot: snapshot})
	if got := kids(m); got != [2]int{2, 6} {
		t.Fatalf("project load: kids %v, want 2/6", got)
	}
	m.Collection = model.Collection{View: "all-open", Title: "All Open"}
	m, _ = model.Update(m, model.SnapshotLoaded{Request: m.RefreshRequest, Query: m.SnapshotQuery(), Snapshot: model.Snapshot{Issues: snapshot.Issues, ChildCounts: map[string]model.ChildCounts{"parent": {Open: 1, Total: 6}}}})
	if got := kids(m); got != [2]int{1, 6} {
		t.Fatalf("collection load: kids %v, want 1/6", got)
	}
	m, _ = model.Update(m, model.SnapshotLoaded{Request: m.RefreshRequest, Query: m.SnapshotQuery(), Snapshot: model.Snapshot{Issues: snapshot.Issues}})
	if got := kids(m); got != [2]int{1, 6} {
		t.Fatalf("a load that read no counts keeps the ones on screen: kids %v", got)
	}
}
