package model

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// LoadRecurTarget reads the Issue Ref names, as `tern-kata recur` takes it: a short id in the workspace's Binding,
// <project>#<short id> or a ULID, which must be in that Binding. The answer is RecurTarget.
type LoadRecurTarget struct{ Workspace, Ref string }

func (LoadRecurTarget) cmd() {}

// RecurTarget answers LoadRecurTarget: the Issue, its Recurrence as it stands (nil when it never recurred) and the Pane's
// day and time zone, the defaults of a new schedule.
type RecurTarget struct {
	Ref, UID, ShortID, ScheduledOnDate string
	ProjectID                          int64
	Recurrence                         *Recurrence
	Today, Zone                        string
	Err                                error
}

func (RecurTarget) msg() {}

// Recurring is the live Recurrence, nil when the Issue does not recur or its Recurrence was stopped.
func (t RecurTarget) Recurring() *Recurrence {
	if t.Recurrence == nil || t.Recurrence.Stopped {
		return nil
	}
	return t.Recurrence
}

// Report is the one line `tern-kata recur <ref>` prints: the schedule in words, the next Occurrence's date, the start and
// the zone; or that the Issue does not recur.
func (t RecurTarget) Report() string {
	switch r := t.Recurring(); {
	case t.Recurrence == nil:
		return fmt.Sprintf("%s does not recur.", t.ShortID)
	case r == nil:
		return fmt.Sprintf("The recurrence of %s was stopped.", t.ShortID)
	default:
		return fmt.Sprintf("%s recurs: %s (start %s, %s)", t.ShortID, r.Text(), r.Start, r.TimeZone)
	}
}

// StopReport is the line a stop prints.
func (t RecurTarget) StopReport() string {
	return fmt.Sprintf("%s no longer recurs; its existing Occurrences stay.", t.ShortID)
}

// WithRecurrence is the target as it stands with rec, the recurrence a Guarded change found.
func (t RecurTarget) WithRecurrence(rec Recurrence) RecurTarget {
	t.Recurrence = &rec
	return t
}

// Refusals of `recur`, each saying nothing was written.
var (
	ErrRecurrenceWithheld = errRecurrenceWithheld

	ErrRecurrenceMovedMeanwhile = errors.New("The schedule changed meanwhile; nothing was written. The current schedule is printed above.")
)

// NotRecurringError is the refusal to change or stop an Issue that never recurred.
func NotRecurringError(shortID string) error {
	return fmt.Errorf("%s does not recur; nothing was written.", shortID)
}

// RecurStoppedError is the refusal to change or stop a Recurrence that was stopped.
func RecurStoppedError(shortID string) error {
	return fmt.Errorf("The recurrence of %s was stopped; nothing was written. kata can't make it recur again.", shortID)
}

// RecurRefusal is why a workspace bound to binding cannot make the Issue shortID of Project issueProject recur, owner being
// that Project's name, or nil when it can: like an attachment, a schedule is written from the Project's own Binding.
func RecurRefusal(binding Project, issueProject int64, shortID, owner string) error {
	if CanAttach(binding, issueProject) {
		return nil
	}
	if binding.UID == "" {
		return fmt.Errorf("This workspace is not bound to a kata Project, so it cannot change the schedule of %s. Work from a workspace bound to Project %q.", shortID, owner)
	}
	return fmt.Errorf("%s belongs to Project %q, but this workspace is bound to %q. To change its schedule, work from a workspace bound to %q.", shortID, owner, binding.Name, owner)
}

// RecurFlagNames are the schedule flags of `recur`, without their dashes. A flag that takes no value is not among them: --stop.
var RecurFlagNames = []string{"frequency", "every", "on", "day", "month", "until", "count", "start", "zone", "rule"}

