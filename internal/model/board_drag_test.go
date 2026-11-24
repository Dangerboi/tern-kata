package model_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func dragCard(uid string, labels ...string) model.Issue {
	return model.Issue{UID: uid, ShortID: uid, Title: "Title of " + uid, Status: "open", ProjectID: 7, ProjectName: "work", Labels: labels,
		UpdatedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
}

func dragClosed(uid string, labels ...string) model.Issue {
	issue := dragCard(uid, labels...)
	at := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	issue.Status, issue.ClosedAt = "closed", &at
	return issue
}

// dragBoard is a Pane on the Board's columns layout showing issues, with the default Lanes: column 0 is No lane, 1 Needs
// triage, 2 Needs info, and the last is Done.
func dragBoard(t *testing.T, issues ...model.Issue) model.Model {
	t.Helper()
	m, _ := model.Update(model.Model{}, model.Start{Workspace: "/repo"})
	m, _ = model.Update(m, model.ResizeMsg{Cols: 160})
	project := model.Project{ID: 7, UID: "p", Name: "work"}
	m, _ = loadFirst(m, model.ProjectLoaded{Project: project},
		model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: issues, Catalog: []model.CatalogProject{{Project: project}}}, LocalDate: "2026-10-08", TimeZone: "UTC"})
	m, _ = model.Update(m, model.KeyMsg{Chord: "b"})
	m, _ = model.Update(m, model.LanesLoaded{ProjectUID: "p", Request: m.LanesRequest()})
	if !m.BoardShown() {
		t.Fatal("the Board's columns layout is not shown")
	}
	return m
}

func columnOf(m model.Model, uid string) int {
	col, _ := model.CardAt(m.BoardColumns(), uid)
	return col
}

func moveCmds(cmds []model.Cmd) (moves []model.MoveIssueCmd) {
	for _, cmd := range cmds {
		if move, ok := cmd.(model.MoveIssueCmd); ok {
			moves = append(moves, move)
		}
	}
	return moves
}

func noWrites(t *testing.T, cmds []model.Cmd) {
	t.Helper()
	for _, cmd := range cmds {
		if _, ok := cmd.(model.WaitToast); !ok {
			t.Errorf("a drop emitted %#v, want nothing", cmd)
		}
	}
}

// dropOn is drop with the Lane key the frame drew for its column, as the view fills it in.
func dropOn(m model.Model, drop model.CardDropped) model.CardDropped {
	if columns := m.BoardColumns(); drop.Column >= 0 && drop.Column < len(columns) {
		drop.Lane = columns[drop.Column].Lane
	}
	return drop
}

func TestDropFromAStaleFrameChangesNothing(t *testing.T) {
	m := dragBoard(t, dragCard("a"), dragCard("b", "needs-triage"))
	start := m.BoardDragGen()
	for name, drop := range map[string]model.CardDropped{
		"another Lane's key": {UID: "a", Column: 1, Lane: "needs-info"},
		"no key":             {UID: "a", Column: 2},
		"reorder":            {UID: "b", Column: 1, Lane: "needs-info", Reorder: true, Index: 0},
	} {
		next, cmds := model.Update(m, drop)
		if len(cmds) != 0 || next.Moving != "" || columnOf(next, "a") != 0 || columnOf(next, "b") != 1 {
			t.Errorf("%s: cmds %#v, moving %q, a in column %d, b in column %d; want no write", name, cmds, next.Moving, columnOf(next, "a"), columnOf(next, "b"))
		}
		if next.BoardDragGen() != start+1 {
			t.Errorf("%s: generation %d, want %d", name, next.BoardDragGen(), start+1)
		}
	}
}

func TestDropOnAnotherLaneMovesTheCardAtOnce(t *testing.T) {
	m := dragBoard(t, dragCard("a", "bug", "needs-triage"))
	m, cmds := model.Update(m, dropOn(m, model.CardDropped{UID: "a", Column: 2}))
	moves := moveCmds(cmds)
	if len(cmds) != 1 || len(moves) != 1 {
		t.Fatalf("cmds = %#v, want one Move", cmds)
	}
	want := model.MoveIssueCmd{Workspace: "/repo", UID: "a", ProjectID: 7, Actor: moves[0].Actor, Add: "needs-info", Remove: []string{"needs-triage"}}
	if !reflect.DeepEqual(moves[0], want) {
		t.Errorf("Move = %#v, want %#v", moves[0], want)
	}
	if got := columnOf(m, "a"); got != 2 {
		t.Errorf("card shows in column %d, want 2 (Needs info)", got)
	}
	if m.SelectedUID != "a" {
		t.Errorf("selected %q, want a", m.SelectedUID)
	}
}

func TestDropOnNoLaneRemovesTheLaneLabelsOnly(t *testing.T) {
	m := dragBoard(t, dragCard("a", "needs-triage", "bug"))
	m, cmds := model.Update(m, dropOn(m, model.CardDropped{UID: "a", Column: 0}))
	moves := moveCmds(cmds)
	if len(moves) != 1 || moves[0].Add != "" || !reflect.DeepEqual(moves[0].Remove, []string{"needs-triage"}) {
		t.Fatalf("Moves = %#v, want one removing needs-triage and adding nothing", moves)
	}
	if got := columnOf(m, "a"); got != 0 {
		t.Errorf("card shows in column %d, want 0", got)
	}
}

func TestDropSelectsTheDraggedCard(t *testing.T) {
	m := dragBoard(t, dragCard("a"), dragCard("b", "needs-info"))
	m, _ = model.Update(m, dropOn(m, model.CardDropped{UID: "b", Column: 1}))
	if m.SelectedUID != "b" {
		t.Errorf("selected %q, want b", m.SelectedUID)
	}
}

