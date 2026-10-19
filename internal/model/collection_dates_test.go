package model_test

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"reflect"
	"testing"
	"time"
)

func TestCollectionDateGroups(t *testing.T) {
	closed := func(uid, date string) model.Issue {
		v, err := time.Parse(time.RFC3339, date)
		if err != nil {
			t.Fatal(err)
		}
		return model.Issue{UID: uid, Status: "closed", ClosedAt: &v}
	}
	scheduled := func(uid, date string) model.Issue {
		return model.Issue{UID: uid, Status: "open", ScheduledOnDate: new(date)}
	}
	due := func(uid, date string) model.Issue {
		return model.Issue{UID: uid, Status: "open", DeadlineOnDate: new(date)}
	}
	for _, tt := range []struct {
		name, view, today string
		issues            []model.Issue
		headings          []string
		uids              []string
	}{
		{"upstream Upcoming", "upcoming", "2026-05-15", []model.Issue{scheduled("today", "2026-05-15"), scheduled("tomorrow", "2026-05-16"), scheduled("week", "2026-05-22"), {UID: "undated", Status: "open"}}, []string{"2026-05-16", "2026-05-22"}, []string{"tomorrow", "week"}},
		{"projection wins month edge", "upcoming", "2026-08-31", []model.Issue{{UID: "local-today", Status: "open", ScheduledOnDate: new("2026-08-31"), Metadata: map[string]any{"scheduled_on": "2026-09-01T00:30:00Z"}}, {UID: "tomorrow", Status: "open", ScheduledOnDate: new("2026-09-01"), Metadata: map[string]any{"scheduled_on": "2026-09-01T07:30:00Z"}}}, []string{"2026-09-01"}, []string{"tomorrow"}},
		{"year and week edges", "upcoming", "2026-12-31", []model.Issue{scheduled("new-year", "2027-01-01"), scheduled("sunday", "2027-01-03"), scheduled("monday", "2027-01-04")}, []string{"2027-01-01", "2027-01-03", "2027-01-04"}, []string{"new-year", "sunday", "monday"}},
		{"empty projection excludes fallback", "upcoming", "2026-05-15", []model.Issue{{UID: "empty", Status: "open", ScheduledOnDate: new(""), Metadata: map[string]any{"scheduled_on": "2026-05-16T00:00:00Z"}}, {UID: "raw", Status: "open", Metadata: map[string]any{"scheduled_on": "2026-05-16T00:00:00+14:00"}}}, []string{"2026-05-16"}, []string{"raw"}},
		{"Deadlines", "deadlines", "2026-05-15", []model.Issue{due("overdue", "2026-05-10"), due("today", "2026-05-15"), due("future", "2026-05-20"), due("later", "2026-05-22")}, []string{"Overdue", "Today", "2026-05-20", "2026-05-22"}, []string{"overdue", "today", "future", "later"}},
		{"Today merges evening", "today", "2026-05-15", []model.Issue{scheduled("overdue", "2026-05-14"), {UID: "evening", Status: "open", ScheduledOnDate: new("2026-05-15"), Metadata: map[string]any{"today_bucket": "evening"}}, due("today", "2026-05-15"), scheduled("future", "2026-05-16")}, []string{"Overdue", "Today"}, []string{"overdue", "evening", "today"}},
		{"upstream Logbook", "logbook", "2026-05-15", []model.Issue{closed("earlier", "2026-05-14T09:00:00Z"), closed("latest", "2026-05-15T15:00:00Z"), {UID: "open", Status: "open"}, {UID: "undated", Status: "closed"}}, []string{"2026-05-15", "2026-05-14"}, []string{"latest", "earlier"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			collection := model.Collection{View: tt.view, Title: tt.view}
			m := model.Model{Collection: collection}
			m, _ = model.Update(m, model.SnapshotLoaded{Request: m.RefreshRequest, Query: m.SnapshotQuery(), LocalDate: tt.today, TimeZone: "America/Los_Angeles", Snapshot: model.Snapshot{Issues: tt.issues}})
			var headings, uids []string
			for _, group := range m.Groups {
				headings = append(headings, group.Title)
			}
			for _, issue := range m.Issues {
				uids = append(uids, issue.UID)
			}
			if !reflect.DeepEqual(headings, tt.headings) || !reflect.DeepEqual(uids, tt.uids) {
				t.Fatalf("headings %v Issues %v", headings, uids)
			}
		})
	}
}

func TestLogbookDaysAndDetailTimesAgreeInThePaneZone(t *testing.T) {
	closedAt := time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC)
	issue := model.Issue{UID: "i", Status: "closed", ClosedAt: &closedAt}
	for _, tt := range []struct{ zone, day string }{
		{"America/New_York", "2026-10-18"},
		{"UTC", "2026-10-19"},
		{"Asia/Tokyo", "2026-10-19"},
		{"No/Such_Zone", "2026-10-19"},
	} {
		cal := model.NewPaneCalendar("2026-10-19", tt.zone, "")
		groups := model.GroupIssues(model.Collection{View: "logbook", Title: "logbook"}, []model.Issue{issue}, cal)
		if len(groups) != 1 || groups[0].Title != tt.day {
			t.Errorf("%s: groups %+v, want heading %s", tt.zone, groups, tt.day)
		}
		m := model.Model{Calendar: cal}
		if got := m.DetailTime(closedAt)[:10]; got != tt.day {
			t.Errorf("%s: detail day %s, want %s", tt.zone, got, tt.day)
		}
	}
}

func TestOwnerTextReadsClaimExpiryInThePaneZone(t *testing.T) {
	m := model.Model{Calendar: model.NewPaneCalendar("2026-10-19", "America/New_York", "")}
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	owner := "dana"
	for _, tt := range []struct {
		expires time.Time
		want    string
	}{
		{time.Date(2026, 10, 19, 18, 30, 0, 0, time.UTC), "dana until 14:30"},
		{time.Date(2026, 10, 20, 2, 0, 0, 0, time.UTC), "dana until 22:00"},
		{time.Date(2026, 10, 20, 4, 30, 0, 0, time.UTC), "dana until 2026-10-20 00:30"},
	} {
		if got := m.OwnerText(&owner, &tt.expires, now).String(); got != tt.want {
			t.Errorf("expires %s: %q, want %q", tt.expires, got, tt.want)
		}
	}
}

func TestPaneCalendarDayEndsAtLocalMidnightOnShortAndLongDays(t *testing.T) {
	cal := model.NewPaneCalendar("", "America/New_York", "")
	for _, tt := range []struct {
		name       string
		now        time.Time
		wantEnds   time.Time
		wantLength time.Duration
	}{
		{"spring forward", time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC), time.Date(2026, 3, 9, 4, 0, 0, 0, time.UTC), 23 * time.Hour},
		{"fall back", time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC), time.Date(2026, 11, 2, 5, 0, 0, 0, time.UTC), 25 * time.Hour},
	} {
		ends := cal.DayEnds(tt.now)
		if !ends.Equal(tt.wantEnds) {
			t.Errorf("%s: ends %s, want %s", tt.name, ends, tt.wantEnds)
		}
		start := time.Date(tt.now.Year(), tt.now.Month(), tt.now.Day(), 0, 0, 0, 0, mustZone(t, "America/New_York"))
		if got := ends.Sub(start); got != tt.wantLength {
			t.Errorf("%s: day is %s long, want %s", tt.name, got, tt.wantLength)
		}
	}
}

func mustZone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}
