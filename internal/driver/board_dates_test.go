package driver_test

import (
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

func dated(uid, scheduled, deadline string) model.Issue {
	issue := boardCard(uid)
	if scheduled != "" {
		issue.ScheduledOnDate = &scheduled
	}
	if deadline != "" {
		issue.DeadlineOnDate = &deadline
	}
	return issue
}

func hasNode(nodes []tern.WireNode, id string) bool {
	for _, n := range nodes {
		if n.ID == id {
			return true
		}
	}
	return false
}

// The Pane calendar of these tests is 2026-10-08.
func TestDateBadgeWordsAndTonesFollowTheSixRules(t *testing.T) {
	for _, c := range []struct {
		name, scheduled, deadline, text, tone string
	}{
		{"overdue deadline", "", "2026-10-06", "overdue 2d", "error"},
		{"overdue yesterday", "", "2026-10-07", "overdue 1d", "error"},
		{"deadline today", "", "2026-10-08", "due today", "warning"},
		{"scheduled passed", "2026-10-05", "", "scheduled 3d ago", "warning"},
		{"scheduled today", "2026-10-08", "", "scheduled today", "accent"},
		{"deadline tomorrow", "", "2026-10-09", "due tomorrow", "warning"},
		{"deadline later", "", "2026-10-24", "due 10-24", "muted"},
		{"deadline other year", "", "2027-01-05", "due 2027-01-05", "muted"},
		{"scheduled tomorrow", "2026-10-09", "", "scheduled tomorrow", "muted"},
		{"scheduled later", "2026-10-28", "", "scheduled 10-28", "muted"},
		{"scheduled later other year", "2028-01-02", "", "scheduled 2028-01-02", "muted"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := boardPane(t, dated("a", c.scheduled, c.deadline))
			n := node(t, wireNodes(t, view.Render(s.Model, now)), rowOf(t, s.Model, view.BoardID, "a")+".id.date")
			if n.P["text"] != c.text || n.P["tone"] != c.tone {
				t.Errorf("badge = %v (%v), want %s (%s)", n.P["text"], n.P["tone"], c.text, c.tone)
			}
		})
	}
}

func TestDateBadgePrecedenceDecidesEveryPairing(t *testing.T) {
	for _, c := range []struct{ name, scheduled, deadline, text string }{
		{"overdue deadline beats passed schedule", "2026-10-01", "2026-10-06", "overdue 2d"},
		{"overdue deadline beats today's schedule", "2026-10-08", "2026-10-06", "overdue 2d"},
		{"overdue deadline beats later schedule", "2026-10-20", "2026-10-06", "overdue 2d"},
		{"deadline today beats passed schedule", "2026-10-01", "2026-10-08", "due today"},
		{"deadline today beats schedule today", "2026-10-08", "2026-10-08", "due today"},
		{"deadline today beats later schedule", "2026-10-20", "2026-10-08", "due today"},
		{"passed schedule beats later deadline", "2026-10-05", "2026-10-09", "scheduled 3d ago"},
		{"schedule today beats later deadline", "2026-10-08", "2026-10-09", "scheduled today"},
		{"later deadline beats later schedule", "2026-10-09", "2026-10-30", "due 10-30"},
		{"later deadline beats earlier later schedule", "2026-10-20", "2026-10-21", "due 10-21"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := boardPane(t, dated("a", c.scheduled, c.deadline))
			n := node(t, wireNodes(t, view.Render(s.Model, now)), rowOf(t, s.Model, view.BoardID, "a")+".id.date")
			if n.P["text"] != c.text {
				t.Errorf("badge = %v, want %s", n.P["text"], c.text)
			}
		})
	}
}

func TestDateBadgeIsAbsentOnDoneAndUndatedCardsAndReadsMetadataDates(t *testing.T) {
	done := closedAt("done", time.October, 7)
	late := "2026-10-01"
	done.DeadlineOnDate = &late
	meta := boardCard("meta")
	meta.Metadata = map[string]any{"deadline_on": "2026-10-20T09:30:00Z"}
	s := boardPane(t, done, boardCard("plain"), meta)
	nodes := wireNodes(t, view.Render(s.Model, now))
	for _, uid := range []string{"done", "plain"} {
		if hasNode(nodes, rowOf(t, s.Model, view.BoardID, uid)+".id.date") {
			t.Errorf("card %s draws a Date badge", uid)
		}
	}
	if got := node(t, nodes, rowOf(t, s.Model, view.BoardID, "meta")+".id.date").P["text"]; got != "due 10-20" {
		t.Errorf("metadata-only deadline badge = %v, want due 10-20", got)
	}
}

