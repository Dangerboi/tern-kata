package model

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Frequency is how often a Schedule repeats.
type Frequency string

const (
	Daily   Frequency = "DAILY"
	Weekly  Frequency = "WEEKLY"
	Monthly Frequency = "MONTHLY"
	Yearly  Frequency = "YEARLY"
)

// Frequencies is every Frequency, in the order the card cycles them.
var Frequencies = []Frequency{Daily, Weekly, Monthly, Yearly}

// DayKind says which day of the month a monthly or yearly Schedule falls on.
type DayKind int

const (
	DayFromStart DayKind = iota // no day part: the start date's day
	DayOfMonth                  // the DayOfMonth-th, 1 to 31
	NthWeekday                  // the Nth Weekday: 1 to 5, or -1 for the last
	LastDay                     // the last day of the month
)

// EndKind says when a Schedule stops.
type EndKind int

const (
	EndNever EndKind = iota
	EndUntil         // on Until, a YYYY-MM-DD date
	EndCount         // after Count occurrences
)

// Schedule is a recurrence rule in the shape of kata's web UI Common mode: the one thing the Recurrence card edits. A rule
// Common mode cannot express is raw: Raw holds its text and every other field is zero. RRULE and ParseSchedule are inverse
// for every non-raw Schedule. LastDay is monthly only: the web UI reads a yearly BYMONTHDAY=-1 as raw.
type Schedule struct {
	Raw        string
	Frequency  Frequency
	Interval   int
	Weekdays   []time.Weekday // weekly, in Monday-first order; none means the start date's weekday
	Month      int            // yearly, 1 to 12; 0 means the start date's month
	Day        DayKind
	DayOfMonth int
	Nth        int
	Weekday    time.Weekday // the weekday of NthWeekday
	End        EndKind
	Until      string
	Count      int
}

// IsRaw says the rule is outside the form and Raw holds its text.
func (s Schedule) IsRaw() bool { return s.Raw != "" }

// MondayFirst is the weekdays in the order the web UI sorts and the card lists them.
var MondayFirst = []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday, time.Sunday}

var weekdayCodes = []string{"MO", "TU", "WE", "TH", "FR", "SA", "SU"} // parallel to MondayFirst

// MondayIndex is where d falls in MondayFirst.
func MondayIndex(d time.Weekday) int { return slices.Index(MondayFirst, d) }

func weekdayCode(d time.Weekday) string { return weekdayCodes[MondayIndex(d)] }

func weekdayOf(code string) (time.Weekday, bool) {
	i := slices.Index(weekdayCodes, code)
	if i < 0 {
		return 0, false
	}
	return MondayFirst[i], true
}

// sortWeekdays returns days Monday first, without repeats.
func sortWeekdays(days []time.Weekday) []time.Weekday {
	out := slices.Clone(days)
	slices.SortFunc(out, func(a, b time.Weekday) int { return MondayIndex(a) - MondayIndex(b) })
	return slices.Compact(out)
}

// RRULE is the rule text kata's web UI writes for this Schedule: FREQ, INTERVAL, BYMONTH, the day, then the end.
func (s Schedule) RRULE() string {
	if s.IsRaw() {
		return s.Raw
	}
	parts := []string{"FREQ=" + string(s.Frequency), "INTERVAL=" + strconv.Itoa(s.Interval)}
	if s.Frequency == Yearly && s.Month > 0 {
		parts = append(parts, "BYMONTH="+strconv.Itoa(s.Month))
	}
	switch {
	case s.Frequency == Weekly && len(s.Weekdays) > 0:
		codes := make([]string, 0, len(s.Weekdays))
		for _, d := range sortWeekdays(s.Weekdays) {
			codes = append(codes, weekdayCode(d))
		}
		parts = append(parts, "BYDAY="+strings.Join(codes, ","))
	case s.Frequency == Monthly || s.Frequency == Yearly:
		switch s.Day {
		case DayOfMonth:
			parts = append(parts, "BYMONTHDAY="+strconv.Itoa(s.DayOfMonth))
		case LastDay:
			parts = append(parts, "BYMONTHDAY=-1")
		case NthWeekday:
			parts = append(parts, "BYDAY="+strconv.Itoa(s.Nth)+weekdayCode(s.Weekday))
		}
	}
	switch s.End {
	case EndCount:
		parts = append(parts, "COUNT="+strconv.Itoa(s.Count))
	case EndUntil:
		parts = append(parts, "UNTIL="+strings.ReplaceAll(s.Until, "-", ""))
	}
	return strings.Join(parts, ";")
}

