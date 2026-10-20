package driver_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// laneHeader is the Lane header line the Lane view draws above its list, "" when none is drawn.
func laneHeader(t *testing.T, s driver.State) string {
	t.Helper()
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if n.ID == view.LaneHeaderID {
			return wireText(n)
		}
	}
	return ""
}

// laneRows are the uids of the Issue rows the Lane view draws, in drawn order.
func laneRows(t *testing.T, s driver.State) []string {
	t.Helper()
	rows := []string{}
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if uid, ok := strings.CutPrefix(n.ID, view.IssueID("")); ok && !strings.Contains(uid, ".") {
			rows = append(rows, uid)
		}
	}
	return rows
}

func drawsDetail(t *testing.T, s driver.State) bool {
	t.Helper()
	return drawnIDs(t, view.Render(s.Model, now))[view.DetailID]
}

// laneBoard has two cards in Needs triage, one in Needs info and one in Ready for human.
func laneBoard(t *testing.T, cols int) driver.State {
	t.Helper()
	s := projectPane(t, cols,
		updatedAt(boardCard("t1", "needs-triage"), 9), updatedAt(boardCard("t2", "needs-triage"), 8),
		boardCard("i1", "needs-info"), boardCard("h1", "ready-for-human"))
	if cols >= model.SideBySideCols {
		s = feed(t, s, key("b")).State
	}
	return s
}

func TestEnterOnACardOpensItsLaneViewBesideTheDetail(t *testing.T) {
	s := feed(t, laneBoard(t, 160), key("right")).State // i1, in Needs info
	out := step(t, s, key("enter"))
	if !strings.Contains(laneHeader(t, out.State), "Needs info") {
		t.Errorf("Lane header = %s", laneHeader(t, out.State))
	}
	if got := laneRows(t, out.State); !reflect.DeepEqual(got, []string{"i1"}) {
		t.Errorf("rows = %v, want just i1", got)
	}
	if heads, _ := lanes(t, out.State); len(heads) != 0 {
		t.Error("the Board is still drawn")
	}
	if !drawsDetail(t, out.State) || out.State.Model.SelectedUID != "i1" {
		t.Errorf("detail drawn %v, selected %q", drawsDetail(t, out.State), out.State.Model.SelectedUID)
	}
	if out.State.Model.Focus != model.ColumnList {
		t.Errorf("focus = %v, want the list", out.State.Model.Focus)
	}
	var loads []model.Cmd
	for _, c := range out.Cmds {
		if _, ok := c.(model.LoadDetail); ok {
			loads = append(loads, c)
		}
	}
	if !reflect.DeepEqual(loads, []model.Cmd{model.LoadDetail{Workspace: "/repo", UID: "i1"}}) {
		t.Errorf("commands = %#v, want the detail of i1 to load", out.Cmds)
	}
}

func TestDoubleClickingACardOpensItsLaneView(t *testing.T) {
	s := laneBoard(t, 160)
	out := step(t, s, click(view.CardID(s.Model, "h1"), view.OpenRowClick("h1")))
	if !strings.Contains(laneHeader(t, out.State), "Ready for human") || out.State.Model.SelectedUID != "h1" {
		t.Errorf("header %s, selected %q", laneHeader(t, out.State), out.State.Model.SelectedUID)
	}
}

func TestArrowsSwitchLanesAndEscReturnsToTheBoardWithTheCardKept(t *testing.T) {
	s := feed(t, laneBoard(t, 160), key("enter")).State // t1 in Needs triage
	for _, tc := range []struct{ key, header, selected string }{
		{"right", "Needs info", "i1"},
		{"right", "Ready for human", "h1"},
		{"right", "Ready for human", "h1"}, // Done is empty
		{"left", "Needs info", "i1"},
		{"left", "Needs triage", "t1"},
		{"left", "Needs triage", "t1"}, // No lane is empty
	} {
		s = step(t, s, key(tc.key)).State
		if !strings.Contains(laneHeader(t, s), tc.header) || s.Model.SelectedUID != tc.selected {
			t.Fatalf("%s: header %s, selected %q; want %s, %s", tc.key, laneHeader(t, s), s.Model.SelectedUID, tc.header, tc.selected)
		}
	}
	s = feed(t, s, key("j")).State
	out := step(t, s, key("escape"))
	if heads, _ := lanes(t, out.State); len(heads) != 6 || laneHeader(t, out.State) != "" {
		t.Fatalf("the Board did not come back: %d columns", len(heads))
	}
	if out.State.Model.SelectedUID != "t2" || drawsDetail(t, out.State) {
		t.Errorf("selected %q, detail drawn %v", out.State.Model.SelectedUID, drawsDetail(t, out.State))
	}
	if out.State.Model.TopCard() != model.NoCard {
		t.Error("Esc from the Lane view of a wide Pane opened a card")
	}
}

