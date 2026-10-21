package driver_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// childOf is a parent link from the child to its parent, the child's status as kata sends it.
func childOf(id int64, child, status, parent string) model.Link {
	return model.Link{ID: id, Type: "parent", FromIssueUID: child, FromQualifiedID: "work#" + child, FromStatus: status, ToIssueUID: parent, ToQualifiedID: "work#" + parent, ToStatus: "open"}
}

// blocksTo is a Blocking link from blocker to the Issue it blocks, whose status is the to end's.
func blocksTo(id int64, blocker, blocked, status string) model.Link {
	l := blocks(id, blocker, "open", blocked)
	l.ToStatus = status
	return l
}

func relatedTo(id int64, from, to string) model.Link {
	return model.Link{ID: id, Type: "related", FromIssueUID: from, FromQualifiedID: "work#" + from, FromStatus: "open", ToIssueUID: to, ToQualifiedID: "work#" + to, ToStatus: "open"}
}

// selectedBoard is the Board of the Issues and links with uid's card selected.
func selectedBoard(t *testing.T, uid string, links []model.Link, issues ...model.Issue) driver.State {
	t.Helper()
	return step(t, linkedBoard(t, links, issues...), model.SelectMsg{UID: uid}).State
}

// relationshipsText is the Relationships line drawn on uid's card, its parts joined by spaces, and whether the card draws one.
func relationshipsText(t *testing.T, s driver.State, uid string) (string, bool) {
	t.Helper()
	prefix := view.CardID(s.Model, uid) + ".relationships"
	var words []string
	drawn := false
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if n.ID == prefix {
			drawn = true
		}
		if !strings.HasPrefix(n.ID, prefix+".") {
			continue
		}
		spans, _ := n.P["spans"].([]any)
		for _, span := range spans {
			text, _ := span.(map[string]any)["t"].(string)
			words = append(words, text)
		}
	}
	return strings.Join(words, " "), drawn
}

func refClick(t *testing.T, s driver.State, from, to string) string {
	t.Helper()
	nodes := wireNodes(t, view.Render(s.Model, now))
	id := slices.IndexFunc(nodes, func(n tern.WireNode) bool {
		return strings.HasPrefix(n.ID, view.CardID(s.Model, from)+".relationships.") && strings.HasSuffix(n.ID, "."+to)
	})
	if id < 0 {
		t.Fatalf("no clickable id for %s on %s", to, from)
	}
	actions, _ := nodes[id].P["actions"].(map[string]any)
	action, _ := actions["click"].(string)
	if action == "" {
		t.Fatalf("%s is not clickable", nodes[id].ID)
	}
	return action
}

func TestTheSelectedCardShowsEveryPartOfItsRelationships(t *testing.T) {
	links := []model.Link{
		childOf(1, "task", "open", "par"), childOf(2, "kid1", "open", "task"), childOf(3, "kid2", "closed", "task"),
		blocksTo(4, "task", "later", "open"), blocksTo(5, "task", "done", "closed"), blocks(6, "gate", "open", "task"), blocks(7, "shut", "closed", "task"),
		relatedTo(8, "task", "rel"),
	}
	s := selectedBoard(t, "task", links, boardCard("task"), boardCard("par"))
	want := "parent par 2 children, 1 open blocks later blocked by gate related rel"
	if got, _ := relationshipsText(t, s, "task"); got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
}

func TestEmptyPartsAreLeftOutAndNoRelationshipsDrawNoLine(t *testing.T) {
	s := selectedBoard(t, "task", []model.Link{childOf(1, "kid", "open", "task")}, boardCard("task"), boardCard("lone"))
	if got, _ := relationshipsText(t, s, "task"); got != "1 child, 1 open" {
		t.Errorf("line = %q, want %q", got, "1 child, 1 open")
	}
	s = step(t, s, model.SelectMsg{UID: "lone"}).State
	if got, drawn := relationshipsText(t, s, "lone"); drawn || got != "" {
		t.Errorf("a card without Relationships draws line %q", got)
	}
	if _, drawn := relationshipsText(t, s, "task"); drawn {
		t.Error("a card that is not selected draws the line")
	}
}

func TestRelationshipIDsAreQualifiedOutsideTheProjectAndCutToThree(t *testing.T) {
	far := model.Link{ID: 9, Type: "related", FromIssueUID: "task", FromQualifiedID: "work#task", FromStatus: "open", ToIssueUID: "far", ToQualifiedID: "other#ab12", ToStatus: "open"}
	links := []model.Link{relatedTo(1, "task", "a"), relatedTo(2, "task", "b"), relatedTo(3, "c", "task"), relatedTo(4, "task", "d"), relatedTo(5, "task", "e"), far}
	s := selectedBoard(t, "task", links, boardCard("task"))
	if got, _ := relationshipsText(t, s, "task"); got != "related a b c +3" {
		t.Errorf("line = %q, want %q", got, "related a b c +3")
	}
	s = selectedBoard(t, "task", []model.Link{far}, boardCard("task"))
	if got, _ := relationshipsText(t, s, "task"); got != "related other#ab12" {
		t.Errorf("line = %q, want %q", got, "related other#ab12")
	}
}

