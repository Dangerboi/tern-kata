package view

import "testing"

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
