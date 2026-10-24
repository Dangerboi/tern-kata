package view_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
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
	for _, want := range []string{"Global", "Graph", "Detail", "Children", "Forms", "Filters"} {
		i := slices.Index(lines[at:], want)
		if i < 0 {
			t.Fatalf("heading %q missing or out of order in %q", want, lines)
		}
		at += i + 1
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"?                  help", "q/ctrl+c           quit (q asks y/n first, ctrl+c quits at once)", "j/↓/C-n       down", "pgup/M-v      page up", "I                  inbox", "1/2/3/4/5/6/T/C/F  switch Page", "g/home/C-M-<  first", "G/end/C-M->   last", "↹/C-j", "shift+tab/C-k", "set priority (! then 0-4 or - sets or clears it at once)", "⌫", "expand/collapse all", "toggle flat/nested view", "jump to referenced issue", "new child", "reset filter form", "reply to comment", "cycle reply kind", "make or change recurrence", "set parent", "projects", "menu", "Lettered Pages", "back to the Page shown before", "read the credentials again", "read the federation status again", "show the spoke's detail"} {
		if !strings.Contains(joined, want) {
			t.Errorf("row %q missing in %q", want, lines)
		}
	}
	for _, gone := range []string{"daemons", "Tern headless", "Everywhere", "Issue list", "Menu sidebar"} {
		if strings.Contains(joined, gone) {
			t.Errorf("%q listed although tern-kata has no such binding", gone)
		}
	}
	for id, n := range nodes {
		if strings.HasPrefix(id, "main.help") {
			continue
		}
		if n.K == "table" || strings.HasPrefix(id, "dock") || strings.HasPrefix(id, "layer") {
			t.Errorf("%s drawn beside the help sheet", id)
		}
	}
	// The top bar and the columns stay mounted, hidden, so Tern keeps the scroll offsets for when the sheet closes.
	for _, id := range []string{"main.topbar", "main.columns"} {
		if nodes[id].P["hidden"] != true {
			t.Errorf("%s is not hidden beside the help sheet", id)
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
