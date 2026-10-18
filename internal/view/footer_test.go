package view_test

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// footerLines is the footer's hint grid at a Pane cols wide, one string per line.
func footerLines(t *testing.T, cols int) []string {
	t.Helper()
	nodes := render(t, apply(browsing(), model.ResizeMsg{Cols: cols}))
	var lines []string
	for _, row := range nodes["dock.hints.grid"].C {
		var b strings.Builder
		for _, cell := range row.C {
			b.WriteString(text(cell))
		}
		lines = append(lines, b.String())
	}
	return lines
}

// The footer keeps every hint at any width, as kata's TUI does: a Pane too narrow for one line gets more lines, none wider
// than the Pane, with the hints in the same order and each column lined up under the one above.
func TestFooterWrapsItsHintsOntoMoreLinesInsteadOfHidingThem(t *testing.T) {
	hints := func(lines []string) []string {
		return strings.Fields(strings.ReplaceAll(strings.Join(lines, " "), "▕", " "))
	}
	separators := func(line string) (at []int) {
		for i, r := range []rune(line) {
			if r == '▕' {
				at = append(at, i)
			}
		}
		return at
	}
	// Both widths show the list alone, so they have the same hints.
	roomy, narrow := footerLines(t, model.SideBySideCols-1), footerLines(t, model.MinCols)
	if len(narrow) < 2 || len(narrow) < len(roomy) {
		t.Fatalf("%d columns: %d lines, %d columns: %d lines", model.MinCols, len(narrow), model.SideBySideCols-1, len(roomy))
	}
	if !slices.Equal(hints(narrow), hints(roomy)) {
		t.Errorf("hints differ:\n%q\n%q", hints(narrow), hints(roomy))
	}
	first := separators(narrow[0])
	for _, line := range narrow {
		if n := utf8.RuneCountInString(line); n > model.MinCols {
			t.Errorf("line %q is %d wide in a %d-column Pane", line, n, model.MinCols)
		}
		if at := separators(line); !slices.Equal(at, first[:len(at)]) {
			t.Errorf("line %q separators at %v, first line at %v", line, at, first)
		}
	}
}
