package driver_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

func withDates(uid, scheduled, deadline string) model.Issue {
	issue := boardCard(uid, "ready-for-agent")
	issue.ProjectID = 7
	issue.Metadata = map[string]any{}
	if scheduled != "" {
		issue.ScheduledOnDate, issue.Metadata["scheduled_on"] = &scheduled, scheduled
	}
	if deadline != "" {
		issue.DeadlineOnDate, issue.Metadata["deadline_on"] = &deadline, deadline
	}
	return issue
}

func datesField(t *testing.T, s driver.State, name model.Field) string {
	t.Helper()
	text, ok := nodeText(t, s, view.FieldID(model.CardDates, name))
	if !ok {
		t.Fatalf("the Dates card draws no %s field", name)
	}
	return text
}

func datesPreview(t *testing.T, s driver.State, name model.Field) string {
	t.Helper()
	text, _ := nodeText(t, s, view.LayerID(model.CardDates)+"."+string(name)+"-preview")
	return text
}

// emptied clears the focused field of the open card.
func emptied(t *testing.T, s driver.State) driver.State {
	t.Helper()
	return step(t, s, key("ctrl+u")).State
}

func TestDOpensTheDatesCardPrefilledInEveryModeAndEscClosesItUnchanged(t *testing.T) {
	issue := withDates("a", "2026-10-12", "2026-11-02T17:30:00")
	list := projectPane(t, 160, issue)
	board := boardPane(t, issue)
	lane := step(t, board, key("enter")).State
	detail := step(t, list, key("enter")).State
	for name, s := range map[string]driver.State{"list": list, "Board": board, "Lane view": lane, "detail": detail} {
		t.Run(name, func(t *testing.T) {
			open := step(t, s, key("d"))
			m := open.State.Model
			if m.CurrentMode() != model.ModeDatesForm || m.TopCard() != model.CardDates || len(open.Cmds) != 0 {
				t.Fatalf("mode %s card %v cmds %v", m.CurrentMode(), m.TopCard(), open.Cmds)
			}
			if got := datesField(t, open.State, model.DatesScheduled); got != "2026-10-12" {
				t.Errorf("Scheduled = %q", got)
			}
			if got := datesField(t, open.State, model.DatesDeadline); got != "2026-11-02T17:30:00" {
				t.Errorf("Deadline = %q, want the value as kata holds it", got)
			}
			if !strings.HasPrefix(open.State.Focus, view.LayerID(model.CardDates)) {
				t.Errorf("text focus %q is not on the Dates card", open.State.Focus)
			}
			typedIn := typed(t, open.State, "x")
			closed := step(t, typedIn, key("escape"))
			if closed.State.Model.TopCard() != model.NoCard || len(closed.Cmds) != 0 || !reflect.DeepEqual(closed.State.Model.Issues, s.Model.Issues) {
				t.Fatalf("esc: card %v cmds %v", closed.State.Model.TopCard(), closed.Cmds)
			}
		})
	}
}

func TestDatesCardResolvesEachFormAgainstThePaneCalendarAndPreviewsIt(t *testing.T) {
	for _, tt := range []struct{ input, want string }{
		{"2026-10-24", "2026-10-24 Sat"},
		{"today", "2026-10-08 Thu"},
		{"tomorrow", "2026-10-09 Fri"},
		{"+3d", "2026-10-11 Sun"},
		{"+2w", "2026-10-22 Thu"},
		{"+30d", "2026-11-07 Sat"},
		{"", "no date"},
	} {
		s := emptied(t, step(t, projectPane(t, 160, withDates("a", "2026-10-12", "")), key("d")).State)
		s = typed(t, s, tt.input)
		if got := datesPreview(t, s, model.DatesScheduled); got != tt.want {
			t.Errorf("%q previews %q, want %q", tt.input, got, tt.want)
		}
	}
	s := typed(t, emptied(t, step(t, projectPane(t, 160, withDates("a", "", "")), key("d")).State), "someday")
	if got := datesPreview(t, s, model.DatesScheduled); got != "" {
		t.Errorf("invalid input previews %q", got)
	}
}

func TestDatesCardSavesOnlyChangedKeysInOneCommand(t *testing.T) {
	issue := withDates("a", "2026-10-12", "2026-11-02T17:30:00")
	s := step(t, projectPane(t, 160, issue), key("d")).State
	s = typed(t, emptied(t, s), "+1w")
	out := step(t, s, key("enter"))
	want := model.SetDatesCmd{Workspace: "/repo", UID: "a", ProjectID: 7, Changes: []model.DateChange{
		{Key: "scheduled_on", Label: "Scheduled", Value: "2026-10-15", Was: "2026-10-12"}}}
	if len(out.Cmds) != 1 || !reflect.DeepEqual(out.Cmds[0], want) {
		t.Fatalf("cmds %+v, want one %+v (the Deadline's time of day is left alone)", out.Cmds, want)
	}
	if !out.State.Model.DatesForm.Saving {
		t.Error("the card is not saving")
	}

	done := step(t, out.State, model.DatesResultMsg{UID: "a", Applied: want.Changes})
	if done.State.Model.TopCard() != model.NoCard {
		t.Errorf("card %v still open after the write", done.State.Model.TopCard())
	}
	if _, ok := snapshotFor(done.Cmds); !ok {
		t.Errorf("no fresh snapshot asked for after the write: %v", done.Cmds)
	}
}

