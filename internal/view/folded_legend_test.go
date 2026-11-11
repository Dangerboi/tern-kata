package view_test

import (
	"maps"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

// legendOf is the legend's hints, each hint's text with its click, and the ids of the legend's other nodes.
func legendOf(t *testing.T, m model.Model) (hints map[string]string, order []string, nodes shown) {
	t.Helper()
	nodes = render(t, m)
	hints = map[string]string{}
	for _, cell := range nodes["dock.hints.grid"].C {
		click, _ := nodes[cell.ID].P["actions"].(map[string]any)["click"].(string)
		hints[text(nodes[cell.ID])] = click
		order = append(order, text(nodes[cell.ID]))
	}
	return hints, order, nodes
}

func legendModes(t *testing.T) map[string]model.Model {
	return map[string]model.Model{
		"list":   apply(browsing(), model.FocusColumnMsg{Column: model.ColumnList}),
		"detail": apply(browsing(), model.FocusColumnMsg{Column: model.ColumnDetail}),
		"card":   apply(browsing(), model.KeyMsg{Chord: "n"}),
	}
}

func TestFoldedLegendIsOneLineOfCounterEscAndKeysChip(t *testing.T) {
	for name, m := range legendModes(t) {
		m = apply(m, model.ResizeMsg{Cols: model.FoldedLegendCols - 1})
		hints, _, nodes := legendOf(t, m)
		if len(hints) > 1 {
			t.Errorf("%s: folded legend holds %q", name, hints)
		}
		if _, ok := nodes["dock.hints.position"]; !ok {
			t.Errorf("%s: no counter", name)
		}
		chip := nodes["dock.hints.keys"]
		if chip.K != "button" || text(chip) != "keys" || chip.P["actions"].(map[string]any)["click"] != view.LegendClick() {
			t.Errorf("%s: keys chip = %v", name, chip)
		}
		if esc, ok := nodes["dock.hints.grid.escape"]; ok && esc.P["actions"].(map[string]any)["click"] != view.KeyClick("escape") {
			t.Errorf("%s: esc hint = %v", name, esc)
		}
	}
	list := apply(legendModes(t)["list"], model.ResizeMsg{Cols: model.FoldedLegendCols - 1})
	if hints, _, _ := legendOf(t, list); hints["esc Menu"] != view.KeyClick("escape") {
		t.Errorf("list legend = %q, want the esc Menu hint", hints)
	}
}

func TestUnfoldedLegendHoldsTheFullGridOfItsModeAndAChip(t *testing.T) {
	for name, m := range legendModes(t) {
		want, wantOrder, _ := legendOf(t, apply(m, model.ResizeMsg{Cols: model.FoldedLegendCols}))
		open := apply(m, model.ResizeMsg{Cols: model.FoldedLegendCols - 1}, model.ToggleLegendMsg{})
		got, gotOrder, nodes := legendOf(t, open)
		if len(want) < 2 || !maps.Equal(got, want) || len(gotOrder) != len(wantOrder) {
			t.Errorf("%s: unfolded hints\n%q\nwant\n%q", name, gotOrder, wantOrder)
		}
		if _, ok := nodes["dock.hints.keys"]; !ok {
			t.Errorf("%s: unfolded legend has no chip to fold it", name)
		}
		if _, ok := nodes["dock.hints.position"]; !ok {
			t.Errorf("%s: unfolded legend has no counter", name)
		}
	}
}

func TestLegendFromFoldedLegendColsIsTheGridWithNoChip(t *testing.T) {
	for _, open := range []bool{false, true} {
		m := apply(browsing(), model.ResizeMsg{Cols: model.FoldedLegendCols - 1}, model.ToggleLegendMsg{}, model.ResizeMsg{Cols: model.FoldedLegendCols})
		if open {
			m.LegendOpen = true
		}
		hints, _, nodes := legendOf(t, m)
		if _, ok := nodes["dock.hints.keys"]; ok || len(hints) < 2 {
			t.Errorf("at %d columns the legend has hints %q and chip %v", model.FoldedLegendCols, hints, ok)
		}
	}
}