func TestLaneViewWorksLikeTheList(t *testing.T) {
	s := feed(t, laneBoard(t, 160), key("enter")).State
	if got := step(t, s, key("j")).State.Model.SelectedUID; got != "t2" {
		t.Errorf("j selects %q", got)
	}
	out := step(t, s, key("j"))
	wantOps(t, out, tern.OpReveal(view.IssueID("t2"), "nearest"))
	tabbed := step(t, s, key("tab")).State
	if tabbed.Model.Focus != model.ColumnDetail {
		t.Fatalf("tab focus = %v", tabbed.Model.Focus)
	}
	back := step(t, tabbed, key("escape")).State
	if back.Model.Focus != model.ColumnList || laneHeader(t, back) == "" {
		t.Errorf("Esc from the detail: focus %v, header %q; want the Lane view's list", back.Model.Focus, laneHeader(t, back))
	}
	if got := step(t, s, key("x")).State.Model.TopCard(); got != model.CardClose {
		t.Errorf("x opens %v", got)
	}
	if got := step(t, s, key("b")).State; laneHeader(t, got) != "" || !drawnIDs(t, view.Render(got.Model, now))[view.IssueListID] {
		t.Error("b did not bring the list back")
	}
}

func TestMovingInTheLaneViewSelectsTheNextCardInTheLane(t *testing.T) {
	s := feed(t, laneBoard(t, 160), key("enter")).State // t1, t2 in Needs triage
	out := step(t, s, key(">"))
	wantMove(t, out, "needs-info", "needs-triage")
	if m := moveCmds(out.Cmds)[0]; m.UID != "t1" {
		t.Errorf("Move = %#v", m)
	}
	if got := laneRows(t, out.State); !reflect.DeepEqual(got, []string{"t2"}) || out.State.Model.SelectedUID != "t2" {
		t.Errorf("rows %v, selected %q; want t2 only, selected", got, out.State.Model.SelectedUID)
	}
	// The last card of a Lane hands the selection to the card before it.
	out = step(t, step(t, s, key("j")).State, key(">"))
	if out.State.Model.SelectedUID != "t1" {
		t.Errorf("selected %q after moving the last card, want t1", out.State.Model.SelectedUID)
	}
}

func TestMovingTheOnlyCardOfALaneLeavesItEmpty(t *testing.T) {
	s := projectPane(t, 160, boardCard("solo", "needs-info"))
	s = feed(t, s, key("b"), key("enter")).State
	out := step(t, s, key(">"))
	wantMove(t, out, "ready-for-agent", "needs-info")
	if len(laneRows(t, out.State)) != 0 || out.State.Model.SelectedUID != "" || !strings.Contains(laneHeader(t, out.State), "Needs info") {
		t.Errorf("rows %v, selected %q, header %s", laneRows(t, out.State), out.State.Model.SelectedUID, laneHeader(t, out.State))
	}
}

func TestMovingADoneCardInTheLaneViewPointsAtReopen(t *testing.T) {
	s := feed(t, boardPane(t, closedAt("d", 10, 5)), key("enter")).State
	out := step(t, s, key(">"))
	if len(out.Cmds) != 0 || !containsAll(out.State.Model.Notice, "press r") {
		t.Errorf("cmds %#v, notice %q", out.Cmds, out.State.Model.Notice)
	}
}

func TestNarrowPaneOpensBoardModeInTheSelectedIssuesLane(t *testing.T) {
	s := laneBoard(t, 120)
	s = feed(t, s, key("j"), key("j"), key("j")).State // i1 by list order: t1, t2, h1, i1
	if s.Model.SelectedUID != "i1" {
		t.Fatalf("fixture: selected %q", s.Model.SelectedUID)
	}
	out := step(t, s, key("b"))
	if out.State.Model.Notice != "" {
		t.Errorf("Notice = %q, want none", out.State.Model.Notice)
	}
	if !strings.Contains(laneHeader(t, out.State), "Needs info") || !reflect.DeepEqual(laneRows(t, out.State), []string{"i1"}) {
		t.Errorf("header %s, rows %v", laneHeader(t, out.State), laneRows(t, out.State))
	}
	if drawsDetail(t, out.State) {
		t.Error("the detail is drawn beside the Lane in a narrow Pane")
	}
}

