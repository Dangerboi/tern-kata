package model

import (
	"fmt"
	"time"
)

// DateTone is the tone of a Date badge; the view maps it to a theme tone.
type DateTone int

const (
	DateMuted DateTone = iota
	DateWarning
	DateError
	DateAccent
)

// DateBadge is the one badge an open card shows for its most pressing date.
type DateBadge struct {
	Text string
	Tone DateTone
}

const dayLayout = "2006-01-02"

// IssueDates are the Issue's scheduled date and deadline as the date Collections read them, empty when absent.
func IssueDates(issue Issue) (scheduled, deadline string) {
	return IssueDate(issue.ScheduledOnDate, issue.Metadata, ScheduledKey), IssueDate(issue.DeadlineOnDate, issue.Metadata, DeadlineKey)
}

// readableDates are IssueDates with any day that does not read as YYYY-MM-DD left empty, as the Date badge and Dates line show them.
func readableDates(issue Issue) (scheduled, deadline string) {
	scheduled, deadline = IssueDates(issue)
	if scheduled != "" && !readableDays(scheduled) {
		scheduled = ""
	}
	if deadline != "" && !readableDays(deadline) {
		deadline = ""
	}
	return scheduled, deadline
}

// CalendarDay is the Pane calendar's date read once, for a frame that draws a Date badge on many cards.
type CalendarDay struct {
	day  string
	date time.Time
	ok   bool
}

// NewCalendarDay reads day as YYYY-MM-DD; a day that does not read is a CalendarDay with no date.
func NewCalendarDay(day string) CalendarDay {
	date, err := time.Parse(dayLayout, day)
	return CalendarDay{day: day, date: date, ok: err == nil}
}

// ChooseDateBadge is the Date badge of an open Issue against t, and false when it has none. Days are compared as calendar
// strings without time-zone conversion, as the date Collections do. Without today the earlier date shows plainly (the deadline
// on a tie).
func (t CalendarDay) ChooseDateBadge(issue Issue) (DateBadge, bool) {
	scheduled, deadline := readableDates(issue)
	if scheduled == "" && deadline == "" {
		return DateBadge{}, false
	}
	today := t.day
	if !t.ok {
		if deadline != "" && (scheduled == "" || deadline <= scheduled) {
			return DateBadge{Text: "due " + deadline}, true
		}
		return DateBadge{Text: "scheduled " + scheduled}, true
	}
	// Both days read here: readableDates left out any that did not, and today reads.
	switch {
	case deadline != "" && deadline < today:
		return DateBadge{Text: fmt.Sprintf("overdue %dd", t.daysUntil(deadline)*-1), Tone: t.deadlineTone(deadline)}, true
	case deadline == today:
		return DateBadge{Text: "due today", Tone: t.deadlineTone(deadline)}, true
	case scheduled != "" && scheduled < today:
		return DateBadge{Text: fmt.Sprintf("scheduled %dd ago", t.daysUntil(scheduled)*-1), Tone: t.scheduledTone(scheduled)}, true
	case scheduled == today:
		return DateBadge{Text: "scheduled today", Tone: t.scheduledTone(scheduled)}, true
	case deadline != "":
		if days := t.daysUntil(deadline); days == 1 {
			return DateBadge{Text: "due tomorrow", Tone: t.deadlineTone(deadline)}, true
		}
		return DateBadge{Text: "due " + shortDay(deadline, today), Tone: t.deadlineTone(deadline)}, true
	}
	if t.daysUntil(scheduled) == 1 {
		return DateBadge{Text: "scheduled tomorrow", Tone: t.scheduledTone(scheduled)}, true
	}
	return DateBadge{Text: "scheduled " + shortDay(scheduled, today), Tone: t.scheduledTone(scheduled)}, true
}

// daysUntil is the whole days from today to the readable day; negative for a day before.
func (t CalendarDay) daysUntil(day string) int {
	to, _ := time.Parse(dayLayout, day)
	return int(to.Sub(t.date).Hours() / 24)
}

// deadlineTone is the tone of a readable deadline day against the calendar day, which reads: overdue error, today and
// tomorrow warning, later muted.
func (t CalendarDay) deadlineTone(day string) DateTone {
	switch {
	case day < t.day:
		return DateError
	case t.daysUntil(day) <= 1:
		return DateWarning
	}
	return DateMuted
}

// scheduledTone is the tone of a readable scheduled day against the calendar day, which reads: passed warning, today
// accent, later muted.
func (t CalendarDay) scheduledTone(day string) DateTone {
	switch {
	case day < t.day:
		return DateWarning
	case day == t.day:
		return DateAccent
	}
	return DateMuted
}

// DeadlineTone is the tone of a deadline day against today: overdue error, today and tomorrow warning, later muted.
// Muted when either day is unreadable.
func DeadlineTone(day, today string) DateTone {
	if t := NewCalendarDay(today); t.ok && readableDays(day) {
		return t.deadlineTone(day)
	}
	return DateMuted
}

// ScheduledTone is the tone of a scheduled day against today: passed warning, today accent, later muted.
// Muted when either day is unreadable.
func ScheduledTone(day, today string) DateTone {
	if t := NewCalendarDay(today); t.ok && readableDays(day) {
		return t.scheduledTone(day)
	}
	return DateMuted
}

func readableDays(days ...string) bool {
	for _, d := range days {
		if _, err := time.Parse(dayLayout, d); err != nil {
			return false
		}
	}
	return true
}

// shortDay is day as MM-DD, or YYYY-MM-DD in another year than today.
func shortDay(day, today string) string {
	if day[:4] == today[:4] {
		return day[5:]
	}
	return day
}

// DatesLine is the selected card's line "scheduled YYYY-MM-DD · due YYYY-MM-DD", absent parts left out, empty without dates.
func DatesLine(issue Issue) string {
	scheduled, deadline := readableDates(issue)
	line := ""
	if scheduled != "" {
		line = "scheduled " + scheduled
	}
	if deadline != "" {
		if line != "" {
			line += " · "
		}
		line += "due " + deadline
	}
	return line
}
