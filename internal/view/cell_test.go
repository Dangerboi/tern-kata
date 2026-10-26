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
