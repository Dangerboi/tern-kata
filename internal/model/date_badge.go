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

// ChooseDateBadge is the Date badge of an open Issue against the Pane calendar's date today, and false when it has none.
// Days are compared as calendar strings without time-zone conversion, as the date Collections do. Without today the earlier
// date shows plainly (the deadline on a tie).
func ChooseDateBadge(issue Issue, today string) (DateBadge, bool) {
	scheduled, deadline := IssueDates(issue)
	if _, err := time.Parse(dayLayout, scheduled); err != nil {
		scheduled = ""
	}
	if _, err := time.Parse(dayLayout, deadline); err != nil {
		deadline = ""
	}
	if scheduled == "" && deadline == "" {
		return DateBadge{}, false
	}
	if _, err := time.Parse(dayLayout, today); err != nil {
		if deadline != "" && (scheduled == "" || deadline <= scheduled) {
			return DateBadge{Text: "due " + deadline}, true
		}
		return DateBadge{Text: "scheduled " + scheduled}, true
	}
	switch {
	case deadline != "" && deadline < today:
		return DateBadge{Text: fmt.Sprintf("overdue %dd", daysBetween(deadline, today)), Tone: DeadlineTone(deadline, today)}, true
	case deadline == today:
		return DateBadge{Text: "due today", Tone: DeadlineTone(deadline, today)}, true
	case scheduled != "" && scheduled < today:
		return DateBadge{Text: fmt.Sprintf("scheduled %dd ago", daysBetween(scheduled, today)), Tone: ScheduledTone(scheduled, today)}, true
	case scheduled == today:
		return DateBadge{Text: "scheduled today", Tone: ScheduledTone(scheduled, today)}, true
	case deadline != "":
		if daysBetween(today, deadline) == 1 {
			return DateBadge{Text: "due tomorrow", Tone: DeadlineTone(deadline, today)}, true
		}
		return DateBadge{Text: "due " + shortDay(deadline, today)}, true
	}
	if daysBetween(today, scheduled) == 1 {
		return DateBadge{Text: "scheduled tomorrow"}, true
	}
	return DateBadge{Text: "scheduled " + shortDay(scheduled, today)}, true
}

// DeadlineTone is the tone of a deadline day against today: overdue error, today and tomorrow warning, later muted.
// Muted when either day is unreadable.
func DeadlineTone(day, today string) DateTone {
	if !readableDays(day, today) {
		return DateMuted
	}
	switch {
	case day < today:
		return DateError
	case daysBetween(today, day) <= 1:
		return DateWarning
	}
	return DateMuted
}

// ScheduledTone is the tone of a scheduled day against today: passed warning, today accent, later muted.
// Muted when either day is unreadable.
func ScheduledTone(day, today string) DateTone {
	if !readableDays(day, today) {
		return DateMuted
	}
	switch {
	case day < today:
		return DateWarning
	case day == today:
		return DateAccent
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

// daysBetween is the whole days from the earlier calendar day a to the later b.
func daysBetween(a, b string) int {
	from, _ := time.Parse(dayLayout, a)
	to, _ := time.Parse(dayLayout, b)
	return int(to.Sub(from).Hours() / 24)
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
	scheduled, deadline := IssueDates(issue)
	if _, err := time.Parse(dayLayout, scheduled); err != nil {
		scheduled = ""
	}
	if _, err := time.Parse(dayLayout, deadline); err != nil {
		deadline = ""
	}
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
