package view_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// footerHints are the legend's hints at a Pane cols wide, in reading order, each as its key and label joined.
func footerHints(t *testing.T, cols int) []string {
	t.Helper()
	nodes := render(t, apply(browsing(), model.ResizeMsg{Cols: cols}))
	var hints []string
	for _, cell := range nodes["dock.hints.grid"].C {
		hints = append(hints, text(nodes[cell.ID]))
	}
	return hints
}

// The footer is a key legend: every hint is a muted label after a key span, in a row that wraps instead of a grid counted
// in characters, and from FoldedLegendCols up every width keeps the same hints in the same order.
func TestFooterIsAKeyLegendThatKeepsEveryHintAtAnyWidth(t *testing.T) {
	roomy, narrow := footerHints(t, model.SideBySideCols-1), footerHints(t, model.FoldedLegendCols)
	if len(roomy) < 2 || !slices.Equal(roomy, narrow) {
		t.Errorf("hints differ:\n%q\n%q", narrow, roomy)
	}
	nodes := render(t, apply(browsing(), model.ResizeMsg{Cols: model.FoldedLegendCols}))
	if row := nodes["dock.hints.grid"]; row.P["wrap"] != true {
		t.Errorf("the legend does not wrap: %v", row.P)
	}
	for id, n := range nodes {
		if !strings.HasPrefix(id, "dock.hints.grid.") {
			continue
		}
		spans, _ := n.P["spans"].([]any)
		if len(spans) != 2 {
			t.Fatalf("%s has %d spans", id, len(spans))
		}
		key, label := spans[0].(map[string]any), spans[1].(map[string]any)
		if key["s"] != "key" || label["s"] != "muted" || !strings.HasPrefix(label["t"].(string), " ") {
			t.Errorf("%s spans = %v", id, spans)
		}
	}
}

// Every hint is a node of its own that reads key and label together, so a footer longer than the tree's text cut-off is
// checked by its hint and not by the joined text.
func TestFooterHintReadsKeyAndLabelTogether(t *testing.T) {
	nodes := render(t, apply(browsing(), model.ResizeMsg{Cols: model.MinCols}))
	if got := text(nodes["dock.hints.grid.escape"]); got != "esc Menu" {
		t.Errorf("escape hint reads %q, want %q", got, "esc Menu")
	}
}

// A shifted letter key is drawn apart from the plain one: Tern draws a letter keycap in capitals, so "u" and "shift+u" must
// not both read "U".
func TestFooterDrawsShiftedLettersApartFromPlainOnes(t *testing.T) {
	for name, m := range map[string]model.Model{
		"list":   apply(browsing(), model.FocusColumnMsg{Column: model.ColumnList}),
		"detail": apply(browsing(), model.FocusColumnMsg{Column: model.ColumnDetail}),
	} {
		seen := map[string]string{}
		for id, n := range render(t, m) {
			if !strings.HasPrefix(id, "dock.hints.grid.") {
				continue
			}
			spans, _ := n.P["spans"].([]any)
			key := strings.ToUpper(spans[0].(map[string]any)["t"].(string))
			if other, dup := seen[key]; dup {
				t.Errorf("%s: hints %s and %s both draw the key %q", name, other, id, key)
			}
			seen[key] = id
		}
		if len(seen) < 2 {
			t.Errorf("%s: only %d hints", name, len(seen))
		}
	}
}
