package cronjob_test

import (
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

func at(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestStepAcceptsOnlyPositiveIntervalsKataHolds(t *testing.T) {
	cases := []struct {
		name    string
		seconds int64
		want    time.Duration
		ok      bool
	}{
		{"zero", 0, 0, false},
		{"negative", -1, 0, false},
		{"one second", 1, time.Second, true},
		{"the largest accepted", cronjob.MaxIntervalSeconds, time.Duration(cronjob.MaxIntervalSeconds) * time.Second, true},
		{"one past it", cronjob.MaxIntervalSeconds + 1, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := cronjob.Step(tc.seconds)
			if got != tc.want || ok != tc.ok {
				t.Errorf("Step(%d) = %v, %v; want %v, %v", tc.seconds, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestIntervalRefusesAStepKataDoesNotAccept(t *testing.T) {
	for _, seconds := range []int64{0, -5, cronjob.MaxIntervalSeconds + 1} {
		if _, err := cronjob.Interval(seconds, time.Time{}); err == nil {
			t.Errorf("Interval(%d) accepted", seconds)
		}
	}
	if _, err := cronjob.Interval(cronjob.MaxIntervalSeconds, time.Time{}); err != nil {
		t.Errorf("the largest interval refused: %v", err)
	}
}

func TestZoneTakesTheEmbeddedDirectiveThenTheTimezoneThenUTC(t *testing.T) {
	cases := []struct {
		name, expression, timezone string
		zone, bare                 string
	}{
		{"embedded CRON_TZ wins over the timezone", "CRON_TZ=Asia/Tokyo 0 7 * * *", "Europe/Berlin", "Asia/Tokyo", "0 7 * * *"},
		{"embedded TZ wins over the timezone", "TZ=America/New_York 0 7 * * *", "Europe/Berlin", "America/New_York", "0 7 * * *"},
		{"embedded directive with no timezone", "CRON_TZ=Asia/Tokyo @daily", "", "Asia/Tokyo", "@daily"},
		{"surrounding space is dropped", "  CRON_TZ=Asia/Tokyo   0 7 * * *  ", "", "Asia/Tokyo", "0 7 * * *"},
		{"the timezone when there is no directive", "0 9 * * *", "Europe/Berlin", "Europe/Berlin", "0 9 * * *"},
		{"UTC by default", "0 9 * * *", "", "UTC", "0 9 * * *"},
		{"UTC by default for a descriptor", " @hourly ", "", "UTC", "@hourly"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			zone, bare := cronjob.Zone(tc.expression, tc.timezone)
			if zone != tc.zone || bare != tc.bare {
				t.Errorf("Zone(%q, %q) = %q, %q; want %q, %q", tc.expression, tc.timezone, zone, bare, tc.zone, tc.bare)
			}
		})
	}
}

func TestCronReadsTheZoneTheDirectiveOrTheTimezoneNames(t *testing.T) {
	cases := []struct {
		name, expression, timezone string
		after, want                string
	}{
		{"the directive wins", "CRON_TZ=Asia/Tokyo 0 7 * * *", "Europe/Berlin", "2026-10-08T00:00:00Z", "2026-10-08T22:00:00Z"},
		{"the timezone is used without a directive", "0 7 * * *", "Asia/Tokyo", "2026-10-08T00:00:00Z", "2026-10-08T22:00:00Z"},
		{"UTC is the default", "0 7 * * *", "", "2026-10-08T00:00:00Z", "2026-10-08T07:00:00Z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := cronjob.Cron(tc.expression, tc.timezone, time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			if got := s.Next(at(t, tc.after)); !got.Equal(at(t, tc.want)) {
				t.Errorf("next after %s = %s, want %s", tc.after, got, tc.want)
			}
		})
	}
}

func TestCronRefusesWhatKataDoesNot(t *testing.T) {
	for _, expression := range []string{"every tuesday", "0 0 9 * * *", "", "0 9 * *"} {
		if _, err := cronjob.Cron(expression, "", time.Time{}); err == nil {
			t.Errorf("%q parsed", expression)
		}
	}
	if _, err := cronjob.Cron("0 9 * * *", "Mars/Olympus", time.Time{}); err == nil {
		t.Error("an unknown zone parsed")
	}
}

func TestCronNextAndBetweenFollowTheZoneAcrossADaylightSavingChange(t *testing.T) {
	// Berlin moves to summer time on 2026-03-29 at 02:00, so 09:00 there is 08:00 UTC before and 07:00 UTC after.
	s, err := cronjob.Cron("0 9 * * *", "Europe/Berlin", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	nexts := []struct{ after, want string }{
		{"2026-03-28T00:00:00Z", "2026-03-28T08:00:00Z"},
		{"2026-03-28T08:00:00Z", "2026-03-29T07:00:00Z"},
		{"2026-03-29T07:00:00Z", "2026-03-30T07:00:00Z"},
	}
	for _, tc := range nexts {
		if got := s.Next(at(t, tc.after)); !got.Equal(at(t, tc.want)) || got.Location() != time.UTC {
			t.Errorf("next after %s = %s, want %s in UTC", tc.after, got, tc.want)
		}
	}
	betweens := []struct {
		name     string
		from, to string
		want     int
	}{
		{"three days over the change", "2026-03-27T12:00:00Z", "2026-03-30T12:00:00Z", 3},
		{"from an occurrence is exclusive", "2026-03-28T08:00:00Z", "2026-03-29T07:00:00Z", 1},
		{"to an occurrence is inclusive", "2026-03-28T08:00:00Z", "2026-03-29T06:59:59Z", 0},
		{"the day the clocks change holds one", "2026-03-28T20:00:00Z", "2026-03-29T20:00:00Z", 1},
		{"an empty range", "2026-03-29T00:00:00Z", "2026-03-29T00:00:00Z", 0},
		{"a reversed range", "2026-03-30T00:00:00Z", "2026-03-29T00:00:00Z", 0},
	}
	for _, tc := range betweens {
		t.Run(tc.name, func(t *testing.T) {
			if got := s.Between(at(t, tc.from), at(t, tc.to), 1000); got != tc.want {
				t.Errorf("Between(%s, %s) = %d, want %d", tc.from, tc.to, got, tc.want)
			}
		})
	}
}

func TestBetweenStopsCountingAtTheLimit(t *testing.T) {
	s, err := cronjob.Cron("* * * * *", "UTC", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	from, to := at(t, "2026-10-08T00:00:00Z"), at(t, "2026-10-15T00:00:00Z")
	cases := []struct{ limit, want int }{{10, 10}, {1, 1}, {0, 0}, {60, 60}, {1_000_000, 7 * 24 * 60}}
	for _, tc := range cases {
		if got := s.Between(from, to, tc.limit); got != tc.want {
			t.Errorf("limit %d counted %d, want %d", tc.limit, got, tc.want)
		}
	}
}

func TestAnIntervalCountsFromItsAnchor(t *testing.T) {
	anchor := at(t, "2026-10-08T08:00:00Z")
	s, err := cronjob.Interval(3600, anchor)
	if err != nil {
		t.Fatal(err)
	}
	nexts := []struct{ name, after, want string }{
		{"before the anchor the first is one step after it", "2026-10-08T03:17:00Z", "2026-10-08T09:00:00Z"},
		{"a step before the anchor", "2026-10-08T07:00:00Z", "2026-10-08T09:00:00Z"},
		{"at the anchor", "2026-10-08T08:00:00Z", "2026-10-08T09:00:00Z"},
		{"between two occurrences", "2026-10-08T09:30:00Z", "2026-10-08T10:00:00Z"},
		{"at an occurrence the next is strictly after", "2026-10-08T10:00:00Z", "2026-10-08T11:00:00Z"},
		{"long after the anchor", "2026-11-08T10:30:00Z", "2026-11-08T11:00:00Z"},
	}
	for _, tc := range nexts {
		t.Run(tc.name, func(t *testing.T) {
			if got := s.Next(at(t, tc.after)); !got.Equal(at(t, tc.want)) {
				t.Errorf("next after %s = %s, want %s", tc.after, got, tc.want)
			}
		})
	}
	betweens := []struct {
		name     string
		from, to string
		want     int
	}{
		{"from the anchor", "2026-10-08T08:00:00Z", "2026-10-08T11:00:00Z", 3},
		{"after the anchor", "2026-10-08T12:00:00Z", "2026-10-08T15:00:00Z", 3},
		{"off the grid", "2026-10-08T12:10:00Z", "2026-10-08T14:50:00Z", 2},
		{"within one step", "2026-10-08T12:10:00Z", "2026-10-08T12:50:00Z", 0},
		{"an empty range", "2026-10-08T12:00:00Z", "2026-10-08T12:00:00Z", 0},
		{"a reversed range", "2026-10-08T15:00:00Z", "2026-10-08T12:00:00Z", 0},
		{"from before the anchor counts only the steps after it", "2026-10-08T06:00:00Z", "2026-10-08T10:00:00Z", 2},
		{"a range ending at the anchor holds none", "2026-10-08T05:00:00Z", "2026-10-08T08:00:00Z", 0},
		{"a range wholly before the anchor holds none", "2026-10-08T03:00:00Z", "2026-10-08T06:30:00Z", 0},
		{"from the zero time", "0001-01-01T00:00:00Z", "2026-10-08T11:00:00Z", 3},
	}
	for _, tc := range betweens {
		t.Run(tc.name, func(t *testing.T) {
			if got := s.Between(at(t, tc.from), at(t, tc.to), 1000); got != tc.want {
				t.Errorf("Between(%s, %s) = %d, want %d", tc.from, tc.to, got, tc.want)
			}
		})
	}
}

func TestAnEveryExpressionIsAnchoredAtCreationOrFollowsTheParserWithout(t *testing.T) {
	created := at(t, "2026-10-08T08:00:00Z")
	anchored, err := cronjob.Cron("@every 2h", "", created)
	if err != nil {
		t.Fatal(err)
	}
	unanchored, err := cronjob.Cron("@every 2h", "", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	after := at(t, "2026-10-08T09:00:00Z")
	if got, want := anchored.Next(after), at(t, "2026-10-08T10:00:00Z"); !got.Equal(want) {
		t.Errorf("anchored next = %s, want %s", got, want)
	}
	if got, want := unanchored.Next(after), at(t, "2026-10-08T11:00:00Z"); !got.Equal(want) {
		t.Errorf("unanchored next = %s, want %s", got, want)
	}
	to := at(t, "2026-10-08T15:00:00Z")
	if got := anchored.Between(after, to, 1000); got != 3 {
		t.Errorf("anchored count = %d, want 3 (10:00, 12:00, 14:00)", got)
	}
	if got := unanchored.Between(after, to, 1000); got != 3 {
		t.Errorf("unanchored count = %d, want 3 (11:00, 13:00, 15:00)", got)
	}
	if got := unanchored.Between(after, to, 2); got != 2 {
		t.Errorf("unanchored count over a limit of 2 = %d", got)
	}
}

func TestPerDayCountsAScheduleOverAWeekAndRoundsAnInterval(t *testing.T) {
	monday := at(t, "2026-10-05T00:00:00Z")
	crons := []struct {
		name, expression string
		want             int
	}{
		{"every minute", "* * * * *", 1440},
		{"every 30 minutes", "*/30 * * * *", 48},
		{"hourly", "0 * * * *", 24},
		{"daily", "0 9 * * *", 1},
		{"weekdays only", "0 9 * * 1-5", 1},
		{"weekly is not a daily one", "0 9 * * 1", 0},
		{"twice a week", "0 9 * * 1,4", 0},
		{"never", "0 0 30 2 *", 0},
	}
	for _, tc := range crons {
		t.Run(tc.name, func(t *testing.T) {
			s, err := cronjob.Cron(tc.expression, "UTC", time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			if got := s.PerDay(monday); got != tc.want {
				t.Errorf("PerDay(%q) = %d, want %d", tc.expression, got, tc.want)
			}
		})
	}
	intervals := []struct {
		name    string
		seconds int64
		want    int
	}{
		{"every second", 1, 86400},
		{"every minute", 60, 1440},
		{"every 90 seconds rounds down from 960.5", 90, 960},
		{"hourly", 3600, 24},
		{"every 7 hours rounds down from 3.93", 7 * 3600, 3},
		{"every 10 hours rounds down from 2.9", 10 * 3600, 2},
		{"every 16 hours", 16 * 3600, 2},
		{"every 17 hours", 17 * 3600, 1},
		{"daily", 86400, 1},
		{"a step of a bit over a day rounds to one", 86400 + 43200, 1},
		{"three days rounds to none", 3 * 86400, 0},
	}
	for _, tc := range intervals {
		t.Run("interval "+tc.name, func(t *testing.T) {
			s, err := cronjob.Interval(tc.seconds, monday)
			if err != nil {
				t.Fatal(err)
			}
			if got := s.PerDay(monday); got != tc.want {
				t.Errorf("PerDay(%ds) = %d, want %d", tc.seconds, got, tc.want)
			}
		})
	}
}

func TestHighRateWarningStartsPastTwentyFourIssuesADay(t *testing.T) {
	cases := []struct {
		perDay int
		want   string
	}{
		{0, ""},
		{24, ""},
		{25, "creates about 25 Issues a day; consider an existing Issue"},
		{1440, "creates about 1440 Issues a day; consider an existing Issue"},
	}
	for _, tc := range cases {
		if got := cronjob.HighRateWarning(tc.perDay); got != tc.want {
			t.Errorf("HighRateWarning(%d) = %q, want %q", tc.perDay, got, tc.want)
		}
	}
}
