package driver_test

import (
	"errors"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
)

func undoEntry(n int) model.UndoEntry {
	return model.UndoEntry{Kind: model.UndoPriority, UID: "alpha", ProjectID: 7, ShortID: "aaaa", Actor: "tester",
		InstanceUID: "instance", Principal: "tester", Revision: int64(n), Before: model.IssueState{Revision: int64(n - 1)}}
}

func done(n int) model.FieldWritten {
	entry := undoEntry(n)
	return model.FieldWritten{UID: "alpha", Record: model.WriteRecord{Entry: &entry}}
}

// undoable is the Pane on alpha's detail after changes number 1 to count.
func undoable(t *testing.T, count int) driver.State {
	t.Helper()
	s := onAlphaDetail(t)
	for n := 1; n <= count; n++ {
		s = feed(t, s, done(n)).State
	}
	return s
}

func pressU(t *testing.T, s driver.State) driver.Output { return feed(t, s, key("u")) }

func undoCmd(t *testing.T, out driver.Output) model.UndoChange {
	t.Helper()
	for _, cmd := range out.Cmds {
		if c, ok := cmd.(model.UndoChange); ok {
			return c
		}
	}
	t.Fatalf("u sent %#v, want an undo", out.Cmds)
	return model.UndoChange{}
}

func TestUWithNothingDoneSaysSo(t *testing.T) {
	for name, s := range map[string]driver.State{"list": browsing(t, 160), "detail": onAlphaDetail(t)} {
		out := pressU(t, s)
		if got := out.State.Model.Notice; got != "nothing to undo" || len(out.Cmds) != 0 {
			t.Errorf("%s: notice %q, cmds %#v", name, got, out.Cmds)
		}
	}
}

func TestUInTheListAndTheDetailUndoesTheNewestChange(t *testing.T) {
	for name, back := range map[string]bool{"detail": false, "list": true} {
		s := undoable(t, 2)
		if back {
			s = feed(t, s, key("escape")).State
		}
		if got := undoCmd(t, pressU(t, s)); got.Entry.Revision != 2 || got.Workspace != "/repo" {
			t.Errorf("%s: undo %+v, want the newest change", name, got)
		}
	}
}

func TestTheHistoryHoldsTwentyChangesAndUndoesNewestFirst(t *testing.T) {
	s := undoable(t, 25)
	if got := len(s.Model.Undo.Entries); got != model.UndoLimit {
		t.Fatalf("%d entries held, want %d", got, model.UndoLimit)
	}
	for want := 25; want > 5; want-- {
		out := pressU(t, s)
		cmd := undoCmd(t, out)
		if cmd.Entry.Revision != int64(want) {
			t.Fatalf("undid change %d, want %d", cmd.Entry.Revision, want)
		}
		s = feed(t, out.State, model.ChangeUndone{EntryID: cmd.Entry.ID, Result: model.UndoChanged}).State
		if got, want := s.Model.Notice, "undid priority.set on #aaaa"; got != want {
			t.Fatalf("notice %q, want %q", got, want)
		}
	}
	if got := pressU(t, s).State.Model.Notice; got != "nothing to undo" {
		t.Fatalf("after the twentieth: %q", got)
	}
}

func TestAnUndoHidesNothingFromTheOlderChangesOfTheSameIssue(t *testing.T) {
	s := undoable(t, 2)
	cmd := undoCmd(t, pressU(t, s))
	s = feed(t, s, key("u"), model.ChangeUndone{EntryID: cmd.Entry.ID, Result: model.UndoChanged, FromRevision: 2, ToRevision: 3}).State
	// The older change was guarded on revision 1 after the newer one; it now waits on the revision the undo wrote.
	if got := s.Model.Undo.Entries[0].Revision; got != 3 {
		t.Fatalf("older change guarded on revision %d, want 3", got)
	}
}

func TestASecondUWhileOneUndoIsOutIsRefused(t *testing.T) {
	s := undoable(t, 2)
	out := pressU(t, s)
	again := pressU(t, out.State)
	if len(again.Cmds) != 0 || again.State.Model.Notice != "undo already in progress" {
		t.Fatalf("second u: %q, cmds %#v", again.State.Model.Notice, again.Cmds)
	}
}

func TestEachGuardsMessageAndWhatItDoesToTheHistory(t *testing.T) {
	for _, tt := range []struct {
		conflict string
		clears   bool
	}{
		{"daemon instance changed", true},
		{"daemon principal changed", true},
		{"issue identity changed", false},
		{"issue owner changed since the action", false},
		{"issue priority changed since the action", false},
		{"issue body changed since the action", false},
		{"issue status changed since the action", false},
		{"issue changed since the label edit", false},
		{"created link changed since the action", false},
		{"issue dates changed since the action", false},
	} {
		s := undoable(t, 2)
		out := pressU(t, s)
		cmd := undoCmd(t, out)
		got := feed(t, out.State, model.ChangeUndone{EntryID: cmd.Entry.ID, Result: model.UndoConflict, Conflict: tt.conflict, Ref: "aaaa"}).State.Model
		if tt.clears {
			if want := "cannot undo #aaaa: " + tt.conflict; got.Notice != want || len(got.Undo.Entries) != 0 {
				t.Errorf("%s: notice %q, %d entries", tt.conflict, got.Notice, len(got.Undo.Entries))
			}
			if next, _ := model.Update(got, model.KeyMsg{Chord: "u"}); next.Notice != "nothing to undo: "+tt.conflict {
				t.Errorf("%s: next u says %q", tt.conflict, next.Notice)
			}
			continue
		}
		if want := "skipped undo for #aaaa: " + tt.conflict; got.Notice != want || len(got.Undo.Entries) != 1 {
			t.Errorf("%s: notice %q, %d entries, want the refused one dropped and the older one kept", tt.conflict, got.Notice, len(got.Undo.Entries))
		}
	}
}

