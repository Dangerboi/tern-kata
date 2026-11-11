package driver_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

// legendHintClick is the click act of the legend's hint of chord, empty when the legend does not draw it.
func legendHintClick(t *testing.T, s driver.State, chord string) string {
	t.Helper()
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if n.ID == "dock.hints.grid."+chord {
			actions, _ := n.P["actions"].(map[string]any)
			act, _ := actions["click"].(string)
			return act
		}
	}
	return ""
}

func unfolded(t *testing.T) driver.State {
	t.Helper()
	s := feed(t, browsing(t, 47), click("dock.hints.keys", view.LegendClick())).State
	if !s.Model.LegendOpen {
		t.Fatal("the keys chip did not unfold the legend")
	}
	return s
}

func TestKeysChipUnfoldsTheLegendAndFoldsItAgain(t *testing.T) {
	s := browsing(t, 47)
	if legendHintClick(t, s, "n") != "" {
		t.Fatal("the folded legend draws the n hint")
	}
	s = unfolded(t)
	if legendHintClick(t, s, "n") != view.KeyClick("n") {
		t.Errorf("the unfolded legend has no n hint")
	}
	s = feed(t, s, click("dock.hints.keys", view.LegendClick())).State
	if s.Model.LegendOpen || legendHintClick(t, s, "n") != "" {
		t.Error("the chip did not fold the legend")
	}
}

func TestTappingAHintRunsItsKeyAndFoldsTheLegend(t *testing.T) {
	s := unfolded(t)
	s = feed(t, s, click("dock.hints.grid.n", legendHintClick(t, s, "n"))).State
	if s.Model.TopCard() != model.CardCreate || s.Model.LegendOpen {
		t.Errorf("card %v, legend open %v; want the new Issue card and a folded legend", s.Model.TopCard(), s.Model.LegendOpen)
	}
}

func TestChangingModeFoldsTheLegend(t *testing.T) {
	s := feed(t, unfolded(t), model.FocusColumnMsg{Column: model.ColumnDetail}).State
	if s.Model.CurrentMode() == "" || s.Model.LegendOpen {
		t.Errorf("legend open %v after a change of mode", s.Model.LegendOpen)
	}
}
