package view

import (
	"strings"
	"testing"
)

func TestCellMeasuresDisplayWidth(t *testing.T) {
	for _, text := range []string{"田中太郎", "🤖bot", "alice", "田中太郎田中太郎", "a🤖b"} {
		if got := displayWidth(cell(text, 10)); got != 10 {
			t.Errorf("cell(%q, 10) is %d cells wide, want 10", text, got)
		}
	}
	for _, c := range []struct{ text, want string }{
		{"alice", "alice     "},
		{"alexandria-long", "alexandri…"},
		{"田中", "田中      "},
	} {
		if got := cell(c.text, 10); got != c.want {
			t.Errorf("cell(%q, 10) = %q, want %q", c.text, got, c.want)
		}
	}
}

// Tern draws a glyph the font lacks from a fallback font whose advance is not the character cell's, so a CJK owner padded to
// the column's width in cells comes out narrower than its header. The row's own role names the column widths, the style sheet
// boxes each cell to them, and the owner is cut but not padded.
func TestOwnerCellIsABoxOfTheColumnWidthWhateverItsGlyphs(t *testing.T) {
	cols := listColumns{ownerShown: true, id: 6, owner: 10}
	if got, want := cols.role(), "tern-kata.row tern-kata.id6 tern-kata.ow10"; got != want {
		t.Errorf("role = %q, want %q", got, want)
	}
	if got := (listColumns{id: 6}).role(); strings.Contains(got, ListOwnerRole) {
		t.Errorf("role %q names an owner column that is not drawn", got)
	}
	for _, rule := range []string{
		".sf-item[data-role~='tern-kata.ow10'] .sf-item-value > span:nth-child(2) { display: inline-block; white-space: pre; min-width: 7.2em; }",
		".sf-item[data-role~='tern-kata.id6'] .sf-item-label > span:nth-child(1) { display: inline-block; white-space: pre; min-width: 3.6em; }",
		".sf-item[data-role~='tern-kata.row'] .sf-item-value > span:last-child { display: inline-block; white-space: pre; min-width: 6.0em; }",
	} {
		if !strings.Contains(CSS, rule) {
			t.Errorf("CSS lacks %q", rule)
		}
	}
	for text, want := range map[string]string{"田中太郎": "  田中太郎", "🤖bot": "  🤖bot", "alexandria-long": "  alexandri…"} {
		if got := rightCells("", text, "", cols, "", "")[1].T; got != want {
			t.Errorf("owner cell of %q = %q, want %q", text, got, want)
		}
	}
}

// A column wider than the widest box rule would have no box, so it is held to that width and its cells are cut to it.
func TestColumnsStopAtTheWidestBoxAndCutTheirCells(t *testing.T) {
	cols := listColumns{ownerShown: true, id: 1, owner: ownerWidth}
	cols.fit(maxBoxWidth+30, maxBoxWidth+30)
	if cols.id != maxBoxWidth || cols.owner != maxBoxWidth {
		t.Fatalf("columns id %d, owner %d, want both %d", cols.id, cols.owner, maxBoxWidth)
	}
	for _, token := range strings.Fields(cols.role())[1:] {
		if !strings.Contains(CSS, "[data-role~='"+token+"']") {
			t.Errorf("CSS has no rule for role token %q", token)
		}
	}
	long := strings.Repeat("田", maxBoxWidth)
	if got := displayWidth(cut(long+" until 22:30", cols.owner)); got > maxBoxWidth {
		t.Errorf("owner cell is %d cells wide, want at most %d", got, maxBoxWidth)
	}
}
