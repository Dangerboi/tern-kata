package model

import "time"

const (
	ModeDatePicker Mode   = "date-picker"
	OpenDatePicker Action = "open-date-picker"
	PickDate       Action = "pick-date"
	CloseDatePick  Action = "close-date-picker"
	DateDayBefore  Action = "date-day-before"
	DateDayAfter   Action = "date-day-after"
	DateWeekBefore Action = "date-week-before"
	DateWeekAfter  Action = "date-week-after"
)

// DatePickMsg is a click on a day of the open date picker: the day goes into the field and the picker closes.
type DatePickMsg struct{ Day string }

func (DatePickMsg) msg() {}

// DatePickerState is the calendar a card shows beside one of its date fields. Card and Field say which field it picks for;
// Cursor is the day the keys are on.
type DatePickerState struct {
	Open   bool
	Card   Card
	Field  Field
	Cursor string
}

// DatePicker is what the view draws of the open date picker: the field it sits beside, the day the keys are on, the day the
// field holds now ("" when it holds none that reads), the card's other date ("" for none), and today.
type DatePicker struct {
	Card                        Card
	Field                       Field
	Cursor, Value, Other, Today string
}

// DatePicker reports the open date picker of the card on top.
func (m Model) DatePicker() (DatePicker, bool) {
	entry, ok := m.topEntry()
	if !ok || !m.DatePick.Open || m.DatePick.Card != entry.card {
		return DatePicker{}, false
	}
	text, other, ok := dateFieldOf(&m, entry.card)
	if !ok {
		return DatePicker{}, false
	}
	today := m.paneToday()
	return DatePicker{Card: entry.card, Field: m.DatePick.Field, Cursor: m.DatePick.Cursor, Value: dayOf(*text, today), Other: dayOf(other, today), Today: today}, true
}

// paneToday is the Pane calendar's date, or the day of the instant the Pane last stepped at before the first dated snapshot.
func (m Model) paneToday() string {
	if m.Calendar.Date != "" {
		return m.Calendar.Date
	}
	if m.Now.IsZero() {
		return ""
	}
	return m.Calendar.DayOf(m.Now)
}

// dayOf is the day a date field's text reads as, "" for blank or text that is not a date. A date kata holds with a time reads
// as its day.
func dayOf(text, today string) string {
	if day, err := ParseDateInput(text, today); err == nil {
		return day
	}
	if len(text) >= len(dayLayout) {
		if _, err := time.Parse(dayLayout, text[:len(dayLayout)]); err == nil {
			return text[:len(dayLayout)]
		}
	}
	return ""
}

// dateFieldOf is the text of the date field the card on top holds focus on and the text of the card's other date, ok false when
// the card is not on a date field. The text points into m.
func dateFieldOf(m *Model, card Card) (text *string, other string, ok bool) {
	switch card {
	case CardDates:
		f := &m.DatesForm
		if text = f.Text.at(f.Field); text == nil {
			return nil, "", false
		}
		if f.Field == DatesScheduled {
			return text, f.Text.Deadline, true
		}
		return text, f.Text.Scheduled, true
	case CardCreate:
		f := &m.Create
		switch f.Field {
		case CreateScheduled:
			return &f.Dates.Scheduled, f.Dates.Deadline, true
		case CreateDeadline:
			return &f.Dates.Deadline, f.Dates.Scheduled, true
		}
	case CardRecurrence:
		f := &m.RecurrenceForm
		switch {
		case f.Field == RecurrenceStart && f.Ends == EndUntil:
			return &f.Start, f.Until, true
		case f.Field == RecurrenceStart:
			return &f.Start, "", true
		case f.Field == RecurrenceEnd && f.Ends == EndUntil:
			return &f.Until, f.Start, true
		}
	}
	return nil, "", false
}

// openDatePicker puts the calendar beside the focused date field, its cursor on the day the field reads as, else today.
func openDatePicker(m Model, entry cardEntry) Model {
	if entry.busy(m) {
		return m
	}
	text, _, ok := dateFieldOf(&m, entry.card)
	if !ok {
		return m
	}
	today := m.paneToday()
	cursor := dayOf(*text, today)
	if cursor == "" {
		cursor = today
	}
	if cursor == "" {
		return m
	}
	m.DatePick = DatePickerState{Open: true, Card: entry.card, Field: *entry.focus(&m), Cursor: cursor}
	return m
}

// pickDay writes day into the field the open picker is for and closes it.
func pickDay(m Model, day string) Model {
	picked := m.DatePick
	m.DatePick = DatePickerState{}
	if _, err := time.Parse(dayLayout, day); err != nil {
		return m
	}
	if entry, ok := m.topEntry(); ok && entry.card == picked.Card {
		if text, _, ok := dateFieldOf(&m, picked.Card); ok && *entry.focus(&m) == picked.Field {
			*text = day
			_, caret := entry.text(&m)
			*caret = Caret{}
			m = entry.erased(m, true)
		}
	}
	return m
}

// updateDatePickerKey is one key while the picker is open: the arrows move its cursor, enter picks and esc closes. Any other key
// is swallowed.
func updateDatePickerKey(m Model, msg KeyMsg) Model {
	binding, bound := LookupBindingForMode(msg.Chord, ModeDatePicker)
	if !bound {
		return m
	}
	switch binding.Action {
	case PickDate:
		return pickDay(m, m.DatePick.Cursor)
	case CloseDatePick:
		m.DatePick = DatePickerState{}
	case DateDayBefore:
		m.DatePick.Cursor = shiftDay(m.DatePick.Cursor, -1)
	case DateDayAfter:
		m.DatePick.Cursor = shiftDay(m.DatePick.Cursor, 1)
	case DateWeekBefore:
		m.DatePick.Cursor = shiftDay(m.DatePick.Cursor, -7)
	case DateWeekAfter:
		m.DatePick.Cursor = shiftDay(m.DatePick.Cursor, 7)
	}
	return m
}

func shiftDay(day string, by int) string {
	t, err := time.Parse(dayLayout, day)
	if err != nil {
		return day
	}
	return t.AddDate(0, 0, by).Format(dayLayout)
}

// datePickerOpen says the picker is open for the card on top.
func datePickerOpen(m Model, entry cardEntry) bool {
	return m.DatePick.Open && m.DatePick.Card == entry.card
}
