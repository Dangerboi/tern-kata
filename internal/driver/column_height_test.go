package driver_test

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

// scrollerLines is the list rows the scroller id is capped at, or false when its cap is a fraction of the window.
func scrollerLines(t *testing.T, s driver.State, id string) (int, bool) {
	t.Helper()
	list := node(t, wireNodes(t, view.Render(s.Model, now)), id)
	max, ok := list.P["max"].(map[string]any)
	if !ok {
		return 0, false
	}
	lines, err := strconv.Atoi(fmt.Sprint(max["lines"]))
	if err != nil {
		t.Fatalf("%s is capped at %v", id, list.P["max"])
	}
	return lines, true
}

// Beside each other, the list and the detail scroll inside their columns down to the footer, whatever the Pane's height: each
// is capped in list rows from the rows the Pane has, not at a share of the window.
func TestListAndDetailFillTheirColumnsAtAnyPaneHeight(t *testing.T) {
	const rowPx, listRowPx = 16, 22
	for _, id := range []string{view.IssueListID, view.DetailScrollID} {
		var shorter int
		for _, rows := range []int{39, 80} {
			s := step(t, browsing(t, 160), model.ResizeMsg{Cols: 160, Rows: rows, RowPx: rowPx}).State
			lines, ok := scrollerLines(t, s, id)
			if !ok {
				t.Fatalf("%s in a %d-row Pane is capped at a share of the window, want list rows", id, rows)
			}
			if px := lines * listRowPx; px <= (rows-12)*rowPx || px > rows*rowPx {
				t.Errorf("a %d-row Pane caps %s at %d list rows, %dpx of its %dpx", rows, id, lines, px, rows*rowPx)
			}
			if lines <= shorter {
				t.Errorf("%s: a %d-row Pane gets %d list rows, no more than a shorter Pane's %d", id, rows, lines, shorter)
			}
			shorter = lines
		}
	}
}

// Tern reports a resize only when the width changes, so a Pane made shorter at the same width says so in rows alone, and the caps
// shrink with it: a stale height would let the lists grow past the Pane and scroll the top bar away.
func TestAPaneMadeShorterAtTheSameWidthShrinksTheCaps(t *testing.T) {
	for _, id := range []string{view.IssueListID, view.DetailScrollID} {
		short := step(t, browsing(t, 160), model.ResizeMsg{Cols: 160, Rows: 26, RowPx: 16}).State
		tall := step(t, browsing(t, 160), model.ResizeMsg{Cols: 160, Rows: 80, RowPx: 16}).State
		shrunk := step(t, tall, model.PaneRowsMsg{Rows: 26}).State
		want, _ := scrollerLines(t, short, id)
		if got, _ := scrollerLines(t, shrunk, id); got != want {
			t.Errorf("%s after the Pane shrank to 26 rows is capped at %d list rows, want the %d of a 26-row Pane", id, got, want)
		}
	}
}

func TestTheDetailLeavesRoomForANotice(t *testing.T) {
	lines := func(rows int, notice string) int {
		s := step(t, browsing(t, 160), model.ResizeMsg{Cols: 160, Rows: rows, RowPx: 16}).State
		s.Model.Notice = notice
		n, _ := scrollerLines(t, s, view.DetailScrollID)
		return n
	}
	for rows := 44; rows <= 80; rows++ {
		if got, want := lines(rows, "Lanes not saved"), lines(rows-2, ""); got != want {
			t.Errorf("a %d-row Pane with a Notice caps the detail at %d list rows, want the %d of a Pane two rows shorter", rows, got, want)
		}
	}
}

// A lone column has no card, so its list fills what the column has left as before.
func TestALoneColumnStillFillsItsColumn(t *testing.T) {
	s := step(t, browsing(t, 100), model.ResizeMsg{Cols: 100, Rows: 39, RowPx: 16}).State
	if _, lines := scrollerLines(t, s, view.IssueListID); lines {
		t.Errorf("a lone list is capped in list rows, want the whole window")
	}
}
