package model_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestParentPointerSelectsAndTogglesExpansion(t *testing.T) {
	parent := "parent"
	m, _ := loadFirst(withDetail(model.Model{SelectedUID: "root"}, nil), model.ProjectLoaded{}, model.SnapshotLoaded{Snapshot: hierarchySnapshot()})
	m, _ = model.Update(m, model.ActionMsg{Action: model.ToggleExpansion, Value: parent})
	if m.SelectedUID != parent || !m.ExpandedUIDs[parent] || len(m.VisibleRows()) != 3 {
		t.Fatalf("pointer did not expand parent: selected=%q expanded=%v rows=%v", m.SelectedUID, m.ExpandedUIDs, m.VisibleRows())
	}
	m, _ = model.Update(m, model.ActionMsg{Action: model.ToggleExpansion, Value: parent})
	if m.ExpandedUIDs[parent] || len(m.VisibleRows()) != 2 {
		t.Fatalf("pointer did not collapse parent: expanded=%v rows=%v", m.ExpandedUIDs, m.VisibleRows())
	}
}