func TestClickingAnIDSelectsItsCardAndRevealsIt(t *testing.T) {
	issues := []model.Issue{boardCard("task"), boardCard("par", "ready-for-agent")}
	s := selectedBoard(t, "task", []model.Link{childOf(1, "task", "open", "par")}, issues...)
	out := step(t, s, click("", refClick(t, s, "task", "par")))
	if got := out.State.Model.SelectedUID; got != "par" {
		t.Fatalf("selected %q after the click, want par", got)
	}
	wantOps(t, out, tern.OpReveal(view.CardID(out.State.Model, "par"), "nearest"))
}

func TestClickingAnIDTheBoardDoesNotDrawLeavesANotice(t *testing.T) {
	far := model.Link{ID: 1, Type: "related", FromIssueUID: "task", FromQualifiedID: "work#task", FromStatus: "open", ToIssueUID: "far", ToQualifiedID: "other#ab12", ToStatus: "open"}
	s := selectedBoard(t, "task", []model.Link{far, childOf(2, "task", "open", "par")}, boardCard("task"))
	for _, tt := range []struct{ uid, notice string }{{"far", "other#ab12 isn't on the Board."}, {"par", "par isn't on the Board."}} {
		out := step(t, s, click("", refClick(t, s, "task", tt.uid)))
		if got := out.State.Model; got.SelectedUID != "task" || got.Notice != tt.notice {
			t.Errorf("click on %s: selected %q, notice %q; want task and %q", tt.uid, got.SelectedUID, got.Notice, tt.notice)
		}
		wantOps(t, out)
	}
}

func TestClickingAnIDInTheLaneViewSwitchesToItsLane(t *testing.T) {
	issues := []model.Issue{boardCard("task"), boardCard("par", "ready-for-agent")}
	s := selectedBoard(t, "task", []model.Link{childOf(1, "task", "open", "par")}, issues...)
	lane := step(t, s, key("enter")).State
	lane = step(t, lane, model.StreamEvent{}).State
	if lane.Model.Board.Mode != model.BoardLane {
		t.Fatal("enter did not open the Lane view")
	}
	got := step(t, lane, model.ShowCardMsg{UID: "par"}).State.Model
	if got.SelectedUID != "par" || got.Board.Lane != "ready-for-agent" {
		t.Errorf("selected %q in Lane %q, want par in ready-for-agent", got.SelectedUID, got.Board.Lane)
	}
}

func TestCaretSelectsTheParentOrSaysThereIsNone(t *testing.T) {
	issues := []model.Issue{boardCard("task"), boardCard("par", "ready-for-agent"), boardCard("orphan")}
	s := selectedBoard(t, "task", []model.Link{childOf(1, "task", "open", "par"), childOf(2, "gone", "open", "task"), childOf(3, "task2", "open", "missing")}, issues...)
	out := step(t, s, key("^"))
	if got := out.State.Model.SelectedUID; got != "par" {
		t.Errorf("^ selected %q, want par", got)
	}
	wantOps(t, out, tern.OpReveal(view.CardID(out.State.Model, "par"), "nearest"))

	s = step(t, s, model.SelectMsg{UID: "orphan"}).State
	out = step(t, s, key("^"))
	if m := out.State.Model; m.SelectedUID != "orphan" || m.Notice != "orphan has no parent." {
		t.Errorf("selected %q, notice %q; want orphan and %q", m.SelectedUID, m.Notice, "orphan has no parent.")
	}

	s = selectedBoard(t, "task", []model.Link{childOf(1, "task", "open", "missing")}, boardCard("task"))
	if m := step(t, s, key("^")).State.Model; m.SelectedUID != "task" || m.Notice != "missing isn't on the Board." {
		t.Errorf("selected %q, notice %q; want task and %q", m.SelectedUID, m.Notice, "missing isn't on the Board.")
	}
	if m := step(t, step(t, s, key("^")).State, key("j")).State.Model; m.Notice != "" {
		t.Errorf("notice %q outlived a key", m.Notice)
	}
}

func TestShiftedCaretSelectsTheParentToo(t *testing.T) {
	s := selectedBoard(t, "task", []model.Link{childOf(1, "task", "open", "par")}, boardCard("task"), boardCard("par", "ready-for-agent"))
	if got := step(t, s, tern.Key{Name: "^", Shift: true}).State.Model.SelectedUID; got != "par" {
		t.Errorf("shift+^ selected %q, want par", got)
	}
}

func TestTheHelpSheetsBoardGroupListsCaret(t *testing.T) {
	for _, group := range model.HelpSheet() {
		if group.Title != "Board" {
			continue
		}
		for _, line := range group.Lines {
			if slices.Contains(line.Keys, "^") {
				return
			}
		}
	}
	t.Error("the Board group lists no ^")
}

func TestCaretDoesNothingOutsideTheBoard(t *testing.T) {
	s := feed(t, browsing(t, 160), key("^")).State
	if s.Model.Notice != "" {
		t.Errorf("^ in the list left notice %q", s.Model.Notice)
	}
}
