package model

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const DateInputHint = "YYYY-MM-DD, YYYY-MM-DDTHH:MM[:SS], YYYY-MM-DDTHH:MM:SSZ, today, tomorrow, +Nd or +Nw"

var relativeDate = regexp.MustCompile(`^\+(\d{1,4})([dw])$`)

// ParseDateInput reads a typed date against today, the Pane calendar's date, with the forms kata stores for scheduled_on and
// deadline_on: YYYY-MM-DD, a local YYYY-MM-DDTHH:MM or YYYY-MM-DDTHH:MM:SS, a UTC YYYY-MM-DDTHH:MM:SSZ, and, resolved here to
// YYYY-MM-DD, today, tomorrow, +Nd (N days on) or +Nw (N weeks on). A timed value is returned as typed, since that is what kata
// stores. Blank input gives "" and no error, which callers read as no date. Anything else is an error that does not name a
// field; the caller adds the name. A relative form with no today known (the Pane calendar has no date yet) is
// refused, since it has nothing to count from.
func ParseDateInput(input, today string) (string, error) {
	typed := strings.TrimSpace(input)
	text := strings.ToLower(typed)
	if text == "" {
		return "", nil
	}
	if day, err := time.Parse(dayLayout, text); err == nil {
		return day.Format(dayLayout), nil
	}
	if timedDate(typed) {
		return typed, nil
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

func timedDate(text string) bool {
	for _, layout := range []string{"2006-01-02T15:04", "2006-01-02T15:04:05"} {
		if _, err := time.Parse(layout, text); err == nil {
			return true
		}
	}
	if strings.HasSuffix(text, "Z") {
		_, err := time.Parse(time.RFC3339, text)
		return err == nil
	}
	return false
}

// DateWithWeekday is a parsed date with its weekday, "2026-10-24 Sat". A local time keeps its clock time ("2026-10-24 Sat
// 09:00"); a UTC instant is shown as typed, since its calendar day depends on the zone it is read in. A text that is not a
// date gives "".
func DateWithWeekday(value string) string {
	if strings.HasSuffix(value, "Z") {
		if timedDate(value) {
			return value
		}
		return ""
	}
	if len(value) < len(dayLayout) {
		return ""
	}
	t, err := time.Parse(dayLayout, value[:len(dayLayout)])
	if err != nil {
		return ""
	}
	out := value[:len(dayLayout)] + " " + t.Format("Mon")
	if clock := value[len(dayLayout):]; strings.HasPrefix(clock, "T") {
		out += " " + clock[1:]
	}
	return out
}