var (
	untilPattern  = regexp.MustCompile(`^(\d{4})(\d{2})(\d{2})(T\d{6}Z?)?$`)
	byDayPattern  = regexp.MustCompile(`^(-?\d+)?([A-Z]{2})$`)
	integer       = regexp.MustCompile(`^-?\d+$`)
	knownRuleKeys = []string{"FREQ", "INTERVAL", "COUNT", "UNTIL", "BYDAY", "BYMONTHDAY", "BYMONTH", "BYYEARDAY", "BYWEEKNO",
		"BYHOUR", "BYMINUTE", "BYSECOND", "BYSETPOS", "WKST"}
)

// ParseSchedule reads a rule the way kata's web UI does in Common mode. A rule Common mode shows as Advanced, or rejects, comes
// back raw with its text.
func ParseSchedule(rule string) Schedule {
	if s, ok := parseCommon(rule); ok {
		return s
	}
	return Schedule{Raw: rule}
}

// parseCommon is the web UI's parseRRule narrowed to "Common or not": false for Advanced and invalid rules alike.
func parseCommon(rule string) (Schedule, bool) {
	parts := map[string]string{}
	for _, segment := range strings.Split(rule, ";") {
		key, value, ok := strings.Cut(segment, "=")
		if _, dup := parts[key]; !ok || dup || !slices.Contains(knownRuleKeys, key) {
			return Schedule{}, false
		}
		parts[key] = value
	}
	s := Schedule{Frequency: Frequency(parts["FREQ"]), Interval: 1}
	if !slices.Contains(Frequencies, s.Frequency) {
		return Schedule{}, false
	}
	for _, key := range []string{"BYYEARDAY", "BYWEEKNO", "BYHOUR", "BYMINUTE", "BYSECOND", "BYSETPOS", "WKST"} {
		if _, ok := parts[key]; ok {
			return Schedule{}, false
		}
	}
	if v, ok := parts["INTERVAL"]; ok {
		if s.Interval = positive(v); s.Interval == 0 {
			return Schedule{}, false
		}
	}
	count, hasCount := parts["COUNT"]
	until, hasUntil := parts["UNTIL"]
	if hasCount && hasUntil {
		return Schedule{}, false
	}
	if hasCount {
		if s.Count = positive(count); s.Count == 0 {
			return Schedule{}, false
		}
		s.End = EndCount
	}
	if hasUntil {
		m := untilPattern.FindStringSubmatch(until)
		if m == nil || m[4] != "" {
			return Schedule{}, false
		}
		date, err := time.Parse("20060102", until)
		if err != nil {
			return Schedule{}, false
		}
		s.End, s.Until = EndUntil, date.Format(time.DateOnly)
	}
	byDay, hasDay := parts["BYDAY"]
	byMonthDay, hasMonthDay := parts["BYMONTHDAY"]
	byMonth, hasMonth := parts["BYMONTH"]
	if hasDay && hasMonthDay {
		return Schedule{}, false
	}
	switch s.Frequency {
	case Daily:
		return s, !hasDay && !hasMonthDay && !hasMonth
	case Weekly:
		if hasMonthDay || hasMonth {
			return Schedule{}, false
		}
		if hasDay {
			for _, code := range strings.Split(byDay, ",") {
				d, ok := weekdayOf(code)
				if !ok {
					return Schedule{}, false
				}
				s.Weekdays = append(s.Weekdays, d)
			}
			s.Weekdays = sortWeekdays(s.Weekdays)
		}
		return s, true
	}
	if s.Frequency == Monthly && hasMonth {
		return Schedule{}, false
	}
	if s.Frequency == Yearly {
		if !hasMonth {
			return s, !hasDay && !hasMonthDay
		}
		if !integer.MatchString(byMonth) {
			return Schedule{}, false
		}
		s.Month, _ = strconv.Atoi(byMonth)
		if s.Month < 1 || s.Month > 12 {
			return Schedule{}, false
		}
	}
	switch {
	case hasMonthDay:
		// A single day 1 to 31 (monthly also -1); lists and other negative days are Advanced.
		if !integer.MatchString(byMonthDay) {
			return Schedule{}, false
		}
		day, _ := strconv.Atoi(byMonthDay)
		switch {
		case day == -1 && s.Frequency == Monthly:
			s.Day = LastDay
		case day >= 1 && day <= 31:
			s.Day, s.DayOfMonth = DayOfMonth, day
		default:
			return Schedule{}, false
		}
	case hasDay:
		m := byDayPattern.FindStringSubmatch(byDay)
		if m == nil || m[1] == "" {
			return Schedule{}, false
		}
		nth, _ := strconv.Atoi(m[1])
		weekday, ok := weekdayOf(m[2])
		if !ok || nth == 0 || nth < -1 || nth > 5 {
			return Schedule{}, false
		}
		s.Day, s.Nth, s.Weekday = NthWeekday, nth, weekday
	}
	return s, true
}

