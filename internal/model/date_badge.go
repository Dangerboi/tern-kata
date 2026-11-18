package model

import (
	"fmt"
	"time"
)

type DateTone int

const (
	DateMuted DateTone = iota
	DateWarning
	DateError
)

// DateBadge is the Date badge text and tone. Other is the Issue's other date in the same words ("due 2026-12-30"), empty when
// the Issue has one date; the selected card shows it, since the badge shows the nearer one.
type DateBadge struct {
	Text  string
	Tone  DateTone
	Other string
}

const dayLayout = "2006-01-02"

func IssueDates(issue Issue) (scheduled, deadline string) {
	return IssueDate(issue.ScheduledOnDate, issue.Metadata, ScheduledKey), IssueDate(issue.DeadlineOnDate, issue.Metadata, DeadlineKey)
}

// readableDates are IssueDates with any day that does not read as YYYY-MM-DD left empty, as the Date badge shows them.
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
	dueOther, scheduledOther := "", ""
	if deadline != "" {
		dueOther = "due " + deadline
	}
	if scheduled != "" {
		scheduledOther = "scheduled " + scheduled
	}
	// pickDeadline is the badge showing the deadline; the other date is the one it leaves out.
	pickDeadline := func(text string, tone DateTone) (DateBadge, bool) {
		return DateBadge{Text: text, Tone: tone, Other: scheduledOther}, true
	}
	pickScheduled := func(text string) (DateBadge, bool) {
		return DateBadge{Text: text, Other: dueOther}, true
	}
	today := t.day
	if !t.ok {
		if deadline != "" && (scheduled == "" || deadline <= scheduled) {
			return pickDeadline(dueOther, DateMuted)
		}
		return pickScheduled(scheduledOther)
	}
	switch {
	case deadline != "" && deadline < today:
		return pickDeadline(fmt.Sprintf("overdue %dd", t.daysUntil(deadline)*-1), t.deadlineTone(deadline))
	case deadline == today:
		return pickDeadline("due today", t.deadlineTone(deadline))
	case scheduled != "" && scheduled < today:
		return pickScheduled(scheduledOther)
	case scheduled == today:
		return pickScheduled("scheduled today")
	case deadline != "":
		if t.daysUntil(deadline) == 1 {
			return pickDeadline("due tomorrow", t.deadlineTone(deadline))
		}
		return pickDeadline(dueOther, t.deadlineTone(deadline))
	}
	if t.daysUntil(scheduled) == 1 {
		return pickScheduled("scheduled tomorrow")
	}
	return pickScheduled(scheduledOther)
}

func (t CalendarDay) daysUntil(day string) int {
	to, _ := time.Parse(dayLayout, day)
	return int(to.Sub(t.date).Hours() / 24)
}

func (t CalendarDay) deadlineTone(day string) DateTone {
	switch {
	case day < t.day:
		return DateError
	case t.daysUntil(day) <= 1:
		return DateWarning
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

func readableDays(days ...string) bool {
	for _, d := range days {
		if _, err := time.Parse(dayLayout, d); err != nil {
			return false
		}
	}
	return true
}

// DatesLine is the line "scheduled YYYY-MM-DD · due YYYY-MM-DD" of an Issue, absent parts left out, empty without dates. The
// kata-tern CLI's views output is its only caller; the Pane shows dates through the Date badge.
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
