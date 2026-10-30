package view_test

import (
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

func TestHelpIsAFullPaneSheetOfTheIssuesPage(t *testing.T) {
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
	for _, want := range []string{"?                    help", "q/ctrl+c             quit (q asks y/n first, ctrl+c quits at once)", "j/↓/C-n       down", "pgup/M-v      page up", "I                    inbox", "1/2/3/4/5/6/T/C/F/D  switch Page", "g/home/C-M-<  first", "G/end/C-M->   last", "↹/C-j", "shift+tab/C-k", "set priority (! then 0-4 or - sets or clears it at once)", "⌫", "expand/collapse all", "toggle flat/nested view", "jump to referenced issue", "new child", "reset filter form", "reply to comment", "cycle reply kind", "make or change recurrence", "set parent", "projects", "menu"} {
		if !strings.Contains(joined, want) {
			t.Errorf("row %q missing in %q", want, lines)
		}
	}
	for _, gone := range []string{"Lettered Pages", "read the credentials again", "scroll the inspector", "next run", "Tern headless", "Everywhere", "Issue list", "Menu sidebar"} {
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

// helpOn is the text of the help sheet opened over the Page that the key switches to.
func helpOn(t *testing.T, pageKey string) string {
	t.Helper()
	m := pickerLoaded(true)
	if page, ok := map[string]model.Page{"2": model.PageBoard, "3": model.PageJobs, "4": model.PageRuns, "6": model.PageAttention, "C": model.PageCredentials, "D": model.PageDaemons}[pageKey]; ok {
		m.Page = page
	}
	m = apply(m, model.KeyMsg{Chord: "?"})
	return strings.Join(render(t, m).sheetLines("main.help"), "\n")
}

func TestHelpSheetListsTheGlobalKeysAndTheCurrentPagesKeys(t *testing.T) {
	// own is a line of the Page, other one of a different Page.
	pages := []struct {
		name, key, own string
		others         []string
	}{
		{"Issues", "", "toggle flat/nested view", []string{"next job", "next run", "next request", "show the spoke's detail", "move lanes", "read the credentials again"}},
		{"Board", "2", "toggle Issues and Board Pages", []string{"toggle flat/nested view", "next job", "next run", "next request", "show the spoke's detail"}},
		{"Jobs", "3", "next job", []string{"toggle flat/nested view", "next run", "next request", "toggle Issues and Board Pages"}},
		{"Runs", "4", "next run", []string{"toggle flat/nested view", "next job", "next request", "toggle Issues and Board Pages"}},
		{"Attention", "6", "next request", []string{"toggle flat/nested view", "next job", "next run", "toggle Issues and Board Pages"}},
		{"Credentials", "C", "read the credentials again", []string{"toggle flat/nested view", "next job", "next run", "show the spoke's detail"}},
		{"Daemons", "D", "switch this Pane to the selected daemon", []string{"toggle flat/nested view", "next job", "read the credentials again"}},
	}
	for _, page := range pages {
		sheet := helpOn(t, page.key)
		for _, want := range []string{"help", "quit (q asks y/n first", "inbox", "switch Page", page.own} {
			if !strings.Contains(sheet, want) {
				t.Errorf("%s Page: sheet lacks %q in %q", page.name, want, sheet)
			}
		}
		for _, gone := range page.others {
			if strings.Contains(sheet, gone) {
				t.Errorf("%s Page: sheet lists %q of another Page in %q", page.name, gone, sheet)
			}
		}
	}
}

func TestHelpSheetListsTheAgentKeyOnTheIssuesPageAndTheBoard(t *testing.T) {
	for _, key := range []string{"", "2"} {
		var found string
		for _, line := range strings.Split(helpOn(t, key), "\n") {
			if strings.HasPrefix(line, "w ") {
				found = line
			}
		}
		if found == "" {
			t.Errorf("page key %q: no w line", key)
		}
		// The sheet draws its lines unwrapped: a line longer than the Pane's column clips.
		if n := len([]rune(found)); n > 60 {
			t.Errorf("page key %q: agent line is %d columns: %q", key, n, found)
		}
	}
	for _, line := range strings.Split(helpOn(t, "3"), "\n") {
		if strings.HasPrefix(line, "w ") && strings.Contains(line, "agents") {
			t.Errorf("the agent key is listed on the Jobs Page: %q", line)
		}
	}
}

func TestHelpSheetDropsGroupsWithNoLinesOnThePage(t *testing.T) {
	jobs := helpOn(t, "3")
	for _, gone := range []string{"Graph", "Detail", "Children", "Forms", "Filters", "Board", "Runs", "Lettered Pages"} {
		if slices.Contains(strings.Split(jobs, "\n"), gone) {
			t.Errorf("Jobs Page sheet draws the empty group %q", gone)
		}
	}
}
