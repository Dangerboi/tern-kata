package driver_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// footButton is the click action of the button drawn as id by the card on top of s.
func footButton(t *testing.T, s driver.State, id string) string {
	t.Helper()
	buttons, _ := cardButtonsOf(t, s)
	act, ok := buttons[id]
	if !ok {
		t.Fatalf("no button %s among %v", id, buttons)
	}
	return act
}

func openedCalendar(t *testing.T, s driver.State) driver.State {
	t.Helper()
	s = step(t, s, key("ctrl+d")).State
	if !s.Model.DateCalendar.Open {
		t.Fatal("the calendar did not open")
	}
	return s
}

// A card button does what it says whatever overlay of the card is open: Save dates saves what the fields hold and does not
// pick the calendar cursor's day.
func TestSaveDatesButtonSavesWhileTheCalendarIsOpen(t *testing.T) {
	s := step(t, projectPane(t, 160, withDates("a", "", "")), key("d")).State
	s.Model.DatesForm.Text.Scheduled = "2026-10-15"
	s = step(t, openedCalendar(t, s), key("right")).State
	out := step(t, s, click("layer.dates.foot.save", footButton(t, s, "layer.dates.foot.save")))
	if got := out.State.Model.DatesForm.Text.Scheduled; got != "2026-10-15" {
		t.Errorf("Scheduled = %q, want the typed date", got)
	}
	if out.State.Model.DateCalendar.Open {
		t.Error("the calendar is still open")
	}
	if !out.State.Model.DatesForm.Saving || len(out.Cmds) == 0 {
		t.Error("Save dates did not save")
	}
}

func TestCancelButtonCancelsTheCardWhileTheCalendarIsOpen(t *testing.T) {
	recurrence := func(t *testing.T) driver.State {
		s := editingCardNamed(t, "Recurrence").open(t)
		s.Model.RecurrenceForm.Field = model.RecurrenceStart
		return s
	}
	for name, tt := range map[string]struct {
		open   func(*testing.T) driver.State
		id     string
		closed func(model.Model) bool
	}{
		"Create": {createCard, "layer.create.foot.cancel", func(m model.Model) bool { return m.TopCard() != model.CardCreate || m.Create.DiscardConfirm }},
		"Dates": {func(t *testing.T) driver.State {
			return step(t, projectPane(t, 160, withDates("a", "", "")), key("d")).State
		}, "layer.dates.foot.cancel", func(m model.Model) bool { return !m.DatesForm.Open }},
		"Recurrence": {recurrence, "layer.recurrence.foot.cancel", func(m model.Model) bool { return !m.RecurrenceForm.Open }},
	} {
		t.Run(name, func(t *testing.T) {
			s := openedCalendar(t, tt.open(t))
			out := step(t, s, click(tt.id, footButton(t, s, tt.id)))
			if out.State.Model.DateCalendar.Open {
				t.Error("the calendar is still open")
			}
			if !tt.closed(out.State.Model) {
				t.Errorf("Cancel left the card in %s", out.State.Model.CurrentMode())
			}
		})
	}
}

func TestCancelButtonCancelsTheCardWhileASelectIsOpen(t *testing.T) {
	c := editingCardNamed(t, "Filter")
	s := step(t, c.open(t), &tern.Toggle{ID: view.FieldID(c.card, model.FilterRelationship) + ".control", Open: true}).State
	if !s.Model.OpenDropdown(c.card, model.FilterRelationship).Open {
		t.Fatal("the select did not open")
	}
	out := step(t, s, click("layer.filters.foot.cancel", footButton(t, s, "layer.filters.foot.cancel")))
	if out.State.Model.FilterForm.Open || out.State.Model.Dropdown.Open {
		t.Errorf("Cancel left the Filter card open=%v dropdown=%v", out.State.Model.FilterForm.Open, out.State.Model.Dropdown.Open)
	}
}

func editingCardNamed(t *testing.T, name string) editingCard {
	t.Helper()
	for _, c := range editingCards() {
		if c.name == name {
			return c
		}
	}
	t.Fatalf("no editing card %s", name)
	return editingCard{}
}
