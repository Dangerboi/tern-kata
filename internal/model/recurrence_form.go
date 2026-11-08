package model

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	ModeRecurrenceForm Mode = "recurrence-form"
	ModeRecurrenceStop Mode = "recurrence-stop"
)

const (
	OpenRecurrenceAction  Action = "open-recurrence"
	RecurrenceSave        Action = "recurrence-save"
	RecurrenceCancel      Action = "recurrence-cancel"
	RecurrenceStop        Action = "recurrence-stop"
	RecurrenceStopConfirm Action = "recurrence-stop-confirm"
	RecurrenceStopKeep    Action = "recurrence-stop-keep"
)

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

var RecurrenceFields = recurrenceTable.specs()

var RecurrenceEndKinds = []EndKind{EndNever, EndUntil, EndCount}

// RecurrenceFormState is the Recurrence card. The text fields hold what was typed and the card reads it into a Schedule only
// when saving, so every validation message comes from one place.
type RecurrenceFormState struct {
	formCard
	UID string
	// Change says the Issue already recurs: the card changes Opened, the recurrence it read, instead of making a new one.
	// Raw is the rule text when the form cannot express it. StopConfirm asks y/n before stopping; Stopping says the pending
	// write is the stop. Refused says nothing can be changed: the card only reports Err, and esc closes it.
	Change, StopConfirm, Stopping, Refused bool
	Opened                                 Recurrence
	Raw                                    string
	ProjectID                              int64
	Frequency                              Frequency
	Every                                  string
	Weekdays                               [7]bool // indexed by time.Weekday
	WeekdayAt                              int     // the weekday under the cursor, an index into MondayFirst: the field lists Monday first
	Day                                    string  // "15", "last" (monthly only), "2nd tue"
	Month                                  int
	Ends                                   EndKind
	Until, Count                           string
	Start, Zone                            string
}

// localZoneNote says a stored Local zone, which the card cannot save, is replaced by zone on saving; "" for any other zone.
func localZoneNote(stored, zone string) string {
	if stored != localZone || zone == "" {
		return ""
	}
	return "The stored time zone is Local (the daemon's own zone); saving sets " + zone + "."
}

func (f RecurrenceFormState) ZoneNote() string {
	if !f.Change {
		return ""
	}
	return localZoneNote(f.Opened.TimeZone, strings.TrimSpace(f.Zone))
}

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

func (f RecurrenceFormState) EndText() string {
	if f.Ends == EndCount {
		return f.Count
	}
	return f.Until
}

// newRecurrenceForm is the card for an Issue that does not recur: weekly on the start date's weekday, every 1, never ending.
// The start date is the Issue's scheduled date, else today; the zone is the Pane's.
func newRecurrenceForm(cal PaneCalendar, issue *Issue) RecurrenceFormState {
	f := RecurrenceFormState{Open: true, Field: RecurrenceFields[0].Name, UID: issue.UID, ProjectID: issue.ProjectID, Frequency: Weekly, Every: "1", Month: 1,
		Start: cal.Date, Zone: realZone(cal.Zone(), "UTC")}
	if issue.ScheduledOnDate != nil && *issue.ScheduledOnDate != "" {
		f.Start = *issue.ScheduledOnDate
	}
	if start, err := time.Parse(time.DateOnly, f.Start); err == nil {
		f.Weekdays[start.Weekday()] = true
		f.WeekdayAt = MondayIndex(start.Weekday())
		f.Month = int(start.Month())
	}
	f.Day = f.startDay()
	return f
}

func (f RecurrenceFormState) startDay() string {
	if start, err := time.Parse(time.DateOnly, strings.TrimSpace(f.Start)); err == nil {
		return strconv.Itoa(start.Day())
	}
	return ""
}

