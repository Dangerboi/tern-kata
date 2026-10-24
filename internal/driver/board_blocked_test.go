package driver_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// blocks is a Blocking link from the blocker to the Issue it blocks, the blocker's status as kata sends it.
func blocks(id int64, blocker, status, blocked string) model.Link {
	return model.Link{ID: id, Type: "blocks", FromIssueUID: blocker, FromQualifiedID: "work#" + blocker, FromStatus: status, ToIssueUID: blocked, ToQualifiedID: "work#" + blocked}
}

// linkedBoard is the Board of issues with the Collection snapshot's links.
func linkedBoard(t *testing.T, links []model.Link, issues ...model.Issue) driver.State {
	t.Helper()
	return refreshed(t, boardPane(t, issues...), links, issues)
}

var eventID int64

func nextEvent() int64 { eventID++; return 1000 + eventID }

// refreshed is a live refresh answered with issues and links.
func refreshed(t *testing.T, s driver.State, links []model.Link, issues []model.Issue) driver.State {
	t.Helper()
	out := step(t, s, model.StreamEvent{Generation: s.Model.StreamGeneration, ID: nextEvent(), Type: "issue.linked"})
	snapshot, ok := snapshotFor(out.Cmds, issues...)
	if !ok {
		t.Fatalf("a stream event asked for no snapshot: %#v", out.Cmds)
	}
	snapshot.Snapshot.Links, snapshot.Snapshot.Cursor = links, 1000+eventID
	return step(t, out.State, snapshot).State
}

// blockedText is the Blocked marker drawn on uid's card, "" when it draws none.
func blockedText(t *testing.T, s driver.State, uid string) string {
	t.Helper()
	id := rowOf(t, s.Model, view.BoardID, uid) + ".blocked"
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if n.ID == id {
			spans, _ := n.P["spans"].([]any)
			text, _ := spans[0].(map[string]any)["t"].(string)
			return text
		}
	}
	return ""
}

func blockedNode(t *testing.T, s driver.State, uid string) tern.WireNode {
	t.Helper()
	return node(t, wireNodes(t, view.Render(s.Model, now)), rowOf(t, s.Model, view.BoardID, uid)+".blocked")
}

func TestAnOpenBlockerGivesTheCardABlockedMarkerInTheWarningTone(t *testing.T) {
	s := linkedBoard(t, []model.Link{blocks(1, "gate", "open", "task")}, boardCard("gate"), boardCard("task"))
	if got := blockedText(t, s, "task"); got != "blocked by gate" {
		t.Errorf("marker = %q, want %q", got, "blocked by gate")
	}
	if got := spanStyle(t, blockedNode(t, s, "task")); got != "warning" {
		t.Errorf("marker style = %q, want warning", got)
	}
	if got := blockedText(t, s, "gate"); got != "" {
		t.Errorf("the blocker draws marker %q, want none", got)
	}
	s = step(t, s, model.SelectMsg{UID: "task"}).State
	if got := blockedText(t, s, "task"); got != "blocked by gate" {
		t.Errorf("selected card marker = %q, want it kept", got)
	}
}

func TestAClosedBlockerOrADoneCardShowsNoMarker(t *testing.T) {
	closedBlocker := linkedBoard(t, []model.Link{blocks(1, "gate", "closed", "task")}, boardCard("task"))
	if got := blockedText(t, closedBlocker, "task"); got != "" {
		t.Errorf("a closed blocker left marker %q", got)
	}
	done := closedAt("task", 10, 7)
	doneBoard := linkedBoard(t, []model.Link{blocks(1, "gate", "open", "task")}, done)
	if got := blockedText(t, doneBoard, "task"); got != "" {
		t.Errorf("a Done card shows marker %q", got)
	}
}

func TestManyBlockersAreCutToThreeAndTheRest(t *testing.T) {
	links := []model.Link{blocks(1, "a", "open", "task"), blocks(2, "b", "open", "task"), blocks(3, "c", "closed", "task"),
		blocks(4, "d", "open", "task"), blocks(5, "e", "open", "task"), blocks(6, "f", "open", "task")}
	s := linkedBoard(t, links, boardCard("task"))
	if got := blockedText(t, s, "task"); got != "blocked by a, b, d +2" {
		t.Errorf("marker = %q, want %q", got, "blocked by a, b, d +2")
	}
}

func TestACrossProjectBlockerShowsItsQualifiedIDAndCountsWithoutACard(t *testing.T) {
	link := model.Link{ID: 1, Type: "blocks", FromIssueUID: "far", FromQualifiedID: "other#ab12", FromStatus: "open", ToIssueUID: "task", ToQualifiedID: "work#task"}
	s := linkedBoard(t, []model.Link{link, blocks(2, "gone", "open", "task")}, boardCard("task"))
	if got := blockedText(t, s, "task"); got != "blocked by other#ab12, gone" {
		t.Errorf("marker = %q, want %q", got, "blocked by other#ab12, gone")
	}
}

func TestSelfAndRepeatedBlockingLinksNameABlockerOnce(t *testing.T) {
	links := []model.Link{blocks(1, "task", "open", "task"), blocks(2, "gate", "open", "task"), blocks(3, "gate", "open", "task")}
	s := linkedBoard(t, links, boardCard("task"), boardCard("gate"))
	if got := blockedText(t, s, "task"); got != "blocked by gate" {
		t.Errorf("marker = %q, want %q", got, "blocked by gate")
	}
}

func TestABlockedByLinkIsTheInverse(t *testing.T) {
	link := model.Link{ID: 1, Type: "blocked_by", FromIssueUID: "task", FromQualifiedID: "work#task", FromStatus: "open", ToIssueUID: "gate", ToQualifiedID: "work#gate", ToStatus: "open"}
	s := linkedBoard(t, []model.Link{link}, boardCard("task"), boardCard("gate"))
	if got := blockedText(t, s, "task"); got != "blocked by gate" {
		t.Errorf("marker = %q, want %q", got, "blocked by gate")
	}
	if got := blockedText(t, s, "gate"); got != "" {
		t.Errorf("the blocker draws marker %q, want none", got)
	}
}

func TestALiveRefreshClosingTheBlockerRemovesTheMarker(t *testing.T) {
	issues := []model.Issue{boardCard("gate"), boardCard("task")}
	s := linkedBoard(t, []model.Link{blocks(1, "gate", "open", "task")}, issues...)
	if blockedText(t, s, "task") == "" {
		t.Fatal("no marker before the refresh")
	}
	s = refreshed(t, s, []model.Link{blocks(1, "gate", "closed", "task")}, issues)
	if got := blockedText(t, s, "task"); got != "" {
		t.Errorf("marker %q stayed after the blocker closed", got)
	}
}