// positive reads a whole number of at least 1, else 0.
func positive(s string) int {
	if !integer.MatchString(s) {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 0
	}
	return n
}

var ordinals = map[int]string{-1: "last", 1: "1st", 2: "2nd", 3: "3rd", 4: "4th", 5: "5th"}

// Words reads the Schedule as the web UI does: "Weekly on Mon, Thu", "Every 2 months on the 2nd Tuesday, 10 times". A raw
// Schedule reads as its rule text.
func (s Schedule) Words() string {
	if s.IsRaw() {
		return s.Raw
	}
	names := map[Frequency][2]string{Daily: {"day", "Daily"}, Weekly: {"week", "Weekly"}, Monthly: {"month", "Monthly"}, Yearly: {"year", "Yearly"}}[s.Frequency]
	head := names[1]
	if s.Interval != 1 {
		head = fmt.Sprintf("Every %d %ss", s.Interval, names[0])
	}
	var b strings.Builder
	b.WriteString(head)
	month := time.Month(s.Month).String()
	switch s.Frequency {
	case Weekly:
		if len(s.Weekdays) > 0 {
			days := make([]string, 0, len(s.Weekdays))
			for _, d := range sortWeekdays(s.Weekdays) {
				days = append(days, d.String()[:3])
			}
			b.WriteString(" on " + strings.Join(days, ", "))
		}
	case Monthly:
		switch s.Day {
		case DayOfMonth:
			fmt.Fprintf(&b, " on day %d", s.DayOfMonth)
		case LastDay:
			b.WriteString(" on the last day")
		case NthWeekday:
			fmt.Fprintf(&b, " on the %s %s", ordinals[s.Nth], s.Weekday)
		}
	case Yearly:
		switch {
		case s.Day == DayOfMonth && s.Month > 0:
			fmt.Fprintf(&b, " on %s %d", month, s.DayOfMonth)
		case s.Day == NthWeekday && s.Month > 0:
			fmt.Fprintf(&b, " on the %s %s of %s", ordinals[s.Nth], s.Weekday, month)
		case s.Month > 0:
			b.WriteString(" in " + month)
		}
	}
	switch s.End {
	case EndUntil:
		b.WriteString(" until " + s.Until)
	case EndCount:
		if s.Count == 1 {
			b.WriteString(", 1 time")
		} else {
			fmt.Fprintf(&b, ", %d times", s.Count)
		}
	}
	return b.String()
}
