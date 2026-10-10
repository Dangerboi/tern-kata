package view_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// sheetLines is the text of every node under id, in drawing order.
func (s shown) sheetLines(id string) []string {
	var lines []string
	var walk func(n tern.WireNode)
	walk = func(n tern.WireNode) {
		if v := text(n); v != "" {
			lines = append(lines, v)
		}
		for _, c := range n.C {
			walk(s[c.ID])
		}
	}
	walk(s[id])
	return lines
}

func TestHelpIsAFullPaneSheet(t *testing.T) {
	m, _ := model.Update(pickerLoaded(true), model.KeyMsg{Chord: "?"})
	nodes := render(t, m)
	lines := nodes.sheetLines("main.help")
	if len(lines) < 4 || lines[0] != "tern-kata — keybindings" || lines[len(lines)-1] != "press ? to return" {
		t.Fatalf("sheet = %q", lines)
	}
	at := 0
	for _, want := range []string{"Everywhere", "Issue list", "Menu sidebar", "Issue detail", "Search", "Search results", "Create Issue", "Draft editor", "Discard prompt", "Draft conflict", "Filters", "Error", "Help", "Close Issue", "Label / owner / priority", "Menu", "New Project"} {
		i := slices.Index(lines[at:], want)
		if i < 0 {
			t.Fatalf("heading %q missing or out of order in %q", want, lines)
		}
		at += i + 1
	}
	padded := false
	for _, l := range lines {
		if strings.HasPrefix(l, "j/↓") && strings.HasSuffix(l, "  next Issue") {
			padded = true
		}
	}
	if !padded {
		t.Errorf("no padded `key  action` line for next Issue in %q", lines)
	}
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "Tern headless") || strings.Contains(joined, "Close (esc)") {
		t.Error("headless chords or the Close button listed")
	}
	if strings.Count(joined, "next Issue") != 1 {
		t.Errorf("next Issue rows not merged: %d", strings.Count(joined, "next Issue"))
	}
	for id, n := range nodes {
		if strings.HasPrefix(id, "main.help") {
			continue
		}
		if n.K == "table" || strings.HasPrefix(id, "dock") || strings.HasPrefix(id, "layer") || strings.HasPrefix(id, "main.columns") || strings.HasPrefix(id, "main.top") {
			t.Errorf("%s drawn beside the help sheet", id)
		}
	}
}

func TestHelpClosesWithQuestionMarkAndEscape(t *testing.T) {
	for _, chord := range []string{"?", "escape"} {
		open := apply(pickerLoaded(true), model.KeyMsg{Chord: "?"})
		closed := apply(open, model.KeyMsg{Chord: chord})
		want := render(t, pickerLoaded(true))
		got := render(t, closed)
		sameIDs(t, "main", want, got)
		if closed.Focus != pickerLoaded(true).Focus || closed.SelectedUID != pickerLoaded(true).SelectedUID {
			t.Errorf("%s moved focus or selection", chord)
		}
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
