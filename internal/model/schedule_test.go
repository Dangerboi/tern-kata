package model_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// forms is one Schedule per shape the form can express, with the RRULE kata's web UI writes for it and its words.
var forms = []struct {
	name     string
	schedule model.Schedule
	rrule    string
	words    string
}{
	{"daily", model.Schedule{Frequency: model.Daily, Interval: 1}, "FREQ=DAILY;INTERVAL=1", "Daily"},
	{"every 3 days", model.Schedule{Frequency: model.Daily, Interval: 3}, "FREQ=DAILY;INTERVAL=3", "Every 3 days"},
	{"weekly on the start weekday", model.Schedule{Frequency: model.Weekly, Interval: 1}, "FREQ=WEEKLY;INTERVAL=1", "Weekly"},
	{"weekly on days", model.Schedule{Frequency: model.Weekly, Interval: 1, Weekdays: []time.Weekday{time.Monday, time.Thursday}}, "FREQ=WEEKLY;INTERVAL=1;BYDAY=MO,TH", "Weekly on Mon, Thu"},
	{"weekly with Sunday last", model.Schedule{Frequency: model.Weekly, Interval: 2, Weekdays: []time.Weekday{time.Tuesday, time.Sunday}}, "FREQ=WEEKLY;INTERVAL=2;BYDAY=TU,SU", "Every 2 weeks on Tue, Sun"},
	{"monthly on the start day", model.Schedule{Frequency: model.Monthly, Interval: 1}, "FREQ=MONTHLY;INTERVAL=1", "Monthly"},
	{"monthly day", model.Schedule{Frequency: model.Monthly, Interval: 1, Day: model.DayOfMonth, DayOfMonth: 20}, "FREQ=MONTHLY;INTERVAL=1;BYMONTHDAY=20", "Monthly on day 20"},
	{"monthly last day", model.Schedule{Frequency: model.Monthly, Interval: 2, Day: model.LastDay}, "FREQ=MONTHLY;INTERVAL=2;BYMONTHDAY=-1", "Every 2 months on the last day"},
	{"monthly nth weekday", model.Schedule{Frequency: model.Monthly, Interval: 1, Day: model.NthWeekday, Nth: 2, Weekday: time.Tuesday}, "FREQ=MONTHLY;INTERVAL=1;BYDAY=2TU", "Monthly on the 2nd Tuesday"},
	{"monthly last weekday", model.Schedule{Frequency: model.Monthly, Interval: 1, Day: model.NthWeekday, Nth: -1, Weekday: time.Friday}, "FREQ=MONTHLY;INTERVAL=1;BYDAY=-1FR", "Monthly on the last Friday"},
	{"yearly month", model.Schedule{Frequency: model.Yearly, Interval: 1, Month: 5}, "FREQ=YEARLY;INTERVAL=1;BYMONTH=5", "Yearly in May"},
	{"yearly on the start day", model.Schedule{Frequency: model.Yearly, Interval: 3}, "FREQ=YEARLY;INTERVAL=3", "Every 3 years"},
	{"yearly day", model.Schedule{Frequency: model.Yearly, Interval: 1, Month: 3, Day: model.DayOfMonth, DayOfMonth: 15}, "FREQ=YEARLY;INTERVAL=1;BYMONTH=3;BYMONTHDAY=15", "Yearly on March 15"},
	{"yearly nth weekday", model.Schedule{Frequency: model.Yearly, Interval: 1, Month: 11, Day: model.NthWeekday, Nth: 4, Weekday: time.Thursday}, "FREQ=YEARLY;INTERVAL=1;BYMONTH=11;BYDAY=4TH", "Yearly on the 4th Thursday of November"},
	{"ends on a date", model.Schedule{Frequency: model.Weekly, Interval: 1, Weekdays: []time.Weekday{time.Monday}, End: model.EndUntil, Until: "2026-12-31"}, "FREQ=WEEKLY;INTERVAL=1;BYDAY=MO;UNTIL=20261231", "Weekly on Mon until 2026-12-31"},
	{"ends after N", model.Schedule{Frequency: model.Daily, Interval: 1, End: model.EndCount, Count: 10}, "FREQ=DAILY;INTERVAL=1;COUNT=10", "Daily, 10 times"},
	{"ends after one", model.Schedule{Frequency: model.Monthly, Interval: 1, Day: model.DayOfMonth, DayOfMonth: 1, End: model.EndCount, Count: 1}, "FREQ=MONTHLY;INTERVAL=1;BYMONTHDAY=1;COUNT=1", "Monthly on day 1, 1 time"},
}