// Terms reads the schedule flags of `recur` (names without dashes, values as typed) into the terms to send, through the
// Recurrence card's own reading, so both refuse the same input with the same message. Defaults are the card's: weekly
// on the start date's weekday, every 1, never ending; the start date is --start, else the Issue's scheduled date, else today (an Issue
// that recurs keeps its start), and the weekday, day and month defaults come from it; the zone is the Pane's. --rule replaces the frequency flags.
func (t RecurTarget) Terms(flags map[string]string) (RecurrenceTerms, error) {
	issue := &Issue{UID: t.UID, ProjectID: t.ProjectID}
	zone := t.Zone
	switch {
	case t.Recurring() != nil:
		issue.ScheduledOnDate = &t.Recurring().Start
		if t.Recurring().TimeZone != "Local" { // "Local" is not a zone the card accepts; keep the Pane's
			zone = t.Recurring().TimeZone
		}
	case t.ScheduledOnDate != "":
		issue.ScheduledOnDate = &t.ScheduledOnDate
	}
	if v, ok := flags["start"]; ok {
		if _, err := time.Parse(time.DateOnly, strings.TrimSpace(v)); err != nil {
			return RecurrenceTerms{}, errStartDate
		}
		issue.ScheduledOnDate = &v
	}
	f := newRecurrenceForm(Model{Calendar: NewPaneCalendar(t.Today, zone, "")}, issue)
	if v, ok := flags["zone"]; ok {
		f.Zone = v
	}
	if rule, ok := flags["rule"]; ok {
		if slices.ContainsFunc(RecurFlagNames, func(n string) bool { _, given := flags[n]; return given && n != "rule" && n != "start" && n != "zone" }) {
			return RecurrenceTerms{}, errors.New("--rule replaces the frequency flags; give one or the other.")
		}
		return f.rawTerms(rule)
	}
	if err := f.readFlags(flags); err != nil {
		return RecurrenceTerms{}, err
	}
	return f.terms()
}

// ZoneNote is the note `recur` prints when the stored zone is Local and the terms Terms gives set a real one instead; "" when
// the stored zone is not Local or Terms refuses.
func (t RecurTarget) ZoneNote(flags map[string]string) string {
	rec := t.Recurring()
	if rec == nil {
		return ""
	}
	terms, err := t.Terms(flags)
	if err != nil {
		return ""
	}
	return localZoneNote(rec.TimeZone, terms.TimeZone)
}

// rawTerms is a rule given as text: it goes to kata as written, with the card's checks of the start date and the zone.
func (f RecurrenceFormState) rawTerms(rule string) (RecurrenceTerms, error) {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return RecurrenceTerms{}, errors.New("--rule needs a rule such as FREQ=WEEKLY;BYDAY=MO.")
	}
	start, zone := strings.TrimSpace(f.Start), strings.TrimSpace(f.Zone)
	if _, err := time.Parse(time.DateOnly, start); err != nil {
		return RecurrenceTerms{}, errStartDate
	}
	if _, err := time.LoadLocation(zone); err != nil || zone == "" || zone == "Local" {
		return RecurrenceTerms{}, fmt.Errorf("Unknown time zone %q.", zone)
	}
	return RecurrenceTerms{Rule: rule, Start: start, TimeZone: zone}, nil
}

// readFlags sets the card's fields from the flags. A flag the frequency does not use is refused, not ignored.
func (f *RecurrenceFormState) readFlags(flags map[string]string) error {
	freq, ok := flags["frequency"]
	if !ok {
		return errors.New("Give --frequency daily, weekly, monthly or yearly, or --rule.")
	}
	f.Frequency = Frequency(strings.ToUpper(strings.TrimSpace(freq)))
	if !slices.Contains(Frequencies, f.Frequency) {
		return fmt.Errorf("Frequency must be daily, weekly, monthly or yearly, not %q.", freq)
	}
	if v, ok := flags["every"]; ok {
		f.Every = v
	}
	if v, ok := flags["on"]; ok {
		if f.Frequency != Weekly {
			return errors.New("--on applies to weekly schedules only.")
		}
		f.Weekdays = [7]bool{}
		for _, word := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' }) {
			day, found := weekdayNamed(strings.ToLower(word))
			if !found {
				return fmt.Errorf("%q is not a weekday; list them like mon,thu.", word)
			}
			f.Weekdays[day] = true
		}
	}
	if v, ok := flags["day"]; ok {
		if f.Frequency != Monthly && f.Frequency != Yearly {
			return errors.New("--day applies to monthly and yearly schedules only.")
		}
		f.Day = v
	}
	if v, ok := flags["month"]; ok {
		if f.Frequency != Yearly {
			return errors.New("--month applies to yearly schedules only.")
		}
		month, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || month < 1 || month > 12 {
			return errors.New("Month must be between 1 and 12.")
		}
		f.Month = month
	}
	until, hasUntil := flags["until"]
	count, hasCount := flags["count"]
	switch {
	case hasUntil && hasCount:
		return errors.New("Give --until or --count, not both.")
	case hasUntil:
		f.Ends, f.Until = EndUntil, until
	case hasCount:
		f.Ends, f.Count = EndCount, count
	}
	return nil
}
