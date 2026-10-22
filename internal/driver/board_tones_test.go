package driver_test

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

func node(t *testing.T, nodes []tern.WireNode, id string) tern.WireNode {
	t.Helper()
	at := slices.IndexFunc(nodes, func(n tern.WireNode) bool { return n.ID == id })
	if at < 0 {
		t.Fatalf("the Board draws no node %s", id)
	}
	return nodes[at]
}

// spanStyle is the style of the first span of a text node.
func spanStyle(t *testing.T, n tern.WireNode) string {
	t.Helper()
	spans, _ := n.P["spans"].([]any)
	if len(spans) == 0 {
		t.Fatalf("%s has no spans", n.ID)
	}
	style, _ := spans[0].(map[string]any)["s"].(string)
	return style
}

func TestLaneHeadersTakeTheirOwnTonesAndCustomLanesTakeTheRestInTurn(t *testing.T) {
	s := boardPane(t)
	s = step(t, s, model.LanesLoaded{ProjectUID: "p", Lanes: []string{"needs-triage", "in-review", "ready-for-human", "qa", "blocked", "ship"}}).State
	nodes := wireNodes(t, view.Render(s.Model, now))
	want := []string{"muted", "warning", "accent", "user", "pending", "neutral", "accent", "success"}
	for i, tone := range want {
		head := node(t, nodes, fmt.Sprintf("%s.lane%d.head.title", view.BoardID, i))
		if got := head.P["tone"]; got != tone {
			t.Errorf("column %d header tone = %v, want %s", i, got, tone)
		}
	}
}

func TestCardPartsEachDrawInTheirTone(t *testing.T) {
	open := ownedBy(boardCard("open", "needs-info", "ready-for-agent", "bug"), "ada")
	done := closedAt("done", time.October, 7, "ready-for-human")
	s := boardPane(t, open, done)
	s = step(t, s, model.SelectMsg{UID: "open"}).State
	nodes := wireNodes(t, view.Render(s.Model, now))
	card := func(uid, part string) tern.WireNode {
		id := view.CardID(s.Model, uid)
		if part != "" {
			id += "." + part
		}
		return node(t, nodes, id)
	}

	for _, c := range []struct {
		uid, part, tone string
	}{
		{"open", "id.short", "error"},
		{"open", "owner", "user"},
		{"open", "labels.bug", "info"},
		{"done", "id.short", "success"},
	} {
		if got := card(c.uid, c.part).P["tone"]; got != c.tone {
			t.Errorf("%s %s tone = %v, want %s", c.uid, c.part, got, c.tone)
		}
	}
	if got := spanStyle(t, card("open", "title")); got != "" {
		t.Errorf("an open card's title style = %q, want the normal text colour", got)
	}
	if got := spanStyle(t, card("open", "also")); got != "warning" {
		t.Errorf("the also note style = %q, want warning", got)
	}
	if got := spanStyle(t, card("done", "title")); got != "muted" {
		t.Errorf("a Done card's title style = %q, want muted", got)
	}
	if got := spanStyle(t, card("done", "reopen")); got != "muted" {
		t.Errorf("the reopen hint style = %q, want muted", got)
	}
	if got := card("done", "").P["tone"]; got != "muted" {
		t.Errorf("a Done card's ring tone = %v, want muted", got)
	}
}

// laneListLines is the list rows a Lane's card list is capped at.
func laneListLines(t *testing.T, s driver.State) int {
	t.Helper()
	list := node(t, wireNodes(t, view.Render(s.Model, now)), view.BoardID+".lane0.cards")
	max, _ := list.P["max"].(map[string]any)
	lines, err := strconv.Atoi(fmt.Sprint(max["lines"]))
	if err != nil {
		t.Fatalf("the Lane list is capped at %v, want list rows", list.P["max"])
	}
	return lines
}

func TestLaneListsAreCappedToTheRowsTheirPaneHas(t *testing.T) {
	s := boardPane(t, boardCard("a"))
	const rowPx, listRowPx = 16, 22
	for _, rows := range []int{39, 80} {
		lines := laneListLines(t, step(t, s, model.ResizeMsg{Cols: 160, Rows: rows, RowPx: rowPx}).State)
		if px := lines * listRowPx; px <= (rows-12)*rowPx || px > rows*rowPx {
			t.Errorf("a %d-row Pane caps a Lane list at %d list rows, %dpx of its %dpx", rows, lines, px, rows*rowPx)
		}
	}
}

// Each line under the columns costs its text row and the gap above it.
func TestLaneListsLeaveRoomForTheNoticeAndTheDisconnectedLine(t *testing.T) {
	const rowPx = 16
	pane := func(rows int, notice string, stale bool) int {
		s := step(t, boardPane(t, boardCard("a")), model.ResizeMsg{Cols: 160, Rows: rows, RowPx: rowPx}).State
		s.Model.Notice, s.Model.Stale = notice, stale
		return laneListLines(t, s)
	}
	for rows := 44; rows <= 80; rows++ {
		if got, want := pane(rows, "Lanes not saved", false), pane(rows-2, "", false); got != want {
			t.Errorf("a %d-row Pane with a Notice caps a Lane list at %d list rows, want the %d of a Pane two rows shorter", rows, got, want)
		}
		if got, want := pane(rows, "", true), pane(rows-2, "", false); got != want {
			t.Errorf("a %d-row Pane while disconnected caps a Lane list at %d list rows, want the %d of a Pane two rows shorter", rows, got, want)
		}
		if got, want := pane(rows, "Lanes not saved", true), pane(rows-4, "", false); got != want {
			t.Errorf("a %d-row Pane with both caps a Lane list at %d list rows, want the %d of a Pane four rows shorter", rows, got, want)
		}
	}
}

func TestAWrappingNoticeLowersTheLaneListCapMoreThanAShortOne(t *testing.T) {
	s := boardPane(t, boardCard("a"))
	s = step(t, s, model.ResizeMsg{Cols: 160, Rows: 60, RowPx: 16}).State
	short, long := s, s
	short.Model.Notice = "Lanes not saved."
	long.Model.Notice = strings.Repeat("unreadable lane list ", 40)
	if shortLines, longLines := laneListLines(t, short), laneListLines(t, long); longLines >= shortLines {
		t.Errorf("a Notice wrapping over several rows left %d list rows, a one-row Notice %d; want fewer", longLines, shortLines)
	}
}