func TestDropOnDoneOpensTheCloseCard(t *testing.T) {
	m := dragBoard(t, dragCard("a", "needs-triage"), dragCard("b"))
	done := len(m.BoardColumns()) - 1
	m, cmds := model.Update(m, dropOn(m, model.CardDropped{UID: "b", Column: done}))
	if !m.CloseForm.Open || m.CloseForm.UID != "b" {
		t.Fatalf("Close card = %+v, want open for b", m.CloseForm)
	}
	noWrites(t, cmds)
	if got := columnOf(m, "b"); got != 0 {
		t.Errorf("card shows in column %d, want it still in No lane", got)
	}
}

func TestDropOnDoneOfARefusedIssueOpensNoCloseCard(t *testing.T) {
	m := dragBoard(t, dragCard("a"))
	m.Stale = true
	done := len(m.BoardColumns()) - 1
	m, cmds := model.Update(m, dropOn(m, model.CardDropped{UID: "a", Column: done}))
	if m.CloseForm.Open {
		t.Errorf("Close card opened while disconnected, as the x key would not")
	}
	noWrites(t, cmds)
}

func TestClosedCardDroppedOnALaneOffersReopen(t *testing.T) {
	m := dragBoard(t, dragClosed("d"))
	m, cmds := model.Update(m, dropOn(m, model.CardDropped{UID: "d", Column: 1}))
	if len(moveCmds(cmds)) != 0 {
		t.Errorf("a closed card was moved: %#v", cmds)
	}
	if m.Toast == nil || m.Toast.Text != "d is closed." || m.Toast.Label != "Reopen" || m.Toast.Chord != "r" || m.Toast.UID != "d" {
		t.Errorf("toast = %+v, want the Reopen toast for d", m.Toast)
	}
	if got := columnOf(m, "d"); got != len(m.BoardColumns())-1 {
		t.Errorf("closed card shows in column %d, want Done", got)
	}
}

func TestReorderAndOwnLaneDropsChangeNothing(t *testing.T) {
	for name, drop := range map[string]model.CardDropped{
		"reorder":  {UID: "a", Column: 1, Reorder: true, Index: 0},
		"own Lane": {UID: "a", Column: 1},
	} {
		t.Run(name, func(t *testing.T) {
			m := dragBoard(t, dragCard("a", "needs-triage"), dragCard("b", "needs-triage"))
			before := m
			m, cmds := model.Update(m, dropOn(m, drop))
			if len(cmds) != 0 || m.Moving != "" || m.Toast != nil || m.Notice != "" || m.CloseForm.Open {
				t.Errorf("cmds %#v, moving %q, toast %v, notice %q", cmds, m.Moving, m.Toast, m.Notice)
			}
			if !reflect.DeepEqual(m.Issues, before.Issues) || !reflect.DeepEqual(m.BoardColumns(), before.BoardColumns()) {
				t.Errorf("the drop changed the Board")
			}
		})
	}
}

func TestDropIgnoresAnUnknownCardOrColumn(t *testing.T) {
	m := dragBoard(t, dragCard("a"))
	for _, drop := range []model.CardDropped{{UID: "nope", Column: 1}, {UID: "a", Column: 99}, {UID: "a", Column: -1}} {
		next, cmds := model.Update(m, dropOn(m, drop))
		if len(cmds) != 0 || columnOf(next, "a") != 0 {
			t.Errorf("%+v: cmds %#v, card in column %d", drop, cmds, columnOf(next, "a"))
		}
	}
}

func TestDropWhileAnotherMoveIsInFlightSendsNothing(t *testing.T) {
	m := dragBoard(t, dragCard("a"), dragCard("b"))
	m, cmds := model.Update(m, dropOn(m, model.CardDropped{UID: "a", Column: 1}))
	if len(moveCmds(cmds)) != 1 {
		t.Fatalf("first drop cmds = %#v", cmds)
	}
	m, cmds = model.Update(m, dropOn(m, model.CardDropped{UID: "b", Column: 2}))
	if len(cmds) != 0 {
		t.Errorf("second drop emitted %#v", cmds)
	}
	if got := columnOf(m, "b"); got != 0 {
		t.Errorf("b shows in column %d, want it left in No lane", got)
	}
}

func TestDropWhileDisconnectedFailsLikeTheKey(t *testing.T) {
	m := dragBoard(t, dragCard("a"))
	m.Stale = true
	m, cmds := model.Update(m, dropOn(m, model.CardDropped{UID: "a", Column: 1}))
	if len(cmds) != 0 || m.Notice == "" || columnOf(m, "a") != 0 {
		t.Errorf("cmds %#v, notice %q, column %d; want a refusal Notice and the card in place", cmds, m.Notice, columnOf(m, "a"))
	}
}

func TestEveryDropCountsInTheDragGeneration(t *testing.T) {
	m := dragBoard(t, dragCard("a", "needs-triage"))
	start := m.BoardDragGen()
	for i, drop := range []model.CardDropped{
		{UID: "a", Column: 1, Reorder: true},
		{UID: "a", Column: 1},
		{UID: "nope", Column: 2},
		{UID: "a", Column: 2},
	} {
		m, _ = model.Update(m, dropOn(m, drop))
		if got := m.BoardDragGen(); got != start+i+1 {
			t.Errorf("after drop %d the generation is %d, want %d", i, got, start+i+1)
		}
	}
}

func TestDropOffTheBoardOnlyCountsInTheDragGeneration(t *testing.T) {
	m, _ := model.Update(model.Model{}, model.CardDropped{UID: "a", Column: 1})
	if m.BoardDragGen() != 1 {
		t.Errorf("generation = %d, want 1", m.BoardDragGen())
	}
}