// changeRecurrenceForm is the card for an Issue whose recurrence is rec, prefilled from its Schedule. A rule the form cannot
// express shows as text above the form, which then holds the defaults; saving the form replaces the rule.
func changeRecurrenceForm(cal PaneCalendar, issue *Issue, rec Recurrence) RecurrenceFormState {
	started := *issue
	started.ScheduledOnDate = &rec.Start
	f := newRecurrenceForm(cal, &started)
	f.Change, f.Opened = true, rec
	f.Zone = realZone(rec.TimeZone, f.Zone)
	s := rec.Schedule()
	if s.IsRaw() {
		f.Raw = s.Raw
		return f
	}
	f.Frequency, f.Every, f.Ends, f.Until, f.Count = s.Frequency, strconv.Itoa(s.Interval), s.End, s.Until, strconv.Itoa(s.Count)
	if s.End != EndCount {
		f.Count = ""
	}
	if len(s.Weekdays) > 0 {
		f.Weekdays = [7]bool{}
		for _, d := range s.Weekdays {
			f.Weekdays[d] = true
		}
		f.WeekdayAt = MondayIndex(s.Weekdays[0])
	}
	if s.Month > 0 {
		f.Month = s.Month
	}
	switch s.Day {
	case DayOfMonth:
		f.Day = strconv.Itoa(s.DayOfMonth)
	case LastDay:
		f.Day = "last"
	case NthWeekday:
		f.Day = "last"
		if s.Nth > 0 {
			f.Day = strconv.Itoa(s.Nth)
		}
		f.Day += " " + strings.ToLower(s.Weekday.String()[:3])
	}
	return f
}

// openRecurrence is `O`. An Issue that does not recur gets a card to make it recur; one that does gets the card to change or
// stop it, once the detail has its recurrence: with the detail hidden or not yet loaded, the detail is asked for and the card
// opens when it arrives.
func openRecurrence(m Model) Model {
	issue := closeIssue(m)
	if m.Stale || issue == nil {
		return m
	}
	if issue.RecurrenceID == nil {
		m.RecurrenceForm = newRecurrenceForm(m.Calendar, issue)
		return m
	}
	return m.wantRecurrence(issue.UID)
}

var (
	errRecurrenceStoppedHere = errors.New("The recurrence was stopped; kata can't make this Issue recur again.")
	ErrRecurrenceWithheld    = errors.New("kata does not show this recurrence to your access, so it can't be changed from here.")
)

// openWantedRecurrence opens the card for the loaded recurrence of the Issue the detail shows. A stopped one, or one kata
// withholds, gets a card that only says so: kata refuses to put an Issue into a second recurrence.
func openWantedRecurrence(m Model) Model {
	issue, rec := closeIssue(m), m.authority.Recurrence
	switch {
	case issue == nil:
	case rec == nil:
		m.RecurrenceForm = RecurrenceFormState{Open: true, Refused: true, UID: issue.UID, Err: ErrRecurrenceWithheld}
	case rec.Stopped:
		m.RecurrenceForm = RecurrenceFormState{Open: true, Refused: true, UID: issue.UID, Err: errRecurrenceStoppedHere}
	default:
		m.RecurrenceForm = changeRecurrenceForm(m.Calendar, issue, *rec)
	}
	return m
}

var (
	errEveryTooSmall  = errors.New("Every must be a whole number, 1 or more.")
	errNoWeekday      = errors.New("Pick at least one weekday.")
	errDayRange       = errors.New("Day must be between 1 and 31.")
	errDayEmpty       = errors.New("Day must not be empty: enter 1 to 31, last, or a weekday like 2nd tue or last fri.")
	errDayForm        = errors.New("Day must be a number from 1 to 31, last, or a weekday like 2nd tue or last fri.")
	errYearlyLast     = errors.New("The last day works for monthly schedules only.")
	errCountTooSmall  = errors.New("Occurrences must be a whole number, 1 or more.")
	errStartDate      = errors.New("Start date must be a date like 2026-10-19.")
	errEndDate        = errors.New("End date must be a date like 2026-10-19.")
	errEndBeforeStart = errors.New("End date must not be before the start date.")

	errRecurrenceChanged = errors.New("The schedule changed meanwhile; nothing was written. The card shows the current schedule.")
)

// localZone is the zone name kata stores for "the daemon's own zone"; the card cannot save it.
const localZone = "Local"

// realZone is zone, or fallback when zone is Local: kata reads Local in the daemon's zone, not the Pane's.
func realZone(zone, fallback string) string {
	if zone == localZone {
		return fallback
	}
	return zone
}

func checkZone(zone string) error {
	if _, err := time.LoadLocation(zone); err != nil || zone == "" || zone == localZone {
		return fmt.Errorf("Unknown time zone %q.", zone)
	}
	return nil
}

func parseDate(text string, bad error) (time.Time, error) {
	day, err := time.Parse(time.DateOnly, strings.TrimSpace(text))
	if err != nil {
		return time.Time{}, bad
	}
	return day, nil
}

