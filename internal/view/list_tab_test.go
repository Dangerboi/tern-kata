package view_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestFooterShowsTabDetailOnlyWhenSideBySide(t *testing.T) {
	m := pickerLoaded(true)
	m, _ = model.Update(m, model.KeyMsg{Chord: "j", Text: "j"})
	if _, ok := pickerNodes(t, m)["dock.hints.tab"]; !ok {
		t.Fatal("side by side: no tab hint")
	}
	narrow, _ := model.Update(m, model.ResizeMsg{Cols: 100})
	if _, ok := pickerNodes(t, narrow)["dock.hints.tab"]; ok {
		t.Fatal("single column: tab hint shown")
	}
	hidden, _ := model.Update(m, model.KeyMsg{Chord: "shift+l"})
	if _, ok := pickerNodes(t, hidden)["dock.hints.tab"]; ok {
		t.Fatal("detail hidden: tab hint shown")
	}
}
