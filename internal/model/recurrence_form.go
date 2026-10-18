package model

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

const ModeRecurrenceForm Mode = "recurrence-form"

const (
	OpenRecurrenceAction     Action = "open-recurrence"
	RecurrenceNext           Action = "recurrence-next-field"
	RecurrencePrevious       Action = "recurrence-previous-field"
	RecurrenceNextChoice     Action = "recurrence-next-choice"
	RecurrencePreviousChoice Action = "recurrence-previous-choice"
	RecurrenceSave           Action = "recurrence-save"
	RecurrenceCancel         Action = "recurrence-cancel"
)

// The Recurrence card's fields.
const (
	RecurrenceFrequency Field = "frequency"
	RecurrenceEvery     Field = "every"
	RecurrenceWeekdays  Field = "weekdays"
	RecurrenceDay       Field = "day"
	RecurrenceMonth     Field = "month"
	RecurrenceEnds      Field = "ends"
	RecurrenceEnd       Field = "end"
	RecurrenceStart     Field = "start"
	RecurrenceZone      Field = "zone"
)

// RecurrenceFields is the Recurrence card's field list in tab order. Which of them apply depends on the frequency and the end.
var RecurrenceFields = []FieldSpec{
	{RecurrenceFrequency, false}, {RecurrenceEvery, true}, {RecurrenceWeekdays, false}, {RecurrenceDay, true},
	{RecurrenceMonth, false}, {RecurrenceEnds, false}, {RecurrenceEnd, true}, {RecurrenceStart, true}, {RecurrenceZone, true},
}

// RecurrenceFrequencies and RecurrenceEnds are the choices of the Frequency and Ends fields, in cycling order.
var (
	RecurrenceFrequencies = []Frequency{Daily, Weekly, Monthly, Yearly}
	RecurrenceEndKinds    = []EndKind{EndNever, EndUntil, EndCount}
)

// RecurrenceFormState is the Recurrence card. The text fields hold what was typed and the card reads it into a Schedule only
// when saving, so every validation message comes from one place.
type RecurrenceFormState struct {
	Open, Saving bool
	Field        Field
	UID          string
	ProjectID    int64
	Frequency    Frequency
	Every        string
	Weekdays     [7]bool // indexed by time.Weekday
	WeekdayAt    int     // the weekday under the cursor, 0 for Monday: the field lists Monday first
	Day          string  // "" for the start date's day, "15", "last", "2nd tue"
	Month        int     // 1 to 12
	Ends         EndKind
	Until, Count string
	Start, Zone  string
	Err          error
}

// Shows says the field applies to the card as it stands; the others are hidden and tab skips them.
func (f RecurrenceFormState) Shows(name Field) bool {
	switch name {
	case RecurrenceWeekdays:
		return f.Frequency == Weekly
	case RecurrenceDay:
		return f.Frequency == Monthly || f.Frequency == Yearly
	case RecurrenceMonth:
		return f.Frequency == Yearly
	case RecurrenceEnd:
		return f.Ends != EndNever
	}
	return true
}

// EndText is what the End date or count field holds: the date for an end on a date, the count for an end after N occurrences.
func (f RecurrenceFormState) EndText() string {
	if f.Ends == EndCount {
		return f.Count
	}
	return f.Until
}

// text is the single-line field called name, nil for a choice field.
func (f *RecurrenceFormState) text(name Field) *string {
	switch name {
	case RecurrenceEvery:
		return &f.Every
	case RecurrenceDay:
		return &f.Day
	case RecurrenceEnd:
		if f.Ends == EndCount {
			return &f.Count
		}
		return &f.Until
	case RecurrenceStart:
		return &f.Start
	case RecurrenceZone:
		return &f.Zone
	}
	return nil
}

// newRecurrenceForm is the card for an Issue that does not recur: weekly on the start date's weekday, every 1, never ending.
// The start date is the Issue's scheduled date, else today; the zone is the Pane's.
func newRecurrenceForm(m Model, issue *Issue) RecurrenceFormState {
	f := RecurrenceFormState{Open: true, Field: RecurrenceFields[0].Name, UID: issue.UID, ProjectID: issue.ProjectID, Frequency: Weekly, Every: "1", Month: 1,
		Start: m.LocalDate, Zone: m.TimeZone}
	if issue.ScheduledOnDate != nil && *issue.ScheduledOnDate != "" {
		f.Start = *issue.ScheduledOnDate
	}
	if f.Zone == "" {
		f.Zone = "UTC"
	}
	if start, err := time.Parse(time.DateOnly, f.Start); err == nil {
		f.Weekdays[start.Weekday()] = true
		f.WeekdayAt = (int(start.Weekday()) + 6) % 7
		f.Month = int(start.Month())
	}
	return f
}

