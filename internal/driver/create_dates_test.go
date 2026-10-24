package driver_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

// createCard opens the Create card with a title typed and the cursor on the Scheduled field, five tabs on.
func createCard(t *testing.T) driver.State {
	t.Helper()
	s := typed(t, step(t, projectPane(t, 160, withDates("a", "", "")), key("n")).State, "Water")
	for range 5 {
		s = step(t, s, key("tab")).State
	}
	return s
}

func createField(t *testing.T, s driver.State, name model.Field) string {
	t.Helper()
	text, ok := nodeText(t, s, view.FieldID(model.CardCreate, name))
	if !ok {
		t.Fatalf("the Create card draws no %s field", name)
	}
	return text
}

func createCmd(t *testing.T, out driver.Output) model.CreateIssueCmd {
	t.Helper()
	var found []model.CreateIssueCmd
	for _, c := range out.Cmds {
		if cmd, ok := c.(model.CreateIssueCmd); ok {
			found = append(found, cmd)
		}
	}
	if len(found) != 1 {
		t.Fatalf("cmds %+v, want one create", out.Cmds)
	}
	return found[0]
}

func TestCreateCardReachesScheduledThenDeadlineAfterPriorityByTab(t *testing.T) {
	s := createCard(t)
	if s.Model.Create.Field != model.CreateScheduled {
		t.Fatalf("five tabs reach %s", s.Model.Create.Field)
	}
	if s = step(t, s, key("tab")).State; s.Model.Create.Field != model.CreateDeadline {
		t.Fatalf("one more tab reaches %s", s.Model.Create.Field)
	}
	if s = step(t, s, key("tab")).State; s.Model.Create.Field != model.CreateTitle {
		t.Fatalf("tab after the Deadline reaches %s", s.Model.Create.Field)
	}
	if got := step(t, createCard(t), key("shift+tab")).State.Model.Create.Field; got != model.CreatePriority {
		t.Errorf("back from Scheduled reaches %s", got)
	}
}

func TestCreateCardPreviewsEachDateAgainstThePaneCalendar(t *testing.T) {
	for _, tt := range []struct{ input, want string }{
		{"2026-10-24", "2026-10-24 Sat"},
		{"today", "2026-10-08 Thu"},
		{"tomorrow", "2026-10-09 Fri"},
		{"+2w", "2026-10-22 Thu"},
		{"", "no date"},
		{"someday", ""},
	} {
		s := typed(t, createCard(t), tt.input)
		if got, _ := nodeText(t, s, "layer.create.scheduled-preview"); got != tt.want {
			t.Errorf("Scheduled %q previews %q, want %q", tt.input, got, tt.want)
		}
		if got := createField(t, s, model.CreateScheduled); got != tt.input {
			t.Errorf("Scheduled field holds %q, want %q", got, tt.input)
		}
	}
	s := typed(t, step(t, createCard(t), key("tab")).State, "+3d")
	if got, _ := nodeText(t, s, "layer.create.deadline-preview"); got != "2026-10-11 Sun" {
		t.Errorf("Deadline previews %q", got)
	}
}

func TestCreateSendsTheDatesInTheOneCreateCommand(t *testing.T) {
	s := typed(t, createCard(t), "today")
	s = typed(t, step(t, s, key("tab")).State, "+1w")
	out := step(t, s, key("ctrl+o"))
	cmd := createCmd(t, out)
	if cmd.Title != "Water" || cmd.ScheduledOn != "2026-10-08" || cmd.DeadlineOn != "2026-10-15" || cmd.Key == "" {
		t.Fatalf("command %+v", cmd)
	}
}

func TestCreateWithoutDatesSendsNone(t *testing.T) {
	cmd := createCmd(t, step(t, createCard(t), key("ctrl+o")))
	if cmd.ScheduledOn != "" || cmd.DeadlineOn != "" {
		t.Fatalf("command %+v carries dates", cmd)
	}
}

func TestCreateRefusesAnInvalidDateNamingItsFieldAndCreatesNothing(t *testing.T) {
	s := typed(t, createCard(t), "someday")
	out := step(t, s, key("ctrl+o"))
	if len(out.Cmds) != 0 {
		t.Fatalf("cmds %+v", out.Cmds)
	}
	f := out.State.Model.Create
	if !f.Open || f.Saving || f.Err == nil || !strings.HasPrefix(f.Err.Error(), "Scheduled must be "+model.DateInputHint) {
		t.Fatalf("card open %v saving %v err %v", f.Open, f.Saving, f.Err)
	}

	s = typed(t, step(t, createCard(t), key("tab")).State, "+x")
	out = step(t, s, key("ctrl+o"))
	if err := out.State.Model.Create.Err; len(out.Cmds) != 0 || err == nil || !strings.HasPrefix(err.Error(), "Deadline must be ") {
		t.Fatalf("cmds %+v err %v", out.Cmds, err)
	}
}

func TestCreateKeepsItsKeyForARetryAndMintsAnotherWhenADateChanges(t *testing.T) {
	s := typed(t, createCard(t), "tomorrow")
	first := step(t, s, key("ctrl+o"))
	failed := step(t, first.State, model.IssueCreated{Err: errors.New("kata said no")})
	retry := createCmd(t, step(t, failed.State, key("ctrl+o")))
	if retry.Key != createCmd(t, first).Key {
		t.Errorf("retry key %q differs from %q", retry.Key, createCmd(t, first).Key)
	}
	edited := typed(t, emptied(t, failed.State), "today")
	if changed := createCmd(t, step(t, edited, key("ctrl+o"))); changed.Key == retry.Key {
		t.Errorf("an edited date reuses key %q", changed.Key)
	}
}
