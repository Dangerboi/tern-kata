package driver_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
)

func moveCmds(cmds []model.Cmd) (moves []model.MoveIssueCmd) {
	for _, cmd := range cmds {
		if move, ok := cmd.(model.MoveIssueCmd); ok {
			moves = append(moves, move)
		}
	}
	return moves
}

func wantMove(t *testing.T, out driver.Output, add string, remove ...string) {
	t.Helper()
	moves := moveCmds(out.Cmds)
	if len(moves) != 1 || moves[0].Add != add || !reflect.DeepEqual(moves[0].Remove, remove) {
		t.Fatalf("Move cmds = %#v, want one adding %q and removing %v", moves, add, remove)
	}
}

func TestMoveRightAddsTheNextLaneAndLeavingNoLaneAddsTheFirst(t *testing.T) {
	s := boardPane(t, boardCard("a", "bug"))
	out := step(t, s, key(">"))
	wantMove(t, out, "needs-triage")
	wantLane(t, out.State, 0, "No lane")
	wantLane(t, out.State, 1, "Needs triage", "a")
	if out.State.Model.SelectedUID != "a" {
		t.Errorf("selected %q after a Move, want a", out.State.Model.SelectedUID)
	}
	if m := moveCmds(out.Cmds)[0]; m.UID != "a" {
		t.Errorf("Move = %#v", m)
	}
}

func TestMoveLeftIntoNoLaneRemovesOnlyLaneLabels(t *testing.T) {
	s := boardPane(t, boardCard("a", "needs-triage", "bug"))
	out := step(t, s, key("<"))
	wantMove(t, out, "", "needs-triage")
	wantLane(t, out.State, 0, "No lane", "a")
	if labels := out.State.Model.Issues[0].Labels; !reflect.DeepEqual(labels, []string{"bug"}) {
		t.Errorf("labels after the Move = %v, want only bug", labels)
	}
}

func TestMoveOfACardWithTwoLaneLabelsLeavesOne(t *testing.T) {
	s := boardPane(t, boardCard("a", "needs-triage", "ready-for-agent"))
	out := step(t, s, key(">"))
	wantMove(t, out, "needs-info", "needs-triage", "ready-for-agent")
	wantLane(t, out.State, 2, "Needs info", "a")
	out = step(t, s, key("<"))
	wantMove(t, out, "", "needs-triage", "ready-for-agent")
}

func TestMoveKeepsAnExistingTargetLabel(t *testing.T) {
	s := boardPane(t, boardCard("a", "needs-triage", "needs-info"))
	wantMove(t, step(t, s, key(">")), "", "needs-triage")
}

func TestMoveWhileOneIsInFlightEmitsNothing(t *testing.T) {
	out := step(t, boardPane(t, boardCard("a")), key(">"))
	again := step(t, out.State, key(">"))
	if len(again.Cmds) != 0 {
		t.Errorf("second Move emitted %#v", again.Cmds)
	}
	wantLane(t, again.State, 1, "Needs triage", "a")
}

func TestMoveResultAsksForAFreshSnapshotAndFreesTheNextMove(t *testing.T) {
	out := step(t, boardPane(t, boardCard("a")), key(">"))
	done := answered(t, out.State, model.MoveResultMsg{UID: "a"}, boardCard("a", "needs-triage"))
	wantLane(t, done.State, 1, "Needs triage", "a")
	wantMove(t, step(t, done.State, key(">")), "needs-info", "needs-triage")
}

func TestFailedMoveLeavesKatasReasonAndTheSnapshotPutsTheCardBack(t *testing.T) {
	out := step(t, boardPane(t, boardCard("a")), key(">"))
	failed := answered(t, out.State, model.MoveResultMsg{UID: "a", Err: errors.New("label is archived")}, boardCard("a"))
	if failed.State.Model.Notice != "label is archived" {
		t.Errorf("Notice = %q", failed.State.Model.Notice)
	}
	wantLane(t, failed.State, 0, "No lane", "a")
}

func TestMoveIsRefusedWhileDisconnected(t *testing.T) {
	s := boardPane(t, boardCard("a"))
	s.Model.Stale = true
	out := step(t, s, key(">"))
	if len(out.Cmds) != 0 || out.State.Model.Notice == "" {
		t.Errorf("cmds = %#v, notice = %q; want a refusal Notice and no cmd", out.Cmds, out.State.Model.Notice)
	}
	wantLane(t, out.State, 0, "No lane", "a")
}

func TestMoveEndsPointToCloseAndReopen(t *testing.T) {
	last := step(t, boardPane(t, boardCard("a", "ready-for-human")), key(">"))
	if len(last.Cmds) != 0 || !containsAll(last.State.Model.Notice, "press x") {
		t.Errorf("last Lane: cmds %#v, notice %q", last.Cmds, last.State.Model.Notice)
	}
	for _, k := range []string{"<", ">"} {
		done := step(t, boardPane(t, closedAt("d", 10, 5)), key(k))
		if len(done.Cmds) != 0 || !containsAll(done.State.Model.Notice, "press r") {
			t.Errorf("%s on Done: cmds %#v, notice %q", k, done.Cmds, done.State.Model.Notice)
		}
	}
	first := step(t, boardPane(t, boardCard("a")), key("<"))
	if len(first.Cmds) != 0 {
		t.Errorf("< on No lane emitted %#v", first.Cmds)
	}
}

func TestMoveOutOfTheFilteredSetNamesTheLane(t *testing.T) {
	s := boardPane(t, boardCard("a", "needs-triage", "bug"))
	s.Model.Filters.Label = "needs-triage"
	out := step(t, s, key(">"))
	wantMove(t, out, "needs-info", "needs-triage")
	if !containsAll(out.State.Model.Notice, "Needs info") {
		t.Errorf("Notice = %q, want it to name Needs info", out.State.Model.Notice)
	}
	s.Model.Filters.Label = "bug"
	if n := step(t, s, key(">")).State.Model.Notice; n != "" {
		t.Errorf("Move inside the filtered set left Notice %q", n)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}