// openRecurrence is `O`. An Issue that already recurs, or whose recurrence was stopped, does not open a card to make a new one:
// kata refuses to put an Issue into a second recurrence.
func openRecurrence(m Model) Model {
	issue := closeIssue(m)
	if m.Stale || issue == nil || issue.RecurrenceID != nil {
		return m
	}
	m.RecurrenceForm = newRecurrenceForm(m, issue)
	return m
}

var (
	errEveryTooSmall  = errors.New("Every must be a whole number, 1 or more.")
	errNoWeekday      = errors.New("Pick at least one weekday.")
	errDayRange       = errors.New("Day must be between 1 and 31.")
	errDayForm        = errors.New("Day must be a number from 1 to 31, last, or a weekday like 2nd tue or last fri.")
	errCountTooSmall  = errors.New("Count must be a whole number, 1 or more.")
	errStartDate      = errors.New("Start date must be a date like 2026-10-19.")
	errEndDate        = errors.New("End date must be a date like 2026-10-19.")
	errEndBeforeStart = errors.New("End date must not be before the start date.")
)

// schedule reads the card into the Schedule it would send, or says what is wrong with it. Nothing leaves the card until it does.
func (f RecurrenceFormState) schedule() (Schedule, error) {
	s := Schedule{Frequency: f.Frequency, End: f.Ends}
	var err error
	if s.Interval, err = strconv.Atoi(strings.TrimSpace(f.Every)); err != nil || s.Interval < 1 {
		return Schedule{}, errEveryTooSmall
	}
	switch f.Frequency {
	case Weekly:
		for d, on := range f.Weekdays {
			if on {
				s.Weekdays = append(s.Weekdays, time.Weekday(d))
			}
		}
		if len(s.Weekdays) == 0 {
			return Schedule{}, errNoWeekday
		}
	case Monthly, Yearly:
		if err := parseDay(strings.TrimSpace(f.Day), &s); err != nil {
			return Schedule{}, err
		}
		if f.Frequency == Yearly {
			s.Month = f.Month
		}
	}
	start, err := time.Parse(time.DateOnly, strings.TrimSpace(f.Start))
	if err != nil {
		return Schedule{}, errStartDate
	}
	switch f.Ends {
	case EndCount:
		if s.Count, err = strconv.Atoi(strings.TrimSpace(f.Count)); err != nil || s.Count < 1 {
			return Schedule{}, errCountTooSmall
		}
	case EndUntil:
		until, err := time.Parse(time.DateOnly, strings.TrimSpace(f.Until))
		if err != nil {
			return Schedule{}, errEndDate
		}
		if until.Before(start) {
			return Schedule{}, errEndBeforeStart
		}
		s.Until = until.Format(time.DateOnly)
	}
	if _, err := time.LoadLocation(strings.TrimSpace(f.Zone)); err != nil || strings.TrimSpace(f.Zone) == "" {
		return Schedule{}, fmt.Errorf("Unknown time zone %q.", strings.TrimSpace(f.Zone))
	}
	return s, nil
}

// parseDay reads the Day field into s: empty is the start date's day, a number is that day of the month, "last" the last day,
// and "<n> <weekday>" the nth weekday, n being 1 to 5 or "last".
func parseDay(text string, s *Schedule) error {
	words := strings.Fields(strings.ToLower(text))
	switch {
	case len(words) == 0:
		s.Day = DayFromStart
	case len(words) == 1 && words[0] == "last":
		s.Day = LastDay
	case len(words) == 1:
		n, err := strconv.Atoi(words[0])
		if err != nil {
			return errDayForm
		}
		if n < 1 || n > 31 {
			return errDayRange
		}
		s.Day, s.DayOfMonth = DayOfMonth, n
	case len(words) == 2:
		nth, ok := nthOf(words[0])
		day, found := weekdayNamed(words[1])
		if !ok || !found {
			return errDayForm
		}
		s.Day, s.Nth, s.Weekday = NthWeekday, nth, day
	default:
		return errDayForm
	}
	return nil
}

