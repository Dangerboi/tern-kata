package driver_test

import (
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/view"
)

func TestRecurringCardsStartTheirTitleWithTheRecurrenceMark(t *testing.T) {
	loop := int64(7)
	open := boardCard("open")
	open.RecurrenceID = &loop
	done := closedAt("done", time.October, 7, "ready-for-human")
	done.RecurrenceID = &loop
	s := boardPane(t, open, done, boardCard("plain"))
	nodes := wireNodes(t, view.Render(s.Model, now))

	for uid, want := range map[string]string{"open": "↻ Title of open", "done": "↻ Title of done", "plain": "Title of plain"} {
		n := node(t, nodes, rowOf(t, s.Model, view.BoardID, uid)+".title")
		spans, _ := n.P["spans"].([]any)
		if got, _ := spans[0].(map[string]any)["t"].(string); got != want {
			t.Errorf("%s card title = %q, want %q", uid, got, want)
		}
	}
}
