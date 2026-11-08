package model

import "time"

const (
	ModeCalendar   Mode   = "calendar"
	ShowCalendar   Action = "show-calendar"
	PickDate       Action = "pick-date"
	HideCalendar   Action = "hide-calendar"
	DateDayBefore  Action = "date-day-before"
	DateDayAfter   Action = "date-day-after"
	DateWeekBefore Action = "date-week-before"
	DateWeekAfter  Action = "date-week-after"
)

type CalendarDayMsg struct{ Day string }

func (CalendarDayMsg) msg() {}

type CalendarState struct {
	Open   bool
	Card   Card
	Field  Field
	Cursor string
}

type CalendarView struct {
	Card                        Card
	Field                       Field
	Cursor, Value, Other, Today string
}

func (m Model) ShownCalendar() (CalendarView, bool) {
	entry, ok := m.topEntry()
	if !ok || !m.DateCalendar.Open || m.DateCalendar.Card != entry.card {
		return CalendarView{}, false
	}
	text, other, ok := dateFieldOf(&m, entry.card)
	if !ok {
		return CalendarView{}, false
	}
	today := m.paneToday()
	return CalendarView{Card: entry.card, Field: m.DateCalendar.Field, Cursor: m.DateCalendar.Cursor, Value: dayOf(*text, today), Other: dayOf(other, today), Today: today}, true
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

// A date kata holds with a time reads as its day.
func dayOf(text, today string) string {
	if day, err := ParseDateInput(text, today); err == nil {
		return day[:min(len(day), len(dayLayout))]
	}
	if len(text) >= len(dayLayout) {
		if _, err := time.Parse(dayLayout, text[:len(dayLayout)]); err == nil {
			return text[:len(dayLayout)]
		}
	}
	return ""
}

// The text points into m.
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

func showCalendar(m Model, entry cardEntry) Model {
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
	m.DateCalendar = CalendarState{Open: true, Card: entry.card, Field: *entry.focus(&m), Cursor: cursor}
	return m
}

func pickDay(m Model, day string) Model {
	picked := m.DateCalendar
	m.DateCalendar = CalendarState{}
	if _, err := time.Parse(dayLayout, day); err != nil {
		return m
	}
	if entry, ok := m.topEntry(); ok && entry.card == picked.Card {
		if text, _, ok := dateFieldOf(&m, picked.Card); ok && *entry.focus(&m) == picked.Field {
			*text = day
			_, caret := entry.text(&m)
			*caret = Caret{}
			m = entry.erased(m, ClearInput, true)
		}
	}
	return m
}

// updateCalendarKey is one key while the calendar is open, read through the ModeCalendar bindings: the arrows move its cursor,
// enter picks and esc closes. Any other key closes the calendar and reaches the card as it would with none; ctrl+d, which opens
// the calendar, only closes it. A click on a card's button is not this key: it closes the calendar first (ButtonPressMsg). It
// reports whether the card still has the key to handle.
func updateCalendarKey(m Model, entry cardEntry, msg KeyMsg) (Model, bool) {
	binding, bound := LookupBindingForMode(msg.Chord, ModeCalendar)
	if !bound {
		m.DateCalendar = CalendarState{}
		if opens, ok := LookupBindingForMode(msg.Chord, entry.mode(m)); ok && opens.Action == ShowCalendar {
			return m, false
		}
		return m, true
	}
	switch binding.Action {
	case PickDate:
		return pickDay(m, m.DateCalendar.Cursor), false
	case HideCalendar:
		m.DateCalendar = CalendarState{}
	case DateDayBefore:
		m.DateCalendar.Cursor = shiftDay(m.DateCalendar.Cursor, -1)
	case DateDayAfter:
		m.DateCalendar.Cursor = shiftDay(m.DateCalendar.Cursor, 1)
	case DateWeekBefore:
		m.DateCalendar.Cursor = shiftDay(m.DateCalendar.Cursor, -7)
	case DateWeekAfter:
		m.DateCalendar.Cursor = shiftDay(m.DateCalendar.Cursor, 7)
	}
	return m, false
}

func shiftDay(day string, by int) string {
	t, err := time.Parse(dayLayout, day)
	if err != nil {
		return day
	}
	return t.AddDate(0, 0, by).Format(dayLayout)
}

func calendarOpen(m Model, entry cardEntry) bool {
	return m.DateCalendar.Open && m.DateCalendar.Card == entry.card
}

func (m Model) settleCalendar() Model {
	if !m.DateCalendar.Open {
		return m
	}
	if entry, ok := m.topEntry(); ok && entry.card == m.DateCalendar.Card && entry.focus != nil && !entry.busy(m) && *entry.focus(&m) == m.DateCalendar.Field {
		if _, _, ok := dateFieldOf(&m, entry.card); ok {
			return m
		}
	}
	m.DateCalendar = CalendarState{}
	return m
}
