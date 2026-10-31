package driver_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// pickerCard is one card with a date field the calendar opens beside.
type pickerCard struct {
	name  string
	card  model.Card
	setup func(t *testing.T) driver.State
	// field is the text of the date field the picker is for; other is the card's other date.
	field func(m model.Model) string
}

func pickerCards() []pickerCard {
	return []pickerCard{
		{"Dates card", model.CardDates, func(t *testing.T) driver.State {
			s := step(t, projectPane(t, 160, withDates("a", "2026-10-12", "2026-10-30")), key("d")).State
			return s
		}, func(m model.Model) string { return m.DatesForm.Text.Scheduled }},
		{"Create card", model.CardCreate, func(t *testing.T) driver.State {
			s := createCard(t)
			return typed(t, s, "2026-10-12")
		}, func(m model.Model) string { return m.Create.Dates.Scheduled }},
		{"Recurrence card", model.CardRecurrence, func(t *testing.T) driver.State {
			s := step(t, projectPane(t, 160, withDates("a", "", "")), key("shift+o")).State
			s.Model.RecurrenceForm.Field = model.RecurrenceStart
			s.Model.RecurrenceForm.Start = "2026-10-12"
			return s
		}, func(m model.Model) string { return m.RecurrenceForm.Start }},
	}
}

func calendarProps(t *testing.T, s driver.State, card model.Card) map[string]any {
	t.Helper()
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if n.ID == view.CalendarID(card) {
			return n.P
		}
	}
	return nil
}

func TestCtrlDOpensACalendarBesideTheDateFieldAndTheKeyLineNamesIt(t *testing.T) {
	for _, tt := range pickerCards() {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.setup(t)
			if calendarProps(t, s, tt.card) != nil {
				t.Fatal("a calendar is drawn before the key")
			}
			open := step(t, s, key("ctrl+d")).State
			if open.Model.CurrentMode() != model.ModeDatePicker {
				t.Fatalf("mode %s", open.Model.CurrentMode())
			}
			props := calendarProps(t, open, tt.card)
			if props == nil {
				t.Fatal("no calendar is drawn")
			}
			if props["cursor"] != "2026-10-12" || props["today"] != boardToday || props["value"] != "2026-10-12" {
				t.Errorf("calendar props %v", props)
			}
			var hinted bool
			for b := range model.BindingsForMode(model.ModeDatePicker, true) {
				hinted = hinted || b.ShowInFooter
			}
			if !hinted {
				t.Error("the key line shows no calendar keys")
			}
			closed := step(t, open, key("escape")).State
			if closed.Model.TopCard() != tt.card || calendarProps(t, closed, tt.card) != nil || tt.field(closed.Model) != "2026-10-12" {
				t.Errorf("esc: card %v field %q", closed.Model.TopCard(), tt.field(closed.Model))
			}
		})
	}
}

func TestCalendarArrowsMoveTheCursorAndEnterPicksIntoTheField(t *testing.T) {
	for _, tt := range pickerCards() {
		t.Run(tt.name, func(t *testing.T) {
			s := step(t, tt.setup(t), key("ctrl+d")).State
			for _, chord := range []string{"right", "right", "down", "left", "up", "down"} {
				s = step(t, s, key(chord)).State
			}
			if got := s.Model.DatePick.Cursor; got != "2026-10-20" {
				t.Fatalf("cursor %s, want 2026-10-20", got)
			}
			if got := tt.field(s.Model); got != "2026-10-12" {
				t.Fatalf("moving changed the field to %q", got)
			}
			picked := step(t, s, key("enter"))
			if got := tt.field(picked.State.Model); got != "2026-10-20" {
				t.Errorf("field %q after enter", got)
			}
			if picked.State.Model.TopCard() != tt.card || picked.State.Model.DatePick.Open || len(picked.Cmds) != 0 {
				t.Errorf("enter: card %v picker open %v cmds %v", picked.State.Model.TopCard(), picked.State.Model.DatePick.Open, picked.Cmds)
			}
		})
	}
}

func TestClickingADayPicksItAndClosesTheCalendar(t *testing.T) {
	for _, tt := range pickerCards() {
		t.Run(tt.name, func(t *testing.T) {
			s := step(t, tt.setup(t), key("ctrl+d")).State
			picked := step(t, s, &tern.Change{ID: view.CalendarID(tt.card), Value: "2026-11-03"}).State
			if got := tt.field(picked.Model); got != "2026-11-03" || picked.Model.DatePick.Open {
				t.Errorf("field %q, picker open %v", got, picked.Model.DatePick.Open)
			}
		})
	}
}

func TestCalendarCursorStartsOnTodayForBlankOrRelativeTextAndMarksTheOtherDate(t *testing.T) {
	s := step(t, projectPane(t, 160, withDates("a", "", "2026-10-30")), key("d")).State
	open := step(t, s, key("ctrl+d")).State
	props := calendarProps(t, open, model.CardDates)
	if props["cursor"] != boardToday || props["value"] != nil {
		t.Errorf("blank field: props %v", props)
	}
	marks, _ := props["marks"].([]any)
	if len(marks) != 1 || !strings.Contains(fmt.Sprint(marks[0]), "2026-10-30") {
		t.Errorf("marks %v, want the Deadline", props["marks"])
	}
	tomorrow := typed(t, s, "tomorrow")
	if got := step(t, tomorrow, key("ctrl+d")).State.Model.DatePick.Cursor; got != "2026-10-09" {
		t.Errorf("cursor for tomorrow %s", got)
	}
}

func TestCtrlDOnAFieldThatIsNotADateDoesNothing(t *testing.T) {
	s := typed(t, step(t, projectPane(t, 160, withDates("a", "", "")), key("n")).State, "Water")
	if got := step(t, s, key("ctrl+d")).State.Model.DatePick.Open; got {
		t.Error("the calendar opened on the title")
	}
}

func TestRecurrenceCalendarServesTheEndDateOnlyWhileTheRuleEndsOnADate(t *testing.T) {
	s := step(t, projectPane(t, 160, withDates("a", "", "")), key("shift+o")).State
	s.Model.RecurrenceForm.Field = model.RecurrenceEnd
	s.Model.RecurrenceForm.Start = "2026-10-12"
	if step(t, s, key("ctrl+d")).State.Model.DatePick.Open {
		t.Fatal("the calendar opened on an occurrence count")
	}
	s.Model.RecurrenceForm.Ends = model.EndUntil
	s.Model.RecurrenceForm.Until = "2026-12-01"
	open := step(t, s, key("ctrl+d")).State
	props := calendarProps(t, open, model.CardRecurrence)
	if props["cursor"] != "2026-12-01" || !strings.Contains(fmt.Sprint(props["marks"]), "2026-10-12") {
		t.Fatalf("calendar props %v", props)
	}
	picked := step(t, step(t, open, key("down")).State, key("enter")).State
	if got := picked.Model.RecurrenceForm.Until; got != "2026-12-08" {
		t.Errorf("end date %q", got)
	}
}