func TestOtherEndsOfAnUndo(t *testing.T) {
	for _, tt := range []struct {
		name    string
		answer  model.ChangeUndone
		notice  string
		entries int
	}{
		{"already restored", model.ChangeUndone{Result: model.UndoAlready, Ref: "aaaa"}, "#aaaa already restored; no change made", 1},
		{"failed", model.ChangeUndone{Result: model.UndoFailed, Ref: "aaaa", Err: errors.New("boom")}, "undo failed for #aaaa: boom", 2},
		{"unknown", model.ChangeUndone{Result: model.UndoUnknown, Ref: "aaaa", Err: errors.New("deadline")}, "undo outcome unknown; refreshed issue state", 0},
	} {
		s := undoable(t, 2)
		out := pressU(t, s)
		tt.answer.EntryID = undoCmd(t, out).Entry.ID
		got := feed(t, out.State, tt.answer).State.Model
		if got.Notice != tt.notice || len(got.Undo.Entries) != tt.entries || got.Undo.InFlight {
			t.Errorf("%s: notice %q, %d entries, in flight %v", tt.name, got.Notice, len(got.Undo.Entries), got.Undo.InFlight)
		}
	}
	// An unknown outcome of the undo clears the history, and the next u says why.
	s := undoable(t, 2)
	out := pressU(t, s)
	s = feed(t, out.State, model.ChangeUndone{EntryID: undoCmd(t, out).Entry.ID, Result: model.UndoUnknown}).State
	if got := pressU(t, s).State.Model.Notice; got != "nothing to undo: undo outcome is unknown" {
		t.Fatalf("after an unknown undo: %q", got)
	}
}

func TestAWriteWithAnUnknownOutcomeClearsTheHistory(t *testing.T) {
	s := undoable(t, 3)
	s = feed(t, s, model.FieldWritten{UID: "alpha", Err: errors.New("deadline exceeded"), Record: model.WriteRecord{Unknown: true}}).State
	if got := pressU(t, s).State.Model.Notice; got != "nothing to undo: previous action's outcome is unknown" {
		t.Fatalf("notice %q", got)
	}
}

func TestAWriteThatCannotBeUndoneClearsTheHistoryWithItsReason(t *testing.T) {
	s := undoable(t, 3)
	s = feed(t, s, model.FieldWritten{UID: "alpha", Record: model.WriteRecord{Boundary: "closing a recurring issue cannot be undone"}}).State
	if got := pressU(t, s).State.Model.Notice; got != "nothing to undo: closing a recurring issue cannot be undone" {
		t.Fatalf("notice %q", got)
	}
}

func TestATimedAssignmentClearsTheHistory(t *testing.T) {
	s := undoable(t, 3)
	s = feed(t, s, key("t"), key("1"), key("h"), key("enter"), model.FieldWritten{UID: "alpha"}).State
	if got := pressU(t, s).State.Model.Notice; got != "nothing to undo: timed assignment cannot be undone" {
		t.Fatalf("notice %q", got)
	}
}

func TestUndoingAReopenThatNeedsEvidenceOpensTheCloseCard(t *testing.T) {
	s := undoable(t, 1)
	entry := undoEntry(2)
	entry.Kind = model.UndoReopen
	s = feed(t, s, model.FieldWritten{UID: "alpha", Record: model.WriteRecord{Entry: &entry}}).State
	out := pressU(t, s)
	cmd := undoCmd(t, out)
	s = feed(t, out.State, model.ChangeUndone{EntryID: cmd.Entry.ID, Result: model.UndoNeedsEvidence, Ref: "aaaa"}).State
	f := s.Model.CloseForm
	if !f.Open || f.UID != "alpha" || f.Values.Reason != "done" || f.UndoEntry != cmd.Entry.ID || len(s.Model.Undo.Entries) != 2 {
		t.Fatalf("close card %+v, history %d", f, len(s.Model.Undo.Entries))
	}
	s = feed(t, s,
		model.CloseFieldMsg{Field: model.CloseMessage, Value: "Finished again after the reopen, so closing it as done once more."},
		model.CloseFieldMsg{Field: model.CloseCommit, Value: "0123456789abcdef"},
	).State
	out = feed(t, s, key("ctrl+o"))
	sent := undoCmd(t, out)
	if sent.Close == nil || sent.Close.Reason != model.ReasonDone || sent.Close.Key == "" || len(sent.Close.Evidence) != 1 || sent.Entry.ID != cmd.Entry.ID {
		t.Fatalf("the card sent %+v", sent)
	}
	s = feed(t, out.State, model.ChangeUndone{EntryID: cmd.Entry.ID, Result: model.UndoChanged, Ref: "aaaa"}).State
	if s.Model.CloseForm.Open || len(s.Model.Undo.Entries) != 1 {
		t.Fatalf("after the close: card %+v, history %d", s.Model.CloseForm, len(s.Model.Undo.Entries))
	}
}

func TestTheCloseCardOfAnUndoRefusesQuickDoneAndAnotherReason(t *testing.T) {
	s := undoable(t, 1)
	out := pressU(t, s)
	cmd := undoCmd(t, out)
	s = feed(t, out.State, model.ChangeUndone{EntryID: cmd.Entry.ID, Result: model.UndoNeedsEvidence}).State
	out = feed(t, s, key("ctrl+d"))
	if len(out.Cmds) != 0 || out.State.Model.CloseForm.Err == nil {
		t.Fatalf("quick done sent %#v, form %+v", out.Cmds, out.State.Model.CloseForm)
	}
}