func TestNarrowLaneViewOpensOnTheFirstShownLaneWhenTheSelectionIsOnNoColumn(t *testing.T) {
	s := projectPane(t, 120, closedAt("old", 8, 1, "needs-info"), boardCard("h1", "ready-for-human"))
	s.Model.SelectedUID = "old" // closed long ago: on no column
	out := step(t, s, key("b"))
	if !strings.Contains(laneHeader(t, out.State), "Ready for human") || out.State.Model.SelectedUID != "h1" {
		t.Errorf("header %s, selected %q", laneHeader(t, out.State), out.State.Model.SelectedUID)
	}
}

func TestNarrowLaneViewEnterShowsTheDetailEscComesBackAndEscAgainOpensTheMenu(t *testing.T) {
	s := feed(t, laneBoard(t, 120), key("b")).State
	shown := step(t, s, key("enter")).State
	if !drawsDetail(t, shown) || laneHeader(t, shown) != "" {
		t.Fatalf("Enter did not show the detail alone: detail %v, header %q", drawsDetail(t, shown), laneHeader(t, shown))
	}
	if !shown.Model.Board.On() {
		t.Fatal("focusing the detail ended Board mode")
	}
	back := step(t, shown, key("escape")).State
	if laneHeader(t, back) == "" || drawsDetail(t, back) {
		t.Fatalf("Esc did not return to the Lane view: header %q, detail %v", laneHeader(t, back), drawsDetail(t, back))
	}
	menu := step(t, back, key("escape")).State
	if menu.Model.TopCard() != model.CardPicker {
		t.Errorf("Esc from the Lane view opened %v, want the Menu", menu.Model.TopCard())
	}
}

func TestResizingBelowSideBySideWidthSwitchesTheBoardToTheLaneView(t *testing.T) {
	s := feed(t, laneBoard(t, 160), key("right")).State // i1
	out := step(t, s, resize(120))
	if !out.State.Model.Board.On() || !strings.Contains(laneHeader(t, out.State), "Needs info") || out.State.Model.SelectedUID != "i1" {
		t.Errorf("header %s, selected %q", laneHeader(t, out.State), out.State.Model.SelectedUID)
	}
	wide := step(t, out.State, resize(160))
	if !strings.Contains(laneHeader(t, wide.State), "Needs info") || !drawsDetail(t, wide.State) {
		t.Error("widening the Pane lost the Lane view or its detail")
	}
}

func TestBoardReadsNoDetailButTheLaneViewDoes(t *testing.T) {
	selectedOf := func(s driver.State) *string {
		event := model.StreamEvent{Generation: s.Model.StreamGeneration, ID: 1, Type: "issue.updated"}
		read, ok := snapshotFor(step(t, s, event).Cmds)
		if !ok {
			t.Fatal("a stream event asked for no snapshot")
		}
		return read.Query.SelectedIssueUID
	}
	board := laneBoard(t, 160)
	if got := selectedOf(board); got != nil {
		t.Errorf("the Board's snapshot read selects %q", *got)
	}
	if got := selectedOf(feed(t, board, key("enter")).State); got == nil || *got != "t1" {
		t.Errorf("the Lane view's snapshot read selects %v, want t1", got)
	}
}

func TestLeavingTheLaneViewForTheBoardHidesTheDetailAgain(t *testing.T) {
	s := feed(t, laneBoard(t, 160), key("enter")).State
	out := step(t, s, key("escape"))
	for _, cmd := range out.Cmds {
		if _, ok := cmd.(model.LoadDetail); ok {
			t.Errorf("commands = %#v", out.Cmds)
		}
	}
	if !slices.Contains(out.State.Model.VisibleColumns(), model.ColumnList) || slices.Contains(out.State.Model.VisibleColumns(), model.ColumnDetail) {
		t.Errorf("columns = %v", out.State.Model.VisibleColumns())
	}
}
