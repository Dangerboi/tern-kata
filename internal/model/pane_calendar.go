package model

import "time"

// PaneCalendar is the one day and time zone the Pane reads every date in. The zero value is UTC with no date.
// Scheduled and deadline dates are not read through it: they compare as kata stores them.
type PaneCalendar struct {
	Date string // today's local date, YYYY-MM-DD; empty before the first dated Collection snapshot
	Zone string
	Note string // set when UTC stands in for an unknown or unloadable zone
	loc  *time.Location
}

// NewPaneCalendar reads date in zone; an unloadable zone reads as UTC.
func NewPaneCalendar(date, zone, note string) PaneCalendar {
	loc, _ := time.LoadLocation(zone)
	return PaneCalendar{Date: date, Zone: zone, Note: note, loc: loc}
}

func (c PaneCalendar) in(t time.Time) time.Time {
	if c.loc == nil {
		return t.UTC()
	}
	return t.In(c.loc)
}

// DayOf is the local day an instant falls on, YYYY-MM-DD.
func (c PaneCalendar) DayOf(t time.Time) string { return c.in(t).Format("2006-01-02") }

// Stamp reads an instant as date and time.
func (c PaneCalendar) Stamp(t time.Time) string { return c.in(t).Format("2006-01-02 15:04") }

// Read reads an instant as the time alone when it falls on the local day of now, else as date and time.
func (c PaneCalendar) Read(t, now time.Time) string {
	if c.DayOf(t) == c.DayOf(now) {
		return c.in(t).Format("15:04")
	}
	return c.Stamp(t)
}

// DayEnds is local midnight of the day after the one now falls on; daylight-saving days are 23 or 25 hours long.
func (c PaneCalendar) DayEnds(now time.Time) time.Time {
	y, mo, d := c.in(now).Date()
	loc := c.loc
	if loc == nil {
		loc = time.UTC
	}
	return time.Date(y, mo, d+1, 0, 0, 0, 0, loc)
}

// EndOfDate is when the local day named by Date ends; an empty or unreadable Date has none.
func (c PaneCalendar) EndOfDate() (time.Time, bool) {
	loc := c.loc
	if loc == nil {
		loc = time.UTC
	}
	day, err := time.ParseInLocation("2006-01-02", c.Date, loc)
	if err != nil {
		return time.Time{}, false
	}
	return c.DayEnds(day.Add(12 * time.Hour)), true
}