func (f RecurrenceFormState) terms() (RecurrenceTerms, error) {
	start, zone := strings.TrimSpace(f.Start), strings.TrimSpace(f.Zone)
	s := Schedule{Frequency: f.Frequency, End: f.Ends}
	var err error
	if s.Interval, err = strconv.Atoi(strings.TrimSpace(f.Every)); err != nil || s.Interval < 1 {
		return RecurrenceTerms{}, errEveryTooSmall
	}
	switch f.Frequency {
	case Weekly:
		for _, d := range MondayFirst {
			if f.Weekdays[d] {
				s.Weekdays = append(s.Weekdays, d)
			}
		}
		if len(s.Weekdays) == 0 {
			return RecurrenceTerms{}, errNoWeekday
		}
	case Monthly, Yearly:
		if err := parseDay(strings.TrimSpace(f.Day), &s); err != nil {
			return RecurrenceTerms{}, err
		}
		if f.Frequency == Yearly {
			s.Month = f.Month
		}
	}
	startDate, err := parseDate(start, errStartDate)
	if err != nil {
		return RecurrenceTerms{}, err
	}
	switch f.Ends {
	case EndCount:
		if s.Count, err = strconv.Atoi(strings.TrimSpace(f.Count)); err != nil || s.Count < 1 {
			return RecurrenceTerms{}, errCountTooSmall
		}
	case EndUntil:
		until, err := parseDate(f.Until, errEndDate)
		if err != nil {
			return RecurrenceTerms{}, err
		}
		if until.Before(startDate) {
			return RecurrenceTerms{}, errEndBeforeStart
		}
		s.Until = until.Format(time.DateOnly)
	}
	if err := checkZone(zone); err != nil {
		return RecurrenceTerms{}, err
	}
	return RecurrenceTerms{Rule: s.RRULE(), Start: start, TimeZone: zone}, nil
}

// parseDay reads the Day field into s: a number is that day of the month, "last" the last day (monthly only: the web UI reads
// a yearly last day as an Advanced rule), and "<n> <weekday>" the nth weekday, n being 1 to 5 or "last".
func parseDay(text string, s *Schedule) error {
	words := strings.Fields(strings.ToLower(text))
	switch {
	case len(words) == 0:
		return errDayEmpty
	case len(words) == 1 && words[0] == "last" && s.Frequency == Yearly:
		return errYearlyLast
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

func nthOf(word string) (int, bool) {
	if word == "last" {
		return -1, true
	}
	n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(word, "st"), "nd"), "rd"), "th"))
	return n, err == nil && n >= 1 && n <= 5
}

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
	if f.Refused || f.refuseSubmit(m, "making this Issue recur") {
		return m, nil
	}
	terms, err := f.terms()
	if err != nil {
		f.Err = err
		return m, nil
	}
	f.Saving, f.Err = true, nil
	if f.Change {
		return m, []Cmd{ChangeRecurrenceCmd{Workspace: m.Workspace, UID: f.UID, ProjectID: f.ProjectID, Actor: m.Actor, RecurrenceUID: f.Opened.UID,
			RecurrenceTerms: terms, Opened: f.Opened.RecurrenceTerms}}
	}
	return m, []Cmd{CreateRecurrenceCmd{Workspace: m.Workspace, UID: f.UID, ProjectID: f.ProjectID, Actor: m.Actor, RecurrenceTerms: terms}}
}

func updateRecurrence(m Model, msg Msg) (Model, []Cmd) {
	result, ok := msg.(RecurrenceResultMsg)
	if !ok || m.RecurrenceForm.UID != result.UID {
		return m, nil
	}
	m.RecurrenceForm.Saving, m.RecurrenceForm.Stopping, m.RecurrenceForm.Err = false, false, result.Err
	if result.Changed != nil {
		// Someone else changed the schedule: nothing was written, and the card shows what is there now.
		if issue := closeIssue(m); issue != nil {
			m.RecurrenceForm = changeRecurrenceForm(m.Calendar, issue, *result.Changed)
			m.RecurrenceForm.Err = errRecurrenceChanged
		}
		m = m.issueWritten(result.UID)
		return snapshotChanged(m)
	}
	if result.Err == nil {
		m.RecurrenceForm.Open = false
		m = m.issueWritten(result.UID)
		return snapshotChanged(m)
	}
	return m, nil
}

func chooseFrequency(f *RecurrenceFormState, i int) bool {
	if i < 0 || i >= len(Frequencies) {
		return false
	}
	f.Frequency = Frequencies[i]
	if f.Day == "" {
		f.Day = f.startDay()
	}
	return true
}

func chooseEnds(f *RecurrenceFormState, i int) bool {
	if i < 0 || i >= len(RecurrenceEndKinds) {
		return false
	}
	f.Ends = RecurrenceEndKinds[i]
	return true
}

