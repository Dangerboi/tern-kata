package model_test

import (
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/model"
)

func TestPointerHintsUseKeyboardDispatch(t *testing.T) {
	for _, chord := range []string{"n", "x", "/", "?", "page_down"} {
		t.Run(chord, func(t *testing.T) {
			m := model.Model{Project: model.Project{ID: 7}, SelectedUID: "issue", DetailUID: "issue", Issues: []model.Issue{{UID: "issue", Status: "open", ProjectID: 7}}}
			keyboard, keyCmds := model.Update(m, model.KeyMsg{Chord: chord})
			pointer, clickCmds := model.Update(m, model.ActionMsg{Action: "key", Value: chord})
			if !reflect.DeepEqual(pointer, keyboard) || !reflect.DeepEqual(clickCmds, keyCmds) {
				t.Fatalf("pointer differs from keyboard: pointer=%+v %#v keyboard=%+v %#v", pointer, clickCmds, keyboard, keyCmds)
			}
		})
	}
}

func TestParentPointerSelectsAndTogglesExpansion(t *testing.T) {
	parent := "parent"
	m, _ := model.Update(model.Model{SelectedUID: "root", DetailUID: "root"}, model.ProjectLoaded{Snapshot: hierarchySnapshot()})
	m, _ = model.Update(m, model.ActionMsg{Action: model.ToggleExpansion, Value: parent})
	if m.SelectedUID != parent || !m.ExpandedUIDs[parent] || len(m.VisibleRows()) != 3 {
		t.Fatalf("pointer did not expand parent: selected=%q expanded=%v rows=%v", m.SelectedUID, m.ExpandedUIDs, m.VisibleRows())
	}
	m, _ = model.Update(m, model.ActionMsg{Action: model.ToggleExpansion, Value: parent})
	if m.ExpandedUIDs[parent] || len(m.VisibleRows()) != 2 {
		t.Fatalf("pointer did not collapse parent: expanded=%v rows=%v", m.ExpandedUIDs, m.VisibleRows())
	}
}
