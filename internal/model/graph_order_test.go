package model_test

import (
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// siblingsSnapshot has a parent p with children a..e in creation order, and the given blocks links among them
// (each pair is blocker, blocked).
func siblingsSnapshot(blocks ...[2]string) model.Snapshot {
	snap := model.Snapshot{Issues: []model.Issue{{UID: "p", Title: "P", UpdatedAt: ago(0)}}}
	for i, uid := range []string{"a", "b", "c", "d", "e"} {
		snap.Issues = append(snap.Issues, model.Issue{UID: uid, Title: uid, UpdatedAt: ago(i + 1)})
		snap.Links = append(snap.Links, model.Link{Type: "parent", FromIssueUID: uid, ToIssueUID: "p"})
	}
	for _, b := range blocks {
		snap.Links = append(snap.Links, model.Link{Type: "blocks", FromIssueUID: b[0], ToIssueUID: b[1]})
	}
	return snap
}

func childOrder(m model.Model) string {
	var uids []string
	for _, row := range m.VisibleRows() {
		if row.Depth == 1 {
			uids = append(uids, row.UID)
		}
	}
	return strings.Join(uids, "")
}

func TestGraphOrder(t *testing.T) {
	for _, tt := range []struct {
		name        string
		blocks      [][2]string
		topological string
	}{
		{"unlinked siblings keep creation order", nil, "abcde"},
		{"chain puts blockers first", [][2]string{{"e", "d"}, {"d", "c"}, {"c", "b"}}, "aedcb"},
		{"diamond", [][2]string{{"d", "b"}, {"d", "c"}, {"b", "a"}, {"c", "a"}}, "dbcae"},
		{"cycle stays in creation order after the rest", [][2]string{{"b", "c"}, {"c", "b"}, {"e", "a"}}, "deabc"},
		{"links outside the siblings are ignored", [][2]string{{"p", "a"}, {"a", "zzz"}}, "abcde"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := loadFirst(model.Model{}, model.ProjectLoaded{}, model.SnapshotLoaded{Snapshot: siblingsSnapshot(tt.blocks...)})
			m, _ = model.Update(m, model.KeyMsg{Chord: "right"})
			if got := childOrder(m); got != "abcde" {
				t.Fatalf("creation order = %q", got)
			}
			m, _ = model.Update(m, model.KeyMsg{Chord: "o"})
			if got := childOrder(m); got != tt.topological || m.Notice != "graph order: topological" {
				t.Fatalf("topological = %q notice %q, want %q", got, m.Notice, tt.topological)
			}
			m, _ = model.Update(m, model.KeyMsg{Chord: "o"})
			if got := childOrder(m); got != "abcde" || m.Notice != "graph order: temporal" {
				t.Fatalf("toggled back = %q notice %q", got, m.Notice)
			}
		})
	}
}

func TestGraphOrderBlockedByLink(t *testing.T) {
	snap := siblingsSnapshot()
	snap.Links = append(snap.Links, model.Link{Type: "blocked_by", FromIssueUID: "a", ToIssueUID: "e"})
	m, _ := loadFirst(model.Model{}, model.ProjectLoaded{}, model.SnapshotLoaded{Snapshot: snap})
	m, _ = model.Update(m, model.KeyMsg{Chord: "right"})
	m, _ = model.Update(m, model.KeyMsg{Chord: "o"})
	if got := childOrder(m); got != "bcdea" {
		t.Fatalf("order = %q", got)
	}
}

func TestGraphOrderKeepsSelection(t *testing.T) {
	m, _ := loadFirst(model.Model{}, model.ProjectLoaded{}, model.SnapshotLoaded{Snapshot: siblingsSnapshot([2]string{"e", "a"})})
	m, _ = model.Update(m, model.KeyMsg{Chord: "right"})
	m, _ = model.Update(m, model.SelectMsg{UID: "c"})
	m, _ = model.Update(m, model.KeyMsg{Chord: "o"})
	if m.SelectedUID != "c" {
		t.Fatalf("selected %q", m.SelectedUID)
	}
}

func TestGraphOrderOutsideNestedList(t *testing.T) {
	m, _ := loadFirst(model.Model{}, model.ProjectLoaded{}, model.SnapshotLoaded{Snapshot: siblingsSnapshot([2]string{"e", "a"})})
	flat, _ := model.Update(m, model.KeyMsg{Chord: "v"})
	flat, _ = model.Update(flat, model.KeyMsg{Chord: "o"})
	if flat.GraphOrder || flat.Notice != "" {
		t.Fatalf("flat view: order %v notice %q", flat.GraphOrder, flat.Notice)
	}
	m.Cols, m.Focus = 160, model.ColumnDetail
	after, _ := model.Update(m, model.KeyMsg{Chord: "o"})
	if after.GraphOrder || after.Notice != "" {
		t.Fatalf("detail: order %v notice %q", after.GraphOrder, after.Notice)
	}
}