func chooseMonth(f *RecurrenceFormState, i int) bool {
	if i < 0 || i >= 12 {
		return false
	}
	f.Month = i + 1
	return true
}

func toggleWeekday(f *RecurrenceFormState, i int) bool {
	if i < 0 || i >= len(MondayFirst) {
		return false
	}
	f.WeekdayAt = i
	f.Weekdays[MondayFirst[i]] = !f.Weekdays[MondayFirst[i]]
	return true
}

// Left and right move the weekday cursor in the Weekdays field, which picks the day under the cursor by index.
var recurrenceTable = fieldTable[RecurrenceFormState]{
	form:   func(m *Model) *RecurrenceFormState { return &m.RecurrenceForm },
	busy:   func(f RecurrenceFormState) bool { return f.Saving || f.StopConfirm || f.Refused },
	hidden: func(m Model, name Field) bool { return !m.RecurrenceForm.Shows(name) },
	fields: []formField[RecurrenceFormState]{
		choiceField(RecurrenceFrequency,
			func(f *RecurrenceFormState, back bool) {
				chooseFrequency(f, cycleIndex(slices.Index(Frequencies, f.Frequency), len(Frequencies), back))
			},
			chooseFrequency),
		textField(RecurrenceEvery, func(f *RecurrenceFormState) *string { return &f.Every }),
		choiceField(RecurrenceWeekdays,
			func(f *RecurrenceFormState, back bool) { f.WeekdayAt = cycleIndex(f.WeekdayAt, 7, back) },
			toggleWeekday),
		textField(RecurrenceDay, func(f *RecurrenceFormState) *string { return &f.Day }),
		choiceField(RecurrenceMonth,
			func(f *RecurrenceFormState, back bool) { chooseMonth(f, cycleIndex(f.Month-1, 12, back)) },
			chooseMonth),
		textField(RecurrenceStart, func(f *RecurrenceFormState) *string { return &f.Start }),
		choiceField(RecurrenceEnds,
			func(f *RecurrenceFormState, back bool) {
				chooseEnds(f, cycleIndex(slices.Index(RecurrenceEndKinds, f.Ends), len(RecurrenceEndKinds), back))
			},
			chooseEnds),
		textField(RecurrenceEnd, func(f *RecurrenceFormState) *string {
			if f.Ends == EndCount {
				return &f.Count
			}
			return &f.Until
		}),
		textField(RecurrenceZone, func(f *RecurrenceFormState) *string { return &f.Zone }),
	},
}

func recurrenceKey(m Model, msg KeyMsg) (Model, []Cmd) {
	f := &m.RecurrenceForm
	if f.StopConfirm {
		return answerStop(m, msg)
	}
	binding, bound := LookupBindingForMode(msg.Chord, ModeRecurrenceForm)
	field := recurrenceTable.textAt(f, f.Field)
	switch {
	case f.Refused && !(bound && binding.Action == RecurrenceCancel):
	case bound && binding.Action == RecurrenceCancel:
		f.Open = false
	case bound && binding.Action == RecurrenceSave:
		return submitRecurrence(m)
	case bound && binding.Action == RecurrenceStop && f.Change:
		f.StopConfirm, f.Err = true, nil
	case bound && binding.Action == InputSpace && field == nil && f.Field == RecurrenceWeekdays:
		if toggleWeekday(f, f.WeekdayAt) {
			f.Err = nil
		}
	case bound && binding.Action == InputSpace && field == nil:
		recurrenceTable.stepFocused(&m, false)
	case field != nil:
		recurrenceTable.typeKey(&m, binding, bound, msg)
	}
	return m, nil
}

func answerStop(m Model, msg KeyMsg) (Model, []Cmd) {
	f := &m.RecurrenceForm
	binding, bound := LookupBindingForMode(msg.Chord, ModeRecurrenceStop)
	switch {
	case bound && binding.Action == RecurrenceStopKeep:
		f.StopConfirm = false
	case bound && binding.Action == RecurrenceStopConfirm:
		f.StopConfirm = false
		if f.refuseStale(m, "stopping this recurrence") {
			return m, nil
		}
		f.Saving, f.Stopping, f.Err = true, true, nil
		return m, []Cmd{StopRecurrenceCmd{Workspace: m.Workspace, UID: f.UID, ProjectID: f.ProjectID, Actor: m.Actor, RecurrenceUID: f.Opened.UID, Opened: f.Opened.RecurrenceTerms}}
	}
	return m, nil
}
