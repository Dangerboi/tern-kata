package model

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// DateInputHint says which forms ParseDateInput reads, for a message that refuses anything else.
const DateInputHint = "YYYY-MM-DD, today, tomorrow, +Nd or +Nw"

var relativeDate = regexp.MustCompile(`^\+(\d{1,4})([dw])$`)

// ParseDateInput reads a typed calendar date against today, the Pane calendar's date: YYYY-MM-DD, today, tomorrow, +Nd (N days
// on) or +Nw (N weeks on). Blank input gives "" and no error, which callers read as no date. Anything else is an error that
// does not name a field; the caller adds the name. A relative form with no today known (the Pane calendar has no date yet) is
// refused, since it has nothing to count from.
func ParseDateInput(input, today string) (string, error) {
	text := strings.ToLower(strings.TrimSpace(input))
	if text == "" {
		return "", nil
	}
	if day, err := time.Parse(dayLayout, text); err == nil {
		return day.Format(dayLayout), nil
	}
	var days int
	switch match := relativeDate.FindStringSubmatch(text); {
	case text == "today":
	case text == "tomorrow":
		days = 1
	case match != nil:
		days, _ = strconv.Atoi(match[1])
		if match[2] == "w" {
			days *= 7
		}
	default:
		return "", errors.New("must be " + DateInputHint + ".")
	}
	base, err := time.Parse(dayLayout, today)
	if err != nil {
		return "", errors.New("cannot count from today yet; the Pane does not know the date. Give " + dayLayout + ".")
	}
	return base.AddDate(0, 0, days).Format(dayLayout), nil
}

// DateWithWeekday is a YYYY-MM-DD day with its weekday, "2026-10-24 Sat"; a text that is not a day gives "".
func DateWithWeekday(day string) string {
	t, err := time.Parse(dayLayout, day)
	if err != nil {
		return ""
	}
	return day + " " + t.Format("Mon")
}
