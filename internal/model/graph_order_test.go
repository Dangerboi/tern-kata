package model_test

import (
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func siblingsSnapshot(links ...model.Link) model.Snapshot {
	snap := model.Snapshot{Issues: []model.Issue{{UID: "p", Title: "P", UpdatedAt: ago(0)}}}
	for i, uid := range []string{"a", "b", "c", "d", "e"} {
		snap.Issues = append(snap.Issues, model.Issue{UID: uid, Title: uid, UpdatedAt: ago(i + 1)})
		snap.Links = append(snap.Links, model.Link{Type: "parent", FromIssueUID: uid, ToIssueUID: "p"})
	}
	snap.Links = append(snap.Links, links...)
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

// A blocked_by link runs from the blocked Issue to its blocker, the other way round from a blocks link.
func TestGraphOrderReadsBlockedByLinks(t *testing.T) {
	snap := siblingsSnapshot(model.Link{Type: "blocked_by", FromIssueUID: "a", ToIssueUID: "e"})
	m, _ := loadFirst(model.Model{}, model.ProjectLoaded{}, model.SnapshotLoaded{Snapshot: snap})
	m, _ = model.Update(m, model.KeyMsg{Chord: "right"})
	m, _ = model.Update(m, model.KeyMsg{Chord: "o"})
	if got := childOrder(m); got != "bcdea" {
		t.Fatalf("order = %q", got)
	}
}

func TestGraphOrderKeepsSelection(t *testing.T) {
	snap := siblingsSnapshot(model.Link{Type: "blocks", FromIssueUID: "e", ToIssueUID: "a"})
	m, _ := loadFirst(model.Model{}, model.ProjectLoaded{}, model.SnapshotLoaded{Snapshot: snap})
	m, _ = model.Update(m, model.KeyMsg{Chord: "right"})
	m, _ = model.Update(m, model.SelectMsg{UID: "c"})
	m, _ = model.Update(m, model.KeyMsg{Chord: "o"})
	if m.SelectedUID != "c" {
		t.Fatalf("selected %q", m.SelectedUID)
	}
}
