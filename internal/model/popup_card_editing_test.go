package model_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func cardNamed(t *testing.T, name string) popupCard {
	t.Helper()
	i := slices.IndexFunc(popupCards(), func(c popupCard) bool { return c.name == name })
	if i < 0 {
		t.Fatalf("card %q has no row", name)
	}
	return popupCards()[i]
}

// fieldEditingKeys are the keys of the shared field-editing actions plus space, which steps a choice.
var fieldEditingKeys = []string{"tab", "shift+tab", "right", "left", "space", "backspace", "ctrl+u"}

// A card that is busy (saving, asking to discard, showing a conflict or refusing) takes none of the field-editing keys: its
// state is unchanged, focus stays and no text field holds the caret.
func TestBusyCardsIgnoreFieldEditingKeys(t *testing.T) {
	cases := []struct {
		card, busy string
		make       func(model.Model) model.Model
	}{
		{"Create", "saving", func(m model.Model) model.Model { m.Create.Saving = true; return m }},
		{"Create", "asking to discard", func(m model.Model) model.Model { m.Create.DiscardConfirm = true; return m }},
		{"Close", "saving", func(m model.Model) model.Model { m.CloseForm.Saving = true; return m }},
		{"Dates", "saving", func(m model.Model) model.Model { m.DatesForm.Saving = true; return m }},
		{"Recurrence", "saving", func(m model.Model) model.Model { m.RecurrenceForm.Saving = true; return m }},
		{"Recurrence", "asking to stop", func(m model.Model) model.Model { m.RecurrenceForm.StopConfirm = true; return m }},
		{"Recurrence", "refused", func(m model.Model) model.Model { m.RecurrenceForm.Refused = true; return m }},
		{"Job", "saving", func(m model.Model) model.Model { m.JobForm.Saving = true; return m }},
		{"Job", "showing a conflict", func(m model.Model) model.Model { m.JobForm.Conflict = &model.CronJob{UID: "J"}; return m }},
		{"Lanes", "saving", func(m model.Model) model.Model { m.LanesForm.Saving = true; return m }},
		{"Worker", "saving", func(m model.Model) model.Model { m.WorkerCard.Saving = true; return m }},
		{"Worker", "stopping", func(m model.Model) model.Model { m.WorkerCard.Stopping = true; return m }},
		{"Worker", "switching supervision", func(m model.Model) model.Model { m.WorkerCard.Switching = true; return m }},
		{"Worker", "asking how to restart", func(m model.Model) model.Model { m.WorkerCard.Restart = true; return m }},
		{"Worker", "asking how to stop", func(m model.Model) model.Model { m.WorkerCard.AskOff = true; return m }},
		{"Filter form", "saving", func(m model.Model) model.Model { m.FilterForm.Saving = true; return m }},
	}
	for _, tt := range cases {
		t.Run(tt.card+" "+tt.busy, func(t *testing.T) {
			c := cardNamed(t, tt.card)
			focused, _ := model.Update(c.open(), model.FocusFieldMsg{Field: c.textField})
			busy := tt.make(focused)
			if _, _, ok := busy.TextCaret(); ok {
				t.Fatalf("a busy card holds the text caret")
			}
			var want model.Model
			c.graft(&want, busy)
			for _, key := range fieldEditingKeys {
				m, cmds := clickPress(busy, key)
				var got model.Model
				c.graft(&got, m)
				if !reflect.DeepEqual(got, want) || len(cmds) != 0 {
					t.Errorf("%s changed the busy card or sent commands: %+v", key, cmds)
				}
				if _, _, ok := m.TextCaret(); ok {
					t.Errorf("after %s a busy card holds the text caret", key)
				}
			}
		})
	}
}

