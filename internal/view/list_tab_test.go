package view_test

import (
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestFooterShowsTabDetailOnlyWhenSideBySide(t *testing.T) {
	tabHint := func(m model.Model) bool {
		for id := range pickerNodes(t, m) {
			if strings.HasPrefix(id, "dock.hints.grid.") && strings.HasSuffix(id, ".tab") {
				return true
			}
		}
		return false
	}
	m := pickerLoaded(true)
	m, _ = model.Update(m, model.KeyMsg{Chord: "j", Text: "j"})
	if !tabHint(m) {
		t.Fatal("side by side: no tab hint")
	}
	narrow, _ := model.Update(m, model.ResizeMsg{Cols: 100})
	if tabHint(narrow) {
		t.Fatal("single column: tab hint shown")
	}
	hidden, _ := model.Update(m, model.KeyMsg{Chord: "shift+l"})
	if tabHint(hidden) {
		t.Fatal("detail hidden: tab hint shown")
	}
}
