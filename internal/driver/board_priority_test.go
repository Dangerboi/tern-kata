package driver_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

func TestCardShowsItsPriorityAsABadgeToneLikeTheList(t *testing.T) {
	var issues []model.Issue
	for p := int64(0); p <= 4; p++ {
		issue := boardCard(fmt.Sprintf("p%d", p))
		issue.Priority = &p
		issues = append(issues, issue)
	}
	doneIssue := closedAt("done", time.October, 7, "ready-for-human")
	two := int64(2)
	doneIssue.Priority = &two
	issues = append(issues, doneIssue, boardCard("none"))
	s := boardPane(t, issues...)
	nodes := wireNodes(t, view.Render(s.Model, now))

	for p, tone := range []string{"error", "warning", "info", "muted", "muted"} {
		n := node(t, nodes, rowOf(t, s.Model, view.BoardID, fmt.Sprintf("p%d", p))+".id.prio")
		if got := n.P["text"]; got != fmt.Sprintf("P%d", p) {
			t.Errorf("P%d card badge text = %v", p, got)
		}
		if got := n.P["tone"]; got != tone {
			t.Errorf("P%d badge tone = %v, want %s", p, got, tone)
		}
	}
	if got := node(t, nodes, rowOf(t, s.Model, view.BoardID, "done")+".id.prio").P["text"]; got != "P2" {
		t.Errorf("a Done card's priority badge = %v, want P2", got)
	}
	noPrio := rowOf(t, s.Model, view.BoardID, "none") + ".id.prio"
	for _, n := range nodes {
		if n.ID == noPrio {
			t.Errorf("a card without a priority draws a priority badge")
		}
	}
}