// The caret sits in the focused field when the card is not busy and the field takes text.
func TestTextCaretFollowsTheFocusedField(t *testing.T) {
	for _, c := range popupCards() {
		if c.card == model.NoCard || c.card == model.CardBackfill {
			continue
		}
		m := c.open()
		if c.textField != "" {
			m, _ = model.Update(m, model.FocusFieldMsg{Field: c.textField})
		}
		want := c.textField
		if c.card == model.CardLanes {
			want = model.LanesNames
		}
		card, field, ok := m.TextCaret()
		if !ok || card != c.card || field != want {
			t.Errorf("%s: caret (%d, %q, %v), want (%d, %q, true)", c.name, card, field, ok, c.card, want)
		}
	}
	for _, tt := range []struct {
		card  string
		field model.Field
	}{
		{"Recurrence", model.RecurrenceFrequency},
		{"Filter form", model.FilterStatus},
		{"Worker", model.WorkerFieldTrust},
		{"Job", model.JobFieldKind},
	} {
		c := cardNamed(t, tt.card)
		m, _ := model.Update(c.open(), model.FocusFieldMsg{Field: tt.field})
		if _, field, ok := m.TextCaret(); ok || field != tt.field {
			t.Errorf("%s: caret in the choice field %q (%q, %v)", tt.card, tt.field, field, ok)
		}
	}
	create := cardNamed(t, "Create")
	m, _ := model.Update(create.open(), model.FocusFieldMsg{Field: model.CreateBody})
	if _, field, ok := m.TextCaret(); !ok || field != model.CreateBody {
		t.Errorf("Create body: caret (%q, %v)", field, ok)
	}
	backfill := cardNamed(t, "Backfill")
	if _, _, ok := backfill.open().TextCaret(); ok {
		t.Errorf("Backfill has no text input but holds the caret")
	}
}

// Tab skips the fields the card hides.
func TestTabSkipsHiddenFields(t *testing.T) {
	c := cardNamed(t, "Recurrence")
	m, _ := model.Update(c.open(), model.FocusFieldMsg{Field: model.RecurrenceWeekdays})
	// Weekly hides the day of month, the month and the end date (the card ends never).
	for _, want := range []model.Field{model.RecurrenceStart, model.RecurrenceEnds, model.RecurrenceZone, model.RecurrenceFrequency} {
		m, _ = clickPress(m, "tab")
		if got := m.RecurrenceForm.Field; got != want {
			t.Fatalf("tab reached %q, want %q", got, want)
		}
	}
	for _, want := range []model.Field{model.RecurrenceZone, model.RecurrenceEnds} {
		m, _ = clickPress(m, "shift+tab")
		if got := m.RecurrenceForm.Field; got != want {
			t.Fatalf("shift+tab reached %q, want %q", got, want)
		}
	}
}

// Right, left and space step a choice field; in a text field right and left move the caret and space is typed.
func TestChoiceKeysStepChoiceFields(t *testing.T) {
	t.Run("Recurrence", func(t *testing.T) {
		c := cardNamed(t, "Recurrence")
		m, _ := model.Update(c.open(), model.FocusFieldMsg{Field: model.RecurrenceFrequency})
		start := m.RecurrenceForm.Frequency
		next, _ := clickPress(m, "right")
		if next.RecurrenceForm.Frequency == start {
			t.Fatalf("right left the frequency at %q", start)
		}
		if back, _ := clickPress(next, "left"); back.RecurrenceForm.Frequency != start {
			t.Fatalf("left after right gave %q, want %q", back.RecurrenceForm.Frequency, start)
		}
		if spaced, _ := clickPress(m, "space"); spaced.RecurrenceForm.Frequency != next.RecurrenceForm.Frequency {
			t.Fatalf("space gave %q, right gave %q", spaced.RecurrenceForm.Frequency, next.RecurrenceForm.Frequency)
		}
		text, _ := model.Update(c.open(), model.FocusFieldMsg{Field: model.RecurrenceEvery})
		text = typeKeys(text, model.KeyMsg{Text: "ab"}, model.KeyMsg{Chord: "left"}, model.KeyMsg{Chord: "right"}, model.KeyMsg{Chord: "space"})
		if text.RecurrenceForm.Frequency != start || text.RecurrenceForm.Every != "1ab " {
			t.Fatalf("in a text field: frequency %q every %q", text.RecurrenceForm.Frequency, text.RecurrenceForm.Every)
		}
	})
	t.Run("Job", func(t *testing.T) {
		c := cardNamed(t, "Job")
		m, _ := model.Update(c.open(), model.FocusFieldMsg{Field: model.JobFieldEnabled})
		start := m.JobForm.Enabled
		for _, key := range []string{"right", "left", "space"} {
			if got, _ := clickPress(m, key); got.JobForm.Enabled == start {
				t.Errorf("%s left Enabled at %v", key, start)
			}
		}
	})
	t.Run("Worker", func(t *testing.T) {
		c := cardNamed(t, "Worker")
		m, _ := model.Update(c.open(), model.FocusFieldMsg{Field: model.WorkerFieldTrust})
		start := m.WorkerCard.Trusted
		for _, key := range []string{"right", "left", "space"} {
			if got, _ := clickPress(m, key); got.WorkerCard.Trusted == start {
				t.Errorf("%s left Trusted at %v", key, start)
			}
		}
	})
	t.Run("Filter form", func(t *testing.T) {
		c := cardNamed(t, "Filter form")
		m, _ := model.Update(c.open(), model.FocusFieldMsg{Field: model.FilterStatus})
		start := m.FilterForm.Values.Status
		next, _ := clickPress(m, "right")
		if next.FilterForm.Values.Status == start {
			t.Fatalf("right left the status at %q", start)
		}
		if back, _ := clickPress(next, "left"); back.FilterForm.Values.Status != start {
			t.Fatalf("left after right gave %q, want %q", back.FilterForm.Values.Status, start)
		}
		if spaced, _ := clickPress(m, "space"); spaced.FilterForm.Values.Status != next.FilterForm.Values.Status {
			t.Fatalf("space gave %q, right gave %q", spaced.FilterForm.Values.Status, next.FilterForm.Values.Status)
		}
		if cleared, _ := clickPress(next, "backspace"); cleared.FilterForm.Values.Status != "" {
			t.Fatalf("backspace on a choice left %q", cleared.FilterForm.Values.Status)
		}
		text, _ := model.Update(c.open(), model.FocusFieldMsg{Field: model.FilterOwner})
		text = typeKeys(text, model.KeyMsg{Text: "ab"}, model.KeyMsg{Chord: "left"}, model.KeyMsg{Text: "x"})
		if text.FilterForm.Values.Status != start || text.FilterForm.Values.Owner != "axb" {
			t.Fatalf("in a text field: status %q owner %q", text.FilterForm.Values.Status, text.FilterForm.Values.Owner)
		}
	})
}