func TestEveryFormShapeRoundTripsThroughItsRRULE(t *testing.T) {
	for _, f := range forms {
		t.Run(f.name, func(t *testing.T) {
			if got := f.schedule.RRULE(); got != f.rrule {
				t.Errorf("RRULE %q, want %q", got, f.rrule)
			}
			if got := model.ParseSchedule(f.rrule); !reflect.DeepEqual(got, f.schedule) {
				t.Errorf("parsed %+v, want %+v", got, f.schedule)
			}
			if got := f.schedule.Words(); got != f.words {
				t.Errorf("words %q, want %q", got, f.words)
			}
		})
	}
}

func TestRulesTheWebUICommonModeAcceptsParseToTheSameSchedule(t *testing.T) {
	weekly := model.Schedule{Frequency: model.Weekly, Interval: 1, Weekdays: []time.Weekday{time.Monday, time.Wednesday, time.Friday}}
	for rule, want := range map[string]model.Schedule{
		"FREQ=WEEKLY":                         {Frequency: model.Weekly, Interval: 1},
		"FREQ=WEEKLY;BYDAY=FR,MO,WE":          weekly,
		"FREQ=MONTHLY;INTERVAL=1;BYMONTHDAY=-1": {Frequency: model.Monthly, Interval: 1, Day: model.LastDay},
		"FREQ=YEARLY;BYMONTH=5;BYMONTHDAY=05": {Frequency: model.Yearly, Interval: 1, Month: 5, Day: model.DayOfMonth, DayOfMonth: 5},
		"FREQ=DAILY;UNTIL=20261231":           {Frequency: model.Daily, Interval: 1, End: model.EndUntil, Until: "2026-12-31"},
	} {
		if got := model.ParseSchedule(rule); !reflect.DeepEqual(got, want) {
			t.Errorf("%s parsed %+v, want %+v", rule, got, want)
		}
	}
}

func TestRulesOutsideTheFormParseAsRawAndKeepTheirText(t *testing.T) {
	for _, rule := range []string{
		"FREQ=MONTHLY;BYDAY=MO,TU,WE,TH,FR;BYSETPOS=-1",
		"FREQ=MONTHLY;BYMONTHDAY=15;BYDAY=FR",
		"FREQ=DAILY;BYHOUR=9;BYMINUTE=0",
		"FREQ=HOURLY",
		"FREQ=DAILY;COUNT=3;UNTIL=20261231",
		"FREQ=DAILY;UNTIL=20261231T090000Z",
		"FREQ=DAILY;BYDAY=MO",
		"FREQ=WEEKLY;BYDAY=1MO",
		"FREQ=WEEKLY;BYMONTH=3",
		"FREQ=MONTHLY;BYDAY=MO",
		"FREQ=MONTHLY;BYDAY=-2MO",
		"FREQ=MONTHLY;BYMONTHDAY=1,15",
		"FREQ=MONTHLY;BYMONTHDAY=-2",
		"FREQ=MONTHLY;BYMONTH=3",
		"FREQ=YEARLY;BYMONTHDAY=15",
		"FREQ=YEARLY;BYMONTH=3;BYMONTHDAY=-1",
		"FREQ=YEARLY;BYMONTH=13",
		"FREQ=DAILY;INTERVAL=0",
		"FREQ=DAILY;COUNT=0",
		"FREQ=FORTNIGHTLY",
		"RRULE:FREQ=DAILY",
		"FREQ=DAILY;FREQ=DAILY",
		"FREQ=DAILY;",
		"freq=daily",
	} {
		got := model.ParseSchedule(rule)
		if !got.IsRaw() || got.Raw != rule || got.RRULE() != rule || got.Words() != rule {
			t.Errorf("%s parsed %+v, want raw with its text", rule, got)
		}
	}
}
