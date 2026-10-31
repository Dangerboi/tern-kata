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
			if open.Model.CurrentMode() != model.ModeCalendar {
				t.Fatalf("mode %s", open.Model.CurrentMode())
			}
			props := calendarProps(t, open, tt.card)
			if props == nil {
				t.Fatal("no calendar is drawn")
			}
			if props["cursor"] != "2026-10-12" || props["today"] != boardToday || props["value"] != "2026-10-12" {
				t.Errorf("calendar props %v", props)
			}
			footer := footerText(t, open)
			for _, want := range []string{"close calendar", "pick day"} {
				if !strings.Contains(footer, want) {
					t.Errorf("the footer %q lacks %q", footer, want)
				}
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
			if got := s.Model.DateCalendar.Cursor; got != "2026-10-20" {
				t.Fatalf("cursor %s, want 2026-10-20", got)
			}
			if got := tt.field(s.Model); got != "2026-10-12" {
				t.Fatalf("moving changed the field to %q", got)
			}
			picked := step(t, s, key("enter"))
			if got := tt.field(picked.State.Model); got != "2026-10-20" {
				t.Errorf("field %q after enter", got)
			}
			if picked.State.Model.TopCard() != tt.card || picked.State.Model.DateCalendar.Open || len(picked.Cmds) != 0 {
				t.Errorf("enter: card %v picker open %v cmds %v", picked.State.Model.TopCard(), picked.State.Model.DateCalendar.Open, picked.Cmds)
			}
		})
	}
}

func TestClickingADayPicksItAndClosesTheCalendar(t *testing.T) {
	for _, tt := range pickerCards() {
		t.Run(tt.name, func(t *testing.T) {
			s := step(t, tt.setup(t), key("ctrl+d")).State
			picked := step(t, s, &tern.Change{ID: view.CalendarID(tt.card), Value: "2026-11-03"}).State
			if got := tt.field(picked.Model); got != "2026-11-03" || picked.Model.DateCalendar.Open {
				t.Errorf("field %q, picker open %v", got, picked.Model.DateCalendar.Open)
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
	if got := step(t, tomorrow, key("ctrl+d")).State.Model.DateCalendar.Cursor; got != "2026-10-09" {
		t.Errorf("cursor for tomorrow %s", got)
	}
}

func TestCtrlDOnAFieldThatIsNotADateDoesNothing(t *testing.T) {
	s := typed(t, step(t, projectPane(t, 160, withDates("a", "", "")), key("n")).State, "Water")
	if got := step(t, s, key("ctrl+d")).State.Model.DateCalendar.Open; got {
		t.Error("the calendar opened on the title")
	}
}

func TestRecurrenceCalendarServesTheEndDateOnlyWhileTheRuleEndsOnADate(t *testing.T) {
	s := step(t, projectPane(t, 160, withDates("a", "", "")), key("shift+o")).State
	s.Model.RecurrenceForm.Field = model.RecurrenceEnd
	s.Model.RecurrenceForm.Start = "2026-10-12"
	if step(t, s, key("ctrl+d")).State.Model.DateCalendar.Open {
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

// The calendar belongs to one date field of one card: a click on another field takes it away, and it never keeps the keys while it is not drawn.
func TestACalendarClosesWhenItsFieldLosesFocus(t *testing.T) {
	t.Run("a click on another field", func(t *testing.T) {
		s := step(t, createCard(t), key("ctrl+d")).State
		s = step(t, s, &tern.Focus{ID: view.FieldID(model.CardCreate, model.CreateTitle)}).State
		if s.Model.DateCalendar.Open || s.Model.CurrentMode() != model.ModeCreate {
			t.Fatalf("calendar open %v, mode %s after the click", s.Model.DateCalendar.Open, s.Model.CurrentMode())
		}
		if got := typed(t, s, "x").Model.Create.Title; got != "Waterx" {
			t.Errorf("title %q after typing, want Waterx", got)
		}
	})
	t.Run("the Ends select leaving On a date", func(t *testing.T) {
		s := step(t, projectPane(t, 160, withDates("a", "", "")), key("shift+o")).State
		s.Model.RecurrenceForm.Field = model.RecurrenceEnd
		s.Model.RecurrenceForm.Ends = model.EndUntil
		s.Model.RecurrenceForm.Until = "2026-12-01"
		s = step(t, s, key("ctrl+d")).State
		if !s.Model.DateCalendar.Open {
			t.Fatal("the calendar did not open")
		}
		s = step(t, s, &tern.Change{ID: view.FieldID(model.CardRecurrence, model.RecurrenceEnds) + ".control", Value: "0"}).State
		if s.Model.DateCalendar.Open {
			t.Errorf("the calendar stayed open with the rule ending %v", s.Model.RecurrenceForm.Ends)
		}
	})
}

// A footer button's chord does what its button says even while the calendar is open.
func TestAFooterChordClosesTheCalendarAndRuns(t *testing.T) {
	s := step(t, createCard(t), key("ctrl+d")).State
	out := step(t, s, key("ctrl+o"))
	if out.State.Model.DateCalendar.Open || len(out.Cmds) == 0 {
		t.Errorf("save chord: calendar open %v, commands %d, want it closed and the Issue sent", out.State.Model.DateCalendar.Open, len(out.Cmds))
	}
	if step(t, s, key("ctrl+d")).State.Model.DateCalendar.Open {
		t.Error("ctrl+d on the open calendar did not close it")
	}
}

// The hint under a date field names the key the keymap binds, not a literal.
func TestTheCalendarHintNamesTheBoundKey(t *testing.T) {
	want := model.CardKeys(model.ModeCreate, model.ShowCalendar)[0].Key + " opens a calendar"
	for _, n := range wireNodes(t, view.Render(createCard(t).Model, now)) {
		if strings.HasSuffix(n.ID, "-calendar-hint") {
			if got := wireText(n); !strings.Contains(got, want) {
				t.Errorf("hint %s, want it to read %q", got, want)
			}
			return
		}
	}
	t.Error("no calendar hint is drawn")
}