// The Filter form keeps enter as next field, and the other forms do not take it so.
func TestEnterIsNextFieldOnlyInTheFilterForm(t *testing.T) {
	c := cardNamed(t, "Filter form")
	m, _ := model.Update(c.open(), model.FocusFieldMsg{Field: model.FilterOwner})
	if m, _ = clickPress(m, "enter"); m.FilterForm.Field != model.FilterLabel || !m.FilterForm.Open {
		t.Fatalf("enter reached %q, open %v", m.FilterForm.Field, m.FilterForm.Open)
	}
}

// Every keymap row of a field-editing action in a card's mode comes with the card's wording and the footer entries stay.
func TestFieldEditingRowsKeepTheCardsWording(t *testing.T) {
	for _, tt := range []struct {
		mode          model.Mode
		delete, clear string
		footer        []string
	}{
		{model.ModeCloseForm, "erase", "clear field", []string{"tab"}},
		{model.ModeDatesForm, "delete", "clear field", []string{"tab"}},
		{model.ModeRecurrenceForm, "erase", "clear field", []string{"tab", "right"}},
		{model.ModeJobForm, "erase", "clear field", []string{"tab", "right"}},
		{model.ModeLanesForm, "delete", "clear field", []string{"tab"}},
		{model.ModeWorkerCard, "erase", "clear field", []string{"tab", "right"}},
		{model.ModeFilterForm, "delete", "clear field", []string{"tab", "right"}},
		{model.ModeFieldForm, "erase", "clear input", nil},
		{model.ModeProjectForm, "delete", "clear name", nil},
		{model.ModePicker, "delete", "clear filter", nil},
	} {
		var delete, clear string
		var footer []string
		for _, b := range model.Keymap {
			if b.Mode != tt.mode {
				continue
			}
			switch b.Action {
			case model.DeleteInput:
				delete = b.Help
			case model.ClearInput:
				clear = b.Help
			}
			if b.ShowInFooter && (b.Action == model.NextField || b.Action == model.NextChoice) {
				footer = append(footer, b.Key)
			}
		}
		if delete != tt.delete || clear != tt.clear || !slices.Equal(footer, tt.footer) {
			t.Errorf("%s: delete %q clear %q footer %v, want %q %q %v", tt.mode, delete, clear, footer, tt.delete, tt.clear, tt.footer)
		}
	}
}