// nthOf reads "1" to "5", "1st" to "5th" or "last" as the ordinal an NthWeekday takes.
func nthOf(word string) (int, bool) {
	if word == "last" {
		return -1, true
	}
	n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(word, "st"), "nd"), "rd"), "th"))
	return n, err == nil && n >= 1 && n <= 5
}

// weekdayNamed reads a weekday by its first three or more letters.
func weekdayNamed(word string) (time.Weekday, bool) {
	for d := time.Sunday; d <= time.Saturday; d++ {
		if len(word) >= 3 && strings.HasPrefix(strings.ToLower(d.String()), word) {
			return d, true
		}
	}
	return 0, false
}

func submitRecurrence(m Model) (Model, []Cmd) {
	f := &m.RecurrenceForm
	if !f.Open || f.Saving {
		return m, nil
	}
	if m.Stale {
		f.Err = errors.New("Daemon disconnected; reconnect before making this Issue recur.")
		return m, nil
	}
	s, err := f.schedule()
	if err != nil {
		f.Err = err
		return m, nil
	}
	f.Saving, f.Err = true, nil
	return m, []Cmd{CreateRecurrenceCmd{Workspace: m.Workspace, UID: f.UID, ProjectID: f.ProjectID, Actor: m.Actor, Rule: s.RRULE(), Start: strings.TrimSpace(f.Start), TimeZone: strings.TrimSpace(f.Zone)}}
}

func updateRecurrence(m Model, msg Msg) (Model, []Cmd) {
	result, ok := msg.(RecurrenceResultMsg)
	if !ok || m.RecurrenceForm.UID != result.UID {
		return m, nil
	}
	m.RecurrenceForm.Saving, m.RecurrenceForm.Err = false, result.Err
	if result.Err == nil {
		m.RecurrenceForm.Open = false
		m = m.issueWritten(result.UID)
		return snapshotChanged(m)
	}
	return m, nil
}

// step moves a choice field one choice on, or the weekday cursor; back goes the other way.
func (f *RecurrenceFormState) step(back bool) {
	d := 1
	if back {
		d = -1
	}
	cycle := func(i, n int) int { return ((i+d)%n + n) % n }
	switch f.Field {
	case RecurrenceFrequency:
		f.Frequency = RecurrenceFrequencies[cycle(slices.Index(RecurrenceFrequencies, f.Frequency), len(RecurrenceFrequencies))]
	case RecurrenceEnds:
		f.Ends = RecurrenceEndKinds[cycle(slices.Index(RecurrenceEndKinds, f.Ends), len(RecurrenceEndKinds))]
	case RecurrenceMonth:
		f.Month = cycle(f.Month-1, 12) + 1
	case RecurrenceWeekdays:
		f.WeekdayAt = cycle(f.WeekdayAt, 7)
	}
}

// recurrenceKey is one key for the Recurrence card on top. Left, right and space move through a choice field's choices; in the
// Weekdays field left and right move the cursor and space toggles the day under it, and in a text field space is typed.
func recurrenceKey(m Model, msg KeyMsg) (Model, []Cmd) {
	f := &m.RecurrenceForm
	if f.Saving {
		return m, nil
	}
	binding, bound := LookupBindingForMode(msg.Chord, ModeRecurrenceForm)
	field := f.text(f.Field)
	switch {
	case bound && binding.Action == RecurrenceCancel:
		f.Open = false
	case bound && binding.Action == RecurrenceNext:
		cycleField(&m, 1)
	case bound && binding.Action == RecurrencePrevious:
		cycleField(&m, -1)
	case bound && binding.Action == RecurrenceSave:
		return submitRecurrence(m)
	case bound && binding.Action == InputSpace && field == nil && f.Field == RecurrenceWeekdays:
		day := time.Weekday((f.WeekdayAt + 1) % 7)
		f.Weekdays[day] = !f.Weekdays[day]
		f.Err = nil
	case bound && binding.Action == InputSpace && field == nil, bound && binding.Action == RecurrenceNextChoice && field == nil:
		f.step(false)
		f.Err = nil
	case bound && binding.Action == RecurrencePreviousChoice && field == nil:
		f.step(true)
		f.Err = nil
	case field != nil:
		var edited bool
		if bound && binding.Action == InputSpace {
			bound = false // a space in a text field is typed
		}
		if *field, edited = editText(*field, binding, bound, msg); edited {
			f.Err = nil
		}
	}
	return m, nil
}