func TestDatesCardSetsAndClearsBothAndGuardsOnWhatItOpenedWith(t *testing.T) {
	s := step(t, projectPane(t, 160, withDates("a", "", "2026-10-30")), key("d")).State
	s = typed(t, s, "today")
	s = step(t, s, key("tab")).State
	s = emptied(t, s)
	out := step(t, s, key("enter"))
	want := []model.DateChange{
		{Key: "scheduled_on", Label: "Scheduled", Value: "2026-10-08"},
		{Key: "deadline_on", Label: "Deadline", Value: "", Was: "2026-10-30"},
	}
	if len(out.Cmds) != 1 || !reflect.DeepEqual(out.Cmds[0].(model.SetDatesCmd).Changes, want) {
		t.Fatalf("cmds %+v, want changes %+v", out.Cmds, want)
	}
}

func TestDatesCardRefusesInvalidInputNamingTheField(t *testing.T) {
	s := step(t, projectPane(t, 160, withDates("a", "2026-10-12", "2026-10-30")), key("d")).State
	s = step(t, s, key("tab")).State
	s = typed(t, emptied(t, s), "next week")
	out := step(t, s, key("enter"))
	f := out.State.Model.DatesForm
	if len(out.Cmds) != 0 || !f.Open || f.Err == nil || !strings.HasPrefix(f.Err.Error(), "Deadline must be ") || f.Saving {
		t.Fatalf("cmds %v open %v err %v", out.Cmds, f.Open, f.Err)
	}
	// The refusal is drawn, and the next edit takes it away.
	if text, ok := nodeText(t, out.State, view.LayerID(model.CardDates)+".error"); !ok || !strings.HasPrefix(text, "Deadline") {
		t.Errorf("drawn error %q %v", text, ok)
	}
	if step(t, out.State, key("x")).State.Model.DatesForm.Err != nil {
		t.Error("an edit left the refusal")
	}
	for _, bad := range []string{"2026-13-01", "2026-02-30", "+d", "+3", "-1d", "26-10-12"} {
		s := step(t, projectPane(t, 160, withDates("a", "", "")), key("d")).State
		out := step(t, typed(t, s, bad), key("enter"))
		if f := out.State.Model.DatesForm; len(out.Cmds) != 0 || f.Err == nil || !strings.HasPrefix(f.Err.Error(), "Scheduled must be ") {
			t.Errorf("%q: cmds %v err %v", bad, out.Cmds, f.Err)
		}
	}
}

func TestDatesCardSavingNothingChangedSendsNothing(t *testing.T) {
	s := step(t, projectPane(t, 160, withDates("a", "2026-10-12", "2026-11-02T17:30:00")), key("d")).State
	out := step(t, s, key("enter"))
	if len(out.Cmds) != 0 || out.State.Model.TopCard() != model.NoCard {
		t.Fatalf("cmds %v card %v", out.Cmds, out.State.Model.TopCard())
	}
}

func TestDatesCardReportsAChangedKeyAndKeepsTheCardOpen(t *testing.T) {
	s := step(t, projectPane(t, 160, withDates("a", "2026-10-12", "2026-10-30")), key("d")).State
	s = typed(t, emptied(t, s), "+1d")
	s = step(t, s, key("tab")).State
	s = typed(t, emptied(t, s), "+2d")
	out := step(t, s, key("enter"))
	first := model.DateChange{Key: "scheduled_on", Label: "Scheduled", Value: "2026-10-09", Was: "2026-10-12"}
	// The Scheduled write landed; the Deadline moved meanwhile.
	failed := step(t, out.State, model.DatesResultMsg{UID: "a", Applied: []model.DateChange{first}, Err: model.DatesChangedError("Deadline")})
	f := failed.State.Model.DatesForm
	if !f.Open || f.Saving || f.Err == nil || !strings.Contains(f.Err.Error(), "Deadline changed elsewhere") {
		t.Fatalf("open %v saving %v err %v", f.Open, f.Saving, f.Err)
	}
	if _, ok := snapshotFor(failed.Cmds); !ok {
		t.Errorf("the written Scheduled was not read back: %v", failed.Cmds)
	}
	if got := datesField(t, failed.State, model.DatesDeadline); got != "+2d" {
		t.Errorf("Deadline text %q lost", got)
	}
	// Saving again writes only what is still unwritten, guarded on what is held now.
	again := step(t, failed.State, key("enter"))
	if len(again.Cmds) != 1 || len(again.Cmds[0].(model.SetDatesCmd).Changes) != 1 || again.Cmds[0].(model.SetDatesCmd).Changes[0].Key != "deadline_on" {
		t.Fatalf("second save: %+v", again.Cmds)
	}

	// A failure with nothing written keeps the card open with the reason and asks for nothing.
	plain := step(t, out.State, model.DatesResultMsg{UID: "a", Err: errors.New("kata said no")})
	if f := plain.State.Model.DatesForm; !f.Open || f.Err == nil || f.Err.Error() != "kata said no" || len(plain.Cmds) != 0 {
		t.Fatalf("open %v err %v cmds %v", f.Open, f.Err, plain.Cmds)
	}
}

func TestDatesCardIsRefusedWhileTheDaemonIsDisconnected(t *testing.T) {
	s := projectPane(t, 160, withDates("a", "2026-10-12", ""))
	s.Model.Stale = true
	if got := step(t, s, key("d")).State.Model.TopCard(); got != model.NoCard {
		t.Fatalf("d opened %v while disconnected", got)
	}
	// A card already open refuses the save when the daemon drops.
	s = step(t, projectPane(t, 160, withDates("a", "2026-10-12", "")), key("d")).State
	s = typed(t, s, "1")
	s.Model.Stale = true
	out := step(t, s, key("enter"))
	if f := out.State.Model.DatesForm; len(out.Cmds) != 0 || !f.Open || f.Err == nil || !strings.Contains(f.Err.Error(), "disconnected") {
		t.Fatalf("cmds %v open %v err %v", out.Cmds, f.Open, f.Err)
	}
}