func TestDateBadgeSitsAfterThePriorityBadgeAndBeforeRelationshipTags(t *testing.T) {
	issue := dated("a", "", "2026-10-08")
	p := int64(1)
	issue.Priority = &p
	s := boardPane(t, issue)
	row := node(t, wireNodes(t, view.Render(s.Model, now)), rowOf(t, s.Model, view.BoardID, "a")+".id")
	var keys []string
	for _, c := range row.C {
		keys = append(keys, c.ID)
	}
	prefix := rowOf(t, s.Model, view.BoardID, "a") + ".id."
	want := []string{prefix + "short", prefix + "prio", prefix + "date"}
	if len(keys) != 3 || keys[0] != want[0] || keys[1] != want[1] || keys[2] != want[2] {
		t.Errorf("id row = %v, want %v", keys, want)
	}
}

func TestBeforeThePaneCalendarIsKnownTheBadgeShowsTheEarlierDatePlainlyMuted(t *testing.T) {
	for _, c := range []struct{ name, scheduled, deadline, text string }{
		{"deadline only", "", "2026-10-24", "due 2026-10-24"},
		{"schedule only", "2026-10-28", "", "scheduled 2026-10-28"},
		{"earlier schedule", "2026-10-20", "2026-10-24", "scheduled 2026-10-20"},
		{"earlier deadline", "2026-10-28", "2026-10-24", "due 2026-10-24"},
		{"tie goes to the deadline", "2026-10-24", "2026-10-24", "due 2026-10-24"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := boardPane(t, dated("a", c.scheduled, c.deadline))
			s.Model.Calendar = model.PaneCalendar{}
			n := node(t, wireNodes(t, view.Render(s.Model, now)), rowOf(t, s.Model, view.BoardID, "a")+".id.date")
			if n.P["text"] != c.text || n.P["tone"] != "muted" {
				t.Errorf("badge = %v (%v), want %s (muted)", n.P["text"], n.P["tone"], c.text)
			}
		})
	}
}

func TestAMidnightSnapshotTurnsDueTodayIntoOverdueWithoutAKey(t *testing.T) {
	s := boardPane(t, dated("a", "", boardToday))
	text := func(s model.Model) any {
		return node(t, wireNodes(t, view.Render(s, now)), rowOf(t, s, view.BoardID, "a")+".id.date").P["text"]
	}
	if got := text(s.Model); got != "due today" {
		t.Fatalf("before midnight = %v", got)
	}
	out := step(t, s, model.DayEnded{Day: boardToday})
	snapshot, ok := snapshotFor(out.Cmds, dated("a", "", boardToday))
	if !ok {
		t.Fatalf("the day's end asked for no snapshot: %#v", out.Cmds)
	}
	snapshot.LocalDate = "2026-10-09"
	s = step(t, out.State, snapshot).State
	if got := text(s.Model); got != "overdue 1d" {
		t.Errorf("after midnight = %v, want overdue 1d", got)
	}
}

func TestSelectedCardShowsTheDatesLine(t *testing.T) {
	for _, c := range []struct{ name, scheduled, deadline, line string }{
		{"both", "2026-10-28", "2026-11-02", "scheduled 2026-10-28 · due 2026-11-02"},
		{"scheduled only", "2026-10-28", "", "scheduled 2026-10-28"},
		{"deadline only", "", "2026-11-02", "due 2026-11-02"},
		{"none", "", "", ""},
		{"unreadable deadline skipped", "2026-10-28", "soon", "scheduled 2026-10-28"},
		{"only unreadable", "", "soon", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := boardPane(t, dated("a", c.scheduled, c.deadline), dated("b", "2026-10-28", "2026-11-02"))
			s = step(t, s, model.SelectMsg{UID: "a"}).State
			nodes := wireNodes(t, view.Render(s.Model, now))
			id := rowOf(t, s.Model, view.BoardID, "a") + ".dates"
			if c.line == "" {
				if hasNode(nodes, id) {
					t.Errorf("a card without dates draws a Dates line")
				}
			} else if got := spanText(t, node(t, nodes, id)); got != c.line {
				t.Errorf("Dates line = %q, want %q", got, c.line)
			}
			if hasNode(nodes, rowOf(t, s.Model, view.BoardID, "b")+".dates") {
				t.Errorf("an unselected card draws a Dates line")
			}
		})
	}
}

func spanText(t *testing.T, n tern.WireNode) string {
	t.Helper()
	spans, _ := n.P["spans"].([]any)
	if len(spans) == 0 {
		t.Fatalf("%s has no spans", n.ID)
	}
	text, _ := spans[0].(map[string]any)["t"].(string)
	return text
}
